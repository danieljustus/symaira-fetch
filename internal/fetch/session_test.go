package fetch

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},
		{"with_underscore", "with_underscore"},
		{"with spaces", "with_spaces"},
		{"with@special#chars!", "with_special_chars_"},
		{"CamelCase123", "CamelCase123"},
		{"", ""},
		{"a b c", "a_b_c"},
		{"hello/world", "hello_world"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSessionStoreGet(t *testing.T) {
	dir := t.TempDir()
	store := newSessionStore(dir)

	jar := store.get("test-session")
	if jar == nil {
		t.Fatal("expected non-nil jar")
	}
	if jar.name != "test-session" {
		t.Errorf("expected name 'test-session', got %q", jar.name)
	}
	if jar.store != store {
		t.Error("expected jar.store to reference the session store")
	}
}

func TestSessionStoreGetCaches(t *testing.T) {
	dir := t.TempDir()
	store := newSessionStore(dir)

	jar1 := store.get("test-session")
	jar2 := store.get("test-session")

	if jar1 != jar2 {
		t.Error("expected same jar instance for same name")
	}
}

func TestCookieJarLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	store := newSessionStore(dir)

	jar := &cookieJar{name: "test", store: store}
	jar.load()

	if len(jar.cookies) != 0 {
		t.Errorf("expected 0 cookies, got %d", len(jar.cookies))
	}
}

func TestCookieJarLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	store := newSessionStore(dir)

	cookieData := `[{"name":"session","value":"abc123","domain":".example.com","path":"/","secure":true}]`
	cookiePath := filepath.Join(dir, "test.json")
	os.WriteFile(cookiePath, []byte(cookieData), 0600)

	jar := &cookieJar{name: "test", store: store}
	jar.load()

	if len(jar.cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(jar.cookies))
	}
	c := jar.cookies[0]
	if c.Name != "session" {
		t.Errorf("expected name 'session', got %q", c.Name)
	}
	if c.Value != "abc123" {
		t.Errorf("expected value 'abc123', got %q", c.Value)
	}
	if c.Domain != ".example.com" {
		t.Errorf("expected domain '.example.com', got %q", c.Domain)
	}
	if !c.Secure {
		t.Error("expected secure=true")
	}
}

func TestCookieJarLoadCorruptFile(t *testing.T) {
	dir := t.TempDir()
	store := newSessionStore(dir)

	cookiePath := filepath.Join(dir, "test.json")
	os.WriteFile(cookiePath, []byte("not valid json"), 0600)

	jar := &cookieJar{name: "test", store: store}
	jar.load()

	if len(jar.cookies) != 0 {
		t.Errorf("expected 0 cookies for corrupt file, got %d", len(jar.cookies))
	}
}

func TestCookieJarSave(t *testing.T) {
	dir := t.TempDir()
	store := newSessionStore(dir)

	jar := &cookieJar{name: "test", store: store}
	jar.cookies = []*http.Cookie{
		{Name: "session", Value: "abc123", Domain: ".example.com", Path: "/", Secure: true},
	}
	jar.save()

	cookiePath := filepath.Join(dir, "test.json")
	data, err := os.ReadFile(cookiePath)
	if err != nil {
		t.Fatalf("failed to read saved cookie: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty cookie file")
	}

	// Verify we can load it back
	jar2 := &cookieJar{name: "test", store: store}
	jar2.load()

	if len(jar2.cookies) != 1 {
		t.Fatalf("expected 1 cookie after reload, got %d", len(jar2.cookies))
	}
	if jar2.cookies[0].Name != "session" {
		t.Errorf("expected name 'session', got %q", jar2.cookies[0].Name)
	}
}

func TestCookieJarSaveEmptyDir(t *testing.T) {
	store := newSessionStore("")
	jar := &cookieJar{name: "test", store: store}
	jar.cookies = []*http.Cookie{
		{Name: "session", Value: "abc123"},
	}

	// Should not panic
	jar.save()

	if len(jar.cookies) != 1 {
		t.Error("cookies should still be in memory")
	}
}

func TestCookieJarFilePath(t *testing.T) {
	dir := t.TempDir()
	store := newSessionStore(dir)

	jar := &cookieJar{name: "test-session", store: store}
	path := jar.filePath()

	expected := filepath.Join(dir, "test-session.json")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestCookieJarFilePathSanitizedName(t *testing.T) {
	dir := t.TempDir()
	store := newSessionStore(dir)

	jar := &cookieJar{name: "test/session", store: store}
	path := jar.filePath()

	expected := filepath.Join(dir, "test_session.json")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}
