package keeper

import (
	"context"
	"errors"
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

// Anvil's well-known first test key. Never use it on a real network.
const testKey = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

type fakeClient struct {
	failEstimateFor common.Address
	sent            []*types.Transaction
}

func (c *fakeClient) ChainID(context.Context) (*big.Int, error) { return big.NewInt(11155111), nil }
func (c *fakeClient) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return 7, nil
}
func (c *fakeClient) SuggestGasTipCap(context.Context) (*big.Int, error) { return big.NewInt(1e9), nil }
func (c *fakeClient) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return &types.Header{BaseFee: big.NewInt(2e9)}, nil
}
func (c *fakeClient) EstimateGas(_ context.Context, msg ethereum.CallMsg) (uint64, error) {
	if c.failEstimateFor != (common.Address{}) && common.BytesToAddress(msg.Data[16:36]) == c.failEstimateFor {
		return 0, errors.New("execution reverted: NotKickable")
	}
	return 50_000, nil
}
func (c *fakeClient) SendTransaction(_ context.Context, tx *types.Transaction) error {
	c.sent = append(c.sent, tx)
	return nil
}

type fakeCandidates []store.KickCandidate

func (f fakeCandidates) KickCandidates(context.Context, time.Time, int) ([]store.KickCandidate, error) {
	return f, nil
}

var contract = common.HexToAddress("0x5000000000000000000000000000000000000005")

func newKeeper(t *testing.T, client *fakeClient, c fakeCandidates) *Keeper {
	t.Helper()
	d, err := chain.NewDecoder()
	if err != nil {
		t.Fatal(err)
	}
	k, err := New(client, c, d.ABI(), contract, testKey, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestTickSendsKicksWithIncreasingNonces(t *testing.T) {
	alice := common.HexToAddress("0x00000000000000000000000000000000000a11ce")
	bob := common.HexToAddress("0x0000000000000000000000000000000000000b0b")
	client := &fakeClient{}
	k := newKeeper(t, client, fakeCandidates{{User: alice.Hex(), PositionID: 3}, {User: bob.Hex(), PositionID: 0}})

	if k.Address() != common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266") {
		t.Fatalf("unexpected keeper address %s", k.Address())
	}

	n, err := k.Tick(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("sent %d err=%v", n, err)
	}

	first := client.sent[0]
	if first.Nonce() != 7 || client.sent[1].Nonce() != 8 {
		t.Fatalf("nonces %d, %d", first.Nonce(), client.sent[1].Nonce())
	}
	if *first.To() != contract || first.Gas() != 60_000 || first.ChainId().Int64() != 11155111 {
		t.Fatalf("unexpected tx %+v", first)
	}
	if first.GasFeeCap().Int64() != 5e9 { // tip 1 gwei + 2 x base fee 2 gwei
		t.Fatalf("fee cap %s", first.GasFeeCap())
	}

	method, err := k.abi.MethodById(first.Data()[:4])
	if err != nil || method.Name != "kick" {
		t.Fatalf("calldata is not kick(): %v", err)
	}
	args, err := method.Inputs.Unpack(first.Data()[4:])
	if err != nil || args[0].(common.Address) != alice || args[1].(*big.Int).Int64() != 3 {
		t.Fatalf("args %v err=%v", args, err)
	}

	signer := types.LatestSignerForChainID(big.NewInt(11155111))
	if from, err := types.Sender(signer, first); err != nil || from != k.Address() {
		t.Fatalf("signed by %s err=%v", from, err)
	}
}

func TestTickSkipsPositionsThatCantBeKicked(t *testing.T) {
	alice := common.HexToAddress("0x00000000000000000000000000000000000a11ce")
	bob := common.HexToAddress("0x0000000000000000000000000000000000000b0b")
	client := &fakeClient{failEstimateFor: alice}
	k := newKeeper(t, client, fakeCandidates{{User: alice.Hex(), PositionID: 1}, {User: bob.Hex(), PositionID: 2}})

	n, err := k.Tick(context.Background())
	if err != nil || n != 1 || len(client.sent) != 1 || client.sent[0].Nonce() != 7 {
		t.Fatalf("sent %d err=%v", n, err)
	}
}

func TestTickWithNothingToDo(t *testing.T) {
	client := &fakeClient{}
	k := newKeeper(t, client, nil)
	if n, err := k.Tick(context.Background()); n != 0 || err != nil || len(client.sent) != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestInvalidKey(t *testing.T) {
	d, _ := chain.NewDecoder()
	if _, err := New(&fakeClient{}, nil, d.ABI(), contract, "not-a-key", time.Minute, slog.Default()); err == nil {
		t.Fatal("expected an error for an invalid key")
	}
}
