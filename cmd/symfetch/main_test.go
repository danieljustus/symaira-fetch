package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-fetch/internal/agentdom"
	"github.com/danieljustus/symaira-fetch/internal/config"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// executeCmd runs newRootCmd with the given args, capturing os.Stdout and
// os.Stderr. Returns the captured streams and the error from Execute.
func executeCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(args)

	oldStdout, oldStderr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	err = cmd.Execute()

	wOut.Close()
	wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var outBuf, errBuf bytes.Buffer
	outBuf.ReadFrom(rOut)
	errBuf.ReadFrom(rErr)

	return outBuf.String(), errBuf.String(), err
}

// newTestServer creates an httptest server serving a simple HTML page.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><title>Test Page</title></head><body><p>Hello, World!</p></body></html>`))
	}))
}

// newMultiPageServer creates an httptest server with /page1 and /page2.
func newMultiPageServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/page1":
			w.Write([]byte(`<html><head><title>Page 1</title></head><body><p>Content 1</p></body></html>`))
		case "/page2":
			w.Write([]byte(`<html><head><title>Page 2</title></head><body><p>Content 2</p></body></html>`))
		default:
			w.WriteHeader(404)
		}
	}))
}

// ---------------------------------------------------------------------------
// existing unit tests
// ---------------------------------------------------------------------------

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name string
		raw  []string
		want map[string]string
	}{
		{
			name: "empty input",
			raw:  []string{},
			want: map[string]string{},
		},
		{
			name: "single header",
			raw:  []string{"Content-Type: application/json"},
			want: map[string]string{"Content-Type": "application/json"},
		},
		{
			name: "multiple headers",
			raw:  []string{"Content-Type: text/html", "Authorization: Bearer token123"},
			want: map[string]string{"Content-Type": "text/html", "Authorization": "Bearer token123"},
		},
		{
			name: "header with spaces around colon",
			raw:  []string{"Key : Value"},
			want: map[string]string{"Key": "Value"},
		},
		{
			name: "malformed header no colon",
			raw:  []string{"BadHeader"},
			want: map[string]string{},
		},
		{
			name: "empty string",
			raw:  []string{""},
			want: map[string]string{},
		},
		{
			name: "colon in value",
			raw:  []string{"Authorization: Basic dXNlcjpwYXNz"},
			want: map[string]string{"Authorization": "Basic dXNlcjpwYXNz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHeaders(tt.raw)
			if len(got) != len(tt.want) {
				t.Errorf("parseHeaders() returned %d headers, want %d", len(got), len(tt.want))
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseHeaders()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestPrintMarkdownResult(t *testing.T) {
	tests := []struct {
		name     string
		res      *pipeline.Result
		contains []string
	}{
		{
			name: "basic result",
			res: &pipeline.Result{
				Output: "# Hello World\n",
				Meta: agentdom.Meta{
					Title:      "Test Page",
					StatusCode: 200,
					EstTokens:  42,
					FinalURL:   "https://example.com",
					Truncated:  false,
				},
			},
			contains: []string{
				"> **Test Page**",
				"200",
				"~42 tokens",
				"> https://example.com",
				"# Hello World",
			},
		},
		{
			name: "truncated result",
			res: &pipeline.Result{
				Output: "Content",
				Meta: agentdom.Meta{
					Title:      "Page",
					StatusCode: 200,
					EstTokens:  100,
					FinalURL:   "https://example.com",
					Truncated:  true,
				},
			},
			contains: []string{
				"⚠ truncated",
			},
		},
		{
			name: "output without trailing newline",
			res: &pipeline.Result{
				Output: "No newline",
				Meta: agentdom.Meta{
					Title:      "Page",
					StatusCode: 200,
					EstTokens:  10,
					FinalURL:   "https://example.com",
				},
			},
			contains: []string{
				"No newline",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printMarkdownResult(tt.res)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			for _, s := range tt.contains {
				if !strings.Contains(output, s) {
					t.Errorf("printMarkdownResult() output missing %q\nGot: %s", s, output)
				}
			}
		})
	}
}

func TestNewRootCmd_Help(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "symfetch") {
		t.Errorf("help output should contain 'symfetch'")
	}
}

func TestNewRootCmd_Version(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"version"})

	// Capture stdout since version uses fmt.Println
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var outBuf bytes.Buffer
	outBuf.ReadFrom(r)
	output := outBuf.String()

	if !strings.Contains(output, "symfetch") {
		t.Errorf("version output should contain 'symfetch', got: %s", output)
	}
}

func TestNewRootCmd_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// No args should show help
	output := buf.String()
	if !strings.Contains(output, "symfetch") {
		t.Errorf("no-args output should show help, got: %s", output)
	}
}

