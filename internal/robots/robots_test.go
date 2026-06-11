package robots

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseBasic(t *testing.T) {
	content := `
User-agent: *
Disallow: /admin/
Disallow: /private/
Allow: /admin/public/
`
	groups := parse(content)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if len(g.agents) != 1 || g.agents[0] != "*" {
		t.Fatalf("expected agent *, got %v", g.agents)
	}
	if len(g.rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(g.rules))
	}
	if g.rules[0].path != "/admin/" || g.rules[0].allowed {
		t.Errorf("rule 0: expected Disallow /admin/, got %+v", g.rules[0])
	}
	if g.rules[1].path != "/private/" || g.rules[1].allowed {
		t.Errorf("rule 1: expected Disallow /private/, got %+v", g.rules[1])
	}
	if g.rules[2].path != "/admin/public/" || !g.rules[2].allowed {
		t.Errorf("rule 2: expected Allow /admin/public/, got %+v", g.rules[2])
	}
}

func TestParseMultipleAgents(t *testing.T) {
	content := `
User-agent: Googlebot
Disallow: /secret/

User-agent: *
Disallow: /admin/
`
	groups := parse(content)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].agents[0] != "googlebot" {
		t.Errorf("group 0 agent: expected googlebot, got %s", groups[0].agents[0])
	}
	if groups[1].agents[0] != "*" {
		t.Errorf("group 1 agent: expected *, got %s", groups[1].agents[0])
	}
}

func TestParseComments(t *testing.T) {
	content := `
# This is a comment
User-agent: * # inline comment
Disallow: /admin/ # another comment
`
	groups := parse(content)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(groups[0].rules))
	}
	if groups[0].rules[0].path != "/admin/" {
		t.Errorf("expected /admin/, got %s", groups[0].rules[0].path)
	}
}

func TestParseMalformed(t *testing.T) {
	content := `
this is not valid
User-agent
Disallow
: no key
User-agent: *
: no key again
Disallow: /ok/
random garbage line
`
	groups := parse(content)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(groups[0].rules))
	}
	if groups[0].rules[0].path != "/ok/" {
		t.Errorf("expected /ok/, got %s", groups[0].rules[0].path)
	}
}

func TestParseEmptyDisallow(t *testing.T) {
	content := `
User-agent: *
Disallow:
Disallow: /admin/
`
	groups := parse(content)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].rules) != 1 {
		t.Fatalf("expected 1 rule (empty Disallow ignored), got %d", len(groups[0].rules))
	}
}

func TestIsAllowedWildcard(t *testing.T) {
	groups := parse(`
User-agent: *
Disallow: /admin/
Disallow: /private/
Allow: /admin/public/
`)
	tests := []struct {
		path    string
		allowed bool
	}{
		{"/", true},
		{"/index.html", true},
		{"/admin/", false},
		{"/admin/settings", false},
		{"/admin/public/", true},
		{"/admin/public/page", true},
		{"/private/", false},
		{"/private/data", false},
	}
	for _, tt := range tests {
		got := isAllowed(groups, "TestBot", tt.path)
		if got != tt.allowed {
			t.Errorf("isAllowed(*, %q) = %v, want %v", tt.path, got, tt.allowed)
		}
	}
}

func TestIsAllowedSpecificAgent(t *testing.T) {
	groups := parse(`
User-agent: Googlebot
Disallow: /secret/

User-agent: *
Disallow: /admin/
`)
	if isAllowed(groups, "Googlebot", "/secret/") {
		t.Error("Googlebot should be disallowed from /secret/")
	}
	if !isAllowed(groups, "Googlebot", "/admin/") {
		t.Error("Googlebot should be allowed on /admin/ (no rule for it)")
	}
	if isAllowed(groups, "OtherBot", "/admin/") {
		t.Error("OtherBot should be disallowed from /admin/")
	}
	if !isAllowed(groups, "OtherBot", "/secret/") {
		t.Error("OtherBot should be allowed on /secret/")
	}
}

