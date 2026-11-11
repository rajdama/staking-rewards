// Package chain decodes StakingRewards events from raw Ethereum logs.
package chain

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// EventType is the kind of staking event, as stored and served by the API.
type EventType string

const (
	EventStake    EventType = "stake"
	EventWithdraw EventType = "withdraw"
	EventKick     EventType = "kick"
	EventClaim    EventType = "claim"
)

// Event is one decoded contract event.
type Event struct {
	Type          EventType
	User          common.Address
	PositionID    *big.Int // stake, withdraw, kick
	Amount        *big.Int // stake, withdraw, claim
	MultiplierBps uint16   // stake
	UnlockTime    uint64   // stake
	Caller        common.Address
	BlockNumber   uint64
	TxHash        common.Hash
	LogIndex      uint
}

// StakingABI holds the events the indexer reads and the kick() function the keeper calls.
const StakingABI = `[
  {"type":"event","name":"Staked","anonymous":false,"inputs":[
    {"name":"user","type":"address","indexed":true},
    {"name":"positionId","type":"uint256","indexed":true},
    {"name":"amount","type":"uint256","indexed":false},
    {"name":"multiplierBps","type":"uint16","indexed":false},
    {"name":"unlockTime","type":"uint64","indexed":false}]},
  {"type":"event","name":"Withdrawn","anonymous":false,"inputs":[
    {"name":"user","type":"address","indexed":true},
    {"name":"positionId","type":"uint256","indexed":true},
    {"name":"amount","type":"uint256","indexed":false}]},
  {"type":"event","name":"Kicked","anonymous":false,"inputs":[
    {"name":"user","type":"address","indexed":true},
    {"name":"positionId","type":"uint256","indexed":true},
    {"name":"caller","type":"address","indexed":true}]},
  {"type":"event","name":"RewardPaid","anonymous":false,"inputs":[
    {"name":"user","type":"address","indexed":true},
    {"name":"reward","type":"uint256","indexed":false}]},
  {"type":"function","name":"kick","stateMutability":"nonpayable","inputs":[
    {"name":"user","type":"address"},
    {"name":"positionId","type":"uint256"}],"outputs":[]}
]`

// Decoder turns logs into Events.
type Decoder struct {
	abi abi.ABI
}

func NewDecoder() (*Decoder, error) {
	parsed, err := abi.JSON(strings.NewReader(StakingABI))
	if err != nil {
		return nil, fmt.Errorf("parse ABI: %w", err)
	}
	return &Decoder{abi: parsed}, nil
}

// ABI exposes the parsed ABI (used by the keeper to encode kick calls).
func (d *Decoder) ABI() abi.ABI { return d.abi }

// Topics returns the event signatures to filter logs by.
func (d *Decoder) Topics() []common.Hash {
	return []common.Hash{
		d.abi.Events["Staked"].ID,
		d.abi.Events["Withdrawn"].ID,
		d.abi.Events["Kicked"].ID,
		d.abi.Events["RewardPaid"].ID,
	}
}

// Decode returns the event in a log, or ok=false for events the indexer doesn't track.
func (d *Decoder) Decode(lg types.Log) (ev Event, ok bool, err error) {
	if len(lg.Topics) == 0 {
		return ev, false, nil
	}
	event, err := d.abi.EventByID(lg.Topics[0])
	if err != nil {
		return ev, false, nil // not one of ours
	}

	ev = Event{BlockNumber: lg.BlockNumber, TxHash: lg.TxHash, LogIndex: lg.Index}
	values := map[string]any{}
	if err := d.abi.UnpackIntoMap(values, event.Name, lg.Data); err != nil {
		return ev, false, fmt.Errorf("unpack %s: %w", event.Name, err)
	}

	switch event.Name {
	case "Staked":
		if err := needTopics(lg, 3); err != nil {
			return ev, false, err
		}
		ev.Type = EventStake
		ev.User = topicAddress(lg.Topics[1])
		ev.PositionID = topicUint(lg.Topics[2])
		ev.Amount = values["amount"].(*big.Int)
		ev.MultiplierBps = values["multiplierBps"].(uint16)
		ev.UnlockTime = values["unlockTime"].(uint64)
	case "Withdrawn":
		if err := needTopics(lg, 3); err != nil {
			return ev, false, err
		}
		ev.Type = EventWithdraw
		ev.User = topicAddress(lg.Topics[1])
		ev.PositionID = topicUint(lg.Topics[2])
		ev.Amount = values["amount"].(*big.Int)
	case "Kicked":
		if err := needTopics(lg, 4); err != nil {
			return ev, false, err
		}
		ev.Type = EventKick
		ev.User = topicAddress(lg.Topics[1])
		ev.PositionID = topicUint(lg.Topics[2])
		ev.Caller = topicAddress(lg.Topics[3])
	case "RewardPaid":
		if err := needTopics(lg, 2); err != nil {
			return ev, false, err
		}
		ev.Type = EventClaim
		ev.User = topicAddress(lg.Topics[1])
		ev.Amount = values["reward"].(*big.Int)
	default:
		return ev, false, nil
	}
	return ev, true, nil
}

func needTopics(lg types.Log, n int) error {
	if len(lg.Topics) < n {
		return fmt.Errorf("log %s:%d has %d topics, want %d", lg.TxHash.Hex(), lg.Index, len(lg.Topics), n)
	}
	return nil
}

func topicAddress(h common.Hash) common.Address { return common.BytesToAddress(h.Bytes()) }

func topicUint(h common.Hash) *big.Int { return new(big.Int).SetBytes(h.Bytes()) }
