package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AdminHandler struct {
	DB *pgxpool.Pool
}

type reconcileMismatch struct {
	AccountID            string `json:"account_id"`
	BalanceCents         int64  `json:"balance_cents"`
	ComputedBalanceCents int64  `json:"computed_balance_cents"`
}

type reconcileResp struct {
	Checked    int                 `json:"checked"`
	Mismatches []reconcileMismatch `json:"mismatches"`
}

func (h *AdminHandler) ReconcileBalances(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := h.DB.Query(ctx, `
		SELECT
			a.id,
			a.balance_cents,
			COALESCE(SUM(
				CASE
					WHEN t.to_account_id = a.id THEN t.amount_cents
					ELSE -t.amount_cents
				END
			), 0) AS computed_balance
		FROM accounts a
		LEFT JOIN transactions t
			ON t.from_account_id = a.id OR t.to_account_id = a.id
		GROUP BY a.id, a.balance_cents
		ORDER BY a.id
	`)
	if err != nil {
		http.Error(w, "failed to reconcile balances", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	resp := reconcileResp{
		Mismatches: []reconcileMismatch{},
	}

	for rows.Next() {
		var (
			accountID uuid.UUID
			stored    int64
			computed  int64
		)
		if err := rows.Scan(&accountID, &stored, &computed); err != nil {
			http.Error(w, "failed to scan reconciliation result", http.StatusInternalServerError)
			return
		}
		resp.Checked++
		if stored != computed {
			log.Printf("reconcile mismatch account_id=%s stored=%d computed=%d", accountID, stored, computed)
			resp.Mismatches = append(resp.Mismatches, reconcileMismatch{
				AccountID:            accountID.String(),
				BalanceCents:         stored,
				ComputedBalanceCents: computed,
			})
		}
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read reconciliation result", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
