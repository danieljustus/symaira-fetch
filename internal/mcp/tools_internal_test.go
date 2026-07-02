package mcp

import (
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

func TestFormatWithMeta_NilDoc_NoPanic(t *testing.T) {
	res := &pipeline.Result{
		Output: "cached output",
		Doc:    nil,
		Meta: agentdom.Meta{
			FinalURL:   "https://example.com",
			StatusCode: 200,
			Title:      "Test",
			EstTokens:  10,
		},
	}

	var panicVal interface{}
	func() {
		defer func() { panicVal = recover() }()
		_ = formatWithMeta(res, pipeline.FormatMarkdown, true)
	}()

	if panicVal != nil {
		t.Fatalf("formatWithMeta panicked with nil Doc: %v", panicVal)
	}
}
