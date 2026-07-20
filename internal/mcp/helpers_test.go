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
			return nil, &pipeline.FetchError{URL: req.URL, Err: fmt.Errorf("HTTP 500")}
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
