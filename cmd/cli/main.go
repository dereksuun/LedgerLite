package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const defaultAPIURL = "http://localhost:8080"

type accountItem struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Currency     string    `json:"currency"`
	BalanceCents int64     `json:"balance_cents"`
	CreatedAt    time.Time `json:"created_at"`
}

type listAccountsResp struct {
	Items      []accountItem `json:"items"`
	NextCursor string        `json:"next_cursor"`
}

type statementItem struct {
	ID            string    `json:"id"`
	FromAccountID string    `json:"from_account_id"`
	ToAccountID   string    `json:"to_account_id"`
	AmountCents   int64     `json:"amount_cents"`
	DeltaCents    int64     `json:"delta_cents"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
}

type statementResp struct {
	AccountID string          `json:"account_id"`
	Items     []statementItem `json:"items"`
}

type createAccountResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type createTransactionResp struct {
	ID            string    `json:"id"`
	FromAccountID string    `json:"from_account_id"`
	ToAccountID   string    `json:"to_account_id"`
	AmountCents   int64     `json:"amount_cents"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "create-account":
		runCreateAccount(os.Args[2:])
	case "list-accounts":
		runListAccounts(os.Args[2:])
	case "transfer":
		runTransfer(os.Args[2:])
	case "statement":
		runStatement(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  cli create-account --name \"Alice\"")
	fmt.Fprintln(os.Stderr, "  cli list-accounts [--limit 50] [--cursor <cursor>]")
	fmt.Fprintln(os.Stderr, "  cli transfer --from <uuid> --to <uuid> --amount 1000 [--description \"...\"] [--idempotency-key <key>]")
	fmt.Fprintln(os.Stderr, "  cli statement --account <uuid> [--limit 50]")
	fmt.Fprintln(os.Stderr, "Env:")
	fmt.Fprintln(os.Stderr, "  API_URL (default: http://localhost:8080)")
}

func runCreateAccount(args []string) {
	fs := flag.NewFlagSet("create-account", flag.ExitOnError)
	name := fs.String("name", "", "account name")
	_ = fs.Parse(args)

	if strings.TrimSpace(*name) == "" {
		fmt.Fprintln(os.Stderr, "name is required")
		os.Exit(1)
	}

	req := map[string]string{"name": strings.TrimSpace(*name)}
	var resp createAccountResp
	if err := doJSON(context.Background(), http.MethodPost, apiURL("/accounts"), req, nil, &resp); err != nil {
		fail(err)
	}

	fmt.Printf("created account: %s (%s)\n", resp.ID, resp.Name)
}

func runListAccounts(args []string) {
	fs := flag.NewFlagSet("list-accounts", flag.ExitOnError)
	limit := fs.Int("limit", 50, "page size")
	cursor := fs.String("cursor", "", "pagination cursor")
	_ = fs.Parse(args)

	query := fmt.Sprintf("?limit=%d", *limit)
	if *cursor != "" {
		query += "&cursor=" + *cursor
	}

	var resp listAccountsResp
	if err := doJSON(context.Background(), http.MethodGet, apiURL("/accounts"+query), nil, nil, &resp); err != nil {
		fail(err)
	}

	fmt.Println("ID\tNAME\tCURRENCY\tBALANCE\tCREATED_AT")
	for _, item := range resp.Items {
		fmt.Printf("%s\t%s\t%s\t%d\t%s\n",
			item.ID,
			item.Name,
			item.Currency,
			item.BalanceCents,
			item.CreatedAt.Format(time.RFC3339),
		)
	}
	if resp.NextCursor != "" {
		fmt.Printf("next_cursor: %s\n", resp.NextCursor)
	}
}

func runTransfer(args []string) {
	fs := flag.NewFlagSet("transfer", flag.ExitOnError)
	fromID := fs.String("from", "", "from account UUID")
	toID := fs.String("to", "", "to account UUID")
	amount := fs.Int64("amount", 0, "amount cents")
	description := fs.String("description", "", "description")
	idemKey := fs.String("idempotency-key", "", "idempotency key")
	_ = fs.Parse(args)

	if strings.TrimSpace(*fromID) == "" || strings.TrimSpace(*toID) == "" || *amount <= 0 {
		fmt.Fprintln(os.Stderr, "from, to, and amount are required")
		os.Exit(1)
	}

	if *idemKey == "" {
		*idemKey = uuid.NewString()
	}

	req := map[string]any{
		"from_account_id": strings.TrimSpace(*fromID),
		"to_account_id":   strings.TrimSpace(*toID),
		"amount_cents":    *amount,
		"description":     strings.TrimSpace(*description),
	}

	headers := map[string]string{"Idempotency-Key": *idemKey}
	var resp createTransactionResp
	if err := doJSON(context.Background(), http.MethodPost, apiURL("/transactions"), req, headers, &resp); err != nil {
		fail(err)
	}

	fmt.Printf("transaction: %s from=%s to=%s amount=%d idempotency_key: %s\n", resp.ID, resp.FromAccountID, resp.ToAccountID, resp.AmountCents, *idemKey)
}

func runStatement(args []string) {
	fs := flag.NewFlagSet("statement", flag.ExitOnError)
	accountID := fs.String("account", "", "account UUID")
	limit := fs.Int("limit", 50, "page size")
	_ = fs.Parse(args)

	if strings.TrimSpace(*accountID) == "" {
		fmt.Fprintln(os.Stderr, "account is required")
		os.Exit(1)
	}

	url := fmt.Sprintf("/accounts/%s/statement?limit=%d", strings.TrimSpace(*accountID), *limit)
	var resp statementResp
	if err := doJSON(context.Background(), http.MethodGet, apiURL(url), nil, nil, &resp); err != nil {
		fail(err)
	}

	fmt.Println("ID\tDELTA\tAMOUNT\tFROM\tTO\tDESC\tCREATED_AT")
	for _, item := range resp.Items {
		fmt.Printf("%s\t%d\t%d\t%s\t%s\t%s\t%s\n",
			item.ID,
			item.DeltaCents,
			item.AmountCents,
			item.FromAccountID,
			item.ToAccountID,
			item.Description,
			item.CreatedAt.Format(time.RFC3339),
		)
	}
}

func apiURL(path string) string {
	base := strings.TrimRight(os.Getenv("API_URL"), "/")
	if base == "" {
		base = defaultAPIURL
	}
	return base + path
}

func doJSON(ctx context.Context, method, url string, body any, headers map[string]string, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("request failed: %s", msg)
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
