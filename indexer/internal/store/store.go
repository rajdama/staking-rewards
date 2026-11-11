// Package store persists indexed events in Postgres and answers the API's queries.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rajdama/staking-rewards/indexer/internal/chain"
)

const schema = `
CREATE TABLE IF NOT EXISTS events (
  id             BIGSERIAL PRIMARY KEY,
  block_number   BIGINT       NOT NULL,
  block_time     TIMESTAMPTZ  NOT NULL,
  tx_hash        TEXT         NOT NULL,
  log_index      INTEGER      NOT NULL,
  type           TEXT         NOT NULL,
  user_address   TEXT         NOT NULL,
  position_id    BIGINT,
  amount         NUMERIC(78,0),
  multiplier_bps INTEGER,
  unlock_time    BIGINT,
  caller         TEXT,
  UNIQUE (tx_hash, log_index)
);
CREATE INDEX IF NOT EXISTS events_user_idx ON events (user_address);
CREATE INDEX IF NOT EXISTS events_time_idx ON events (block_time);

CREATE TABLE IF NOT EXISTS positions (
  user_address   TEXT          NOT NULL,
  position_id    BIGINT        NOT NULL,
  amount         NUMERIC(78,0) NOT NULL,
  multiplier_bps INTEGER       NOT NULL,
  unlock_time    BIGINT        NOT NULL,
  open           BOOLEAN       NOT NULL,
  PRIMARY KEY (user_address, position_id)
);

CREATE TABLE IF NOT EXISTS sync_state (
  id         INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  last_block BIGINT  NOT NULL
);
`

// IndexedEvent is a decoded event plus its block time.
type IndexedEvent struct {
	chain.Event
	BlockTime time.Time
}

// Store wraps a Postgres connection pool.
type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// Migrate creates the tables if they don't exist.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schema)
	return err
}

// LastBlock returns the last fully indexed block, or ok=false if nothing is indexed yet.
func (s *Store) LastBlock(ctx context.Context) (block uint64, ok bool, err error) {
	err = s.pool.QueryRow(ctx, `SELECT last_block FROM sync_state WHERE id = 1`).Scan(&block)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return block, err == nil, err
}

