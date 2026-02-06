//go:build !windows

package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	defaultListenAddress = "127.0.0.1:8787"
	defaultAPIURL        = "http://localhost:8080"
)

//go:embed web/index.html
var indexHTML string

type rpcRequest struct {
	APIURL  string                     `json:"api_url"`
	Method  string                     `json:"method"`
	Path    string                     `json:"path"`
	Headers map[string]string          `json:"headers"`
	Body    map[string]json.RawMessage `json:"body"`
}

type rpcResponse struct {
	Status int               `json:"status"`
	Body   string            `json:"body"`
	Header map[string]string `json:"headers,omitempty"`
	Error  string            `json:"error,omitempty"`
}

func main() {
	listenAddr := flag.String("listen", defaultListenAddress, "local address for GUI server")
	noOpen := flag.Bool("no-open", false, "do not try to open browser automatically")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveIndex)
	mux.HandleFunc("/rpc", handleRPC)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	srv := &http.Server{
		Addr:              *listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("gui: listen failed: %v", err)
	}
	defer ln.Close()

	urlToOpen := "http://" + ln.Addr().String()
	log.Printf("gui: listening at %s", urlToOpen)
	log.Printf("gui: default API target is %s", defaultAPIURL)

	if !*noOpen {
		tryOpenBrowser(urlToOpen)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-stopCtx.Done():
		log.Printf("gui: shutting down...")
	case serveErr := <-errCh:
		if serveErr != nil && serveErr != http.ErrServerClosed {
			log.Fatalf("gui: server error: %v", serveErr)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("gui: shutdown error: %v", err)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, indexHTML)
}

func handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	var req rpcRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, fmt.Sprintf("invalid payload: %v", err))
		return
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	targetBase := strings.TrimSpace(req.APIURL)
	if targetBase == "" {
		targetBase = defaultAPIURL
	}
	targetBase = strings.TrimRight(targetBase, "/")

	if _, err := url.ParseRequestURI(targetBase); err != nil {
		writeRPCError(w, http.StatusBadRequest, "api_url is invalid")
		return
	}

	path := strings.TrimSpace(req.Path)
	if path == "" || !strings.HasPrefix(path, "/") {
		writeRPCError(w, http.StatusBadRequest, "path must start with '/'")
		return
	}

	targetURL := targetBase + path

	var body io.Reader
	if req.Body != nil && len(req.Body) > 0 {
		raw, err := json.Marshal(req.Body)
		if err != nil {
			writeRPCError(w, http.StatusBadRequest, "body is invalid JSON")
			return
		}
		body = bytes.NewReader(raw)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, method, targetURL, body)
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, err.Error())
		return
	}

	if body != nil && req.Headers["Content-Type"] == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		writeRPCError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		writeRPCError(w, http.StatusBadGateway, err.Error())
		return
	}

	respHeaders := map[string]string{}
	for _, key := range []string{"Content-Type", "Location"} {
		if value := resp.Header.Get(key); value != "" {
			respHeaders[key] = value
		}
	}

	writeRPC(w, resp.StatusCode, rpcResponse{
		Status: resp.StatusCode,
		Body:   string(rawResp),
		Header: respHeaders,
	})
}

func writeRPCError(w http.ResponseWriter, code int, msg string) {
	writeRPC(w, code, rpcResponse{
		Status: code,
		Error:  msg,
	})
}

func writeRPC(w http.ResponseWriter, code int, payload rpcResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func tryOpenBrowser(targetURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", targetURL)
	case "darwin":
		cmd = exec.Command("open", targetURL)
	default:
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("gui: could not auto-open browser: %v", err)
	}
}
