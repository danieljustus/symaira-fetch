package pipeline_test

import (
	"encoding/json"
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

func TestFetchErrorWithRecovery(t *testing.T) {
	inner := fmt.Errorf("HTTP 404")
	err := &pipeline.FetchError{
		URL: "https://example.com/a/b/c",
		Err:  inner,
		Recovery: &pipeline.RecoveryHints{
			NearestAncestor: "https://example.com/a/b/",
			AncestorStatus:  200,
		},
	}

	if err.Error() != "fetch https://example.com/a/b/c: HTTP 404" {
		t.Errorf("Error() = %q", err.Error())
	}
	if err.Unwrap() != inner {
		t.Error("Unwrap() should return inner error")
	}
	if err.Recovery == nil {
		t.Fatal("expected non-nil Recovery")
	}
	if err.Recovery.NearestAncestor != "https://example.com/a/b/" {
		t.Errorf("Recovery.NearestAncestor = %q", err.Recovery.NearestAncestor)
	}
	if err.Recovery.AncestorStatus != 200 {
		t.Errorf("Recovery.AncestorStatus = %d", err.Recovery.AncestorStatus)
	}
}

func TestFetchErrorWithoutRecovery(t *testing.T) {
	inner := fmt.Errorf("HTTP 500")
	err := &pipeline.FetchError{
		URL: "https://example.com",
		Err:  inner,
	}

	if err.Recovery != nil {
		t.Errorf("expected nil Recovery, got %+v", err.Recovery)
	}
}

func TestFetchErrorRecoveryJSON(t *testing.T) {
	fe := &pipeline.FetchError{
		URL: "https://example.com/a/b/c",
		Err:  fmt.Errorf("HTTP 404"),
		Recovery: &pipeline.RecoveryHints{
			NearestAncestor: "https://example.com/a/b/",
			AncestorStatus:  200,
		},
	}

	type jsonErr struct {
		URL     string `json:"url"`
		Recovery *struct {
			NearestAncestor string `json:"nearest_ancestor"`
			AncestorStatus  int    `json:"ancestor_status"`
		} `json:"recovery,omitempty"`
	}

	j := jsonErr{
		URL: fe.URL,
	}
	if fe.Recovery != nil {
		j.Recovery = &struct {
			NearestAncestor string `json:"nearest_ancestor"`
			AncestorStatus  int    `json:"ancestor_status"`
		}{
			NearestAncestor: fe.Recovery.NearestAncestor,
			AncestorStatus:  fe.Recovery.AncestorStatus,
		}
	}

	data, marshalErr := json.Marshal(j)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}

	var out jsonErr
	if unmarshalErr := json.Unmarshal(data, &out); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if out.Recovery == nil {
		t.Fatal("expected non-nil Recovery after JSON round-trip")
	}
	if out.Recovery.NearestAncestor != "https://example.com/a/b/" {
		t.Errorf("NearestAncestor = %q after round-trip", out.Recovery.NearestAncestor)
	}
	if out.Recovery.AncestorStatus != 200 {
		t.Errorf("AncestorStatus = %d after round-trip", out.Recovery.AncestorStatus)
	}
}

func TestFetchErrorRecoveryUnwrap(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	err := &pipeline.FetchError{
		URL: "https://example.com",
		Err:  inner,
		Recovery: &pipeline.RecoveryHints{
			NearestAncestor: "https://example.com/",
			AncestorStatus:  200,
		},
	}

	if !errors.Is(err, inner) {
		t.Error("errors.Is should find inner error even with Recovery set")
	}

	var target *pipeline.FetchError
	if !errors.As(err, &target) {
		t.Error("errors.As should find *FetchError")
	}
	if target.Recovery == nil {
		t.Fatal("Recovery should survive errors.As")
	}
}
