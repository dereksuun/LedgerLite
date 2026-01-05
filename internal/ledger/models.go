package ledger

import "github.com/google/uuid"

type TxRequest struct {
	Type	   string                 `json:"type"`
	AccountID  uuid.UUID              `json:"account_id"`
	FromAccountID *uuid.UUID            `json:"from_account_id,omitempty"`
	ToAccountID   *uuid.UUID            `json:"to_account_id,omitempty"`
	AmountCents     int64                  `json:"amount_cents"`
	Description string                 `json:"description,omitempty"`
	Currency   string                 `json:"currency"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type TxResponse struct {
	TransactionID uuid.UUID `json:"transaction_id"`
}