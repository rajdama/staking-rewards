package chain

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	alice  = common.HexToAddress("0x00000000000000000000000000000000000a11ce")
	keeper = common.HexToAddress("0x000000000000000000000000000000000000beef")
	tx     = common.HexToHash("0x1234")
)

func newDecoder(t *testing.T) *Decoder {
	t.Helper()
	d, err := NewDecoder()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func pack(t *testing.T, d *Decoder, event string, args ...any) []byte {
	t.Helper()
	data, err := d.abi.Events[event].Inputs.NonIndexed().Pack(args...)
	if err != nil {
		t.Fatalf("pack %s: %v", event, err)
	}
	return data
}

func addrTopic(a common.Address) common.Hash { return common.BytesToHash(a.Bytes()) }

func uintTopic(n int64) common.Hash { return common.BigToHash(big.NewInt(n)) }

func TestDecodeStaked(t *testing.T) {
	d := newDecoder(t)
	amount, _ := new(big.Int).SetString("1500000000000000000000", 10)
	lg := types.Log{
		Topics:      []common.Hash{d.abi.Events["Staked"].ID, addrTopic(alice), uintTopic(7)},
		Data:        pack(t, d, "Staked", amount, uint16(20000), uint64(1_800_000_000)),
		BlockNumber: 42,
		TxHash:      tx,
		Index:       3,
	}

	ev, ok, err := d.Decode(lg)
	if err != nil || !ok {
		t.Fatalf("decode: ok=%v err=%v", ok, err)
	}
	if ev.Type != EventStake || ev.User != alice || ev.PositionID.Int64() != 7 {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if ev.Amount.Cmp(amount) != 0 || ev.MultiplierBps != 20000 || ev.UnlockTime != 1_800_000_000 {
		t.Fatalf("unexpected values: %+v", ev)
	}
	if ev.BlockNumber != 42 || ev.TxHash != tx || ev.LogIndex != 3 {
		t.Fatalf("unexpected log metadata: %+v", ev)
	}
}

func TestDecodeWithdrawn(t *testing.T) {
	d := newDecoder(t)
	lg := types.Log{
		Topics: []common.Hash{d.abi.Events["Withdrawn"].ID, addrTopic(alice), uintTopic(2)},
		Data:   pack(t, d, "Withdrawn", big.NewInt(100)),
	}
	ev, ok, err := d.Decode(lg)
	if err != nil || !ok || ev.Type != EventWithdraw || ev.Amount.Int64() != 100 || ev.PositionID.Int64() != 2 {
		t.Fatalf("got %+v ok=%v err=%v", ev, ok, err)
	}
}

func TestDecodeKicked(t *testing.T) {
	d := newDecoder(t)
	lg := types.Log{
		Topics: []common.Hash{d.abi.Events["Kicked"].ID, addrTopic(alice), uintTopic(1), addrTopic(keeper)},
	}
	ev, ok, err := d.Decode(lg)
	if err != nil || !ok || ev.Type != EventKick || ev.Caller != keeper || ev.User != alice {
		t.Fatalf("got %+v ok=%v err=%v", ev, ok, err)
	}
}

func TestDecodeRewardPaid(t *testing.T) {
	d := newDecoder(t)
	lg := types.Log{
		Topics: []common.Hash{d.abi.Events["RewardPaid"].ID, addrTopic(alice)},
		Data:   pack(t, d, "RewardPaid", big.NewInt(55)),
	}
	ev, ok, err := d.Decode(lg)
	if err != nil || !ok || ev.Type != EventClaim || ev.Amount.Int64() != 55 || ev.User != alice {
		t.Fatalf("got %+v ok=%v err=%v", ev, ok, err)
	}
}

func TestDecodeIgnoresUnknownEvents(t *testing.T) {
	d := newDecoder(t)
	lg := types.Log{Topics: []common.Hash{common.HexToHash("0xdeadbeef")}}
	if _, ok, err := d.Decode(lg); ok || err != nil {
		t.Fatalf("expected unknown event to be skipped, ok=%v err=%v", ok, err)
	}
	if _, ok, err := d.Decode(types.Log{}); ok || err != nil {
		t.Fatalf("expected log without topics to be skipped, ok=%v err=%v", ok, err)
	}
}

func TestDecodeRejectsMissingTopics(t *testing.T) {
	d := newDecoder(t)
	lg := types.Log{
		Topics: []common.Hash{d.abi.Events["Withdrawn"].ID, addrTopic(alice)},
		Data:   pack(t, d, "Withdrawn", big.NewInt(1)),
	}
	if _, _, err := d.Decode(lg); err == nil {
		t.Fatal("expected an error for a log with too few topics")
	}
}

// The event signatures must match the Solidity contract exactly, or no logs will match.
func TestTopicsMatchSolidity(t *testing.T) {
	d := newDecoder(t)
	want := map[string]string{
		"Staked":     "Staked(address,uint256,uint256,uint16,uint64)",
		"Withdrawn":  "Withdrawn(address,uint256,uint256)",
		"Kicked":     "Kicked(address,uint256,address)",
		"RewardPaid": "RewardPaid(address,uint256)",
	}
	for name, sig := range want {
		if got := d.abi.Events[name].Sig; got != sig {
			t.Errorf("%s signature = %s, want %s", name, got, sig)
		}
	}
	if len(d.Topics()) != 4 {
		t.Fatalf("want 4 topics, got %d", len(d.Topics()))
	}
}
