package pipeline

import (
	"fmt"
)

type FetchError struct {
	URL string
	Err error
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
