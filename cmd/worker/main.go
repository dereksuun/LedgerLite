package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	ID          string
	EventType   string
	AggregateID string
	Payload     []byte
	CreatedAt   time.Time
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// fallback (ajuste se sua senha/host forem diferentes)
		dsn = "postgres://postgres:postgres@localhost:5432/ledgerlite?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("worker: failed to connect: %v", err)
	}
	defer pool.Close()

	interval := 700 * time.Millisecond
	log.Printf("worker: started (interval=%s)", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := processBatch(ctx, pool, 25); err != nil {
			log.Printf("worker: batch error: %v", err)
		}
	}
}

func processBatch(ctx context.Context, pool *pgxpool.Pool, limit int) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id::text, event_type, aggregate_id::text, payload, created_at
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	events := make([]Event, 0, limit)
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.EventType, &e.AggregateID, &e.Payload, &e.CreatedAt); err != nil {
			return err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(events) == 0 {
		// nada pra fazer
		return tx.Commit(ctx)
	}

	// “Publicar” (por enquanto: log estruturado)
	for _, e := range events {
		pretty := map[string]any{}
		_ = json.Unmarshal(e.Payload, &pretty)

		log.Printf("publish event=%s outbox_id=%s aggregate_id=%s payload=%v",
			e.EventType, e.ID, e.AggregateID, pretty)

		_, err := tx.Exec(ctx, `
			UPDATE outbox_events
			SET published_at = now()
			WHERE id = $1::uuid
		`, e.ID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
