package mcp

import (
	"strings"
	"testing"
)

// ---- intArg ----

func TestIntArg(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]interface{}
		key     string
		wantVal int
		wantOK  bool
		wantErr string // empty means no error
	}{
		{
			name:    "absent key",
			args:    map[string]interface{}{},
			key:     "timeout_seconds",
			wantVal: 0,
			wantOK:  false,
			wantErr: "",
		},
		{
			name:    "canonical JSON number",
			args:    map[string]interface{}{"max_chars": float64(5000)},
			key:     "max_chars",
			wantVal: 5000,
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "zero",
			args:    map[string]interface{}{"max_chars": float64(0)},
			key:     "max_chars",
			wantVal: 0,
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "negative value",
			args:    map[string]interface{}{"max_chars": float64(-1)},
			key:     "max_chars",
			wantVal: -1,
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "stringified integer",
			args:    map[string]interface{}{"timeout_seconds": "30"},
			key:     "timeout_seconds",
			wantVal: 30,
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "stringified zero",
			args:    map[string]interface{}{"limit": "0"},
			key:     "limit",
			wantVal: 0,
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "stringified negative",
			args:    map[string]interface{}{"top_k": "-5"},
			key:     "top_k",
			wantVal: -5,
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "malformed string",
			args:    map[string]interface{}{"max_chars": "not-a-number"},
			key:     "max_chars",
			wantVal: 0,
			wantOK:  true,
			wantErr: `invalid argument "max_chars": not a valid integer (got not-a-number)`,
		},
		{
			name:    "empty string",
			args:    map[string]interface{}{"max_chars": ""},
			key:     "max_chars",
			wantVal: 0,
			wantOK:  true,
			wantErr: `invalid argument "max_chars": not a valid integer (got )`,
		},
		{
			name:    "float string",
			args:    map[string]interface{}{"timeout_seconds": "3.14"},
			key:     "timeout_seconds",
			wantVal: 0,
			wantOK:  true,
			wantErr: `invalid argument "timeout_seconds": not a valid integer (got 3.14)`,
		},
		{
			name:    "boolean value",
			args:    map[string]interface{}{"max_chars": true},
			key:     "max_chars",
			wantVal: 0,
			wantOK:  true,
			wantErr: `invalid argument "max_chars": unsupported type bool (expected number) (got true)`,
		},
		{
			name:    "nil value",
			args:    map[string]interface{}{"max_chars": nil},
			key:     "max_chars",
			wantVal: 0,
			wantOK:  true,
			wantErr: `invalid argument "max_chars": unsupported type <nil> (expected number) (got <nil>)`,
		},
		{
			name:    "array value",
			args:    map[string]interface{}{"max_chars": []int{1}},
			key:     "max_chars",
			wantVal: 0,
			wantOK:  true,
			wantErr: `invalid argument "max_chars": unsupported type []int (expected number) (got [1])`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOK, err := intArg(tt.args, tt.key)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if err.Error() != tt.wantErr {
					t.Errorf("error message:\ngot:  %q\nwant: %q", err.Error(), tt.wantErr)
				}
				var argErr *ArgError
				if !isArgError(err) {
					t.Errorf("expected *ArgError, got %T", err)
				}
				_ = argErr // avoid unused lint
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if gotVal != tt.wantVal {
				t.Errorf("val: got %d, want %d", gotVal, tt.wantVal)
			}
			if gotOK != tt.wantOK {
				t.Errorf("ok: got %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

// ---- boolArg ----

func TestBoolArg(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]interface{}
		key     string
		wantVal bool
		wantOK  bool
		wantErr string
	}{
		{
			name:    "absent key",
			args:    map[string]interface{}{},
			key:     "raw",
			wantVal: false,
			wantOK:  false,
			wantErr: "",
		},
		{
			name:    "canonical true",
			args:    map[string]interface{}{"raw": true},
			key:     "raw",
			wantVal: true,
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "canonical false",
			args:    map[string]interface{}{"include_links": false},
			key:     "include_links",
			wantVal: false,
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "stringified true",
			args:    map[string]interface{}{"frontmatter": "true"},
			key:     "frontmatter",
			wantVal: true,
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "stringified false",
			args:    map[string]interface{}{"store_full_text": "false"},
			key:     "store_full_text",
			wantVal: false,
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "uppercase TRUE",
			args:    map[string]interface{}{"raw": "TRUE"},
			key:     "raw",
			wantVal: false,
			wantOK:  true,
			wantErr: `invalid argument "raw": not a valid boolean (expected "true" or "false") (got TRUE)`,
		},
		{
			name:    "mixed case True",
			args:    map[string]interface{}{"raw": "True"},
			key:     "raw",
			wantVal: false,
			wantOK:  true,
			wantErr: `invalid argument "raw": not a valid boolean (expected "true" or "false") (got True)`,
		},
		{
			name:    "yes string",
			args:    map[string]interface{}{"include_links": "yes"},
			key:     "include_links",
			wantVal: false,
			wantOK:  true,
			wantErr: `invalid argument "include_links": not a valid boolean (expected "true" or "false") (got yes)`,
		},
		{
			name:    "number 1",
			args:    map[string]interface{}{"raw": "1"},
			key:     "raw",
			wantVal: false,
			wantOK:  true,
			wantErr: `invalid argument "raw": not a valid boolean (expected "true" or "false") (got 1)`,
		},
		{
			name:    "number 0",
			args:    map[string]interface{}{"raw": "0"},
			key:     "raw",
			wantVal: false,
			wantOK:  true,
			wantErr: `invalid argument "raw": not a valid boolean (expected "true" or "false") (got 0)`,
		},
		{
			name:    "float64 type",
			args:    map[string]interface{}{"raw": float64(1)},
			key:     "raw",
			wantVal: false,
			wantOK:  true,
			wantErr: `invalid argument "raw": unsupported type float64 (expected boolean) (got 1)`,
		},
		{
			name:    "nil value",
			args:    map[string]interface{}{"raw": nil},
			key:     "raw",
			wantVal: false,
			wantOK:  true,
			wantErr: `invalid argument "raw": unsupported type <nil> (expected boolean) (got <nil>)`,
		},
		{
			name:    "empty string",
			args:    map[string]interface{}{"wayback_fallback": ""},
			key:     "wayback_fallback",
			wantVal: false,
			wantOK:  true,
			wantErr: `invalid argument "wayback_fallback": not a valid boolean (expected "true" or "false") (got )`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOK, err := boolArg(tt.args, tt.key)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if err.Error() != tt.wantErr {
					t.Errorf("error message:\ngot:  %q\nwant: %q", err.Error(), tt.wantErr)
				}
				if !isArgError(err) {
					t.Errorf("expected *ArgError, got %T", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if gotVal != tt.wantVal {
				t.Errorf("val: got %v, want %v", gotVal, tt.wantVal)
			}
			if gotOK != tt.wantOK {
				t.Errorf("ok: got %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

// ---- stringArg ----

func TestStringArg(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]interface{}
		key     string
		wantVal string
		wantOK  bool
		wantErr string
	}{
		{
			name:    "absent key",
			args:    map[string]interface{}{},
			key:     "format",
			wantVal: "",
			wantOK:  false,
			wantErr: "",
		},
		{
			name:    "string value",
			args:    map[string]interface{}{"format": "markdown"},
			key:     "format",
			wantVal: "markdown",
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "empty string",
			args:    map[string]interface{}{"css_selector": ""},
			key:     "css_selector",
			wantVal: "",
			wantOK:  true,
			wantErr: "",
		},
		{
			name:    "float64 type",
			args:    map[string]interface{}{"format": float64(3)},
			key:     "format",
			wantVal: "",
			wantOK:  true,
			wantErr: `invalid argument "format": unsupported type float64 (expected string) (got 3)`,
		},
		{
			name:    "bool type",
			args:    map[string]interface{}{"css_selector": true},
			key:     "css_selector",
			wantVal: "",
			wantOK:  true,
			wantErr: `invalid argument "css_selector": unsupported type bool (expected string) (got true)`,
		},
		{
			name:    "nil value",
			args:    map[string]interface{}{"query": nil},
			key:     "query",
			wantVal: "",
			wantOK:  true,
			wantErr: `invalid argument "query": unsupported type <nil> (expected string) (got <nil>)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOK, err := stringArg(tt.args, tt.key)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if err.Error() != tt.wantErr {
					t.Errorf("error message:\ngot:  %q\nwant: %q", err.Error(), tt.wantErr)
				}
				if !isArgError(err) {
					t.Errorf("expected *ArgError, got %T", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if gotVal != tt.wantVal {
				t.Errorf("val: got %q, want %q", gotVal, tt.wantVal)
			}
			if gotOK != tt.wantOK {
				t.Errorf("ok: got %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

// ---- ArgError ----

func TestArgErrorMessage(t *testing.T) {
	err := &ArgError{Field: "max_chars", Value: "abc", Reason: "not a valid integer"}
	msg := err.Error()
	if !strings.Contains(msg, "max_chars") {
		t.Errorf("expected field name in error, got: %s", msg)
	}
	if !strings.Contains(msg, "not a valid integer") {
		t.Errorf("expected reason in error, got: %s", msg)
	}
	if !strings.Contains(msg, "abc") {
		t.Errorf("expected value in error, got: %s", msg)
	}
}

// ---- helpers ----

// isArgError reports whether err is an *ArgError.
func isArgError(err error) bool {
	_, ok := err.(*ArgError)
	return ok
}
