package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type statementItem struct {
	ID            string    `json:"id"`
	FromAccountID string    `json:"from_account_id"`
	ToAccountID   string    `json:"to_account_id"`
	AmountCents   int64     `json:"amount_cents"`
	DeltaCents    int64     `json:"delta_cents"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
}

type statementResp struct {
	AccountID string          `json:"account_id"`
	Items     []statementItem `json:"items"`
}

func (h *AccountsHandler) GetAccountStatement(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	accountID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "id must be a valid UUID", http.StatusBadRequest)
		return
	}

	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
		if n > 200 {
			n = 200
		}
		limit = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var exists bool
	if err := h.DB.QueryRow(ctx, `Select true FROM accounts WHERE id = $1`, accountID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to check account existence", http.StatusInternalServerError)
		return
	}

	rows, err := h.DB.Query(ctx, `
			SELECT
				id,
				from_account_id,
				to_account_id,
				amount_cents,
				COALESCE(description, '') as description,
				created_at,
				CASE WHEN from_account_id = $1 THEN -amount_cents ELSE amount_cents END AS delta_cents
			FROM transactions
			WHERE from_account_id = $1 OR to_account_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2
	`, accountID, limit)
	if err != nil {
		http.Error(w, "failed to fetch statement", http.StatusInternalServerError)
		return
	}

	defer rows.Close()

	items := make([]statementItem, 0, limit)
	for rows.Next() {
		var (
			tid       uuid.UUID
			fromID    uuid.UUID
			toID      uuid.UUID
			amount    int64
			desc      string
			createdAt time.Time
			delta     int64
		)

		if err := rows.Scan(&tid, &fromID, &toID, &amount, &desc, &createdAt, &delta); err != nil {
			http.Error(w, "failed to scan statement item", http.StatusInternalServerError)
			return
		}

		items = append(items, statementItem{
			ID:            tid.String(),
			FromAccountID: fromID.String(),
			ToAccountID:   toID.String(),
			AmountCents:   amount,
			DeltaCents:    delta,
			Description:   desc,
			CreatedAt:     createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read statement", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(statementResp{
		AccountID: accountID.String(),
		Items:     items,
	})
}
