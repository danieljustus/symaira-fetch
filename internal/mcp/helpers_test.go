package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-fetch/internal/batch"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

type mockClient struct {
	fetchFunc func(ctx context.Context, req fetch.Request) (*fetch.Response, error)
	closeFunc func() error
}

func (m *mockClient) Fetch(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, req)
	}
	return &fetch.Response{
		StatusCode: 200,
		Body:       []byte("<html><body><p>Hello</p></body></html>"),
	}, nil
}

func (m *mockClient) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestMakeFetchURLHandler_InvalidJSON(t *testing.T) {
	handler := makeFetchURLHandler(&mockClient{}, pipeline.StaticEngine{})
	_, err := handler(context.Background(), []byte("{invalid"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid input") {
		t.Errorf("expected 'invalid input' in error, got: %s", err.Error())
	}
}

func TestMakeFetchURLHandler_MissingURL(t *testing.T) {
	handler := makeFetchURLHandler(&mockClient{}, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{"format": "text"})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
	if !strings.Contains(err.Error(), "missing required argument 'url'") {
		t.Errorf("expected missing url error, got: %s", err.Error())
	}
}

func TestMakeFetchURLHandler_MaxCharsExtraction(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":       "https://example.com",
		"max_chars": float64(5000),
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchURLHandler_MaxCharsCapped(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":       "https://example.com",
		"max_chars": float64(600000),
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchURLHandler_TimeoutExtraction(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":             "https://example.com",
		"timeout_seconds": float64(10),
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchURLHandler_TimeoutCapped(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":             "https://example.com",
		"timeout_seconds": float64(200),
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchURLHandler_RawModeSuccess(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("raw response body"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url": "https://example.com",
		"raw": true,
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	if body != "raw response body" {
		t.Errorf("expected 'raw response body', got: %s", body)
	}
}

func TestMakeFetchURLHandler_RawModeError(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return nil, &pipeline.FetchError{URL: req.URL, Err: fmt.Errorf("HTTP 500"), StatusCode: 500}
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url": "https://example.com",
		"raw": true,
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for raw mode fetch failure")
	}
	if !strings.Contains(err.Error(), "http_5xx") {
		t.Errorf("expected http_5xx in error, got: %s", err.Error())
	}
}

func TestMakeFetchURLHandler_SuccessfulNonRaw(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><head><title>Test</title></head><body><p>Hello World</p></body></html>"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url": "https://example.com",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchBatchHandler_InvalidJSON(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	_, err := handler(context.Background(), []byte("{invalid"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid input") {
		t.Errorf("expected 'invalid input' in error, got: %s", err.Error())
	}
}

func TestMakeFetchBatchHandler_NonStringURLContinue(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls": []interface{}{123, "", "https://example.com"},
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchBatchHandler_MaxCharsExtraction(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls":      []interface{}{"https://example.com"},
		"max_chars": float64(5000),
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchBatchHandler_MaxCharsCapped(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls":      []interface{}{"https://example.com"},
		"max_chars": float64(600000),
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchBatchHandler_ConcurrencyExtraction(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls":        []interface{}{"https://example.com"},
		"concurrency": float64(6),
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchBatchHandler_ConcurrencyCapped(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls":        []interface{}{"https://example.com"},
		"concurrency": float64(20),
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchBatchHandler_Success(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls": []interface{}{"https://example.com", "https://example.org"},
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	if !strings.Contains(data, "example.com") {
		t.Errorf("expected example.com in result, got: %s", data)
	}
}

// ---- Integration: stringified values through handlers ----

func TestMakeFetchURLHandler_StringifiedMaxChars(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	// Marshal with stringified "5000" to simulate a client sending a string literal
	input, _ := json.Marshal(map[string]interface{}{
		"url":       "https://example.com",
		"max_chars": "5000",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchURLHandler_StringifiedTimeout(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":             "https://example.com",
		"timeout_seconds": "10",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchURLHandler_StringifiedBoolArgs(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":             "https://example.com",
		"include_links":   "true",
		"frontmatter":     "false",
		"store_full_text": "true",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchURLHandler_StringifiedTopK(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":   "https://example.com",
		"top_k": "5",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchURLHandler_StringifiedCharLimit(t *testing.T) {
	srv := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	handler := makeFetchURLHandler(srv, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":        "https://example.com",
		"char_limit": "20000",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---- Integration: malformed values produce errors ----

func TestMakeFetchURLHandler_MalformedMaxChars(t *testing.T) {
	handler := makeFetchURLHandler(&mockClient{}, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":       "https://example.com",
		"max_chars": "not-a-number",
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for malformed max_chars")
	}
	if !strings.Contains(err.Error(), "max_chars") {
		t.Errorf("expected field name in error, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "not a valid integer") {
		t.Errorf("expected reason in error, got: %s", err.Error())
	}
}

func TestMakeFetchURLHandler_MalformedTimeout(t *testing.T) {
	handler := makeFetchURLHandler(&mockClient{}, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":             "https://example.com",
		"timeout_seconds": "abc",
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for malformed timeout_seconds")
	}
}

func TestMakeFetchURLHandler_MalformedBoolString(t *testing.T) {
	handler := makeFetchURLHandler(&mockClient{}, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":           "https://example.com",
		"include_links": "yes",
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for malformed bool string 'yes'")
	}
}

func TestMakeFetchURLHandler_MalformedBoolType(t *testing.T) {
	handler := makeFetchURLHandler(&mockClient{}, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url": "https://example.com",
		"raw": float64(1),
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for float64 passed as bool")
	}
}

func TestMakeFetchURLHandler_MalformedStringArgType(t *testing.T) {
	handler := makeFetchURLHandler(&mockClient{}, pipeline.StaticEngine{})
	input, _ := json.Marshal(map[string]interface{}{
		"url":          "https://example.com",
		"css_selector": true,
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for bool passed as string")
	}
}

func TestMakeFetchBatchHandler_StringifiedMaxChars(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls":      []interface{}{"https://example.com"},
		"max_chars": "5000",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchBatchHandler_MalformedMaxChars(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls":      []interface{}{"https://example.com"},
		"max_chars": "bad-value",
	})
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for malformed max_chars")
	}
}

func TestMakeFetchBatchHandler_StringifiedConcurrency(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls":        []interface{}{"https://example.com"},
		"concurrency": "6",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchBatchHandler_StringifiedCharLimit(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls":       []interface{}{"https://example.com"},
		"char_limit": "10000",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeFetchBatchHandler_StringifiedStoreFullText(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, batch.NewAdaptivePool(2, 8))
	input, _ := json.Marshal(map[string]interface{}{
		"urls":            []interface{}{"https://example.com"},
		"store_full_text": "true",
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeWaybackHandler_StringifiedLimit(t *testing.T) {
	// Serve a minimal valid CDX response from a local server so the handler
	// never reaches the default web.archive.org endpoint (which has a 30s
	// client timeout and stalls the test when no CDX server is reachable).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The stringified limit "50" must be coerced to int and propagated
		// as the CDX limit query parameter.
		if got := r.URL.Query().Get("limit"); got != "50" {
			t.Errorf("expected coerced limit=50 in CDX query, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		// Header row only: a valid CDX response with no snapshots.
		json.NewEncoder(w).Encode([][]string{
			{"timestamp", "original", "mimetype", "statuscode", "digest", "length"},
		})
	}))
	defer server.Close()

	oldURL := CdxBaseURL
	CdxBaseURL = server.URL
	defer func() { CdxBaseURL = oldURL }()

	handler := makeWaybackSnapshotsHandler()
	// Pass stringified limit with a URL that passes scheme validation, so the
	// request reaches the (mock) CDX client.
	input, _ := json.Marshal(map[string]interface{}{
		"url":   "https://example.com",
		"limit": "50",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := handler(ctx, input)
	if _, ok := err.(*ArgError); ok {
		t.Fatalf("unexpected ArgError for stringified limit: %v", err)
	}
	if err != nil {
		t.Fatalf("unexpected error from mock CDX server: %v", err)
	}
}

func TestMakeWaybackHandler_MalformedLimit(t *testing.T) {
	handler := makeWaybackSnapshotsHandler()
	input, _ := json.Marshal(map[string]interface{}{
		"url":   "https://example.com",
		"limit": "not-a-number",
	})
	// The malformed limit fails argument validation before any CDX request is
	// made; the timeout only guards against regressions that reach the network.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := handler(ctx, input)
	if err == nil {
		t.Fatal("expected error for malformed limit")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("expected field name in error, got: %s", err.Error())
	}
}

func TestMakeWaybackHandler_NonStringForFrom(t *testing.T) {
	handler := makeWaybackSnapshotsHandler()
	input, _ := json.Marshal(map[string]interface{}{
		"url":  "https://example.com",
		"from": true,
	})
	// A non-string "from" fails argument validation before any CDX request is
	// made; the timeout only guards against regressions that reach the network.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := handler(ctx, input)
	if err == nil {
		t.Fatal("expected error for boolean passed as from")
	}
	if !strings.Contains(err.Error(), "from") {
		t.Errorf("expected field name in error, got: %s", err.Error())
	}
}

func TestMakeFetchURLHandler_EscalateHint(t *testing.T) {
	// SPA-skeleton HTML: >2048 bytes, near-empty visible text, hydration island.
	spaHTML := `<html><head><title>SPA</title></head><body>` +
		`<div id="app"></div>` +
		`<script id="__NEXT_DATA__" type="application/json">{"page":"/"}</script>` +
		strings.Repeat("<!-- padding for the SPA detection threshold -->\n", 60) +
		`</body></html>`

	client := &mockClient{fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
		return &fetch.Response{
			FinalURL:   "https://example.com",
			StatusCode: 200,
			Body:       []byte(spaHTML),
		}, nil
	}}
	handler := makeFetchURLHandler(client, pipeline.StaticEngine{})
	// Unique port on a public host per run: the pipeline cache is keyed by
	// URL (a fixed example.com key could collide with earlier runs), and
	// CheckSSRF requires a resolvable, non-private hostname.
	rawURL := fmt.Sprintf("https://example.com:%d", 20000+time.Now().UnixNano()%40000)
	input, _ := json.Marshal(map[string]interface{}{
		"url": rawURL,
	})
	result, err := handler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	if !strings.Contains(text, "symbrowse") {
		t.Errorf("expected symbrowse escalate hint in fetch_url output, got: %s", text)
	}
	if !strings.Contains(text, "spa_skeleton") {
		t.Errorf("expected spa_skeleton reason in output, got: %s", text)
	}
}
