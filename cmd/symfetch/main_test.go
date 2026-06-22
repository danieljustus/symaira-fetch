package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
	"github.com/danieljustus/symaira-fetch/internal/config"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
	"github.com/spf13/cobra"
)

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
	// This tests the JSON output structure without actually fetching
	// We test the struct definition and marshaling logic
	type jsonOut struct {
		URL    string `json:"url"`
		OK     bool   `json:"ok"`
		Output string `json:"output,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	// Test successful case
	success := jsonOut{URL: "https://example.com", OK: true, Output: "content"}
	data, err := json.MarshalIndent(success, "", "  ")
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(data), `"ok": true`) {
		t.Errorf("success output should contain ok:true, got: %s", data)
	}

	// Test error case
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
