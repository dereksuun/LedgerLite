package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"ledgerlite/internal/outbox"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MetricsHandler struct {
	DB *pgxpool.Pool
}

func (h *MetricsHandler) Prometheus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	pending, err := outbox.CountPending(ctx, h.DB)
	if err != nil {
		http.Error(w, "failed to collect metrics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "outbox_pending %d\n", pending)
}
