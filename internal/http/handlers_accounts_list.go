package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type listAccountsItem struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Currency     string    `json:"currency"`
	BalanceCents int64     `json:"balance_cents"`
	CreatedAt    time.Time `json:"created_at"`
}

type listAccountsResp struct {
	Items      []listAccountsItem `json:"items"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

func (h *AccountsHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
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

	var rowsQuery string
	var rowsArgs []any

	if hasCursor {
		rowsQuery = `
			SELECT id, name, currency, balance_cents, created_at
			FROM accounts
			WHERE (created_at, id) < ($1, $2)
			ORDER BY created_at DESC, id DESC
			LIMIT $3
		`
		rowsArgs = []any{cursorTime, cursorID, limit}
	} else {
		rowsQuery = `
			SELECT id, name, currency, balance_cents, created_at
			FROM accounts
			ORDER BY created_at DESC, id DESC
			LIMIT $1
		`
		rowsArgs = []any{limit}
	}

	rows, err := h.DB.Query(ctx, rowsQuery, rowsArgs...)
	if err != nil {
		http.Error(w, "failed to list accounts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := make([]listAccountsItem, 0, limit)
	for rows.Next() {
		var (
			id        uuid.UUID
			name      string
			currency  string
			balance   int64
			createdAt time.Time
		)
		if err := rows.Scan(&id, &name, &currency, &balance, &createdAt); err != nil {
			http.Error(w, "failed to scan accounts", http.StatusInternalServerError)
			return
		}
		items = append(items, listAccountsItem{
			ID:           id.String(),
			Name:         name,
			Currency:     currency,
			BalanceCents: balance,
			CreatedAt:    createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read accounts", http.StatusInternalServerError)
		return
	}

	resp := listAccountsResp{Items: items}
	if len(items) == limit {
		last := items[len(items)-1]
		lastTime := last.CreatedAt
		lastID, _ := uuid.Parse(last.ID)
		resp.NextCursor = encodeCursor(lastTime, lastID)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
