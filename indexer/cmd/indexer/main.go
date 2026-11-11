// Command indexer follows the StakingRewards contract, stores its events in Postgres,
// serves them over an HTTP API, and optionally runs a keeper that resets expired boosts.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rajdama/staking-rewards/indexer/internal/api"
	"github.com/rajdama/staking-rewards/indexer/internal/chain"
	"github.com/rajdama/staking-rewards/indexer/internal/config"
	"github.com/rajdama/staking-rewards/indexer/internal/indexer"
	"github.com/rajdama/staking-rewards/indexer/internal/keeper"
	"github.com/rajdama/staking-rewards/indexer/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}

	client, err := ethclient.DialContext(ctx, cfg.RPCURL)
	if err != nil {
		return err
	}
	defer client.Close()

	decoder, err := chain.NewDecoder()
	if err != nil {
		return err
	}

	ix := indexer.New(client, db, decoder, indexer.Options{
		Contract:      cfg.StakingAddress,
		StartBlock:    cfg.StartBlock,
		BatchSize:     cfg.BatchSize,
		Confirmations: cfg.Confirmations,
		PollInterval:  cfg.PollInterval,
	}, log.With("component", "indexer"))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.New(db, log.With("component", "api")).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 3)
	go func() { errs <- ix.Run(ctx) }()
	go func() {
		log.Info("api listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()
	if cfg.KeeperPrivateKey != "" {
		kp, err := keeper.New(client, db, decoder.ABI(), cfg.StakingAddress, cfg.KeeperPrivateKey,
			cfg.KeeperInterval, log.With("component", "keeper"))
		if err != nil {
			return err
		}
		log.Info("keeper enabled", "address", kp.Address().Hex())
		go func() { errs <- kp.Run(ctx) }()
	}

	select {
	case <-ctx.Done():
	case err = <-errs:
	}
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	return err
}
