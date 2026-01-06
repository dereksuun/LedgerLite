package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type createAccountReq struct {
	Name string `json:"name"`
}

type createAccountResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (h *AccountsHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountReq

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	newID := uuid.NewString()

	var id string
	err := h.DB.QueryRow(ctx,
		`INSERT INTO accounts (id, name) VALUES ($1, $2) RETURNING id`,
		newID, req.Name,
	).Scan(&id)

	if err != nil {
		log.Printf("CreateAccount DB error: %v", err) // <- importante pra ver o erro real
		http.Error(w, "failed to create account", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(createAccountResp{ID: id, Name: req.Name})
}
