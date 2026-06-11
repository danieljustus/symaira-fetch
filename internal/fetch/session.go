package fetch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

//lint:ignore U1000 session infrastructure for future cookie persistence
type sessionStore struct {
	mu   sync.Mutex
	dir  string
	jars map[string]*cookieJar
}

func newSessionStore(dir string) *sessionStore {
	return &sessionStore{
		dir:  dir,
		jars: make(map[string]*cookieJar),
	}
}

//lint:ignore U1000 session infrastructure for future cookie persistence
func (s *sessionStore) get(name string) *cookieJar {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jars[name]; ok {
		return j
	}
	j := &cookieJar{name: name, store: s}
	j.load()
	s.jars[name] = j
	return j
}

//lint:ignore U1000 session infrastructure for future cookie persistence
type cookieJar struct {
	mu      sync.Mutex
	name    string
	store   *sessionStore
	cookies []*http.Cookie
}

//lint:ignore U1000 session infrastructure for future cookie persistence
type storedCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
	Secure bool   `json:"secure"`
}

//lint:ignore U1000 session infrastructure for future cookie persistence
func (j *cookieJar) load() {
	if j.store.dir == "" {
		return
	}
	path := j.filePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var stored []storedCookie
	if err := json.Unmarshal(data, &stored); err != nil {
		return
	}
	j.cookies = make([]*http.Cookie, 0, len(stored))
	for _, sc := range stored {
		j.cookies = append(j.cookies, &http.Cookie{
			Name:   sc.Name,
			Value:  sc.Value,
			Domain: sc.Domain,
			Path:   sc.Path,
			Secure: sc.Secure,
		})
	}
}

//lint:ignore U1000 session infrastructure for future cookie persistence
func (j *cookieJar) save() {
	if j.store.dir == "" {
		return
	}
	if err := os.MkdirAll(j.store.dir, 0700); err != nil {
		return
	}
	stored := make([]storedCookie, 0, len(j.cookies))
	for _, c := range j.cookies {
		stored = append(stored, storedCookie{
			Name:   c.Name,
			Value:  c.Value,
			Domain: c.Domain,
			Path:   c.Path,
			Secure: c.Secure,
		})
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return
	}
	tmp := j.filePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, j.filePath())
}

//lint:ignore U1000 session infrastructure for future cookie persistence
func (j *cookieJar) filePath() string {
	return filepath.Join(j.store.dir, fmt.Sprintf("%s.json", sanitizeName(j.name)))
}

//lint:ignore U1000 session infrastructure for future cookie persistence
func sanitizeName(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}
