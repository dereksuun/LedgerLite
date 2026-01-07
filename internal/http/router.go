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
	if d.Accounts != nil {
		r.Get("/health", d.Accounts.Health)
		r.Get("/healthz", d.Accounts.Health)

		r.Post("/accounts", d.Accounts.CreateAccount)
		r.Get("/accounts/{id}", d.Accounts.GetAccount)
	}

	// Readiness (depende de DB)
	if d.Ready != nil {
		r.Get("/readyz", d.Ready.Ready)
	}

	// Funcionalidade
	if d.Accounts != nil {
		r.Post("/accounts", d.Accounts.CreateAccount)
		r.Get("/accounts/{id}", d.Accounts.GetAccount)
	}

	if d.Tx != nil {
		r.Post("/transactions", d.Tx.CreateTransaction)
	}

	return r
}
