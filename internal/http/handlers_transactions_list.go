package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type listTransactionsItem struct {
	ID            string    `json:"id"`
	FromAccountID string    `json:"from_account_id"`
	ToAccountID   string    `json:"to_account_id"`
	AmountCents   int64     `json:"amount_cents"`
	DeltaCents    int64     `json:"delta_cents"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
}

type listTransactionsResp struct {
	AccountID  string                 `json:"account_id"`
	Items      []listTransactionsItem `json:"items"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

func (h *TransactionsHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	accountIDRaw := r.URL.Query().Get("account_id")
	accountID, err := uuid.Parse(accountIDRaw)
	if err != nil {
		http.Error(w, "account_id must be a valid UUID", http.StatusBadRequest)
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

	var (
		cursorTime time.Time
		cursorID   uuid.UUID
		hasCursor  bool
	)
	if rawCursor := r.URL.Query().Get("cursor"); rawCursor != "" {
		parsedTime, parsedID, err := decodeCursor(rawCursor)
		if err != nil {
			http.Error(w, "cursor is invalid", http.StatusBadRequest)
			return
		}
		cursorTime = parsedTime
		cursorID = parsedID
		hasCursor = true
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var exists bool
	if err := h.DB.QueryRow(ctx, `SELECT true FROM accounts WHERE id = $1`, accountID).Scan(&exists); err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to check account existence", http.StatusInternalServerError)
		return
	}

	var rowsQuery string
	var rowsArgs []any

	if hasCursor {
		rowsQuery = `
			SELECT
				id,
				from_account_id,
				to_account_id,
				amount_cents,
				COALESCE(description, '') as description,
				created_at,
				CASE WHEN from_account_id = $1 THEN -amount_cents ELSE amount_cents END AS delta_cents
			FROM transactions
			WHERE (from_account_id = $1 OR to_account_id = $1)
				AND (created_at, id) < ($2, $3)
			ORDER BY created_at DESC, id DESC
			LIMIT $4
		`
		rowsArgs = []any{accountID, cursorTime, cursorID, limit}
	} else {
		rowsQuery = `
			SELECT
				id,
				from_account_id,
				to_account_id,
				amount_cents,
				COALESCE(description, '') as description,
				created_at,
				CASE WHEN from_account_id = $1 THEN -amount_cents ELSE amount_cents END AS delta_cents
			FROM transactions
			WHERE (from_account_id = $1 OR to_account_id = $1)
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		`
		rowsArgs = []any{accountID, limit}
	}

	rows, err := h.DB.Query(ctx, rowsQuery, rowsArgs...)
	if err != nil {
		http.Error(w, "failed to list transactions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := make([]listTransactionsItem, 0, limit)
	for rows.Next() {
		var (
			id        uuid.UUID
			fromID    uuid.UUID
			toID      uuid.UUID
			amount    int64
			desc      string
			createdAt time.Time
			delta     int64
		)
		if err := rows.Scan(&id, &fromID, &toID, &amount, &desc, &createdAt, &delta); err != nil {
			http.Error(w, "failed to scan transactions", http.StatusInternalServerError)
			return
		}
		items = append(items, listTransactionsItem{
			ID:            id.String(),
			FromAccountID: fromID.String(),
			ToAccountID:   toID.String(),
			AmountCents:   amount,
			DeltaCents:    delta,
			Description:   desc,
			CreatedAt:     createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read transactions", http.StatusInternalServerError)
		return
	}

	resp := listTransactionsResp{
		AccountID: accountID.String(),
		Items:     items,
	}
	if len(items) == limit {
		last := items[len(items)-1]
		lastTime := last.CreatedAt
		lastID, _ := uuid.Parse(last.ID)
		resp.NextCursor = encodeCursor(lastTime, lastID)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
