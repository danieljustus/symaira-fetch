package render

import (
	"encoding/json"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
)

// JSON serialises the Document as pretty-printed JSON.
func JSON(doc *agentdom.Document) (string, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
