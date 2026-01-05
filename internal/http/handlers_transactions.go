package httpapi

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TransactionsHandler struct {
	DB *pgxpool.Pool
}

func (h *TransactionsHandler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented yet", http.StatusNotImplemented)
}
