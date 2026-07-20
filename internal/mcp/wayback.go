package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/danieljustus/symaira-corekit/mcpserver"
	"github.com/danieljustus/symaira-fetch/internal/apicommon"
	"github.com/danieljustus/symaira-fetch/internal/archive"
)

var CdxBaseURL = ""

func registerWaybackTools(srv *mcpserver.Server) {
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "wayback_snapshots",
		Description: "List available Wayback Machine snapshots for a URL. Returns timestamps, HTTP status codes, and MIME types for each captured version. Useful for finding historical versions of a page or checking if a page has been archived.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"url": {"type": "string", "description": "The URL to look up in the Wayback Machine"},
				"from": {"type": "string", "description": "Start date filter (format: YYYYMMDD or YYYYMMDDHHmmss)"},
				"to": {"type": "string", "description": "End date filter (format: YYYYMMDD or YYYYMMDDHHmmss)"},
				"limit": {"type": "integer", "description": "Maximum number of snapshots to return (default: 100)"},
				"match_type": {"type": "string", "description": "URL matching mode: exact (default), prefix, or host", "enum": ["exact", "prefix", "host"]}
			},
			"required": ["url"]
		}`),
		Handler: makeWaybackSnapshotsHandler(),
	})
}

func makeWaybackSnapshotsHandler() func(ctx context.Context, input json.RawMessage) (any, error) {
	return func(ctx context.Context, input json.RawMessage) (any, error) {
		var args map[string]interface{}
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, fmt.Errorf("invalid input: %w", err)
		}

		rawURL, ok := args["url"].(string)
		if !ok || rawURL == "" {
			return nil, fmt.Errorf("missing required argument 'url'")
		}

		if err := apicommon.ValidateURLScheme(rawURL); err != nil {
			return nil, err
		}

		query := archive.CDXQuery{URL: rawURL}

		if from, ok := args["from"].(string); ok {
			query.From = from
		}
		if to, ok := args["to"].(string); ok {
			query.To = to
		}
		if v, ok := args["limit"].(float64); ok && v > 0 {
			query.Limit = int(v)
		}
		if mt, ok := args["match_type"].(string); ok {
			query.MatchType = mt
		}

		client := archive.NewCDXClient(CdxBaseURL, nil)
		snaps, err := client.Lookup(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("[wayback] %w", err)
		}

		type snapResult struct {
			Timestamp string `json:"timestamp"`
			URL       string `json:"url"`
			Status    string `json:"status"`
			MimeType  string `json:"mime_type"`
			Digest    string `json:"digest"`
		}

		results := make([]snapResult, len(snaps))
		for i, s := range snaps {
			results[i] = snapResult{
				Timestamp: s.Timestamp,
				URL:       s.Original,
				Status:    s.StatusCode,
				MimeType:  s.MimeType,
				Digest:    s.Digest,
			}
		}

		if len(results) == 0 {
			return "No snapshots found for this URL.", nil
		}

		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return nil, err
		}
		return string(data), nil
	}
}
