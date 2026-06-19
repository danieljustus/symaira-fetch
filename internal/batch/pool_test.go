package batch_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/batch"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

func TestHostOf(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/path", "example.com"},
		{"http://example.com:8080/path", "example.com:8080"},
		{"https://user:pass@example.com/path", "example.com"},
		{"http://192.168.1.1/api", "192.168.1.1"},
		{"https://example.com", "example.com"},
		{"ftp://example.com/file", "example.com"},
	}

	for _, tt := range tests {
		got := batch.HostOf(tt.url)
		if got != tt.want {
			t.Errorf("hostOf(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
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

func TestPoolOrderPreserved(t *testing.T) {
	// Serve pages with distinct identifiers to verify output order matches input order.
	servers := make([]*httptest.Server, 3)
	for i := range servers {
		i := i
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<html><body><p>Page number %d content text.</p></body></html>`, i)
		}))
		t.Cleanup(srv.Close)
		servers[i] = srv
	}

	items := make([]batch.Item, len(servers))
	for i, srv := range servers {
		items[i] = batch.Item{URL: srv.URL}
	}

	c := newTestClient(t)
	pool := batch.Pool{Workers: 3}
	results := pool.RunBatch(context.Background(), c, pipeline.StaticEngine{}, items, pipeline.Options{
		Format: pipeline.FormatText,
		Content: pipeline.ContentOptions{
			MaxChars: 5000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})

	if len(results) != len(items) {
		t.Fatalf("expected %d results, got %d", len(items), len(results))
	}

	for i, res := range results {
		if res.URL != servers[i].URL {
			t.Errorf("result[%d] URL mismatch: want %s got %s", i, servers[i].URL, res.URL)
		}
		if !res.OK {
			t.Errorf("result[%d] failed: %s", i, res.Error)
		}
	}
}

func TestPoolBadURLDoesNotBlockOthers(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, `<html><body><p>Good page content here.</p></body></html>`)
	}))
	t.Cleanup(good.Close)

	items := []batch.Item{
		{URL: good.URL},
		{URL: "http://[invalid"}, // malformed URL
		{URL: good.URL},
	}

	c := newTestClient(t)
	pool := batch.Pool{Workers: 3}
	results := pool.RunBatch(context.Background(), c, pipeline.StaticEngine{}, items, pipeline.Options{
		Format: pipeline.FormatText,
		Content: pipeline.ContentOptions{
			MaxChars: 5000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].OK {
		t.Errorf("first result should be OK, got error: %s", results[0].Error)
	}
	if results[1].OK {
		t.Errorf("second result (bad URL) should have failed")
	}
	if !results[2].OK {
		t.Errorf("third result should be OK, got error: %s", results[2].Error)
	}
}

func TestPoolContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `<html><body><p>Content</p></body></html>`)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	items := []batch.Item{{URL: srv.URL}, {URL: srv.URL}}
	c := newTestClient(t)
	pool := batch.Pool{Workers: 2}

	// Should not hang — just return (possibly with errors)
	results := pool.RunBatch(ctx, c, pipeline.StaticEngine{}, items, pipeline.Options{
		Format: pipeline.FormatText,
		Content: pipeline.ContentOptions{
			MaxChars: 5000,
		},
		Security: pipeline.SecurityOptions{
			AllowPrivate: true,
		},
	})

	if len(results) != 2 {
		t.Errorf("expected 2 result slots, got %d", len(results))
	}
}
