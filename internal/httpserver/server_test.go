package httpserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/httpserver"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

// mockClient implements fetch.Client for testing.
type mockClient struct {
	fetchFunc func(ctx context.Context, req fetch.Request) (*fetch.Response, error)
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

func (m *mockClient) Close() error { return nil }

// newTestServer creates a Server with mock dependencies and registers its
// handlers on a net/http ServeMux for testing.
func newTestServer(token string, client fetch.Client) (*httpserver.Server, *http.ServeMux) {
	srv := &httpserver.Server{
		Addr:   "127.0.0.1:0",
		Token:  token,
		Client: client,
		Engine: pipeline.StaticEngine{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /fetch", srv.HandleFetch)
	mux.HandleFunc("GET /healthz", srv.HandleHealthz)
	return srv, mux
}

func TestHealthz(t *testing.T) {
	srv, mux := newTestServer("", &mockClient{})
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]bool
	json.NewDecoder(resp.Body).Decode(&body)
	if !body["ok"] {
		t.Error("expected ok:true")
	}
}

func TestFetch_MissingTokenReturns401(t *testing.T) {
	srv, mux := newTestServer("secret-token", &mockClient{})
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{"url":"https://example.com"}`)
	resp, err := http.Post(ts.URL+"/fetch", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["ok"] != false {
		t.Error("expected ok:false")
	}
}

func TestFetch_WrongTokenReturns401(t *testing.T) {
	srv, mux := newTestServer("secret-token", &mockClient{})
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	for _, tok := range []string{"wrong-token", "secret-toke", "secret-tokenX"} {
		body := strings.NewReader(`{"url":"https://example.com"}`)
		req, _ := http.NewRequest("POST", ts.URL+"/fetch", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("token %q: expected 401, got %d", tok, resp.StatusCode)
		}
	}
}

func TestFetch_ValidTokenSucceeds(t *testing.T) {
	client := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><head><title>Test</title></head><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	srv, mux := newTestServer("secret-token", client)
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{"url":"https://example.com"}`)
	req, _ := http.NewRequest("POST", ts.URL+"/fetch", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["ok"] != true {
		t.Errorf("expected ok:true, got: %v", result)
	}
}

func TestFetch_LocalhostAllowsNoToken(t *testing.T) {
	client := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><head><title>Test</title></head><body><p>Hello</p></body></html>"),
			}, nil
		},
	}
	srv, mux := newTestServer("", client) // empty token, localhost
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{"url":"https://example.com"}`)
	resp, err := http.Post(ts.URL+"/fetch", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["ok"] != true {
		t.Errorf("expected ok:true, got: %v", result)
	}
}

func TestFetch_PrivateIPTargetReturns400(t *testing.T) {
	srv, mux := newTestServer("", &mockClient{})
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{"url":"http://192.168.1.1/admin"}`)
	resp, err := http.Post(ts.URL+"/fetch", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["ok"] != false {
		t.Error("expected ok:false")
	}
	errStr, _ := result["error"].(string)
	if !strings.Contains(errStr, "blocked_private") && !strings.Contains(errStr, "private") {
		t.Errorf("expected blocked_private error, got: %s", errStr)
	}
}

func TestFetch_MissingURLReturns400(t *testing.T) {
	srv, mux := newTestServer("", &mockClient{})
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{"format":"markdown"}`)
	resp, err := http.Post(ts.URL+"/fetch", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["ok"] != false {
		t.Error("expected ok:false")
	}
}

func TestFetch_InvalidJSONReturns400(t *testing.T) {
	srv, mux := newTestServer("", &mockClient{})
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{invalid}`)
	resp, err := http.Post(ts.URL+"/fetch", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestFetch_UnsupportedSchemeReturns400(t *testing.T) {
	srv, mux := newTestServer("", &mockClient{})
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{"url":"ftp://example.com/file"}`)
	resp, err := http.Post(ts.URL+"/fetch", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	errStr, _ := result["error"].(string)
	if !strings.Contains(errStr, "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %s", errStr)
	}
}

func TestFetch_RawModeReturnsBody(t *testing.T) {
	client := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("raw response data"),
				FinalURL:   "https://example.com",
				Protocol:   "HTTP/2.0",
			}, nil
		},
	}
	srv, mux := newTestServer("", client)
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{"url":"https://example.com","raw":true}`)
	resp, err := http.Post(ts.URL+"/fetch", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["ok"] != true {
		t.Errorf("expected ok:true, got: %v", result)
	}
	if result["content"] != "raw response data" {
		t.Errorf("expected raw content, got: %v", result["content"])
	}
}

func TestFetch_TimeoutCapped(t *testing.T) {
	client := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("expected context to have deadline")
				return nil, fmt.Errorf("no deadline")
			}
			maxAllowed := time.Now().Add(120 * time.Second)
			if deadline.After(maxAllowed) {
				t.Errorf("timeout should be capped at 120s, deadline: %v", deadline)
			}
			return &fetch.Response{
				StatusCode: 200,
				Body:       []byte("<html><body><p>OK</p></body></html>"),
			}, nil
		},
	}
	srv, mux := newTestServer("", client)
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{"url":"https://example.com","timeout_seconds":999}`)
	resp, err := http.Post(ts.URL+"/fetch", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIsLocalhost(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{":8787", false},        // all interfaces
		{"0.0.0.0:8787", false}, // all interfaces
		{"[::]:8787", false},    // all interfaces
		{"127.0.0.1:8787", true},
		{"[::1]:8787", true},
		{"localhost:8787", true},
		{"192.168.1.1:8787", false},
		{"10.0.0.1:8787", false},
		{"example.com:8787", false},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			// Use the same logic as isLocalhost in server.go
			got := isLocalhostTest(tt.addr)
			if got != tt.want {
				t.Errorf("isLocalhost(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestServerStartRequiresTokenOnNonLocalhost(t *testing.T) {
	// Attempting to start on a non-localhost address without a token
	// should return an error. We can't actually bind, so we test the
	// validation logic by checking the error from Start.
	err := httpserver.Start("192.168.1.1:9999", "", "chrome", "")
	if err == nil {
		t.Fatal("expected error for non-localhost without token")
	}
	if !strings.Contains(err.Error(), "bearer token required") {
		t.Errorf("expected 'bearer token required' error, got: %s", err.Error())
	}
}

func TestFetch_FetchErrorReturnsCorrectStatus(t *testing.T) {
	client := &mockClient{
		fetchFunc: func(ctx context.Context, req fetch.Request) (*fetch.Response, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	srv, mux := newTestServer("", client)
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{"url":"https://example.com"}`)
	resp, err := http.Post(ts.URL+"/fetch", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestFetch_PrivateIP127Returns400(t *testing.T) {
	srv, mux := newTestServer("", &mockClient{})
	_ = srv
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := strings.NewReader(`{"url":"http://127.0.0.1:8080/admin"}`)
	resp, err := http.Post(ts.URL+"/fetch", "application/json", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for loopback target, got %d", resp.StatusCode)
	}
}

// isLocalhostTest mirrors the isLocalhost logic for external test package.
func isLocalhostTest(addr string) bool {
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
