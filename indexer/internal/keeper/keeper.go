// Package keeper resets expired lock boosts by calling StakingRewards.kick().
//
// Without a keeper, a position keeps its boost after the lock ends until someone kicks it,
// which slightly over-rewards that staker. Running this keeps rewards fair for everyone.
package keeper

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/rajdama/staking-rewards/indexer/internal/store"
)

// Client is the subset of an Ethereum RPC client the keeper needs.
type Client interface {
	ChainID(ctx context.Context) (*big.Int, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
}

// Candidates lists positions to kick (the Postgres store in production).
type Candidates interface {
	KickCandidates(ctx context.Context, now time.Time, limit int) ([]store.KickCandidate, error)
}

type Keeper struct {
	client   Client
	data     Candidates
	abi      abi.ABI
	contract common.Address
	key      *ecdsa.PrivateKey
	from     common.Address
	interval time.Duration
	log      *slog.Logger
}

func New(client Client, data Candidates, stakingABI abi.ABI, contract common.Address, hexKey string,
	interval time.Duration, log *slog.Logger) (*Keeper, error) {
	key, err := crypto.HexToECDSA(trim0x(hexKey))
	if err != nil {
		return nil, fmt.Errorf("invalid KEEPER_PRIVATE_KEY: %w", err)
	}
	return &Keeper{
		client:   client,
		data:     data,
		abi:      stakingABI,
		contract: contract,
		key:      key,
		from:     crypto.PubkeyToAddress(key.PublicKey),
		interval: interval,
		log:      log,
	}, nil
}

// Address is the keeper's account (it needs a little testnet ETH for gas).
func (k *Keeper) Address() common.Address { return k.from }

func (k *Keeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(k.interval)
	defer ticker.Stop()
	for {
		if n, err := k.Tick(ctx); err != nil {
			k.log.Error("keeper tick failed", "err", err)
		} else if n > 0 {
			k.log.Info("kicked expired boosts", "count", n)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Tick sends one kick() per expired boosted position and returns how many were sent.
func (k *Keeper) Tick(ctx context.Context) (int, error) {
	candidates, err := k.data.KickCandidates(ctx, time.Now(), 20)
	if err != nil {
		return 0, fmt.Errorf("load candidates: %w", err)
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	chainID, err := k.client.ChainID(ctx)
	if err != nil {
		return 0, err
	}
	nonce, err := k.client.PendingNonceAt(ctx, k.from)
	if err != nil {
		return 0, err
	}
	tip, err := k.client.SuggestGasTipCap(ctx)
	if err != nil {
		return 0, err
	}
	head, err := k.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, err
	}
	feeCap := new(big.Int).Add(tip, new(big.Int).Mul(head.BaseFee, big.NewInt(2)))
	signer := types.LatestSignerForChainID(chainID)

	sent := 0
	for _, c := range candidates {
		data, err := k.abi.Pack("kick", common.HexToAddress(c.User), big.NewInt(c.PositionID))
		if err != nil {
			return sent, err
		}
		gas, err := k.client.EstimateGas(ctx, ethereum.CallMsg{From: k.from, To: &k.contract, Data: data})
		if err != nil {
			// Usually means someone else kicked it first; the indexer will catch up.
			k.log.Warn("skip kick", "user", c.User, "position", c.PositionID, "err", err)
			continue
		}
		tx, err := types.SignTx(types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			GasTipCap: tip,
			GasFeeCap: feeCap,
			Gas:       gas * 12 / 10,
			To:        &k.contract,
			Data:      data,
		}), signer, k.key)
		if err != nil {
			return sent, err
		}
		if err := k.client.SendTransaction(ctx, tx); err != nil {
			return sent, fmt.Errorf("send kick: %w", err)
		}
		k.log.Info("sent kick", "user", c.User, "position", c.PositionID, "tx", tx.Hash().Hex())
		nonce++
		sent++
	}
	return sent, nil
}

func trim0x(s string) string {
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		return s[2:]
	}
	return s
}
