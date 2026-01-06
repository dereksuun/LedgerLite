package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Deps struct {
	Accounts *AccountsHandler
	Tx       *TransactionsHandler
	Ready    *ReadyzHandler
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	r.Use(middleware.Timeout(60 * time.Second))

	// Liveness (não depende de DB)
	r.Get("/health", d.Accounts.Health)
	r.Get("/healthz", d.Accounts.Health)

	// Readiness (depende de DB)
	if d.Ready != nil {
		r.Get("/readyz", d.Ready.Ready)
	}

	// Funcionalidade
	r.Post("/accounts", d.Accounts.CreateAccount)
	// r.Post("/transactions", d.Tx.CreateTransaction)

	return r
}
