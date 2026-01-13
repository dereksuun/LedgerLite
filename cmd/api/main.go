package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"ledgerlite/internal/db"
	httpapi "ledgerlite/internal/http"
)

func main() {
	// Default local: se não setar DATABASE_URL, usa localhost
	if os.Getenv("DATABASE_URL") == "" {
		os.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/ledgerlite?sslmode=disable")
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	accountsHandler := &httpapi.AccountsHandler{DB: pool}
	txHandler := &httpapi.TransactionsHandler{DB: pool}
	readyHandler := &httpapi.ReadyzHandler{DB: pool} // <- aqui estava o bug (era DB: db)
	metricsHandler := &httpapi.MetricsHandler{DB: pool}
	adminHandler := &httpapi.AdminHandler{DB: pool}

	router := httpapi.NewRouter(httpapi.Deps{
		Accounts: accountsHandler,
		Tx:       txHandler,
		Ready:    readyHandler,
		Metrics:  metricsHandler,
		Admin:    adminHandler,
	})

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("API running on :8080")
	log.Fatal(srv.ListenAndServe())
}
