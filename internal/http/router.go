package httpapi

import (
	"net/http"

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

	r.Get("/health", d.Accounts.Health)
	r.Get("/readyz", d.Ready.Ready)

	// por enquanto só health e readyz. depois habilita:
	// r.Post("/accounts", d.Accounts.CreateAccount)
	// r.Post("/transactions", d.Tx.CreateTransaction)

	return r
}