// ApplyBatch stores events and moves the cursor to `toBlock`, all in one transaction.
// Re-applying the same events is a no-op, so a crash mid-batch is safe to retry.
func (s *Store) ApplyBatch(ctx context.Context, events []IndexedEvent, toBlock uint64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	for _, ev := range events {
		inserted, err := insertEvent(ctx, tx, ev)
		if err != nil {
			return err
		}
		if !inserted {
			continue // already applied
		}
		if err := applyToPositions(ctx, tx, ev); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO sync_state (id, last_block) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET last_block = EXCLUDED.last_block`, int64(toBlock)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertEvent(ctx context.Context, tx pgx.Tx, ev IndexedEvent) (bool, error) {
	var positionID *int64
	if ev.PositionID != nil {
		v := ev.PositionID.Int64()
		positionID = &v
	}
	var amount *string
	if ev.Amount != nil {
		v := ev.Amount.String()
		amount = &v
	}
	var multiplier *int32
	var unlock *int64
	if ev.Type == chain.EventStake {
		m := int32(ev.MultiplierBps)
		u := int64(ev.UnlockTime)
		multiplier, unlock = &m, &u
	}
	var caller *string
	if ev.Type == chain.EventKick {
		c := addr(ev.Caller.Hex())
		caller = &c
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO events (block_number, block_time, tx_hash, log_index, type, user_address,
		                    position_id, amount, multiplier_bps, unlock_time, caller)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::text::numeric, $9, $10, $11)
		ON CONFLICT (tx_hash, log_index) DO NOTHING`,
		int64(ev.BlockNumber), ev.BlockTime, ev.TxHash.Hex(), int32(ev.LogIndex), string(ev.Type),
		addr(ev.User.Hex()), positionID, amount, multiplier, unlock, caller)
	if err != nil {
		return false, fmt.Errorf("insert event: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func applyToPositions(ctx context.Context, tx pgx.Tx, ev IndexedEvent) error {
	user := addr(ev.User.Hex())
	var err error
	switch ev.Type {
	case chain.EventStake:
		_, err = tx.Exec(ctx, `
			INSERT INTO positions (user_address, position_id, amount, multiplier_bps, unlock_time, open)
			VALUES ($1, $2, $3::text::numeric, $4, $5, TRUE)
			ON CONFLICT (user_address, position_id) DO NOTHING`,
			user, ev.PositionID.Int64(), ev.Amount.String(), int32(ev.MultiplierBps), int64(ev.UnlockTime))
	case chain.EventWithdraw:
		_, err = tx.Exec(ctx, `UPDATE positions SET open = FALSE WHERE user_address = $1 AND position_id = $2`,
			user, ev.PositionID.Int64())
	case chain.EventKick:
		_, err = tx.Exec(ctx, `UPDATE positions SET multiplier_bps = 10000 WHERE user_address = $1 AND position_id = $2`,
			user, ev.PositionID.Int64())
	}
	if err != nil {
		return fmt.Errorf("apply %s: %w", ev.Type, err)
	}
	return nil
}

// addr normalizes addresses to lowercase so lookups don't depend on checksum casing.
func addr(a string) string { return strings.ToLower(a) }

// ---------------------------------------------------------------------------
// Queries used by the API
// ---------------------------------------------------------------------------

type Stats struct {
	TotalStaked      string `json:"totalStaked"`
	TotalEffective   string `json:"totalEffective"`
	Stakers          int64  `json:"stakers"`
	OpenPositions    int64  `json:"openPositions"`
	TotalClaimed     string `json:"totalClaimed"`
	LastIndexedBlock int64  `json:"lastIndexedBlock"`
}

type Activity struct {
	Type          string  `json:"type"`
	User          string  `json:"user"`
	PositionID    *int64  `json:"positionId,omitempty"`
	Amount        *string `json:"amount,omitempty"`
	MultiplierBps *int32  `json:"multiplierBps,omitempty"`
	TxHash        string  `json:"txHash"`
	BlockNumber   int64   `json:"blockNumber"`
	Timestamp     int64   `json:"timestamp"`
}

type Leader struct {
	User      string `json:"user"`
	Staked    string `json:"staked"`
	Effective string `json:"effective"`
	Claimed   string `json:"claimed"`
}

type HistoryPoint struct {
	Timestamp   int64  `json:"timestamp"`
	TotalStaked string `json:"totalStaked"`
}

// KickCandidate is an open position whose lock expired but still has a boost.
type KickCandidate struct {
	User       string
	PositionID int64
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.pool.QueryRow(ctx, `
		SELECT
		  COALESCE(SUM(amount), 0)::text,
		  COALESCE(SUM(div(amount * multiplier_bps, 10000)), 0)::text,
		  COUNT(DISTINCT user_address),
		  COUNT(*),
		  (SELECT COALESCE(SUM(amount), 0)::text FROM events WHERE type = 'claim'),
		  (SELECT COALESCE(MAX(last_block), 0) FROM sync_state)
		FROM positions WHERE open`).
		Scan(&st.TotalStaked, &st.TotalEffective, &st.Stakers, &st.OpenPositions, &st.TotalClaimed, &st.LastIndexedBlock)
	return st, err
}

// Activity returns the newest events, optionally for one user.
func (s *Store) Activity(ctx context.Context, limit int, user string) ([]Activity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT type, user_address, position_id, amount::text, multiplier_bps, tx_hash, block_number,
		       EXTRACT(EPOCH FROM block_time)::bigint
		FROM events
		WHERE ($2 = '' OR user_address = $2)
		ORDER BY block_number DESC, log_index DESC
		LIMIT $1`, limit, addr(user))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Activity, error) {
		var a Activity
		err := r.Scan(&a.Type, &a.User, &a.PositionID, &a.Amount, &a.MultiplierBps, &a.TxHash, &a.BlockNumber, &a.Timestamp)
		return a, err
	})
}

// Leaderboard ranks users by boosted balance.
func (s *Store) Leaderboard(ctx context.Context, limit int) ([]Leader, error) {
	rows, err := s.pool.Query(ctx, `
		WITH balances AS (
		  SELECT user_address,
		         SUM(amount) AS staked,
		         SUM(div(amount * multiplier_bps, 10000)) AS effective
		  FROM positions WHERE open GROUP BY user_address
		),
		claims AS (
		  SELECT user_address, SUM(amount) AS claimed FROM events WHERE type = 'claim' GROUP BY user_address
		)
		SELECT b.user_address, b.staked::text, b.effective::text, COALESCE(c.claimed, 0)::text
		FROM balances b LEFT JOIN claims c USING (user_address)
		ORDER BY b.effective DESC, b.user_address
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Leader, error) {
		var l Leader
		err := r.Scan(&l.User, &l.Staked, &l.Effective, &l.Claimed)
		return l, err
	})
}

// History returns total staked at the end of each of the last `days` days (UTC), oldest first.
func (s *Store) History(ctx context.Context, days int) ([]HistoryPoint, error) {
	rows, err := s.pool.Query(ctx, `
		WITH deltas AS (
		  SELECT date_trunc('day', block_time AT TIME ZONE 'UTC') AS day,
		         SUM(CASE type WHEN 'stake' THEN amount WHEN 'withdraw' THEN -amount ELSE 0 END) AS delta
		  FROM events GROUP BY 1
		),
		running AS (
		  SELECT day, SUM(delta) OVER (ORDER BY day) AS total FROM deltas
		),
		series AS (
		  SELECT generate_series(
		    date_trunc('day', now() AT TIME ZONE 'UTC') - make_interval(days => $1 - 1),
		    date_trunc('day', now() AT TIME ZONE 'UTC'),
		    interval '1 day') AS day
		)
		SELECT EXTRACT(EPOCH FROM s.day)::bigint,
		       COALESCE((SELECT r.total FROM running r WHERE r.day <= s.day ORDER BY r.day DESC LIMIT 1), 0)::text
		FROM series s
		ORDER BY s.day`, days)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (HistoryPoint, error) {
		var p HistoryPoint
		err := r.Scan(&p.Timestamp, &p.TotalStaked)
		return p, err
	})
}

// KickCandidates lists open boosted positions whose lock has expired by `now`.
func (s *Store) KickCandidates(ctx context.Context, now time.Time, limit int) ([]KickCandidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_address, position_id FROM positions
		WHERE open AND multiplier_bps > 10000 AND unlock_time <= $1
		ORDER BY unlock_time LIMIT $2`, now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (KickCandidate, error) {
		var k KickCandidate
		err := r.Scan(&k.User, &k.PositionID)
		return k, err
	})
}
