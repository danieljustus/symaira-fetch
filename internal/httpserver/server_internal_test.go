package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
