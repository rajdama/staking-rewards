package indexer

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/rajdama/staking-rewards/indexer/internal/chain"
	"github.com/rajdama/staking-rewards/indexer/internal/store"
)

type fakeClient struct {
	head    uint64
	logs    []types.Log
	queries []ethereum.FilterQuery
}

func (c *fakeClient) BlockNumber(context.Context) (uint64, error) { return c.head, nil }

func (c *fakeClient) FilterLogs(_ context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	c.queries = append(c.queries, q)
	var out []types.Log
	for _, lg := range c.logs {
		if lg.BlockNumber >= q.FromBlock.Uint64() && lg.BlockNumber <= q.ToBlock.Uint64() {
			out = append(out, lg)
		}
	}
	return out, nil
}

func (c *fakeClient) HeaderByNumber(_ context.Context, n *big.Int) (*types.Header, error) {
	return &types.Header{Number: n, Time: 1_700_000_000 + n.Uint64()*12}, nil
}

type fakeSink struct {
	last    uint64
	hasLast bool
	events  []store.IndexedEvent
}

func (s *fakeSink) LastBlock(context.Context) (uint64, bool, error) { return s.last, s.hasLast, nil }

func (s *fakeSink) ApplyBatch(_ context.Context, events []store.IndexedEvent, to uint64) error {
	s.events = append(s.events, events...)
	s.last, s.hasLast = to, true
	return nil
}

var contract = common.HexToAddress("0x5000000000000000000000000000000000000005")

func claimLog(t *testing.T, d *chain.Decoder, block uint64) types.Log {
	t.Helper()
	ev := d.ABI().Events["RewardPaid"]
	data, err := ev.Inputs.NonIndexed().Pack(big.NewInt(int64(block)))
	if err != nil {
		t.Fatal(err)
	}
	return types.Log{
		Address:     contract,
		Topics:      []common.Hash{ev.ID, common.HexToHash("0xa11ce")},
		Data:        data,
		BlockNumber: block,
		TxHash:      common.BigToHash(big.NewInt(int64(block))),
	}
}

func newTestIndexer(t *testing.T, client *fakeClient, sink *fakeSink, start, batch, confirmations uint64) *Indexer {
	t.Helper()
	d, err := chain.NewDecoder()
	if err != nil {
		t.Fatal(err)
	}
	return New(client, sink, d, Options{
		Contract:      contract,
		StartBlock:    start,
		BatchSize:     batch,
		Confirmations: confirmations,
		PollInterval:  time.Millisecond,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestStepIndexesInBatchesUpToSafeHead(t *testing.T) {
	d, _ := chain.NewDecoder()
	client := &fakeClient{head: 125}
	client.logs = []types.Log{claimLog(t, d, 105), claimLog(t, d, 115), claimLog(t, d, 121), claimLog(t, d, 124)}
	sink := &fakeSink{}
	ix := newTestIndexer(t, client, sink, 100, 10, 3) // safe head = 122

	caughtUp, err := ix.Step(context.Background())
	if err != nil || caughtUp {
		t.Fatalf("step 1: caughtUp=%v err=%v", caughtUp, err)
	}
	if sink.last != 109 || len(sink.events) != 1 {
		t.Fatalf("after step 1: last=%d events=%d", sink.last, len(sink.events))
	}

	if _, err := ix.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	caughtUp, err = ix.Step(context.Background())
	if err != nil || !caughtUp {
		t.Fatalf("step 3: caughtUp=%v err=%v", caughtUp, err)
	}
	if sink.last != 122 {
		t.Fatalf("should stop at the safe head 122, got %d", sink.last)
	}
	if len(sink.events) != 3 { // block 124 is not confirmed yet
		t.Fatalf("want 3 events, got %d", len(sink.events))
	}

	ev := sink.events[0]
	if ev.Type != chain.EventClaim || ev.Amount.Int64() != 105 {
		t.Fatalf("unexpected event %+v", ev)
	}
	if want := time.Unix(1_700_000_000+105*12, 0).UTC(); !ev.BlockTime.Equal(want) {
		t.Fatalf("block time = %v, want %v", ev.BlockTime, want)
	}

	// Nothing new: no further queries.
	queries := len(client.queries)
	if caughtUp, _ := ix.Step(context.Background()); !caughtUp || len(client.queries) != queries {
		t.Fatal("expected no work when caught up")
	}
}

func TestStepResumesFromCursor(t *testing.T) {
	client := &fakeClient{head: 1000}
	sink := &fakeSink{last: 500, hasLast: true}
	ix := newTestIndexer(t, client, sink, 100, 50, 0)

	if _, err := ix.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	q := client.queries[0]
	if q.FromBlock.Uint64() != 501 || q.ToBlock.Uint64() != 550 {
		t.Fatalf("queried %d-%d, want 501-550", q.FromBlock, q.ToBlock)
	}
	if len(q.Addresses) != 1 || q.Addresses[0] != contract || len(q.Topics[0]) != 4 {
		t.Fatalf("unexpected filter %+v", q)
	}
}

func TestStepWaitsForConfirmations(t *testing.T) {
	client := &fakeClient{head: 2}
	ix := newTestIndexer(t, client, &fakeSink{}, 0, 10, 5)
	caughtUp, err := ix.Step(context.Background())
	if err != nil || !caughtUp || len(client.queries) != 0 {
		t.Fatalf("caughtUp=%v err=%v queries=%d", caughtUp, err, len(client.queries))
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ix := newTestIndexer(t, &fakeClient{head: 10}, &fakeSink{}, 0, 100, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := ix.Run(ctx); err != context.DeadlineExceeded {
		t.Fatalf("Run returned %v", err)
	}
}
