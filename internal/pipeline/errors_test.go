package pipeline_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

func TestFetchError(t *testing.T) {
	inner := errors.New("connection refused")
	err := &pipeline.FetchError{URL: "https://example.com", Err: inner}

	if err.Error() != "fetch https://example.com: connection refused" {
		t.Errorf("Error() = %q", err.Error())
	}
	if err.Unwrap() != inner {
		t.Error("Unwrap() should return inner error")
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is should find inner error")
	}
}

func TestParseError(t *testing.T) {
	inner := errors.New("invalid HTML")
	err := &pipeline.ParseError{URL: "https://example.com", Err: inner}

	if err.Error() != "parse https://example.com: invalid HTML" {
		t.Errorf("Error() = %q", err.Error())
	}
	if err.Unwrap() != inner {
		t.Error("Unwrap() should return inner error")
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is should find inner error")
	}
}

func TestRenderError(t *testing.T) {
	inner := errors.New("template execution failed")
	err := &pipeline.RenderError{Format: "markdown", Err: inner}

	if err.Error() != "render markdown: template execution failed" {
		t.Errorf("Error() = %q", err.Error())
	}
	if err.Unwrap() != inner {
		t.Error("Unwrap() should return inner error")
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is should find inner error")
	}
}

func TestBlockedError(t *testing.T) {
	err := &pipeline.BlockedError{URL: "http://127.0.0.1", Reason: "SSRF blocked"}

	if err.Error() != "blocked http://127.0.0.1: SSRF blocked" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestErrorsAs_FetchError(t *testing.T) {
	inner := fmt.Errorf("timeout")
	err := &pipeline.FetchError{URL: "https://example.com", Err: inner}

	var target *pipeline.FetchError
	if !errors.As(err, &target) {
		t.Error("errors.As should find *FetchError")
	}
	if target.URL != "https://example.com" {
		t.Errorf("target.URL = %q", target.URL)
	}
}

func TestErrorsAs_ParseError(t *testing.T) {
	inner := fmt.Errorf("bad markup")
	err := &pipeline.ParseError{URL: "https://example.com", Err: inner}

	var target *pipeline.ParseError
	if !errors.As(err, &target) {
		t.Error("errors.As should find *ParseError")
	}
}

func TestErrorsAs_RenderError(t *testing.T) {
	inner := fmt.Errorf("render failed")
	err := &pipeline.RenderError{Format: "json", Err: inner}

	var target *pipeline.RenderError
	if !errors.As(err, &target) {
		t.Error("errors.As should find *RenderError")
	}
	if target.Format != "json" {
		t.Errorf("target.Format = %q", target.Format)
	}
}

func TestErrorsAs_BlockedError(t *testing.T) {
	err := &pipeline.BlockedError{URL: "http://10.0.0.1", Reason: "private"}

	var target *pipeline.BlockedError
	if !errors.As(err, &target) {
		t.Error("errors.As should find *BlockedError")
	}
	if target.Reason != "private" {
		t.Errorf("target.Reason = %q", target.Reason)
	}
}
