package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
)

func QuerySchema(islands []agentdom.DataIsland, schemaPath string) (string, error) {
	if !strings.HasPrefix(schemaPath, "@") {
		return "", fmt.Errorf("schema path must start with @, e.g., @Recipe:name")
	}

	parts := strings.SplitN(schemaPath[1:], ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("schema path must have format @Type:path, e.g., @Recipe:name")
	}

	schemaType := parts[0]
	fieldPath := parts[1]

	var targetJSON []byte
	for _, island := range islands {
		if island.Source != "ld+json" {
			continue
		}

		data, err := unmarshalIsland(island.JSON)
		if err != nil {
			continue
		}

		if matchesType(data, schemaType) {
			targetJSON = island.JSON
			break
		}
	}

	if targetJSON == nil {
		return "", fmt.Errorf("no JSON-LD with @type %q found", schemaType)
	}

	return traversePath(targetJSON, fieldPath)
}

func unmarshalIsland(data json.RawMessage) (map[string]interface{}, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func matchesType(data map[string]interface{}, schemaType string) bool {
	if t, ok := data["@type"].(string); ok {
		if strings.EqualFold(t, schemaType) {
			return true
		}
	}

	if t, ok := data["type"].(string); ok {
		if strings.EqualFold(t, schemaType) {
			return true
		}
	}

	if graph, ok := data["@graph"].([]interface{}); ok {
		for _, item := range graph {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["@type"].(string); ok {
					if strings.EqualFold(t, schemaType) {
						return true
					}
				}
			}
		}
	}

	return false
}

func traversePath(data json.RawMessage, fieldPath string) (string, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", fmt.Errorf("failed to parse JSON-LD: %w", err)
	}

	parts := strings.Split(fieldPath, ".")
	var current interface{} = raw

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return "", fmt.Errorf("field %q not found", part)
			}
			current = val
		case []interface{}:
			return "", fmt.Errorf("cannot traverse into array with field %q", part)
		default:
			return "", fmt.Errorf("cannot traverse into %T", current)
		}
	}

	result, err := json.Marshal(current)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(result), nil
}
