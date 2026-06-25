package pipeline_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/danieljustus/symaira-fetch/internal/cache"
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

func TestPipelineCachedPrivateURLBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	cacher := cache.New(tmpDir, 1*time.Hour, 0)

	privateURL := "http://127.0.0.1:9999/secret"
	cacher.Put(privateURL, "chrome", "markdown", "", "", []byte("secret content"), cache.Meta{
		URL:      privateURL,
		FinalURL: "http://127.0.0.1:9999/secret",
	})

	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	_, err := pipeline.Run(context.Background(), c, eng, privateURL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Cache: pipeline.CacheOptions{
			Instance: cacher,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: false,
		},
	})
	if err == nil {
		t.Fatal("expected error for cached private URL when AllowPrivate=false")
	}
}

func TestPipelineCachedPrivateRedirectBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	cacher := cache.New(tmpDir, 1*time.Hour, 0)

	publicURL := "http://example.com/page"
	privateRedirect := "http://127.0.0.1:9999/admin"
	cacher.Put(publicURL, "chrome", "markdown", "", "", []byte("redirected content"), cache.Meta{
		URL:      publicURL,
		FinalURL: privateRedirect,
	})

	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	_, err := pipeline.Run(context.Background(), c, eng, publicURL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Cache: pipeline.CacheOptions{
			Instance: cacher,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: false,
		},
	})
	if err == nil {
		t.Fatal("expected error for cached public URL with private redirect target when AllowPrivate=false")
	}
}

func TestPipelineCachedPrivateURLAllowed(t *testing.T) {
	privateSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>private content</body></html>"))
	}))
	defer privateSrv.Close()

	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	_, err := pipeline.Run(context.Background(), c, eng, privateSrv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars: 20000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatalf("expected no error with AllowPrivate=true, got: %v", err)
	}
}

func TestPipelineSSRFBeforeCacheLookup(t *testing.T) {
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	_, err := pipeline.Run(context.Background(), c, eng, "http://127.0.0.1:9999/secret", pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Security: pipeline.SecurityOptions{
			AllowPrivate: false,
		},
	})
	if err == nil {
		t.Fatal("expected error for private URL when AllowPrivate=false")
	}
	var blockedErr *fetch.ErrBlockedPrivate
	if !errors.As(err, &blockedErr) {
		t.Errorf("expected ErrBlockedPrivate, got %T: %v", err, err)
	}
}

func longArticleHTML() string {
	body := "<p>Paragraph one with some content. </p>"
	for i := 0; i < 50; i++ {
		body += "<p>This is paragraph " + strings.Repeat("x", 20) + " with enough text to exceed a small character budget. </p>"
	}
	return `<!DOCTYPE html><html><head><title>Long Article</title></head><body>
<article>` + body + `</article></body></html>`
}

func linkArticleHTML() string {
	return `<!DOCTYPE html><html><head><title>Link Article</title></head><body>
<article>
<p>Paragraph about our products and services.</p>
<div><a href="/guide">Complete Guide</a></div>
<div><a href="/faq">FAQ</a></div>
<div><a href="/about">About Us</a></div>
<p>Additional paragraphs with enough content to fill the output.</p>
<p>More text here to give the builder room to process all elements.</p>
</article></body></html>`
}

func TestPipelineCacheDifferentMaxCharsDontShare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(longArticleHTML()))
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	cacher := cache.New(tmpDir, 1*time.Hour, 0)
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	small, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatText,
		Content: pipeline.ContentOptions{
			MaxChars: 50,
		},
		Cache: pipeline.CacheOptions{
			Instance: cacher,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	large, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatText,
		Content: pipeline.ContentOptions{
			MaxChars: 20000,
		},
		Cache: pipeline.CacheOptions{
			Instance: cacher,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	smallLen := utf8.RuneCountInString(small.Output)
	largeLen := utf8.RuneCountInString(large.Output)
	if smallLen >= largeLen {
		t.Errorf("expected small max_chars output (%d chars) to be shorter than large (%d chars)", smallLen, largeLen)
	}
}

func TestPipelineCacheLinksDisabledThenEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(linkArticleHTML()))
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	cacher := cache.New(tmpDir, 1*time.Hour, 0)
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	noLinks, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars:     20000,
			IncludeLinks: false,
		},
		Cache: pipeline.CacheOptions{
			Instance: cacher,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	withLinks, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars:     20000,
			IncludeLinks: true,
		},
		Cache: pipeline.CacheOptions{
			Instance: cacher,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if noLinks.Output == withLinks.Output {
		t.Error("expected different output when include_links toggles")
	}
}

