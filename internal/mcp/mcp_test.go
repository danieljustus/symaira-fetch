package mcp_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/mcp"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

// runRPC sends a sequence of JSON-RPC requests and returns the output lines.
func runRPC(t *testing.T, requests []map[string]interface{}) []string {
	t.Helper()

	var in bytes.Buffer
	for _, req := range requests {
		line, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		in.Write(line)
		in.WriteByte('\n')
	}

	client, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	eng := pipeline.StaticEngine{}

	var out bytes.Buffer
	if err := mcp.ServeIO(&in, &out, client, eng); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	var lines []string
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseResponse(t *testing.T, line string) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("invalid JSON: %v — line: %s", err, line)
	}
	return resp
}

func TestMCPInitialize(t *testing.T) {
	lines := runRPC(t, []map[string]interface{}{
		{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{}},
	})
	if len(lines) == 0 {
		t.Fatal("expected response")
	}
	resp := parseResponse(t, lines[0])
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object, got: %v", resp)
	}
	if result["protocolVersion"] == nil {
		t.Error("expected protocolVersion in initialize response")
	}
}

func TestMCPToolsList(t *testing.T) {
	lines := runRPC(t, []map[string]interface{}{
		{"jsonrpc": "2.0", "id": 1, "method": "tools/list"},
	})
	if len(lines) == 0 {
		t.Fatal("expected response")
	}
	resp := parseResponse(t, lines[0])
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	tools, ok := result["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("expected non-empty tools list")
	}

	// Verify expected tool names
	names := map[string]bool{}
	for _, tool := range tools {
		if m, ok := tool.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				names[name] = true
			}
		}
	}
	for _, want := range []string{"fetch_url", "fetch_batch"} {
		if !names[want] {
			t.Errorf("expected tool %q in tools/list", want)
		}
	}
}

func TestMCPFetchURLBlocksPrivate(t *testing.T) {
	// MCP mode always has AllowPrivate=false — fetching loopback must return blocked_private.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `<html><body><p>Hello</p></body></html>`)
	}))
	defer srv.Close()

	lines := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_url",
				"arguments": map[string]interface{}{
					"url":    srv.URL, // httptest binds to 127.0.0.1
					"format": "markdown",
				},
			},
		},
	})
	if len(lines) == 0 {
		t.Fatal("expected response")
	}
	resp := parseResponse(t, lines[0])
	// Result should be present (tool errors are returned as isError:true, not JSON-RPC errors)
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	if result["isError"] != true {
		t.Error("expected isError=true for private address fetch")
	}
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "blocked_private") {
		t.Errorf("expected blocked_private in error text, got: %s", text)
	}
}

func TestMCPStdoutPurity(t *testing.T) {
	// Only valid JSON-RPC frames should appear on stdout — no log output.
	lines := runRPC(t, []map[string]interface{}{
		{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{}},
		{"jsonrpc": "2.0", "id": 2, "method": "tools/list"},
	})

	for _, line := range lines {
		if !strings.HasPrefix(line, "{") {
			t.Errorf("non-JSON line found on stdout: %q", line)
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("invalid JSON on stdout: %v — line: %s", err, line)
			continue
		}
		if obj["jsonrpc"] == nil {
			t.Errorf("stdout line missing jsonrpc field: %s", line)
		}
	}
}

func TestMCPUnknownMethod(t *testing.T) {
	lines := runRPC(t, []map[string]interface{}{
		{"jsonrpc": "2.0", "id": 99, "method": "nonexistent/method"},
	})
	if len(lines) == 0 {
		t.Fatal("expected error response")
	}
	resp := parseResponse(t, lines[0])
	if resp["error"] == nil {
		t.Error("expected error for unknown method")
	}
}

func TestMCPParseError(t *testing.T) {
	var in bytes.Buffer
	in.WriteString("{broken json\n")

	client, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	var out bytes.Buffer
	mcp.ServeIO(&in, &out, client, pipeline.StaticEngine{})

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("expected parse error response")
	}
	resp := parseResponse(t, lines[0])
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object: %v", resp)
	}
	if code, ok := errObj["code"].(float64); !ok || code != -32700 {
		t.Errorf("expected parse error code -32700, got %v", errObj["code"])
	}
}

func TestMCPFetchBatchReturnsArray(t *testing.T) {
	// Even when all URLs fail (private addresses), batch should return a JSON array.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `<html><body><p>Content</p></body></html>`)
	}))
	defer srv.Close()

	lines := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_batch",
				"arguments": map[string]interface{}{
					"urls":   []interface{}{srv.URL, srv.URL},
					"format": "text",
				},
			},
		},
	})
	if len(lines) == 0 {
		t.Fatal("expected response")
	}
	resp := parseResponse(t, lines[0])
	// Should get a result (not a JSON-RPC level error)
	if resp["result"] == nil {
		t.Fatalf("expected result, got: %v", resp)
	}
	result := resp["result"].(map[string]interface{})
	content := result["content"].([]interface{})
	if len(content) == 0 {
		t.Fatal("expected content")
	}
	text := content[0].(map[string]interface{})["text"].(string)
	// Output is either a JSON array (success) or an error message containing "blocked_private"
	if !strings.Contains(text, "[") && !strings.Contains(text, "blocked_private") {
		t.Errorf("unexpected batch output: %s", text[:min(200, len(text))])
	}
}

func TestMCPFetchURLRejectsFileScheme(t *testing.T) {
	lines := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_url",
				"arguments": map[string]interface{}{
					"url": "file:///etc/passwd",
				},
			},
		},
	})
	if len(lines) == 0 {
		t.Fatal("expected response")
	}
	resp := parseResponse(t, lines[0])
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	if result["isError"] != true {
		t.Error("expected isError=true for file:// URL")
	}
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %s", text)
	}
}

func TestMCPFetchURLRejectsGopherScheme(t *testing.T) {
	lines := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_url",
				"arguments": map[string]interface{}{
					"url": "gopher://example.com",
				},
			},
		},
	})
	if len(lines) == 0 {
		t.Fatal("expected response")
	}
	resp := parseResponse(t, lines[0])
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	if result["isError"] != true {
		t.Error("expected isError=true for gopher:// URL")
	}
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %s", text)
	}
}

func TestMCPFetchBatchRejectsFileScheme(t *testing.T) {
	lines := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_batch",
				"arguments": map[string]interface{}{
					"urls": []interface{}{"file:///etc/passwd", "https://example.com"},
				},
			},
		},
	})
	if len(lines) == 0 {
		t.Fatal("expected response")
	}
	resp := parseResponse(t, lines[0])
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	if result["isError"] != true {
		t.Error("expected isError=true for batch with file:// URL")
	}
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %s", text)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
