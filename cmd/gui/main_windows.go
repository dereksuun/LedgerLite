//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const defaultAPIURL = "http://localhost:8080"

type guiApp struct {
	mw     *walk.MainWindow
	client *http.Client

	apiURL *walk.LineEdit
	status *walk.Label
	output *walk.TextEdit

	createName *walk.LineEdit
	listLimit  *walk.LineEdit
	listCursor *walk.LineEdit

	transferFrom        *walk.LineEdit
	transferTo          *walk.LineEdit
	transferAmount      *walk.LineEdit
	transferDescription *walk.LineEdit
	transferIdemKey     *walk.LineEdit

	statementAccount *walk.LineEdit
	statementLimit   *walk.LineEdit
}

func main() {
	app := &guiApp{
		client: &http.Client{Timeout: 12 * time.Second},
	}

	if _, err := (MainWindow{
		AssignTo: &app.mw,
		Title:    "LedgerLite Desktop",
		MinSize:  Size{Width: 980, Height: 760},
		Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					Label{Text: "API URL"},
					LineEdit{AssignTo: &app.apiURL, Text: defaultAPIURL},
					PushButton{
						Text:      "Health",
						MaxSize:   Size{Width: 120, Height: 0},
						OnClicked: app.onHealth,
					},
					PushButton{
						Text:      "Clear Output",
						MaxSize:   Size{Width: 120, Height: 0},
						OnClicked: app.onClearOutput,
					},
				},
			},
			GroupBox{
				Title:  "Accounts",
				Layout: Grid{Columns: 4},
				Children: []Widget{
					Label{Text: "Name"},
					LineEdit{AssignTo: &app.createName},
					PushButton{
						Text:      "Create Account",
						OnClicked: app.onCreateAccount,
					},
					VSpacer{},

					Label{Text: "List Limit"},
					LineEdit{AssignTo: &app.listLimit, Text: "50"},
					Label{Text: "Cursor (optional)"},
					LineEdit{AssignTo: &app.listCursor},

					HSpacer{},
					PushButton{
						Text:      "List Accounts",
						OnClicked: app.onListAccounts,
					},
					HSpacer{},
					HSpacer{},
				},
			},
			GroupBox{
				Title:  "Transfer",
				Layout: Grid{Columns: 4},
				Children: []Widget{
					Label{Text: "From Account ID"},
					LineEdit{AssignTo: &app.transferFrom},
					Label{Text: "To Account ID"},
					LineEdit{AssignTo: &app.transferTo},

					Label{Text: "Amount (cents)"},
					LineEdit{AssignTo: &app.transferAmount},
					Label{Text: "Description"},
					LineEdit{AssignTo: &app.transferDescription},

					Label{Text: "Idempotency Key (optional)"},
					LineEdit{AssignTo: &app.transferIdemKey},
					PushButton{
						Text:      "Transfer",
						OnClicked: app.onTransfer,
					},
					HSpacer{},
				},
			},
			GroupBox{
				Title:  "Statement",
				Layout: Grid{Columns: 4},
				Children: []Widget{
					Label{Text: "Account ID"},
					LineEdit{AssignTo: &app.statementAccount},
					Label{Text: "Limit"},
					LineEdit{AssignTo: &app.statementLimit, Text: "50"},

					HSpacer{},
					PushButton{
						Text:      "Load Statement",
						OnClicked: app.onStatement,
					},
					HSpacer{},
					HSpacer{},
				},
			},
			Label{AssignTo: &app.status, Text: "Ready"},
			TextEdit{
				AssignTo: &app.output,
				ReadOnly: true,
				VScroll:  true,
			},
		},
	}).Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *guiApp) onClearOutput() {
	if a.output == nil {
		return
	}
	_ = a.output.SetText("")
	a.setStatus("Output cleared")
}

func (a *guiApp) onHealth() {
	base := a.baseURL()
	a.runAction("GET /health", func(ctx context.Context) (int, string, error) {
		return a.doJSON(ctx, http.MethodGet, base+"/health", nil, nil)
	})
}

func (a *guiApp) onCreateAccount() {
	name := strings.TrimSpace(a.createName.Text())
	if name == "" {
		walk.MsgBox(a.mw, "Validation", "Name is required.", walk.MsgBoxIconWarning)
		return
	}

	base := a.baseURL()
	a.runAction("POST /accounts", func(ctx context.Context) (int, string, error) {
		payload := map[string]string{"name": name}
		return a.doJSON(ctx, http.MethodPost, base+"/accounts", payload, nil)
	})
}

