package pipeline

import (
	"context"

	"github.com/danieljustus/symaira-fetch/internal/dom"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
)

// Engine materialises an HTTP response into a parsed DOM tree.
// v1 ships only StaticEngine (parse-only). Future JSEngine implementations
// (QuickJS-via-wazero, Lightpanda/CDP delegate) must satisfy this interface
// and return a dom.Tree — all downstream pipeline stages remain unchanged.
type Engine interface {
	Materialize(ctx context.Context, resp *fetch.Response) (*dom.Tree, error)
}

// StaticEngine parses HTML without executing JavaScript.
type StaticEngine struct{}

func (StaticEngine) Materialize(_ context.Context, resp *fetch.Response) (*dom.Tree, error) {
	return dom.Parse(resp.Body)
}
