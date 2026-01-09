package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TransactionsHandler struct {
	DB *pgxpool.Pool
}

type createTransactionReq struct {
	FromAccountID string `json:"from_account_id"`
	ToAccountID   string `json:"to_account_id"`
	AmountCents   int64  `json:"amount_cents"`
	Description   string `json:"description"`
}

type createTransactionResp struct {
	ID            string    `json:"id"`
	FromAccountID string    `json:"from_account_id"`
	ToAccountID   string    `json:"to_account_id"`
	AmountCents   int64     `json:"amount_cents"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
}

var (
	errAccountNotFound     = errors.New("account not found")
	errInsufficientFunds   = errors.New("insufficient funds")
	errCurrencyMismatch    = errors.New("currency mismatch")
	errIdempotencyConflict = errors.New("idempotency key conflict")
)

func (h *TransactionsHandler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idemKey == "" {
		http.Error(w, "Idempotency-Key header is required", http.StatusBadRequest)
		return
	}

	var req createTransactionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	req.FromAccountID = strings.TrimSpace(req.FromAccountID)
	req.ToAccountID = strings.TrimSpace(req.ToAccountID)
	req.Description = strings.TrimSpace(req.Description)

	// (Opcional, mas saudável) evita descrição gigante
	if len(req.Description) > 280 {
		http.Error(w, "description is too long (max 280 chars)", http.StatusBadRequest)
		return
	}

	fromID, err := uuid.Parse(req.FromAccountID)
	if err != nil {
		http.Error(w, "from_account_id must be a valid UUID", http.StatusBadRequest)
		return
	}
	toID, err := uuid.Parse(req.ToAccountID)
	if err != nil {
		http.Error(w, "to_account_id must be a valid UUID", http.StatusBadRequest)
		return
	}
	if fromID == toID {
		http.Error(w, "from_account_id and to_account_id must be different", http.StatusBadRequest)
		return
	}
	if req.AmountCents <= 0 {
		http.Error(w, "amount_cents must be > 0", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var (
		resp       createTransactionResp
		httpStatus = http.StatusCreated // vira 200 se for replay
	)

	err = withTx(ctx, h.DB, func(tx pgx.Tx) error {
		// 1) tenta inserir (ou pega a existente pelo mesmo Idempotency-Key)
		tid := uuid.New()

		var (
			dbID        uuid.UUID
			dbFrom      uuid.UUID
			dbTo        uuid.UUID
			dbAmount    int64
			dbDesc      string
			dbCreatedAt time.Time
			inserted    bool
		)

		err := tx.QueryRow(ctx, `
			INSERT INTO transactions (id, idempotency_key, from_account_id, to_account_id, amount_cents, description)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (idempotency_key)
			DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
			RETURNING
				id, from_account_id, to_account_id, amount_cents, COALESCE(description, ''),
				created_at, (xmax = 0) AS inserted
		`, tid, idemKey, fromID, toID, req.AmountCents, req.Description).
			Scan(&dbID, &dbFrom, &dbTo, &dbAmount, &dbDesc, &dbCreatedAt, &inserted)
		if err != nil {
			return err
		}

		// 2) se já existia, valida que o request é IGUAL; se for, retorna 200 sem mexer em saldo
		if !inserted {
			if dbFrom != fromID || dbTo != toID || dbAmount != req.AmountCents || dbDesc != req.Description {
				return errIdempotencyConflict
			}

			httpStatus = http.StatusOK
			resp = createTransactionResp{
				ID:            dbID.String(),
				FromAccountID: dbFrom.String(),
				ToAccountID:   dbTo.String(),
				AmountCents:   dbAmount,
				Description:   dbDesc,
				CreatedAt:     dbCreatedAt,
			}
			return nil
		}

		// 3) se é nova, aplica efeitos
		type acc struct {
			id      uuid.UUID
			balance int64
			curr    string
		}
		found := map[uuid.UUID]acc{}

		rows, err := tx.Query(ctx, `
			SELECT id, balance_cents, currency
			FROM accounts
			WHERE id IN ($1, $2)
			ORDER BY id
			FOR UPDATE
		`, fromID, toID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a acc
			if err := rows.Scan(&a.id, &a.balance, &a.curr); err != nil {
				return err
			}
			found[a.id] = a
		}
		if err := rows.Err(); err != nil {
			return err
		}

		fromAcc, ok1 := found[fromID]
		toAcc, ok2 := found[toID]
		if !ok1 || !ok2 {
			return errAccountNotFound
		}
		if fromAcc.curr != "" && toAcc.curr != "" && fromAcc.curr != toAcc.curr {
			return errCurrencyMismatch
		}

		ct, err := tx.Exec(ctx, `
			UPDATE accounts
			SET balance_cents = balance_cents - $1
			WHERE id = $2 AND balance_cents >= $1
		`, req.AmountCents, fromID)
		if err != nil {
			return err
		}
		if ct.RowsAffected() != 1 {
			return errInsufficientFunds
		}

		if _, err := tx.Exec(ctx, `
			UPDATE accounts
			SET balance_cents = balance_cents + $1
			WHERE id = $2
		`, req.AmountCents, toID); err != nil {
			return err
		}

		payload := map[string]any{
			"transaction_id":  dbID.String(),
			"from_account_id": dbFrom.String(),
			"to_account_id":   dbTo.String(),
			"amount_cents":    dbAmount,
			"description":     dbDesc,
			"created_at":      dbCreatedAt,
		}

		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, payload)
			VALUES ($1, 'transaction', $2, 'TransactionCreated', $3::jsonb)
		`, uuid.New(), dbID, string(b))
		if err != nil {
			return err
		}

		resp = createTransactionResp{
			ID:            dbID.String(),
			FromAccountID: dbFrom.String(),
			ToAccountID:   dbTo.String(),
			AmountCents:   dbAmount,
			Description:   dbDesc,
			CreatedAt:     dbCreatedAt,
		}
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, errIdempotencyConflict):
			http.Error(w, "Idempotency-Key already used with different request", http.StatusConflict)
		case errors.Is(err, errAccountNotFound):
			http.Error(w, "account not found", http.StatusNotFound)
		case errors.Is(err, errCurrencyMismatch):
			http.Error(w, "currency mismatch", http.StatusBadRequest)
		case errors.Is(err, errInsufficientFunds):
			http.Error(w, "insufficient funds", http.StatusConflict)
		default:
			http.Error(w, "failed to create transaction", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(resp)
}

// helper: tx wrapper
func withTx(ctx context.Context, pool interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
