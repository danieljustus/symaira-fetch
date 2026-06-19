package pipeline_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

func serveFile(t *testing.T, name string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("testdata/%s: %v", name, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(t *testing.T) fetch.Client {
	t.Helper()
	c, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestPipelineNewsArticle(t *testing.T) {
	srv := serveFile(t, "news_article.html")
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	res, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars: 20000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should contain article content
	if !strings.Contains(res.Output, "Mars") {
		t.Errorf("expected Mars in output, got:\n%s", res.Output[:min(500, len(res.Output))])
	}
	if res.Meta.Title != "Big News Story" {
		t.Errorf("expected title 'Big News Story', got %q", res.Meta.Title)
	}
	if res.Meta.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", res.Meta.StatusCode)
	}
	if res.Meta.CharCount == 0 {
		t.Error("expected non-zero char count")
	}
}

func TestPipelineNextJSDataIsland(t *testing.T) {
	srv := serveFile(t, "nextjs_shell.html")
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	res, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars: 20000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have extracted the __NEXT_DATA__ island
	if len(res.Doc.Islands) == 0 {
		t.Fatal("expected at least one data island")
	}
	found := false
	for _, island := range res.Doc.Islands {
		if island.Source == "__NEXT_DATA__" {
			found = true
			if !strings.Contains(string(island.JSON), "pageProps") {
				t.Errorf("expected pageProps in island JSON, got: %s", island.JSON)
			}
		}
	}
	if !found {
		t.Error("expected __NEXT_DATA__ island")
	}
}

func TestPipelineFormPage(t *testing.T) {
	srv := serveFile(t, "form_page.html")
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	res, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars: 20000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have classified interactive elements with @eN IDs
	if len(res.Doc.Interactive) == 0 {
		t.Fatal("expected interactive elements")
	}

	// Verify @eN IDs are assigned
	for _, el := range res.Doc.Interactive {
		if el.AgentID == "" {
			t.Errorf("interactive element %q has no agent ID", el.Category)
		}
		if !strings.HasPrefix(el.AgentID, "@e") {
			t.Errorf("expected agent ID starting with @e, got %q", el.AgentID)
		}
	}

	// Should have at least one button and one input
	var hasButton, hasInput bool
	for _, el := range res.Doc.Interactive {
		if el.Category == "button" {
			hasButton = true
		}
		if el.Category == "input" {
			hasInput = true
		}
	}
	if !hasButton {
		t.Error("expected at least one button element")
	}
	if !hasInput {
		t.Error("expected at least one input element")
	}
}

func TestPipelineTinyPageFallback(t *testing.T) {
	srv := serveFile(t, "tiny_page.html")
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	// charThreshold > content → should trigger fallback and still return something
	res, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars:      20000,
			CharThreshold: 500, // way above actual content
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Should still return some output (fallback to full body)
	if res.Meta.CharCount == 0 {
		t.Error("expected non-empty output even for tiny page")
	}
}

func TestPipelineNavHeavy(t *testing.T) {
	srv := serveFile(t, "nav_heavy.html")
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	res, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars: 20000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should find the article content about renewable energy
	if !strings.Contains(res.Output, "energy") && !strings.Contains(res.Output, "Energy") {
		t.Errorf("expected energy content in output, got: %s", res.Output[:min(300, len(res.Output))])
	}
}

func TestPipelineJSONFormat(t *testing.T) {
	srv := serveFile(t, "news_article.html")
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	res, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatJSON,
		Content: pipeline.ContentOptions{
			MaxChars: 20000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(res.Output, `"url"`) {
		t.Errorf("expected JSON output with url field, got: %s", res.Output[:min(200, len(res.Output))])
	}
	if !strings.Contains(res.Output, `"title"`) {
		t.Errorf("expected JSON output with title field")
	}
}

func TestPipelineTextFormat(t *testing.T) {
	srv := serveFile(t, "news_article.html")
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	res, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatText,
		Content: pipeline.ContentOptions{
			MaxChars: 20000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Output, "##") {
		t.Errorf("text format should not contain Markdown headers")
	}
	if !strings.Contains(res.Output, "Mars") {
		t.Errorf("expected Mars in text output")
	}
}

func TestPipelineMaxCharsTruncation(t *testing.T) {
	srv := serveFile(t, "news_article.html")
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	res, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatText,
		Content: pipeline.ContentOptions{
			MaxChars: 50,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Meta.Truncated {
		t.Error("expected truncated=true with MaxChars=50")
	}
}

func TestPipelineHTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	_, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

func TestPipeline_ISO8859_1(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "iso_8859_1.html"))
	if err != nil {
		t.Fatalf("testdata/iso_8859_1.html: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	t.Cleanup(srv.Close)

	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	res, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars: 20000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Output must be valid UTF-8
	if !utf8.ValidString(res.Output) {
		t.Errorf("output is not valid UTF-8:\n%s", res.Output[:min(500, len(res.Output))])
	}

	// Verify expected characters survived the conversion (not mojibake)
	expected := []string{"Universität", "schöne", "Freude", "Straße", "Vorlesung", "Molière"}
	for _, s := range expected {
		if !strings.Contains(res.Output, s) {
			t.Errorf("expected %q in output, got:\n%s", s, res.Output[:min(500, len(res.Output))])
		}
	}

	if res.Meta.Title == "" {
		t.Error("expected non-empty title")
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		want  pipeline.Format
	}{
		{"markdown", pipeline.FormatMarkdown},
		{"Markdown", pipeline.FormatMarkdown},
		{"MARKDOWN", pipeline.FormatMarkdown},
		{"json", pipeline.FormatJSON},
		{"JSON", pipeline.FormatJSON},
		{"text", pipeline.FormatText},
		{"TEXT", pipeline.FormatText},
		{"html", pipeline.FormatHTML},
		{"HTML", pipeline.FormatHTML},
		{"unknown", pipeline.FormatMarkdown},
		{"", pipeline.FormatMarkdown},
		{"xml", pipeline.FormatMarkdown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := pipeline.ParseFormat(tt.input)
			if got != tt.want {
				t.Errorf("ParseFormat(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRunRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Hello Raw</body></html>"))
	}))
	defer srv.Close()

	c := newTestClient(t)

	resp, err := pipeline.RunRaw(context.Background(), c, srv.URL, fetch.Request{
		AllowPrivate: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "Hello Raw") {
		t.Errorf("expected raw body content, got: %s", resp.Body)
	}
}

func TestRunRaw_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	c := newTestClient(t)

	resp, err := pipeline.RunRaw(context.Background(), c, srv.URL, fetch.Request{
		AllowPrivate: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
