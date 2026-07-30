package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

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
	handler := makeWaybackSnapshotsHandler()
	// Pass stringified limit with a URL that won't reach network (empty URL scheme validation fails first)
	// Use a test that doesn't need the CDX server
	input, _ := json.Marshal(map[string]interface{}{
		"url":   "https://example.com",
		"limit": "50",
	})
	_, err := handler(context.Background(), input)
	// May fail because no CDX server is configured, but should NOT fail with ArgError
	if err != nil {
		if _, ok := err.(*ArgError); ok {
			t.Fatalf("unexpected ArgError for stringified limit: %v", err)
		}
		// Other errors (like network) are fine for this mock-free test
	}
}

func TestMakeWaybackHandler_MalformedLimit(t *testing.T) {
	handler := makeWaybackSnapshotsHandler()
	input, _ := json.Marshal(map[string]interface{}{
		"url":   "https://example.com",
		"limit": "not-a-number",
	})
	_, err := handler(context.Background(), input)
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
	_, err := handler(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for boolean passed as from")
	}
	if !strings.Contains(err.Error(), "from") {
		t.Errorf("expected field name in error, got: %s", err.Error())
	}
}
