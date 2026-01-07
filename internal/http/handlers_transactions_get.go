package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type getTransactionResp struct {
	ID            string    `json:"id"`
	FromAccountID string    `json:"from_account_id"`
	ToAccountID   string    `json:"to_account_id"`
	AmountCents   int64     `json:"amount_cents"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
}

func (h *TransactionsHandler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")

	tid, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "id must be a valid UUID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var (
		id        uuid.UUID
		fromID    uuid.UUID
		toID      uuid.UUID
		amount    int64
		desc      string
		createdAt time.Time
	)

	err = h.DB.QueryRow(ctx, `
		SELECT id, from_account_id, to_account_id, amount_cents, COALESCE(description, ''), created_at
		FROM transactions
		WHERE id = $1
	`, tid).Scan(&id, &fromID, &toID, &amount, &desc, &createdAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "transaction not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to fetch transaction", http.StatusInternalServerError)
		return
	}

	resp := getTransactionResp{
		ID:            id.String(),
		FromAccountID: fromID.String(),
		ToAccountID:   toID.String(),
		AmountCents:   amount,
		Description:   desc,
		CreatedAt:     createdAt,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
