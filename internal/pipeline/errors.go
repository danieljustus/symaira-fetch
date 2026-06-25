package pipeline

import (
	"fmt"
)

type RecoveryHints struct {
	NearestAncestor string // nearest ancestor URL that returned status < 400
	AncestorStatus  int    // HTTP status of that ancestor
}

type FetchError struct {
	URL     string
	Err     error
	Recovery *RecoveryHints // nil when no recovery hint is available
}

func (e *FetchError) Error() string {
	return fmt.Sprintf("fetch %s: %v", e.URL, e.Err)
}

func (e *FetchError) Unwrap() error {
	return e.Err
}

type ParseError struct {
	URL string
	Err error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse %s: %v", e.URL, e.Err)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

type RenderError struct {
	Format string
	Err    error
}

func (e *RenderError) Error() string {
	return fmt.Sprintf("render %s: %v", e.Format, e.Err)
}

func (e *RenderError) Unwrap() error {
	return e.Err
}

type BlockedError struct {
	URL    string
	Reason string
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("blocked %s: %s", e.URL, e.Reason)
}

type SelectorError struct {
	Selector string
}

func (e *SelectorError) Error() string {
	return fmt.Sprintf("selector %q matched no elements", e.Selector)
}

type SchemaError struct {
	Path string
	Err  string
}

func (e *SchemaError) Error() string {
	return fmt.Sprintf("schema query %q: %s", e.Path, e.Err)
}
