// Package config loads indexer settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Config struct {
	RPCURL         string
	DatabaseURL    string
	StakingAddress common.Address
	StartBlock     uint64
	Port           string
	PollInterval   time.Duration
	BatchSize      uint64
	Confirmations  uint64
	// Optional: when set, the keeper resets expired lock boosts by calling kick().
	KeeperPrivateKey string
	KeeperInterval   time.Duration
}

// Load reads the configuration. Required: RPC_URL, DATABASE_URL, STAKING_ADDRESS.
func Load() (Config, error) {
	c := Config{
		RPCURL:           os.Getenv("RPC_URL"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		Port:             getenv("PORT", "8080"),
		KeeperPrivateKey: os.Getenv("KEEPER_PRIVATE_KEY"),
	}
	if c.RPCURL == "" {
		return c, fmt.Errorf("RPC_URL is required")
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	addr := os.Getenv("STAKING_ADDRESS")
	if !common.IsHexAddress(addr) {
		return c, fmt.Errorf("STAKING_ADDRESS must be a valid address, got %q", addr)
	}
	c.StakingAddress = common.HexToAddress(addr)

	var err error
	if c.StartBlock, err = getUint("START_BLOCK", 0); err != nil {
		return c, err
	}
	if c.BatchSize, err = getUint("BATCH_SIZE", 2000); err != nil {
		return c, err
	}
	if c.Confirmations, err = getUint("CONFIRMATIONS", 3); err != nil {
		return c, err
	}
	if c.PollInterval, err = getDuration("POLL_INTERVAL", 6*time.Second); err != nil {
		return c, err
	}
	if c.KeeperInterval, err = getDuration("KEEPER_INTERVAL", 10*time.Minute); err != nil {
		return c, err
	}
	if c.BatchSize == 0 {
		return c, fmt.Errorf("BATCH_SIZE must be greater than 0")
	}
	return c, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getUint(key string, fallback uint64) (uint64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a non-negative integer: %w", key, err)
	}
	return n, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration like 10s or 5m: %w", key, err)
	}
	return d, nil
}
