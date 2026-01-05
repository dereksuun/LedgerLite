package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Deps struct {
	Accounts *AccountsHandler
	Tx       *TransactionsHandler
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/health", d.Accounts.Health)

	// por enquanto só health. depois a gente habilita /accounts e /transactions.
	// r.Post("/accounts", d.Accounts.CreateAccount)
	// r.Post("/transactions", d.Tx.CreateTransaction)

	return r
}
