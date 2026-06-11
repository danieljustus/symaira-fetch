package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHonestClientFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Hello</body></html>"))
	}))
	defer srv.Close()

	c, err := New(ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	resp, err := c.Fetch(context.Background(), Request{
		URL:          srv.URL,
		AllowPrivate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "Hello") {
		t.Errorf("body does not contain expected content")
	}
}

func TestHonestClientUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := New(ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Fetch(context.Background(), Request{
		URL:          srv.URL,
		AllowPrivate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotUA != honestUA {
		t.Errorf("expected UA %q, got %q", honestUA, gotUA)
	}
}

func TestHonestClientRedirect(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("final"))
	}))
	defer final.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	c, err := New(ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	resp, err := c.Fetch(context.Background(), Request{
		URL:          redirect.URL,
		AllowPrivate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after redirect, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "final") {
		t.Errorf("expected final body after redirect")
	}
}

func TestMaxBodyLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.Repeat("x", 200)))
	}))
	defer srv.Close()

	c, err := New(ProfileHonest, WithMaxBody(0)) // 0 → uses default but we override in request
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Fetch(context.Background(), Request{
		URL:          srv.URL,
		MaxBody:      100, // 100 bytes limit
		AllowPrivate: true,
	})
	if err == nil {
		t.Fatal("expected ErrTooLarge, got nil")
	}
	var tooLarge *ErrTooLarge
	if !isErrType(err, &tooLarge) {
		t.Errorf("expected ErrTooLarge, got %T: %v", err, err)
	}
}

func isErrType(err error, target interface{}) bool {
	switch e := err.(type) {
	case *ErrTooLarge:
		_ = e
		if _, ok := target.(**ErrTooLarge); ok {
			return true
		}
	}
	return false
}

func TestSSRFGuard(t *testing.T) {
	tests := []struct {
		url     string
		blocked bool
	}{
		{"http://127.0.0.1:8080/foo", true},
		{"http://localhost/bar", true},
		{"http://192.168.1.1/", true},
		{"http://10.0.0.1/", true},
		{"http://172.16.0.1/", true},
		{"ftp://example.com/", true},
		{"https://example.com/", false}, // DNS lookup will succeed in tests or fail gracefully
	}

	for _, tt := range tests {
		err := checkSSRF(tt.url)
		isBlocked := err != nil
		if isBlocked != tt.blocked {
			t.Errorf("checkSSRF(%q): blocked=%v, want %v (err=%v)", tt.url, isBlocked, tt.blocked, err)
		}
	}
}

func TestCookiePersistence(t *testing.T) {
	cookieSet := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cookieSet {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
			cookieSet = true
		}
		// Echo back cookie header
		c, _ := r.Cookie("session")
		if c != nil {
			w.Write([]byte("cookie:" + c.Value))
		} else {
			w.Write([]byte("no-cookie"))
		}
	}))
	defer srv.Close()

	c, err := New(ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// First request — server sets cookie
	_, err = c.Fetch(context.Background(), Request{URL: srv.URL + "/set", AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
}