func (a *guiApp) onListAccounts() {
	limit, err := parsePositiveInt(a.listLimit.Text(), 50)
	if err != nil {
		walk.MsgBox(a.mw, "Validation", err.Error(), walk.MsgBoxIconWarning)
		return
	}

	base := a.baseURL()
	cursor := strings.TrimSpace(a.listCursor.Text())
	a.runAction("GET /accounts", func(ctx context.Context) (int, string, error) {
		values := url.Values{}
		values.Set("limit", strconv.Itoa(limit))
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		endpoint := base + "/accounts?" + values.Encode()
		return a.doJSON(ctx, http.MethodGet, endpoint, nil, nil)
	})
}

func (a *guiApp) onTransfer() {
	from := strings.TrimSpace(a.transferFrom.Text())
	to := strings.TrimSpace(a.transferTo.Text())
	if from == "" || to == "" {
		walk.MsgBox(a.mw, "Validation", "From and To account IDs are required.", walk.MsgBoxIconWarning)
		return
	}

	amount, err := strconv.ParseInt(strings.TrimSpace(a.transferAmount.Text()), 10, 64)
	if err != nil || amount <= 0 {
		walk.MsgBox(a.mw, "Validation", "Amount must be a positive integer (cents).", walk.MsgBoxIconWarning)
		return
	}

	idemKey := strings.TrimSpace(a.transferIdemKey.Text())
	if idemKey == "" {
		idemKey = uuid.NewString()
		_ = a.transferIdemKey.SetText(idemKey)
	}

	description := strings.TrimSpace(a.transferDescription.Text())
	base := a.baseURL()
	a.runAction("POST /transactions", func(ctx context.Context) (int, string, error) {
		payload := map[string]any{
			"from_account_id": from,
			"to_account_id":   to,
			"amount_cents":    amount,
			"description":     description,
		}
		headers := map[string]string{
			"Idempotency-Key": idemKey,
		}
		return a.doJSON(ctx, http.MethodPost, base+"/transactions", payload, headers)
	})
}

func (a *guiApp) onStatement() {
	accountID := strings.TrimSpace(a.statementAccount.Text())
	if accountID == "" {
		walk.MsgBox(a.mw, "Validation", "Account ID is required.", walk.MsgBoxIconWarning)
		return
	}

	limit, err := parsePositiveInt(a.statementLimit.Text(), 50)
	if err != nil {
		walk.MsgBox(a.mw, "Validation", err.Error(), walk.MsgBoxIconWarning)
		return
	}

	base := a.baseURL()
	a.runAction("GET /accounts/{id}/statement", func(ctx context.Context) (int, string, error) {
		values := url.Values{}
		values.Set("limit", strconv.Itoa(limit))
		endpoint := fmt.Sprintf("%s/accounts/%s/statement?%s", base, accountID, values.Encode())
		return a.doJSON(ctx, http.MethodGet, endpoint, nil, nil)
	})
}

func (a *guiApp) runAction(action string, fn func(context.Context) (int, string, error)) {
	a.setStatus(action + " ...")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		code, body, err := fn(ctx)
		a.mw.Synchronize(func() {
			if err != nil {
				a.appendOutput(action, fmt.Sprintf("Request failed: %v", err))
				a.setStatus(action + " failed")
				return
			}

			a.appendOutput(fmt.Sprintf("%s (HTTP %d)", action, code), body)
			if code >= 400 {
				a.setStatus(fmt.Sprintf("%s returned HTTP %d", action, code))
				return
			}

			a.setStatus(action + " ok")
		})
	}()
}

func (a *guiApp) doJSON(ctx context.Context, method, endpoint string, payload any, headers map[string]string) (int, string, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return 0, "", err
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return 0, "", err
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	rawResp, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp.StatusCode, "", readErr
	}

	pretty := prettyBody(rawResp)
	if resp.StatusCode >= 400 {
		return resp.StatusCode, pretty, errors.New(resp.Status)
	}
	return resp.StatusCode, pretty, nil
}

func prettyBody(body []byte) string {
	if len(strings.TrimSpace(string(body))) == 0 {
		return "(empty body)"
	}

	var indented bytes.Buffer
	if err := json.Indent(&indented, body, "", "  "); err == nil {
		return indented.String()
	}
	return string(body)
}

func parsePositiveInt(raw string, fallback int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("value must be a positive integer")
	}
	if n > 200 {
		n = 200
	}
	return n, nil
}

func (a *guiApp) baseURL() string {
	base := strings.TrimSpace(a.apiURL.Text())
	if base == "" {
		base = defaultAPIURL
	}
	return strings.TrimRight(base, "/")
}

func (a *guiApp) setStatus(text string) {
	if a.status != nil {
		_ = a.status.SetText(text)
	}
}

func (a *guiApp) appendOutput(title, body string) {
	if a.output == nil {
		return
	}

	msg := fmt.Sprintf("[%s] %s\n%s\n\n", time.Now().Format("2006-01-02 15:04:05"), title, body)
	a.output.AppendText(msg)
}
