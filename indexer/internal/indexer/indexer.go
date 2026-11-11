// Package indexer follows the chain and stores StakingRewards events.
package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/rajdama/staking-rewards/indexer/internal/chain"
	"github.com/rajdama/staking-rewards/indexer/internal/store"
)

// Client is the subset of an Ethereum RPC client the indexer needs.
type Client interface {
	BlockNumber(ctx context.Context) (uint64, error)
	FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

// Sink is where decoded events go (the Postgres store in production).
type Sink interface {
	LastBlock(ctx context.Context) (uint64, bool, error)
	ApplyBatch(ctx context.Context, events []store.IndexedEvent, toBlock uint64) error
}

type Indexer struct {
	client        Client
	sink          Sink
	decoder       *chain.Decoder
	contract      common.Address
	startBlock    uint64
	batchSize     uint64
	confirmations uint64
	pollInterval  time.Duration
	log           *slog.Logger
}

type Options struct {
	Contract      common.Address
	StartBlock    uint64
	BatchSize     uint64
	Confirmations uint64
	PollInterval  time.Duration
}

func New(client Client, sink Sink, decoder *chain.Decoder, opts Options, log *slog.Logger) *Indexer {
	return &Indexer{
		client:        client,
		sink:          sink,
		decoder:       decoder,
		contract:      opts.Contract,
		startBlock:    opts.StartBlock,
		batchSize:     opts.BatchSize,
		confirmations: opts.Confirmations,
		pollInterval:  opts.PollInterval,
		log:           log,
	}
}

// Run indexes until ctx is cancelled, retrying on errors.
func (ix *Indexer) Run(ctx context.Context) error {
	for {
		caughtUp, err := ix.Step(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			ix.log.Error("indexing step failed, retrying", "err", err)
		}
		wait := time.Duration(0)
		if caughtUp || err != nil {
			wait = ix.pollInterval
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// Step indexes one batch of blocks. It returns caughtUp=true when there was nothing new.
func (ix *Indexer) Step(ctx context.Context) (caughtUp bool, err error) {
	head, err := ix.client.BlockNumber(ctx)
	if err != nil {
		return false, fmt.Errorf("block number: %w", err)
	}
	if head < ix.confirmations {
		return true, nil
	}
	safe := head - ix.confirmations

	from := ix.startBlock
	last, ok, err := ix.sink.LastBlock(ctx)
	if err != nil {
		return false, fmt.Errorf("read cursor: %w", err)
	}
	if ok && last+1 > from {
		from = last + 1
	}
	if from > safe {
		return true, nil
	}
	to := from + ix.batchSize - 1
	if to > safe {
		to = safe
	}

	logs, err := ix.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(from),
		ToBlock:   new(big.Int).SetUint64(to),
		Addresses: []common.Address{ix.contract},
		Topics:    [][]common.Hash{ix.decoder.Topics()},
	})
	if err != nil {
		return false, fmt.Errorf("filter logs %d-%d: %w", from, to, err)
	}

	events := make([]store.IndexedEvent, 0, len(logs))
	blockTimes := map[uint64]time.Time{}
	for _, lg := range logs {
		if lg.Removed {
			continue
		}
		ev, ok, err := ix.decoder.Decode(lg)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		t, seen := blockTimes[lg.BlockNumber]
		if !seen {
			header, err := ix.client.HeaderByNumber(ctx, new(big.Int).SetUint64(lg.BlockNumber))
			if err != nil {
				return false, fmt.Errorf("header %d: %w", lg.BlockNumber, err)
			}
			t = time.Unix(int64(header.Time), 0).UTC()
			blockTimes[lg.BlockNumber] = t
		}
		events = append(events, store.IndexedEvent{Event: ev, BlockTime: t})
	}

	if err := ix.sink.ApplyBatch(ctx, events, to); err != nil {
		return false, fmt.Errorf("store batch: %w", err)
	}
	ix.log.Info("indexed", "from", from, "to", to, "events", len(events), "head", head)
	return to == safe, nil
}
