package outbox

import (
	"context"
	"errors"
	"fmt"
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

	// Se um worker morrer no meio do publish e não limpar processing_at,
	// outro worker pode “reclamar” depois desse tempo.
	LockTimeout time.Duration

	// Evita log spam: em vez de logar pending a cada loop.
	PendingLogEvery time.Duration

	Logger *log.Logger
}

type claimedEvent struct {
	Event
	Attempts int
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
	if w.LockTimeout <= 0 {
		w.LockTimeout = 30 * time.Second
	}
	if w.PendingLogEvery <= 0 {
		w.PendingLogEvery = 5 * time.Second
	}

	nextPendingLog := time.Now()

	for {
		// Processa até “esvaziar” o que está disponível agora
		n, err := w.processUntilEmpty(ctx)
		if err != nil && w.Logger != nil {
			w.Logger.Printf("outbox worker error: %v", err)
		}

		// Log de pending com throttle
		if w.Logger != nil && time.Now().After(nextPendingLog) {
			if err := w.logPending(ctx); err != nil {
				w.Logger.Printf("outbox pending count error: %v", err)
			}
			nextPendingLog = time.Now().Add(w.PendingLogEvery)
		}

		// Se não teve nada pra fazer, dorme até o próximo poll
		if n == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(w.PollInterval):
			}
		}

		// Se processou algo, loopa de novo sem dormir (pra drenar fila rápido)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (w *Worker) processUntilEmpty(ctx context.Context) (int, error) {
	total := 0
	for {
		n, err := w.processOnce(ctx)
		if err != nil {
			return total, err
		}
		total += n
		if n == 0 {
			return total, nil
		}
	}
}

func (w *Worker) processOnce(ctx context.Context) (int, error) {
	events, err := w.claimBatch(ctx)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}

	for _, ce := range events {
		// respeita cancelamento
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		if err := w.Publisher.Publish(ctx, ce.Event); err != nil {
			// Falhou: salva erro e agenda retry com backoff
			if w.Logger != nil {
				w.Logger.Printf("publish failed: outbox_id=%s err=%v", ce.ID, err)
			}
			if uerr := w.markFailed(ctx, ce.Attempts, ce.ID, err); uerr != nil {
				return 0, uerr
			}
			continue
		}

		// Sucesso: marca published_at (ack)
		if err := w.markPublished(ctx, ce.ID); err != nil {
			return 0, err
		}
	}

	return len(events), nil
}

// claimBatch “reserva” eventos: seta processing_at e devolve os dados.
// Depois a gente publica FORA da transação.
func (w *Worker) claimBatch(ctx context.Context) ([]claimedEvent, error) {
	tx, err := w.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	lockSecs := int(w.LockTimeout.Seconds())
	if lockSecs <= 0 {
		lockSecs = 30
	}

	rows, err := tx.Query(ctx, `
WITH picked AS (
	SELECT id
	FROM outbox_events
	WHERE published_at IS NULL
	  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
	  AND (
	    processing_at IS NULL
	    OR processing_at < now() - make_interval(secs => $2)
	  )
	ORDER BY created_at, id
	LIMIT $1
	FOR UPDATE SKIP LOCKED
)
UPDATE outbox_events o
SET processing_at = now()
FROM picked
WHERE o.id = picked.id
RETURNING o.id, o.aggregate_type, o.aggregate_id, o.event_type, o.payload, o.created_at, o.attempts
`, w.BatchSize, lockSecs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]claimedEvent, 0, w.BatchSize)
	for rows.Next() {
		var ce claimedEvent
		if err := rows.Scan(
			&ce.ID, &ce.AggregateType, &ce.AggregateID,
			&ce.EventType, &ce.Payload, &ce.CreatedAt, &ce.Attempts,
		); err != nil {
			return nil, err
		}
		out = append(out, ce)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (w *Worker) markPublished(ctx context.Context, id uuid.UUID) error {
	_, err := w.DB.Exec(ctx, `
UPDATE outbox_events
SET published_at = now(),
    processing_at = NULL,
    last_error = NULL
WHERE id = $1
`, id)
	return err
}

func (w *Worker) markFailed(ctx context.Context, attempts int, id uuid.UUID, publishErr error) error {
	// attempts aqui é o valor atual; vamos gravar attempts+1
	nextAttempts := attempts + 1
	delay := backoff(nextAttempts)
	next := time.Now().Add(delay)

	errText := publishErr.Error()
	if len(errText) > 500 {
		errText = errText[:500]
	}

	_, err := w.DB.Exec(ctx, `
UPDATE outbox_events
SET attempts = attempts + 1,
    last_error = $2,
    next_attempt_at = $3,
    processing_at = NULL
WHERE id = $1
`, id, errText, next)
	return err
}

// backoff exponencial com teto
func backoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 1 * time.Second
	}
	d := time.Second * time.Duration(1<<uint(min(attempt-1, 6))) // 1,2,4,8,16,32,64
	if d > 60*time.Second {
		return 60 * time.Second
	}
	return d
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
		return 0, fmt.Errorf("count pending: %w", err)
	}
	return pending, nil
}
