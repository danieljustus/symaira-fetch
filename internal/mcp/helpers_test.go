package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
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

func TestFormatWithMeta_Markdown(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Hello World",
		Meta: agentdom.Meta{
			Title:      "Test Page",
			StatusCode: 200,
			EstTokens:  42,
			FinalURL:   "https://example.com",
			Truncated:  false,
		},
	}

	got := formatWithMeta(res, pipeline.FormatMarkdown, false)

	if !strings.Contains(got, "> **Test Page** · 200 · ~42 tokens") {
		t.Errorf("expected metadata header, got: %s", got)
	}
	if !strings.Contains(got, "> https://example.com") {
		t.Errorf("expected final URL, got: %s", got)
	}
	if !strings.Contains(got, "# Hello World") {
		t.Errorf("expected output content, got: %s", got)
	}
	if strings.Contains(got, "truncated") {
		t.Errorf("should not contain truncated warning, got: %s", got)
	}
}

func TestFormatWithMeta_MarkdownTruncated(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Content",
		Meta: agentdom.Meta{
			Title:      "Page",
			StatusCode: 200,
			EstTokens:  100,
			FinalURL:   "https://example.com",
			Truncated:  true,
		},
	}

	got := formatWithMeta(res, pipeline.FormatMarkdown, false)

	if !strings.Contains(got, "· ⚠ truncated") {
		t.Errorf("expected truncated warning, got: %s", got)
	}
}

func TestFormatWithMeta_JSON(t *testing.T) {
	res := &pipeline.Result{
		Output: `{"key": "value"}`,
		Meta:   agentdom.Meta{Title: "Page"},
	}

	got := formatWithMeta(res, pipeline.FormatJSON, false)

	if got != `{"key": "value"}` {
		t.Errorf("expected raw output for JSON format, got: %s", got)
	}
}

func TestFormatWithMeta_Text(t *testing.T) {
	res := &pipeline.Result{
		Output: "Plain text content",
		Meta:   agentdom.Meta{Title: "Page"},
	}

	got := formatWithMeta(res, pipeline.FormatText, false)

	if got != "Plain text content" {
		t.Errorf("expected raw output for text format, got: %s", got)
	}
}

func TestCategoriseError_Nil(t *testing.T) {
	got := categoriseError(nil)
	if got != nil {
		t.Errorf("expected nil, got: %v", got)
	}
}

func TestCategoriseError_BlockedPrivate(t *testing.T) {
	err := &pipeline.BlockedError{URL: "http://127.0.0.1"}
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[blocked_private]") {
		t.Errorf("expected [blocked_private] prefix, got: %s", got.Error())
	}
	if !errors.Is(got, err) {
		t.Error("expected wrapped original error")
	}
}

func TestCategoriseError_TooLarge(t *testing.T) {
	err := &pipeline.FetchError{URL: "http://example.com", Err: &fetch.ErrTooLarge{URL: "http://example.com", Limit: 1024}}
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[too_large]") {
		t.Errorf("expected [too_large] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_HTTP4xx(t *testing.T) {
	err := &pipeline.FetchError{URL: "http://example.com", Err: fmt.Errorf("HTTP 404")}
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[http_4xx]") {
		t.Errorf("expected [http_4xx] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_HTTP5xx(t *testing.T) {
	err := &pipeline.FetchError{URL: "http://example.com", Err: fmt.Errorf("HTTP 503")}
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[http_5xx]") {
		t.Errorf("expected [http_5xx] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_Timeout(t *testing.T) {
	err := context.DeadlineExceeded
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[timeout]") {
		t.Errorf("expected [timeout] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_DNS(t *testing.T) {
	err := &net.DNSError{Err: "no such host", Name: "example.com"}
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[dns]") {
		t.Errorf("expected [dns] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_Unknown(t *testing.T) {
	err := fmt.Errorf("some other error")
	got := categoriseError(err)

	if got.Error() != "some other error" {
		t.Errorf("expected original error message, got: %s", got.Error())
	}
	if !errors.Is(got, err) {
		t.Error("expected wrapped original error")
	}
}

func TestFormatWithMeta_MarkdownFrontmatter(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Hello World",
		Meta: agentdom.Meta{
			Title:      "Test Page",
			StatusCode: 200,
			EstTokens:  42,
			FinalURL:   "https://example.com",
			Truncated:  false,
		},
		Doc: &agentdom.Document{
			URL:     "https://example.com",
			Islands: []agentdom.DataIsland{},
		},
	}

	got := formatWithMeta(res, pipeline.FormatMarkdown, true)

	if !strings.Contains(got, "---") {
		t.Errorf("expected YAML frontmatter delimiters, got: %s", got)
	}
	if !strings.Contains(got, "title: Test Page") {
		t.Errorf("expected title in frontmatter, got: %s", got)
	}
	if !strings.Contains(got, "> **Test Page**") {
		t.Errorf("expected metadata header, got: %s", got)
	}
	if !strings.Contains(got, "# Hello World") {
		t.Errorf("expected output content, got: %s", got)
	}
}

func TestFormatWithMeta_MarkdownFrontmatterWithSchema(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Content",
		Meta: agentdom.Meta{
			Title:      "Recipe Page",
			StatusCode: 200,
			EstTokens:  50,
			FinalURL:   "https://example.com/recipe",
		},
		Doc: &agentdom.Document{
			URL: "https://example.com/recipe",
			Islands: []agentdom.DataIsland{
				{
					Source: "ld+json",
					JSON:   json.RawMessage(`{"@type": "Recipe", "name": "Cake"}`),
				},
			},
		},
	}

	got := formatWithMeta(res, pipeline.FormatMarkdown, true)

	if !strings.Contains(got, "schema_type: Recipe") {
		t.Errorf("expected schema_type in frontmatter, got: %s", got)
	}
}

func TestValidateURLSchemeFTP(t *testing.T) {
	err := validateURLScheme("ftp://example.com/file")
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %v", err)
	}
}

func TestValidateURLSchemeEmpty(t *testing.T) {
	err := validateURLScheme("")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestValidateURLSchemeHTTPS(t *testing.T) {
	err := validateURLScheme("https://example.com")
	if err != nil {
		t.Errorf("expected no error for https, got: %v", err)
	}
}

func TestValidateURLSchemeHTTP(t *testing.T) {
	err := validateURLScheme("http://example.com")
	if err != nil {
		t.Errorf("expected no error for http, got: %v", err)
	}
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

func TestValidateURLScheme_ParseError(t *testing.T) {
	err := validateURLScheme("://invalid\x00")
	if err == nil {
		t.Fatal("expected error for unparseable URL")
	}
	if !strings.Contains(err.Error(), "invalid URL") {
		t.Errorf("expected 'invalid URL' in error, got: %s", err.Error())
	}
}