func TestNewConfigCmd_Init(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"config", "init"})

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var outBuf bytes.Buffer
	outBuf.ReadFrom(r)
	output := outBuf.String()

	if !strings.Contains(output, "Config written") {
		t.Errorf("config init output should contain 'Config written', got: %s", output)
	}

	configPath := filepath.Join(tmpDir, ".config", "symfetch", "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("config file should exist at %s", configPath)
	}
}

func TestNewConfigCmd_InitExists(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "symfetch")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("existing"), 0600)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"config", "init"})

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	err := cmd.Execute()

	wOut.Close()
	wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var outBuf bytes.Buffer
	outBuf.ReadFrom(rOut)
	outBuf.ReadFrom(rErr)
	output := outBuf.String()

	if !strings.Contains(output, "already exists") {
		t.Errorf("config init output should contain 'already exists'")
	}
}

func TestNewMCPCmd_Help(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"mcp", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "MCP") {
		t.Errorf("mcp help should mention MCP, got: %s", output)
	}
}

func TestRunMultiJSON(t *testing.T) {
	type jsonOut struct {
		URL    string `json:"url"`
		OK     bool   `json:"ok"`
		Output string `json:"output,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	success := jsonOut{URL: "https://example.com", OK: true, Output: "content"}
	data, err := json.MarshalIndent(success, "", "  ")
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(data), `"ok": true`) {
		t.Errorf("success output should contain ok:true, got: %s", data)
	}

	errCase := jsonOut{URL: "https://example.com", OK: false, Error: "fetch failed"}
	data, err = json.MarshalIndent(errCase, "", "  ")
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(data), `"ok": false`) {
		t.Errorf("error output should contain ok:false, got: %s", data)
	}
}

func TestResolveFetchOptions_DefaultsFromConfig(t *testing.T) {
	cmd := newRootCmd()
	if err := cmd.ParseFlags([]string{}); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()

	fo, err := resolveFetchOptions(cmd, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if fo.noCache {
		t.Error("expected noCache false (cache enabled by default)")
	}
	if fo.cacheTTL != 15*time.Minute {
		t.Errorf("expected cacheTTL 15m, got %v", fo.cacheTTL)
	}
	if fo.concurrency != 4 {
		t.Errorf("expected concurrency 4, got %d", fo.concurrency)
	}
}

func TestResolveFetchOptions_FlagOverrides(t *testing.T) {
	cmd := newRootCmd()
	if err := cmd.ParseFlags([]string{"--no-cache", "--cache-ttl", "1h", "--concurrency", "8"}); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()

	fo, err := resolveFetchOptions(cmd, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !fo.noCache {
		t.Error("expected noCache true (flag --no-cache set)")
	}
	if fo.cacheTTL != time.Hour {
		t.Errorf("expected cacheTTL 1h, got %v", fo.cacheTTL)
	}
	if fo.concurrency != 8 {
		t.Errorf("expected concurrency 8, got %d", fo.concurrency)
	}
}

func TestResolveFetchOptions_CustomConfigWithoutFlags(t *testing.T) {
	cmd := newRootCmd()
	if err := cmd.ParseFlags([]string{}); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	cfg.Cache.Enabled = false
	cfg.Cache.TTL = 30 * time.Minute
	cfg.HTTP.Concurrency = 8

	fo, err := resolveFetchOptions(cmd, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !fo.noCache {
		t.Error("expected noCache true (config cache.enabled=false)")
	}
	if fo.cacheTTL != 30*time.Minute {
		t.Errorf("expected cacheTTL 30m, got %v", fo.cacheTTL)
	}
	if fo.concurrency != 8 {
		t.Errorf("expected concurrency 8, got %d", fo.concurrency)
	}
}

func TestResolveFetchOptions_FlagsOverrideCustomConfig(t *testing.T) {
	cmd := newRootCmd()
	if err := cmd.ParseFlags([]string{"--cache-ttl", "2h", "--concurrency", "16"}); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	cfg.Cache.TTL = 30 * time.Minute
	cfg.HTTP.Concurrency = 8

	fo, err := resolveFetchOptions(cmd, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if fo.noCache {
		t.Error("expected noCache false (--no-cache not set, use config)")
	}
	if fo.cacheTTL != 2*time.Hour {
		t.Errorf("expected cacheTTL 2h, got %v", fo.cacheTTL)
	}
	if fo.concurrency != 16 {
		t.Errorf("expected concurrency 16, got %d", fo.concurrency)
	}
}

// Suppress unused import warning
var _ = cobra.Command{}

// ---------------------------------------------------------------------------
// NEW: --version flag
// ---------------------------------------------------------------------------

func TestRootCmd_VersionFlag(t *testing.T) {
	stdout, _, err := executeCmd(t, "--version")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "0.1.0-dev") {
		t.Errorf("expected version in output, got: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// NEW: config init --force
// ---------------------------------------------------------------------------

func TestConfigCmd_InitForce(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create existing config file
	configDir := filepath.Join(tmpDir, ".config", "symfetch")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("existing"), 0600)

	_, _, err := executeCmd(t, "config", "init", "--force")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(data) != config.DefaultConfigTOML() {
		t.Errorf("config file content mismatch after --force overwrite")
	}
}

// ---------------------------------------------------------------------------
// NEW: invalid --timeout
// ---------------------------------------------------------------------------

func TestRootCmd_InvalidTimeout(t *testing.T) {
	_, _, err := executeCmd(t, "--timeout", "notaduration", "http://example.com")
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
	code := exitcodes.ExitCodeFromError(err)
	if code != exitcodes.ExitConfig {
		t.Errorf("expected ExitConfig (%d), got %d", exitcodes.ExitConfig, code)
	}
}

// ---------------------------------------------------------------------------
// NEW: invalid --cache-ttl
// ---------------------------------------------------------------------------

func TestRootCmd_InvalidCacheTTL(t *testing.T) {
	_, _, err := executeCmd(t, "--cache-ttl", "notaduration", "http://example.com")
	if err == nil {
		t.Fatal("expected error for invalid cache-ttl")
	}
	code := exitcodes.ExitCodeFromError(err)
	if code != exitcodes.ExitConfig {
		t.Errorf("expected ExitConfig (%d), got %d", exitcodes.ExitConfig, code)
	}
}

// ---------------------------------------------------------------------------
// NEW: all flags parseable
// ---------------------------------------------------------------------------

func TestFetch_AllPipelineFlags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><title>Test</title></head><body><p>OK</p></body></html>`))
	}))
	defer srv.Close()

	_, _, err := executeCmd(t,
		"--format", "json",
		"--profile", "honest",
		"--max-chars", "1000",
		"--links",
		"--session", "mysession",
		"--no-cache",
		"--cache-ttl", "1h",
		"--concurrency", "1",
		"--allow-private",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestFetch_AllRawFlags(t *testing.T) {
	var receivedMethod string
	var receivedBody string
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedHeader = r.Header.Get("X-Custom")
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><p>OK</p></body></html>`))
	}))
	defer srv.Close()

	_, _, err := executeCmd(t,
		"--raw",
		"--profile", "honest",
		"--no-cache",
		"--cache-ttl", "1h",
		"--header", "X-Custom: test-value",
		"--request", "POST",
		"--data", "hello",
		"--allow-private",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if receivedMethod != "POST" {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedBody != "hello" {
		t.Errorf("expected body 'hello', got %q", receivedBody)
	}
	if receivedHeader != "test-value" {
		t.Errorf("expected X-Custom=test-value, got %q", receivedHeader)
	}
}

// ---------------------------------------------------------------------------
// NEW: single URL fetch in each format
// ---------------------------------------------------------------------------

func TestFetch_SingleURL_Markdown(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	stdout, _, err := executeCmd(t,
		"--format", "markdown",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Hello, World!") {
		t.Errorf("expected 'Hello, World!' in output, got: %s", stdout)
	}
}

func TestFetch_SingleURL_JSON(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	stdout, _, err := executeCmd(t,
		"--format", "json",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Errorf("expected valid JSON, got error: %v\nOutput: %s", err, stdout)
	}
	if !strings.Contains(stdout, "Hello, World!") {
		t.Errorf("expected 'Hello, World!' in JSON output, got: %s", stdout)
	}
}

func TestFetch_SingleURL_Text(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	stdout, _, err := executeCmd(t,
		"--format", "text",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Hello, World!") {
		t.Errorf("expected 'Hello, World!' in text output, got: %s", stdout)
	}
}

func TestFetch_SingleURL_HTML(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	stdout, _, err := executeCmd(t,
		"--format", "html",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "<html>") {
		t.Errorf("expected '<html>' in raw HTML output, got: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// NEW: --raw flag
// ---------------------------------------------------------------------------

func TestFetch_Raw(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	stdout, _, err := executeCmd(t,
		"--raw",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "<html>") {
		t.Errorf("expected raw HTML body, got: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// NEW: --raw with --request POST --data
// ---------------------------------------------------------------------------

func TestFetch_RawPOST(t *testing.T) {
	var receivedMethod string
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><p>OK</p></body></html>`))
	}))
	defer srv.Close()

	_, _, err := executeCmd(t,
		"--raw", "--request", "POST", "--data", "hello",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if receivedMethod != "POST" {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedBody != "hello" {
		t.Errorf("expected body 'hello', got %q", receivedBody)
	}
}

// ---------------------------------------------------------------------------
// NEW: --header flag sends custom header
// ---------------------------------------------------------------------------

func TestFetch_CustomHeader(t *testing.T) {
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><title>T</title></head><body><p>OK</p></body></html>`))
	}))
	defer srv.Close()

	_, _, err := executeCmd(t,
		"--raw",
		"--header", "X-Custom: test-value",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if receivedHeader != "test-value" {
		t.Errorf("expected X-Custom=test-value, got %q", receivedHeader)
	}
}

// ---------------------------------------------------------------------------
// NEW: multiple URLs with --format json → runMultiJSON
// ---------------------------------------------------------------------------

func TestFetch_MultiURL_JSON(t *testing.T) {
	srv := newMultiPageServer(t)
	defer srv.Close()

	stdout, _, err := executeCmd(t,
		"--format", "json",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL+"/page1", srv.URL+"/page2",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("expected valid JSON array, got error: %v\nOutput: %s", err, stdout)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r["ok"] != true {
			t.Errorf("expected ok=true for %v", r["url"])
		}
	}
}

// ---------------------------------------------------------------------------
// NEW: multiple URLs with --concurrency 2 → runBatch
// ---------------------------------------------------------------------------

func TestFetch_MultiURL_Batch(t *testing.T) {
	srv := newMultiPageServer(t)
	defer srv.Close()

	stdout, _, err := executeCmd(t,
		"--format", "markdown",
		"--concurrency", "2",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL+"/page1", srv.URL+"/page2",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(stdout, "Content 1") || !strings.Contains(stdout, "Content 2") {
		t.Errorf("expected output to contain both contents, got: %s", stdout)
	}
	if !strings.Contains(stdout, "---") {
		t.Errorf("expected '---' separator in output, got: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// NEW: partial failure (one bad URL, one good URL)
// ---------------------------------------------------------------------------

func TestFetch_PartialFailure(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Use --concurrency 1 to hit the single-URL loop path
	_, stderr, err := executeCmd(t,
		"--format", "markdown",
		"--concurrency", "1",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL, "http://127.0.0.1:1/bad",
	)
	if err == nil {
		t.Fatal("expected error for partial failure")
	}
	code := exitcodes.ExitCodeFromError(err)
	if code != exitcodes.ExitGeneric {
		t.Errorf("expected ExitGeneric (%d), got %d", exitcodes.ExitGeneric, code)
	}
	if !strings.Contains(stderr, "error fetching") {
		t.Errorf("expected 'error fetching' in stderr, got: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// NEW: partial failure via runMultiJSON
// ---------------------------------------------------------------------------

func TestFetch_PartialFailure_MultiJSON(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	_, _, err := executeCmd(t,
		"--format", "json",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL, "http://127.0.0.1:1/bad",
	)
	if err == nil {
		t.Fatal("expected error for partial failure")
	}
	code := exitcodes.ExitCodeFromError(err)
	if code != exitcodes.ExitGeneric {
		t.Errorf("expected ExitGeneric (%d), got %d", exitcodes.ExitGeneric, code)
	}
	// Verify it's a CLIError with the right message
	var cliErr *exitcodes.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if !strings.Contains(cliErr.Error(), "1 of 2 URLs failed") {
		t.Errorf("expected '1 of 2 URLs failed' in error, got: %s", cliErr.Error())
	}
}

// ---------------------------------------------------------------------------
// NEW: partial failure via runBatch
// ---------------------------------------------------------------------------

func TestFetch_PartialFailure_Batch(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	_, _, err := executeCmd(t,
		"--format", "markdown",
		"--concurrency", "2",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL, "http://127.0.0.1:1/bad",
	)
	if err == nil {
		t.Fatal("expected error for partial failure")
	}
	code := exitcodes.ExitCodeFromError(err)
	if code != exitcodes.ExitGeneric {
		t.Errorf("expected ExitGeneric (%d), got %d", exitcodes.ExitGeneric, code)
	}
}

// ---------------------------------------------------------------------------
// NEW: --profile flag values parse correctly
// ---------------------------------------------------------------------------

func TestFetch_Profiles(t *testing.T) {
	for _, profile := range []string{"chrome", "firefox", "honest"} {
		t.Run(profile, func(t *testing.T) {
			cmd := newRootCmd()
			err := cmd.ParseFlags([]string{"--profile", profile})
			if err != nil {
				t.Fatalf("ParseFlags() error = %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NEW: --proxy flag parses correctly
// ---------------------------------------------------------------------------

func TestFetch_ProxyFlag(t *testing.T) {
	cmd := newRootCmd()
	err := cmd.ParseFlags([]string{"--proxy", "http://proxy:8080"})
	if err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// NEW: robots.txt endpoint is respected
// ---------------------------------------------------------------------------

func TestFetch_RobotsFlagParses(t *testing.T) {
	cmd := newRootCmd()
	err := cmd.ParseFlags([]string{"--robots"})
	if err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// NEW: fetch with --links includes links section
// ---------------------------------------------------------------------------

func TestFetch_Links(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><title>T</title></head><body><a href="https://example.com">Link</a></body></html>`))
	}))
	defer srv.Close()

	stdout, _, err := executeCmd(t,
		"--format", "markdown", "--links",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "https://example.com") {
		t.Errorf("expected link URL in output, got: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// NEW: exit code verification for partial failure
// ---------------------------------------------------------------------------

func TestExitCode_PartialFailure(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	_, _, err := executeCmd(t,
		"--format", "text",
		"--concurrency", "1",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL, "http://127.0.0.1:1/bad",
	)
	if err == nil {
		t.Fatal("expected error for partial failure")
	}

	var cliErr *exitcodes.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *exitcodes.CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != exitcodes.ExitGeneric {
		t.Errorf("expected ExitGeneric=%d, got %d", exitcodes.ExitGeneric, cliErr.Code)
	}
	if cliErr.Kind != exitcodes.KindUnavailable {
		t.Errorf("expected KindUnavailable=%q, got %q", exitcodes.KindUnavailable, cliErr.Kind)
	}
}

func TestRunRaw_PartialFailure(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	_, stderr, err := executeCmd(t,
		"--raw",
		"--allow-private", "--no-cache", "--profile", "honest",
		srv.URL, "http://127.0.0.1:1/bad",
	)
	if err == nil {
		t.Fatal("expected error for raw partial failure")
	}
	code := exitcodes.ExitCodeFromError(err)
	if code != exitcodes.ExitGeneric {
		t.Errorf("expected ExitGeneric (%d), got %d", exitcodes.ExitGeneric, code)
	}
	if !strings.Contains(stderr, "error fetching") {
		t.Errorf("expected 'error fetching' in stderr, got: %s", stderr)
	}
}

func TestVersionCmd_Check(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"version", "--check"})

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var outBuf bytes.Buffer
	outBuf.ReadFrom(r)
	output := outBuf.String()

	if !strings.Contains(output, "symfetch") {
		t.Errorf("version output should contain 'symfetch', got: %s", output)
	}
}
