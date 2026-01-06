package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ReadyzHandler struct {
	DB *pgxpool.Pool
}

func (h *ReadyzHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.DB.Ping(ctx); err != nil {
		http.Error(w, "NOT READY", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusNoContent) // 204
}
