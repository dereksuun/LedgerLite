package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledgerlite/internal/outbox"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// fallback (ajuste se sua senha/host forem diferentes)
		dsn = "postgres://postgres:postgres@localhost:5432/ledgerlite?sslmode=disable"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("worker: failed to connect: %v", err)
	}
	defer pool.Close()

	logger := log.Default()
	worker := &outbox.Worker{
		DB:              pool,
		Publisher:       &outbox.LoggerPublisher{Logger: logger},
		BatchSize:       25,
		PollInterval:    700 * time.Millisecond,
		LockTimeout:     30 * time.Second,
		PendingLogEvery: 5 * time.Second,
		Logger:          logger,
	}

	logger.Printf("worker: started (interval=%s batch=%d)", worker.PollInterval, worker.BatchSize)

	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Printf("worker: stopped with error: %v", err)
	}
	logger.Printf("worker: stopped")
}
