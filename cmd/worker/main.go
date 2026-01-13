package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"ledgerlite/internal/db"
	"ledgerlite/internal/outbox"
)

func main() {
	if os.Getenv("DATABASE_URL") == "" {
		os.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/ledgerlite?sslmode=disable")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	logger := log.New(os.Stdout, "", log.LstdFlags)
	publisher := &outbox.LoggerPublisher{Logger: logger}

	worker := &outbox.Worker{
		DB:           pool,
		Publisher:    publisher,
		BatchSize:    envInt("OUTBOX_BATCH_SIZE", 10),
		PollInterval: envDuration("OUTBOX_POLL_INTERVAL", 2*time.Second),
		Logger:       logger,
	}

	logger.Println("outbox worker started")
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return fallback
	}
	return val
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	val, err := time.ParseDuration(raw)
	if err != nil || val <= 0 {
		return fallback
	}
	return val
}