func TestIsAllowedLongestMatch(t *testing.T) {
	groups := parse(`
User-agent: *
Disallow: /a/
Disallow: /a/b/
Allow: /a/b/c/
`)
	tests := []struct {
		path    string
		allowed bool
	}{
		{"/a/", false},
		{"/a/x", false},
		{"/a/b/", false},
		{"/a/b/x", false},
		{"/a/b/c/", true},
		{"/a/b/c/d", true},
	}
	for _, tt := range tests {
		got := isAllowed(groups, "Bot", tt.path)
		if got != tt.allowed {
			t.Errorf("isAllowed(Bot, %q) = %v, want %v", tt.path, got, tt.allowed)
		}
	}
}

func TestIsAllowedWildcardPath(t *testing.T) {
	groups := parse(`
User-agent: *
Disallow: /search*
`)
	tests := []struct {
		path    string
		allowed bool
	}{
		{"/search", false},
		{"/search?q=test", false},
		{"/search/results", false},
		{"/other", true},
	}
	for _, tt := range tests {
		got := isAllowed(groups, "Bot", tt.path)
		if got != tt.allowed {
			t.Errorf("isAllowed(Bot, %q) = %v, want %v", tt.path, got, tt.allowed)
		}
	}
}

func TestIsAllowedNoGroups(t *testing.T) {
	if !isAllowed(nil, "Bot", "/anything") {
		t.Error("no groups should allow everything")
	}
	if !isAllowed([]group{}, "Bot", "/anything") {
		t.Error("empty groups should allow everything")
	}
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		match   bool
	}{
		{"", "/", true},
		{"", "/anything", true},
		{"/admin/", "/admin/", true},
		{"/admin/", "/admin/settings", true},
		{"/admin/", "/other", false},
		{"/search*", "/search", true},
		{"/search*", "/search?q=test", true},
		{"/search*", "/other", false},
	}
	for _, tt := range tests {
		got := matchPath(tt.pattern, tt.path)
		if got != tt.match {
			t.Errorf("matchPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.match)
		}
	}
}

func TestCheckerIntegration(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /admin/
Allow: /admin/public/
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, robotsTxt)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewChecker()
	ctx := context.Background()

	allowed, err := c.Check(ctx, "TestBot", srv.URL+"/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Error("/index.html should be allowed")
	}

	allowed, err = c.Check(ctx, "TestBot", srv.URL+"/admin/settings")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Error("/admin/settings should be disallowed")
	}

	allowed, err = c.Check(ctx, "TestBot", srv.URL+"/admin/public/page")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Error("/admin/public/page should be allowed")
	}
}

func TestCheckerCacheTTL(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		fmt.Fprint(w, "User-agent: *\nDisallow: /admin/\n")
	}))
	defer srv.Close()

	c := NewChecker()
	c.ttl = 50 * time.Millisecond
	ctx := context.Background()

	c.Check(ctx, "Bot", srv.URL+"/page1")
	c.Check(ctx, "Bot", srv.URL+"/page2")
	if callCount != 1 {
		t.Errorf("expected 1 fetch (cached), got %d", callCount)
	}

	time.Sleep(60 * time.Millisecond)

	c.Check(ctx, "Bot", srv.URL+"/page3")
	if callCount != 2 {
		t.Errorf("expected 2 fetches (cache expired), got %d", callCount)
	}
}

func TestChecker404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewChecker()
	ctx := context.Background()

	allowed, err := c.Check(ctx, "Bot", srv.URL+"/anything")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Error("404 robots.txt should allow everything")
	}
}

func TestCheckerFailOpen(t *testing.T) {
	c := NewChecker()
	ctx := context.Background()

	allowed, err := c.Check(ctx, "Bot", "http://127.0.0.1:1/anything")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Error("unreachable server should fail-open (allow)")
	}
}

func TestCheckerNonHTTPScheme(t *testing.T) {
	c := NewChecker()
	ctx := context.Background()

	allowed, err := c.Check(ctx, "Bot", "ftp://example.com/file")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Error("non-HTTP scheme should be allowed")
	}
}

func TestCheckerEmptyPath(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow: /`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, robotsTxt)
	}))
	defer srv.Close()

	c := NewChecker()
	ctx := context.Background()

	allowed, err := c.Check(ctx, "Bot", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Error("root path with Disallow: / should be disallowed")
	}
}
