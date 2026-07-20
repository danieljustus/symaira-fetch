// Package httpserver provides an HTTP REST server mode for symfetch.
// It exposes a POST /fetch endpoint that mirrors the MCP fetch_url tool,
// with mandatory bearer-token authentication for non-localhost listeners.
package httpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/danieljustus/symaira-fetch/internal/apicommon"
	"github.com/danieljustus/symaira-fetch/internal/config"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

const (
	maxTimeoutSec = 120
	maxCharsLimit = 500_000
)

// Server holds configuration for the HTTP server.
type Server struct {
	Addr   string
	Token  string
	Client fetch.Client
	Engine pipeline.Engine
}

// fetchRequest is the JSON body accepted by POST /fetch.
type fetchRequest struct {
	URL              string `json:"url"`
	Format           string `json:"format"`
	MaxChars         int    `json:"max_chars"`
	IncludeLinks     bool   `json:"include_links"`
	Raw              bool   `json:"raw"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	CSSSelector      string `json:"css_selector"`
	Frontmatter      bool   `json:"frontmatter"`
	SchemaPath       string `json:"schema_path"`
	StoreFullText    bool   `json:"store_full_text"`
	CharLimit        int    `json:"char_limit"`
	WaybackTimestamp string `json:"wayback_timestamp"`
	WaybackFallback  bool   `json:"wayback_fallback"`
	Query            string `json:"query"`
	TopK             int    `json:"top_k"`
}

// fetchResponse is the JSON response from POST /fetch.
type fetchResponse struct {
	OK      bool          `json:"ok"`
	Content string        `json:"content,omitempty"`
	Error   string        `json:"error,omitempty"`
	Meta    *responseMeta `json:"meta,omitempty"`
}

// responseMeta is the subset of pipeline metadata returned to the client.
type responseMeta struct {
	Title      string `json:"title,omitempty"`
	URL        string `json:"url,omitempty"`
	FinalURL   string `json:"final_url,omitempty"`
	Lang       string `json:"lang,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	CharCount  int    `json:"char_count,omitempty"`
	EstTokens  int    `json:"est_tokens,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

// Start starts the HTTP server with graceful shutdown on SIGINT/SIGTERM.
// All logs go to stderr; HTTP responses go to the client.
func Start(addr, token, profile, proxy string) error {
	cfg, err := config.Load()
	if err != nil {
		slog.Warn("config error, using defaults", "error", err)
		cfg = config.Defaults()
	}

	client, err := fetch.New(fetch.ParseProfile(profile),
		fetch.WithProxy(proxy),
		fetch.WithTimeout(cfg.HTTP.TimeoutSeconds),
		fetch.WithMaxBody(cfg.HTTP.MaxBodyMB),
	)
	if err != nil {
		return fmt.Errorf("init fetch client: %w", err)
	}
	defer client.Close()

	eng := pipeline.StaticEngine{}

	if !isLocalhost(addr) && token == "" {
		return fmt.Errorf("bearer token required for non-localhost address %s; set --token or SYMFETCH_HTTP_TOKEN", addr)
	}

	srv := &Server{
		Addr:   addr,
		Token:  token,
		Client: client,
		Engine: eng,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /fetch", srv.HandleFetch)
	mux.HandleFunc("GET /healthz", srv.HandleHealthz)

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("HTTP server listening", "addr", addr, "auth", tokenAuthStatus(token, addr))
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down HTTP server")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server error: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

// HandleHealthz is a simple health check endpoint.
func (s *Server) HandleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

// HandleFetch processes POST /fetch requests.
func (s *Server) HandleFetch(w http.ResponseWriter, r *http.Request) {
	if s.Token != "" && !s.authenticate(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"ok":false,"error":"unauthorized: invalid or missing bearer token"}`)
		return
	}

	var req fetchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"ok":false,"error":"invalid JSON body"}`)
		return
	}

	if req.URL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"ok":false,"error":"missing required field: url"}`)
		return
	}

	if err := apicommon.ValidateURLScheme(req.URL); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		resp, _ := json.Marshal(fetchResponse{OK: false, Error: err.Error()})
		w.Write(resp)
		return
	}

	// Enforce SSRF guard: never allow private addresses.
	if err := fetch.CheckSSRF(req.URL); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		resp, _ := json.Marshal(fetchResponse{OK: false, Error: apicommon.CategoriseError(err).Error()})
		w.Write(resp)
		return
	}

	timeoutSec := 30
	if req.TimeoutSeconds > 0 {
		timeoutSec = req.TimeoutSeconds
	}
	if timeoutSec > maxTimeoutSec {
		timeoutSec = maxTimeoutSec
	}

	fetchCtx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	format := pipeline.FormatMarkdown
	if req.Format != "" {
		parsed, err := pipeline.ParseFormat(req.Format)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			resp, _ := json.Marshal(fetchResponse{OK: false, Error: fmt.Sprintf("invalid format: %v", err)})
			w.Write(resp)
			return
		}
		format = parsed
	}

	maxChars := 20000
	if req.MaxChars > 0 {
		maxChars = req.MaxChars
	}
	if maxChars > maxCharsLimit {
		maxChars = maxCharsLimit
	}

	charLimit := pipeline.DefaultCharLimit
	if req.CharLimit > 0 {
		charLimit = req.CharLimit
	}

	waybackFallback := req.WaybackFallback
	if req.WaybackTimestamp != "" {
		waybackFallback = true
	}

	topK := req.TopK

	if req.Raw {
		s.handleRawFetch(w, fetchCtx, req.URL)
		return
	}

	res, err := pipeline.Run(fetchCtx, s.Client, s.Engine, req.URL, pipeline.Options{
		Format: format,
		Content: pipeline.ContentOptions{
			MaxChars:     maxChars,
			IncludeLinks: req.IncludeLinks,
		},
		Cache: pipeline.CacheOptions{
			NoCache: true, // HTTP server does not use cache
		},
		CSSSelector:      req.CSSSelector,
		Frontmatter:      req.Frontmatter,
		SchemaPath:       req.SchemaPath,
		StoreFullText:    req.StoreFullText,
		CharLimit:        charLimit,
		WaybackFallback:  waybackFallback,
		WaybackTimestamp: req.WaybackTimestamp,
		Query:            req.Query,
		TopK:             topK,
		Security: pipeline.SecurityOptions{
			AllowPrivate: false, // always enforced
		},
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(errorToStatus(err))
		resp, _ := json.Marshal(fetchResponse{OK: false, Error: apicommon.CategoriseError(err).Error()})
		w.Write(resp)
		return
	}

	content := apicommon.FormatWithMeta(res, format, req.Frontmatter)
	meta := &responseMeta{
		Title:      res.Meta.Title,
		URL:        req.URL,
		FinalURL:   res.Meta.FinalURL,
		Lang:       res.Meta.Lang,
		StatusCode: res.Meta.StatusCode,
		CharCount:  res.Meta.CharCount,
		EstTokens:  res.Meta.EstTokens,
		Truncated:  res.Meta.Truncated,
		Protocol:   res.Meta.Protocol,
	}

	w.Header().Set("Content-Type", "application/json")
	resp, _ := json.Marshal(fetchResponse{OK: true, Content: content, Meta: meta})
	w.Write(resp)
}