func TestPipelineThinContentFallbackToMDTwin(t *testing.T) {
	spaHTML, err := os.ReadFile(filepath.Join("testdata", "spa_shell.html"))
	if err != nil {
		t.Fatalf("testdata/spa_shell.html: %v", err)
	}
	mdContent, err := os.ReadFile(filepath.Join("testdata", "spa_shell.md"))
	if err != nil {
		t.Fatalf("testdata/spa_shell.md: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if strings.HasSuffix(r.URL.Path, ".md") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(mdContent)
			return
		}
		w.Write(spaHTML)
	}))
	t.Cleanup(srv.Close)

	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	res, err := pipeline.Run(context.Background(), c, eng, srv.URL+"/dashboard", pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars:      20000,
			CharThreshold: 500,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(res.Output, "Dashboard Guide") {
		t.Errorf("expected fallback to .md twin content, got:\n%s", res.Output[:min(500, len(res.Output))])
	}
	if !strings.Contains(res.Output, "Active Users") {
		t.Errorf("expected real article content from .md twin, got:\n%s", res.Output[:min(500, len(res.Output))])
	}
}

func TestPipelineRichContentNoFallback(t *testing.T) {
	srv := serveFile(t, "news_article.html")
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	hitCount := 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		data, _ := os.ReadFile(filepath.Join("testdata", "news_article.html"))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	}))
	t.Cleanup(srv2.Close)

	_, err := pipeline.Run(context.Background(), c, eng, srv.URL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars:      20000,
			CharThreshold: 500,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if hitCount > 1 {
		t.Errorf("expected no fallback fetch for rich content, got %d fetches", hitCount)
	}
}

func TestPipelineThinContentFallbackCached(t *testing.T) {
	spaHTML, err := os.ReadFile(filepath.Join("testdata", "spa_shell.html"))
	if err != nil {
		t.Fatalf("testdata/spa_shell.html: %v", err)
	}
	mdContent, err := os.ReadFile(filepath.Join("testdata", "spa_shell.md"))
	if err != nil {
		t.Fatalf("testdata/spa_shell.md: %v", err)
	}

	fetchCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if strings.HasSuffix(r.URL.Path, ".md") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(mdContent)
			return
		}
		w.Write(spaHTML)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	cacher := cache.New(tmpDir, 1*time.Hour, 0)
	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	originalURL := srv.URL + "/dashboard"

	res1, err := pipeline.Run(context.Background(), c, eng, originalURL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars:      20000,
			CharThreshold: 500,
		},
		Cache: pipeline.CacheOptions{
			Instance: cacher,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res1.Output, "Dashboard Guide") {
		t.Errorf("first run: expected fallback content, got:\n%s", res1.Output[:min(500, len(res1.Output))])
	}

	fetchCountAfterFirst := fetchCount

	res2, err := pipeline.Run(context.Background(), c, eng, originalURL, pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Content: pipeline.ContentOptions{
			MaxChars:      20000,
			CharThreshold: 500,
		},
		Cache: pipeline.CacheOptions{
			Instance: cacher,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res2.Output, "Dashboard Guide") {
		t.Errorf("second run: expected cached fallback content, got:\n%s", res2.Output[:min(500, len(res2.Output))])
	}
	if fetchCount != fetchCountAfterFirst {
		t.Errorf("expected no additional fetches on cache hit, got %d total fetches (was %d after first run)", fetchCount, fetchCountAfterFirst)
	}
}

func TestPipeline404WithReachableAncestor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/a/b/c":
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("not found"))
		case "/a/b/":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><body><p>ancestor content</p></body></html>"))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("not found"))
		}
	}))
	defer srv.Close()

	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	_, err := pipeline.Run(context.Background(), c, eng, srv.URL+"/a/b/c", pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}

	var fetchErr *pipeline.FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	if fetchErr.Recovery == nil {
		t.Fatal("expected recovery hints, got nil")
	}
	if !strings.Contains(fetchErr.Recovery.NearestAncestor, "/a/b/") {
		t.Errorf("expected ancestor containing '/a/b/', got %q", fetchErr.Recovery.NearestAncestor)
	}
	if fetchErr.Recovery.AncestorStatus != http.StatusOK {
		t.Errorf("expected ancestor status 200, got %d", fetchErr.Recovery.AncestorStatus)
	}
}

func TestPipeline404WithNoReachableAncestor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	_, err := pipeline.Run(context.Background(), c, eng, srv.URL+"/a/b/c", pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}

	var fetchErr *pipeline.FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	if fetchErr.Recovery != nil {
		t.Errorf("expected nil recovery hints when all ancestors 404, got %+v", fetchErr.Recovery)
	}
}

func TestPipeline200NoAncestorProbing(t *testing.T) {
	fetchCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body><p>ok</p></body></html>"))
	}))
	defer srv.Close()

	c := newTestClient(t)
	eng := pipeline.StaticEngine{}

	_, err := pipeline.Run(context.Background(), c, eng, srv.URL+"/a/b/c", pipeline.Options{
		Format: pipeline.FormatMarkdown,
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatalf("expected no error for 200 response, got: %v", err)
	}
	if fetchCount != 1 {
		t.Errorf("expected exactly 1 fetch for 200 response (no ancestor probing), got %d", fetchCount)
	}
}
