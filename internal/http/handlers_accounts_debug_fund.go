package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type debugFundReq struct {
	AmountCents int64 `json:"amount_cents"`
}

func (h *AccountsHandler) DebugFundAccount(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	accID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "id must be a valid UUID", http.StatusBadRequest)
		return
	}

	var req debugFundReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.AmountCents <= 0 {
		http.Error(w, "amount_cents must be > 0", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	ct, err := h.DB.Exec(ctx,
		`UPDATE accounts SET balance_cents = balance_cents + $1 WHERE id = $2`,
		req.AmountCents, accID,
	)
	if err != nil {
		http.Error(w, "failed to fund account", http.StatusInternalServerError)
		return
	}
	if ct.RowsAffected() != 1 {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
