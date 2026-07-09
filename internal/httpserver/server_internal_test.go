package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

// internalMockClient implements fetch.Client for internal package tests.
type internalMockClient struct {
	fetchFunc func(ctx context.Context, req fetch.Request) (*fetch.Response, error)
}

func (m *internalMockClient) Fetch(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, req)
	}
	return &fetch.Response{StatusCode: 200}, nil
}

func (m *internalMockClient) Close() error { return nil }

func newInternalServer(client fetch.Client) *Server {
	return &Server{
		Addr:   "127.0.0.1:0",
		Client: client,
		Engine: pipeline.StaticEngine{},
	}
}

func TestHandleRawFetch_Error(t *testing.T) {
	client := &internalMockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	srv := newInternalServer(client)

	rec := httptest.NewRecorder()
	srv.handleRawFetch(rec, context.Background(), "https://example.com")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
	var body fetchResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.OK {
		t.Error("expected ok:false")
	}
	if !strings.Contains(body.Error, "connection refused") {
		t.Errorf("expected error to contain 'connection refused', got: %s", body.Error)
	}
}

func TestCategoriseError(t *testing.T) {
	baseErr := errors.New("base error")

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil",
			err:  nil,
			want: "",
		},
		{
			name: "blocked",
			err:  &pipeline.BlockedError{URL: "https://example.com", Reason: "private IP"},
			want: "[blocked_private]",
		},
		{
			name: "too_large",
			err:  &pipeline.FetchError{URL: "https://example.com", Err: errors.New("too_large: body exceeds limit")},
			want: "[too_large]",
		},
		{
			name: "http_4xx",
			err:  &pipeline.FetchError{URL: "https://example.com", Err: errors.New("HTTP 404 Not Found")},
			want: "[http_4xx]",
		},
		{
			name: "http_4xx_with_recovery",
			err: &pipeline.FetchError{
				URL: "https://example.com/missing",
				Err: errors.New("HTTP 404 Not Found"),
				Recovery: &pipeline.RecoveryHints{
					NearestAncestor: "https://example.com/",
					AncestorStatus:  200,
					Candidates: []pipeline.CandidateURL{
						{URL: "https://example.com/alternative"},
					},
				},
			},
			want: "nearest reachable ancestor",
		},
		{
			name: "http_5xx",
			err:  &pipeline.FetchError{URL: "https://example.com", Err: errors.New("HTTP 500 Internal Server Error")},
			want: "[http_5xx]",
		},
		{
			name: "dns",
			err:  &net.DNSError{Err: "no such host", Name: "example.invalid"},
			want: "[dns]",
		},
		{
			name: "timeout",
			err:  context.DeadlineExceeded,
			want: "[timeout]",
		},
		{
			name: "generic",
			err:  baseErr,
			want: "base error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := categoriseError(tt.err)
			if tt.err == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil error, got nil")
			}
			if !strings.Contains(got.Error(), tt.want) {
				t.Errorf("expected error to contain %q, got: %s", tt.want, got.Error())
			}
		})
	}
}

func TestErrorToStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "blocked",
			err:  &pipeline.BlockedError{URL: "https://example.com"},
			want: http.StatusBadRequest,
		},
		{
			name: "timeout",
			err:  context.DeadlineExceeded,
			want: http.StatusGatewayTimeout,
		},
		{
			name: "http_4xx",
			err:  &pipeline.FetchError{URL: "https://example.com", Err: errors.New("HTTP 404")},
			want: http.StatusBadGateway,
		},
		{
			name: "http_5xx",
			err:  &pipeline.FetchError{URL: "https://example.com", Err: errors.New("HTTP 500")},
			want: http.StatusBadGateway,
		},
		{
			name: "generic",
			err:  errors.New("generic failure"),
			want: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errorToStatus(tt.err)
			if got != tt.want {
				t.Errorf("errorToStatus(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestFormatWithMeta(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Hello\n\nWorld",
		Meta: agentdom.Meta{
			Title:      "Test Page",
			FinalURL:   "https://example.com",
			StatusCode: 200,
			CharCount:  100,
			EstTokens:  25,
			Protocol:   "HTTP/2.0",
		},
		Doc: &agentdom.Document{
			URL:   "https://example.com",
			Title: "Test Page",
			Lang:  "en",
		},
	}

	t.Run("markdown without frontmatter", func(t *testing.T) {
		got := formatWithMeta(res, pipeline.FormatMarkdown, false)
		if !strings.Contains(got, "# Hello") {
			t.Errorf("expected markdown output to contain heading, got: %s", got)
		}
	})

	t.Run("markdown with frontmatter", func(t *testing.T) {
		got := formatWithMeta(res, pipeline.FormatMarkdown, true)
		if !strings.Contains(got, "---") {
			t.Errorf("expected frontmatter delimiters, got: %s", got)
		}
		if !strings.Contains(got, "# Hello") {
			t.Errorf("expected markdown body after frontmatter, got: %s", got)
		}
	})

	t.Run("json returns raw output", func(t *testing.T) {
		got := formatWithMeta(res, pipeline.FormatJSON, true)
		if got != res.Output {
			t.Errorf("expected raw output for JSON, got: %s", got)
		}
	})
}

func TestCategoriseError_HTTP4xxWithCandidates(t *testing.T) {
	candidates := []pipeline.CandidateURL{
		{URL: "https://example.com/one", Title: "One"},
		{URL: "https://example.com/two", Title: "Two"},
	}
	err := &pipeline.FetchError{
		URL: "https://example.com/missing",
		Err: errors.New("HTTP 404 Not Found"),
		Recovery: &pipeline.RecoveryHints{
			NearestAncestor: "https://example.com/",
			AncestorStatus:  200,
			Candidates:      candidates,
		},
	}

	got := categoriseError(err)
	want := "https://example.com/one"
	if !strings.Contains(got.Error(), want) {
		t.Errorf("expected candidate URL in error, got: %s", got.Error())
	}
	want = "https://example.com/two"
	if !strings.Contains(got.Error(), want) {
		t.Errorf("expected second candidate URL in error, got: %s", got.Error())
	}
}

func TestHandleRawFetch_Success(t *testing.T) {
	client := &internalMockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("raw body"),
				FinalURL:   "https://example.com/final",
				Protocol:   "HTTP/2.0",
			}, nil
		},
	}
	srv := newInternalServer(client)

	rec := httptest.NewRecorder()
	srv.handleRawFetch(rec, context.Background(), "https://example.com")

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body fetchResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK {
		t.Error("expected ok:true")
	}
	if body.Content != "raw body" {
		t.Errorf("expected 'raw body', got: %s", body.Content)
	}
	if body.Meta == nil || body.Meta.FinalURL != "https://example.com/final" {
		t.Errorf("expected final URL in meta, got: %+v", body.Meta)
	}
}

func TestErrorToStatus_Nil(t *testing.T) {
	if errorToStatus(nil) != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil error, got %d", errorToStatus(nil))
	}
}

func TestFormatWithMeta_NilDoc(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Hello",
		Meta:   agentdom.Meta{Title: "Test"},
		Doc:    nil,
	}

	got := formatWithMeta(res, pipeline.FormatMarkdown, true)
	if strings.Contains(got, "---") {
		t.Errorf("expected no YAML frontmatter when doc is nil, got: %s", got)
	}
	if !strings.Contains(got, "# Hello") {
		t.Errorf("expected markdown body, got: %s", got)
	}
}
