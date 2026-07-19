package httpserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/danieljustus/symaira-fetch/internal/httpserver"
)

func findFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestStartServerAndShutdown(t *testing.T) {
	port := findFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	token := "test-start-token"

	errCh := make(chan error, 1)
	go func() {
		// Use honest profile, empty proxy
		errCh <- httpserver.Start(addr, token, "honest", "")
	}()

	// Wait for server to start by polling /healthz
	client := &http.Client{Timeout: 50 * time.Millisecond}
	url := fmt.Sprintf("http://%s/healthz", addr)

	var ok bool
	for i := 0; i < 40; i++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ok = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !ok {
		t.Fatal("server failed to start or respond to /healthz")
	}

	// Verify health check response
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("failed to request /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Trigger graceful shutdown by sending os.Interrupt to ourselves
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("failed to find process: %v", err)
	}
	err = p.Signal(os.Interrupt)
	if err != nil {
		t.Fatalf("failed to send interrupt signal: %v", err)
	}

	// Wait for Start to return
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for server shutdown")
	}
}

// TestStartServer_EnforcesConfiguredMaxBody verifies that Start wires the
// configured max-body limit into the fetch client, so oversized upstream
// responses are rejected instead of relying on the client's implicit default.
func TestStartServer_EnforcesConfiguredMaxBody(t *testing.T) {
	t.Setenv("SYMFETCH_HTTP_MAX_BODY_MB", "1")

	oversized := bytes.Repeat([]byte("a"), 2*1024*1024)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(oversized)
	}))
	defer upstream.Close()

	port := findFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	token := "test-maxbody-token"

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpserver.Start(addr, token, "honest", "")
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	healthzURL := fmt.Sprintf("http://%s/healthz", addr)
	var ok bool
	for i := 0; i < 40; i++ {
		resp, err := client.Get(healthzURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ok = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ok {
		t.Fatal("server failed to start or respond to /healthz")
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"url": upstream.URL,
		"raw": true,
	})
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://%s/fetch", addr), bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("fetch request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["ok"] == true {
		t.Fatalf("expected oversized upstream response to be rejected, got ok:true")
	}

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("failed to find process: %v", err)
	}
	if err := p.Signal(os.Interrupt); err != nil {
		t.Fatalf("failed to send interrupt signal: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for server shutdown")
	}
}

func TestStartServer_InvalidAddress(t *testing.T) {
	// Attempt to start on an invalid port/address
	err := httpserver.Start("999.999.999.999:9999", "token", "honest", "")
	if err == nil {
		t.Error("expected error starting on invalid address")
	}
}
