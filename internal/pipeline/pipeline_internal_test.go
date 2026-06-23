package pipeline

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
	"github.com/danieljustus/symaira-fetch/internal/cache"
	"github.com/danieljustus/symaira-fetch/internal/dom"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/robots"
)

type fakeClient struct {
	resp *fetch.Response
	err  error
}

func (c *fakeClient) Fetch(_ context.Context, _ fetch.Request) (*fetch.Response, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.resp, nil
}

func (c *fakeClient) Close() error { return nil }

type fakeEngine struct {
	tree *dom.Tree
	err  error
}

func (e *fakeEngine) Materialize(_ context.Context, _ *fetch.Response) (*dom.Tree, error) {
	return e.tree, e.err
}

type fakeCache struct {
	*cache.Cache
	putErr error
}

func (fc *fakeCache) Put(_, _, _, _, _ string, _ []byte, _ cache.Meta) error {
	return fc.putErr
}

// cachePutter is the minimal interface pipeline needs from a cache.
type cachePutter interface {
	Put(url, profile, format, session, contentKey string, body []byte, meta cache.Meta) error
}

var _ cachePutter = (*cache.Cache)(nil)
var _ cachePutter = (*fakeCache)(nil)

func TestRawHTMLFallback(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{
			name: "empty",
			body: []byte{},
			want: "",
		},
		{
			name: "ascii",
			body: []byte("Hello World"),
			want: "Hello World",
		},
		{
			name: "html",
			body: []byte("<html><body>Content</body></html>"),
			want: "<html><body>Content</body></html>",
		},
		{
			name: "binary-like",
			body: []byte{0x00, 0x01, 0x02},
			want: "\x00\x01\x02",
		},
		{
			name: "unicode",
			body: []byte("Hello \u00e9\u00e8\u00ea"),
			want: "Hello \u00e9\u00e8\u00ea",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rawHTMLFallback(tt.body)
			if got != tt.want {
				t.Errorf("rawHTMLFallback() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIslandSummary(t *testing.T) {
	tests := []struct {
		name     string
		islands  []agentdom.DataIsland
		contains []string
		empty    bool
	}{
		{
			name:    "empty islands",
			islands: []agentdom.DataIsland{},
			empty:   true,
		},
		{
			name: "single island with object",
			islands: []agentdom.DataIsland{
				{
					Source: "__NEXT_DATA__",
					JSON:   []byte(`{"page":"home","props":{"id":1}}`),
				},
			},
			contains: []string{"__NEXT_DATA__", "keys="},
		},
		{
			name: "multiple islands",
			islands: []agentdom.DataIsland{
				{
					Source: "__NEXT_DATA__",
					JSON:   []byte(`{"page":"home"}`),
				},
				{
					Source: "application/ld+json",
					JSON:   []byte(`{"@type":"Article","headline":"Test"}`),
				},
			},
			contains: []string{"__NEXT_DATA__", "application/ld+json"},
		},
		{
			name: "island with array JSON",
			islands: []agentdom.DataIsland{
				{
					Source: "data-list",
					JSON:   []byte(`[1,2,3]`),
				},
			},
			contains: []string{"data-list", "raw JSON"},
		},
		{
			name: "island with malformed JSON",
			islands: []agentdom.DataIsland{
				{
					Source: "bad-json",
					JSON:   []byte(`not valid json`),
				},
			},
			contains: []string{"bad-json", "raw JSON"},
		},
		{
			name: "island with primitive JSON",
			islands: []agentdom.DataIsland{
				{
					Source: "string-value",
					JSON:   []byte(`"just a string"`),
				},
			},
			contains: []string{"string-value", "raw JSON"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IslandSummary(tt.islands)
			if tt.empty {
				if got != "" {
					t.Errorf("IslandSummary() = %q, want empty", got)
				}
				return
			}
			for _, s := range tt.contains {
				if !containsString(got, s) {
					t.Errorf("IslandSummary() = %q, should contain %q", got, s)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRun_CachedPrivateRedirectBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	cacher := cache.New(tmpDir, 1*time.Hour)
	ck := (&ContentOptions{MaxChars: 20000, CharThreshold: 500, MaxIslandBytes: 5000}).ContentKey()
	cacher.Put("https://example.com/page", "chrome", "markdown", "", ck, []byte("cached"), cache.Meta{
		URL:      "https://example.com/page",
		FinalURL: "http://127.0.0.1:9999/admin",
	})

	if _, _, ok := cacher.Get("https://example.com/page", "chrome", "markdown", "", ck); !ok {
		t.Fatal("expected cache hit in setup")
	}

	respBody := []byte("<html><body>fetched after cache discard</body></html>")
	tree, err := dom.Parse(respBody)
	if err != nil {
		t.Fatal(err)
	}

	c := &fakeClient{
		resp: &fetch.Response{
			FinalURL:    "https://example.com/page",
			StatusCode:  http.StatusOK,
			Headers:     map[string][]string{"Content-Type": {"text/html; charset=utf-8"}},
			Body:        respBody,
			ContentType: "text/html; charset=utf-8",
		},
	}
	eng := &fakeEngine{tree: tree}

	res, err := Run(context.Background(), c, eng, "https://example.com/page", Options{
		Format: FormatMarkdown,
		Cache: CacheOptions{
			Instance: cacher,
		},
		Security: SecurityOptions{
			AllowPrivate: false,
		},
	})
	if err != nil {
		t.Fatalf("expected pipeline to complete after cache discard, got: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if !containsString(res.Output, "fetched after cache discard") {
		t.Errorf("expected fetched body to be processed, got output: %q", res.Output)
	}
}

func TestRun_RobotsCheckErrorLogged(t *testing.T) {
	srv := serveInternalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Hello</body></html>"))
	}))

	c, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	checker := robots.NewChecker().WithPrivate(true)
	_, err = Run(context.Background(), c, StaticEngine{}, srv.URL+"/page", Options{
		Format: FormatMarkdown,
		Security: SecurityOptions{
			AllowPrivate:  true,
			Robots:        true,
			RobotsChecker: checker,
		},
	})
	if err != nil {
		t.Fatalf("expected robots.txt error to be logged and fetch to proceed, got: %v", err)
	}
}

func TestRun_MaterializeError(t *testing.T) {
	srv := serveInternalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Hello</body></html>"))
	}))

	c, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	eng := &fakeEngine{err: errors.New("boom")}
	_, err = Run(context.Background(), c, eng, srv.URL, Options{
		Format: FormatMarkdown,
		Security: SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err == nil {
		t.Fatal("expected error for materialize failure")
	}
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Errorf("expected ParseError, got %T: %v", err, err)
	}
}

func TestRun_CachePutFailureLogged(t *testing.T) {
	srv := serveInternalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Hello</body></html>"))
	}))

	c, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if os.Getuid() == 0 {
		t.Skip("root can write into read-only directories")
	}

	tmpDir := t.TempDir()
	baseCache := cache.New(tmpDir, 1*time.Hour)

	// Make the cache shard directory read-only so Put fails.
	// Use a fixed URL to avoid needing access to the unexported key method.
	shard := filepath.Join(tmpDir, "ab")
	if err := os.MkdirAll(shard, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shard, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(shard, 0700) })

	_, err = Run(context.Background(), c, StaticEngine{}, srv.URL, Options{
		Format: FormatMarkdown,
		Cache: CacheOptions{
			Instance: baseCache,
		},
		Security: SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatalf("expected cache put failure to be logged and ignored, got: %v", err)
	}
}

func serveInternalServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_NoCache(t *testing.T) {
	srv := serveInternalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>No Cache</body></html>"))
	}))

	c, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	res, err := Run(context.Background(), c, StaticEngine{}, srv.URL, Options{
		Format: FormatMarkdown,
		Cache: CacheOptions{
			NoCache: true,
		},
		Security: SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsString(res.Output, "No Cache") {
		t.Errorf("expected 'No Cache' in output, got: %q", res.Output)
	}
}

func TestRun_DefaultCacheDir(t *testing.T) {
	srv := serveInternalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Default Dir</body></html>"))
	}))

	c, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	res, err := Run(context.Background(), c, StaticEngine{}, srv.URL, Options{
		Format: FormatMarkdown,
		Cache: CacheOptions{
			Dir: "",
		},
		Security: SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsString(res.Output, "Default Dir") {
		t.Errorf("expected 'Default Dir' in output, got: %q", res.Output)
	}
}

func TestRun_DefaultCacheTTL(t *testing.T) {
	srv := serveInternalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Default TTL</body></html>"))
	}))

	c, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	res, err := Run(context.Background(), c, StaticEngine{}, srv.URL, Options{
		Format: FormatMarkdown,
		Cache: CacheOptions{
			Dir: t.TempDir(),
			TTL: 0,
		},
		Security: SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsString(res.Output, "Default TTL") {
		t.Errorf("expected 'Default TTL' in output, got: %q", res.Output)
	}
}

func TestContentKey_Deterministic(t *testing.T) {
	o1 := ContentOptions{MaxChars: 5000, IncludeLinks: true, CharThreshold: 200, MaxIslandBytes: 1000}
	o2 := ContentOptions{MaxChars: 5000, IncludeLinks: true, CharThreshold: 200, MaxIslandBytes: 1000}
	if o1.ContentKey() != o2.ContentKey() {
		t.Errorf("expected same ContentKey for identical options")
	}

	o3 := ContentOptions{MaxChars: 1000, IncludeLinks: false, CharThreshold: 200, MaxIslandBytes: 1000}
	if o1.ContentKey() == o3.ContentKey() {
		t.Errorf("expected different ContentKey for different MaxChars")
	}
}

func TestRun_CacheHitReturnsDirectly(t *testing.T) {
	tmpDir := t.TempDir()
	cacher := cache.New(tmpDir, 1*time.Hour)

	srv := serveInternalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be hit when cache has valid entry")
	}))

	c := &fakeClient{
		resp: &fetch.Response{},
		err:  nil,
	}
	eng := &fakeEngine{}

	cachedBody := []byte("cached markdown output")
	ck := (&ContentOptions{MaxChars: 20000, CharThreshold: 500, MaxIslandBytes: 5000}).ContentKey()
	cacher.Put(srv.URL, "chrome", "markdown", "", ck, cachedBody, cache.Meta{
		URL:        srv.URL,
		FinalURL:   srv.URL,
		StatusCode: 200,
		Protocol:   "HTTP/1.1",
	})

	res, err := Run(context.Background(), c, eng, srv.URL, Options{
		Format: FormatMarkdown,
		Cache: CacheOptions{
			Instance: cacher,
		},
		Security: SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res.Output != "cached markdown output" {
		t.Errorf("expected cached output, got: %q", res.Output)
	}
}

func TestRun_AllowPrivate_Propagated(t *testing.T) {
	c := &fakeClient{
		resp: &fetch.Response{
			StatusCode: 200,
			Body:       []byte("<html><body>ok</body></html>"),
			Headers:    map[string][]string{"Content-Type": {"text/html; charset=utf-8"}},
			FinalURL:   "http://127.0.0.1:9999/page",
		},
	}
	tree, err := dom.Parse([]byte("<html><body>ok</body></html>"))
	if err != nil {
		t.Fatal(err)
	}
	eng := &fakeEngine{tree: tree}

	_, err = Run(context.Background(), c, eng, "http://127.0.0.1:9999/page", Options{
		Format: FormatMarkdown,
		Cache: CacheOptions{
			NoCache: true,
		},
		Security: SecurityOptions{
			AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatalf("expected no error with AllowPrivate=true, got: %v", err)
	}
}

func TestRun_CacheHit_PrivateRedirect_Discarded(t *testing.T) {
	tmpDir := t.TempDir()
	cacher := cache.New(tmpDir, 1*time.Hour)

	publicURL := "https://example.com/page"
	privateRedirect := "http://127.0.0.1:9999/secret"
	ck := (&ContentOptions{MaxChars: 20000, CharThreshold: 500, MaxIslandBytes: 5000}).ContentKey()
	cacher.Put(publicURL, "chrome", "markdown", "", ck, []byte("cached secret"), cache.Meta{
		URL:      publicURL,
		FinalURL: privateRedirect,
	})

	respBody := []byte("<html><body>fetched fresh</body></html>")
	tree, _ := dom.Parse(respBody)

	c := &fakeClient{
		resp: &fetch.Response{
			FinalURL:    publicURL,
			StatusCode:  http.StatusOK,
			Headers:     map[string][]string{"Content-Type": {"text/html; charset=utf-8"}},
			Body:        respBody,
			ContentType: "text/html; charset=utf-8",
		},
	}
	eng := &fakeEngine{tree: tree}

	res, err := Run(context.Background(), c, eng, publicURL, Options{
		Format: FormatMarkdown,
		Cache: CacheOptions{
			Instance: cacher,
		},
		Security: SecurityOptions{
			AllowPrivate: false,
		},
	})
	if err != nil {
		t.Fatalf("expected pipeline to complete after cache discard, got: %v", err)
	}
	if !containsString(res.Output, "fetched fresh") {
		t.Errorf("expected fresh fetch output, got: %q", res.Output)
	}
}
