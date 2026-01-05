package httpapi

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AccountsHandler struct {
	DB *pgxpool.Pool
}

func (h *AccountsHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK"))
}
