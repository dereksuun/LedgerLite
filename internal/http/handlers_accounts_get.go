package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type getAccountResp struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Currency     string    `json:"currency"`
	BalanceCents int64     `json:"balance_cents"`
	CreatedAt    time.Time `json:"created_at"`
}

func (h *AccountsHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSpace(chi.URLParam(r, "id"))
	accountID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "id must be a valid UUID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var (
		dbID      uuid.UUID
		name      string
		currency  string
		balance   int64
		createdAt time.Time
	)

	err = h.DB.QueryRow(ctx, `
		SELECT id, name, currency, balance_cents, created_at
		FROM accounts
		WHERE id = $1
	`, accountID).Scan(&dbID, &name, &currency, &balance, &createdAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to fetch account", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(getAccountResp{
		ID:           dbID.String(),
		Name:         name,
		Currency:     currency,
		BalanceCents: balance,
		CreatedAt:    createdAt,
	})
}
