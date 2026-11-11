package store

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rajdama/staking-rewards/indexer/internal/chain"
)

// These tests need a real Postgres. CI provides one; locally, set TEST_DATABASE_URL.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if _, err := s.pool.Exec(ctx, `DROP TABLE IF EXISTS events, positions, sync_state`); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

var (
	alice = common.HexToAddress("0x00000000000000000000000000000000000A11CE")
	bob   = common.HexToAddress("0x0000000000000000000000000000000000000B0B")
)

func eth(n int64) *big.Int { return new(big.Int).Mul(big.NewInt(n), big.NewInt(1e18)) }

func ev(typ chain.EventType, user common.Address, pos int64, amount *big.Int, block uint64, logIndex uint, at time.Time) IndexedEvent {
	e := IndexedEvent{
		Event: chain.Event{
			Type:        typ,
			User:        user,
			Amount:      amount,
			BlockNumber: block,
			TxHash:      common.BigToHash(big.NewInt(int64(block))),
			LogIndex:    logIndex,
		},
		BlockTime: at,
	}
	if typ != chain.EventClaim {
		e.PositionID = big.NewInt(pos)
	}
	return e
}

func TestApplyBatchAndQueries(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)

	aliceStake := ev(chain.EventStake, alice, 0, eth(100), 10, 0, dayAgo)
	aliceStake.MultiplierBps = 30000
	aliceStake.UnlockTime = uint64(dayAgo.Unix()) // already expired

	bobStake := ev(chain.EventStake, bob, 0, eth(50), 11, 0, dayAgo)
	bobStake.MultiplierBps = 10000

	bobStake2 := ev(chain.EventStake, bob, 1, eth(20), 12, 0, now)
	bobStake2.MultiplierBps = 15000
	bobStake2.UnlockTime = uint64(now.Add(30 * 24 * time.Hour).Unix())

	batch := []IndexedEvent{
		aliceStake,
		bobStake,
		bobStake2,
		ev(chain.EventWithdraw, bob, 0, eth(50), 13, 0, now),
		ev(chain.EventClaim, alice, 0, eth(7), 13, 1, now),
	}
	if err := s.ApplyBatch(ctx, batch, 20); err != nil {
		t.Fatal(err)
	}
	// Re-applying the same events must not double count.
	if err := s.ApplyBatch(ctx, batch, 20); err != nil {
		t.Fatal(err)
	}

	last, ok, err := s.LastBlock(ctx)
	if err != nil || !ok || last != 20 {
		t.Fatalf("last block = %d ok=%v err=%v", last, ok, err)
	}

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.TotalStaked != eth(120).String() { // alice 100 + bob 20
		t.Errorf("total staked = %s", st.TotalStaked)
	}
	if st.TotalEffective != eth(330).String() { // 100*3 + 20*1.5
		t.Errorf("total effective = %s", st.TotalEffective)
	}
	if st.Stakers != 2 || st.OpenPositions != 2 || st.TotalClaimed != eth(7).String() || st.LastIndexedBlock != 20 {
		t.Errorf("unexpected stats %+v", st)
	}

	leaders, err := s.Leaderboard(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaders) != 2 || leaders[0].User != "0x00000000000000000000000000000000000a11ce" || leaders[0].Claimed != eth(7).String() {
		t.Errorf("unexpected leaderboard %+v", leaders)
	}

	all, err := s.Activity(ctx, 10, "")
	if err != nil || len(all) != 5 || all[0].Type != "claim" {
		t.Fatalf("activity = %+v err=%v", all, err)
	}
	bobs, err := s.Activity(ctx, 10, bob.Hex())
	if err != nil || len(bobs) != 3 {
		t.Fatalf("bob activity = %+v err=%v", bobs, err)
	}

	hist, err := s.History(ctx, 3)
	if err != nil || len(hist) != 3 {
		t.Fatalf("history = %+v err=%v", hist, err)
	}
	if hist[2].TotalStaked != eth(120).String() || hist[1].TotalStaked != eth(150).String() || hist[0].TotalStaked != "0" {
		t.Errorf("unexpected history %+v", hist)
	}

	kick, err := s.KickCandidates(ctx, now, 10)
	if err != nil || len(kick) != 1 || kick[0].PositionID != 0 {
		t.Fatalf("kick candidates = %+v err=%v", kick, err)
	}

	kicked := ev(chain.EventKick, alice, 0, nil, 21, 0, now)
	kicked.Caller = bob
	if err := s.ApplyBatch(ctx, []IndexedEvent{kicked}, 21); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Stats(ctx)
	if st.TotalEffective != eth(130).String() { // alice back to 1x
		t.Errorf("effective after kick = %s", st.TotalEffective)
	}
	if kick, _ = s.KickCandidates(ctx, now, 10); len(kick) != 0 {
		t.Errorf("expected no kick candidates after kick, got %+v", kick)
	}
}

func TestLastBlockEmpty(t *testing.T) {
	s := openTestStore(t)
	if _, ok, err := s.LastBlock(context.Background()); ok || err != nil {
		t.Fatalf("expected empty cursor, ok=%v err=%v", ok, err)
	}
}
