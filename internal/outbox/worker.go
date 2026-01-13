package outbox

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	DB           *pgxpool.Pool
	Publisher    Publisher
	BatchSize    int
	PollInterval time.Duration
	Logger       *log.Logger
}

func (w *Worker) Run(ctx context.Context) error {
	if w.DB == nil {
		return errors.New("outbox worker requires DB")
	}
	if w.Publisher == nil {
		return errors.New("outbox worker requires Publisher")
	}
	if w.BatchSize <= 0 {
		w.BatchSize = 10
	}
	if w.PollInterval <= 0 {
		w.PollInterval = 2 * time.Second
	}

	ticker := time.NewTicker(w.PollInterval)
	defer ticker.Stop()

	for {
		if err := w.processUntilEmpty(ctx); err != nil {
			if w.Logger != nil {
				w.Logger.Printf("outbox worker error: %v", err)
			}
		}

		if err := w.logPending(ctx); err != nil && w.Logger != nil {
			w.Logger.Printf("outbox pending count error: %v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) processUntilEmpty(ctx context.Context) error {
	for {
		n, err := w.processBatch(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

func (w *Worker) processBatch(ctx context.Context) (int, error) {
	tx, err := w.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, created_at
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY created_at, id
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, w.BatchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	events := make([]Event, 0, w.BatchSize)
	for rows.Next() {
		var ev Event
		if err := rows.Scan(&ev.ID, &ev.AggregateType, &ev.AggregateID, &ev.EventType, &ev.Payload, &ev.CreatedAt); err != nil {
			return 0, err
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, tx.Commit(ctx)
	}

	for _, ev := range events {
		if err := w.Publisher.Publish(ctx, ev); err != nil {
			return 0, err
		}
	}

	ids := make([]uuid.UUID, 0, len(events))
	for _, ev := range events {
		ids = append(ids, ev.ID)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET published_at = now()
		WHERE id = ANY($1)
	`, ids); err != nil {
		return 0, err
	}

	return len(events), tx.Commit(ctx)
}

func (w *Worker) logPending(ctx context.Context) error {
	if w.Logger == nil {
		return nil
	}
	pending, err := CountPending(ctx, w.DB)
	if err != nil {
		return err
	}
	w.Logger.Printf("outbox_pending=%d", pending)
	return nil
}

type pendingCounter interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func CountPending(ctx context.Context, q pendingCounter) (int, error) {
	var pending int
	if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE published_at IS NULL`).Scan(&pending); err != nil {
		return 0, err
	}
	return pending, nil
}