// handleRawFetch bypasses the pipeline and returns the raw response body.
func (s *Server) handleRawFetch(w http.ResponseWriter, ctx context.Context, rawURL string) {
	resp, err := s.Client.Fetch(ctx, fetch.Request{URL: rawURL, AllowPrivate: false})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(errorToStatus(err))
		resp, _ := json.Marshal(fetchResponse{OK: false, Error: apicommon.CategoriseError(err).Error()})
		w.Write(resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	out, _ := json.Marshal(fetchResponse{
		OK:      true,
		Content: string(resp.Body),
		Meta: &responseMeta{
			URL:        rawURL,
			FinalURL:   resp.FinalURL,
			StatusCode: resp.StatusCode,
			Protocol:   resp.Protocol,
		},
	})
	w.Write(out)
}

// authenticate checks the Bearer token from the Authorization header.
func (s *Server) authenticate(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	got := strings.TrimPrefix(auth, prefix)
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) == 1
}

// isLocalhost reports whether the address is a loopback-only listener.
// Addresses that bind to all interfaces (e.g. ":8787", "0.0.0.0:8787") are
// NOT considered localhost because they are reachable from the network.
func isLocalhost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// errorToStatus maps pipeline errors to HTTP status codes.
func errorToStatus(err error) int {
	var blockedErr *pipeline.BlockedError
	if errors.As(err, &blockedErr) {
		return http.StatusBadRequest
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	var fetchErr *pipeline.FetchError
	if errors.As(err, &fetchErr) {
		msg := fetchErr.Unwrap().Error()
		if strings.Contains(msg, "HTTP 4") {
			return http.StatusBadGateway
		}
		if strings.Contains(msg, "HTTP 5") {
			return http.StatusBadGateway
		}
	}
	return http.StatusInternalServerError
}

// tokenAuthStatus returns a description for logging.
func tokenAuthStatus(token, addr string) string {
	if token == "" {
		return "none (localhost only)"
	}
	return "bearer"
}
