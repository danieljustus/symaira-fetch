package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/danieljustus/symaira-fetch/internal/archive"
)

var cdxBaseURL = ""

func newSnapshotsCmd() *cobra.Command {
	var (
		flagFrom      string
		flagTo        string
		flagLimit     int
		flagMatchType string
		flagJSON      bool
	)

	cmd := &cobra.Command{
		Use:   "snapshots <url>",
		Short: "List Wayback Machine snapshots for a URL",
		Long: `Query the Wayback Machine CDX API to list available snapshots for a URL.
Returns timestamps, HTTP status codes, and MIME types for each captured version.

Examples:
  symfetch snapshots https://example.com
  symfetch snapshots https://example.com --from 20240101 --to 20241231
  symfetch snapshots https://example.com --limit 10 --json`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			rawURL := args[0]

			if err := validateURLScheme(rawURL); err != nil {
				return err
			}

			query := archive.CDXQuery{
				URL:  rawURL,
				From: flagFrom,
				To:   flagTo,
			}
			if flagLimit > 0 {
				query.Limit = flagLimit
			}
			if flagMatchType != "" {
				query.MatchType = flagMatchType
			}

			client := archive.NewCDXClient(cdxBaseURL, nil)
			ctx := context.Background()

			snaps, err := client.Lookup(ctx, query)
			if err != nil {
				return fmt.Errorf("cdx lookup failed: %w", err)
			}

			if flagJSON {
				return printSnapshotsJSON(snaps)
			}
			printSnapshotsTable(snaps)
			return nil
		},
	}

	cmd.Flags().StringVar(&flagFrom, "from", "", "Start date filter (YYYYMMDD or YYYYMMDDHHmmss)")
	cmd.Flags().StringVar(&flagTo, "to", "", "End date filter (YYYYMMDD or YYYYMMDDHHmmss)")
	cmd.Flags().IntVarP(&flagLimit, "limit", "n", 100, "Maximum number of snapshots to return")
	cmd.Flags().StringVar(&flagMatchType, "match-type", "exact", "URL matching mode: exact, prefix, host")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "Output as JSON")

	return cmd
}

func printSnapshotsTable(snaps []archive.Snapshot) {
	if len(snaps) == 0 {
		fmt.Println("No snapshots found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIMESTAMP\tSTATUS\tMIME TYPE\tSIZE")
	for _, s := range snaps {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			s.Timestamp,
			s.StatusCode,
			s.MimeType,
			formatBytes(s.Length),
		)
	}
	w.Flush()
	fmt.Printf("\n%d snapshot(s) found.\n", len(snaps))
}

func printSnapshotsJSON(snaps []archive.Snapshot) error {
	type snapJSON struct {
		Timestamp  string `json:"timestamp"`
		URL        string `json:"url"`
		Status     string `json:"status"`
		MimeType   string `json:"mime_type"`
		Digest     string `json:"digest"`
		ByteLength string `json:"byte_length"`
	}

	results := make([]snapJSON, len(snaps))
	for i, s := range snaps {
		results[i] = snapJSON{
			Timestamp:  s.Timestamp,
			URL:        s.Original,
			Status:     s.StatusCode,
			MimeType:   s.MimeType,
			Digest:     s.Digest,
			ByteLength: s.Length,
		}
	}

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func validateURLScheme(rawURL string) error {
	// Minimal scheme check — reuses pattern from internal/mcp/tools.go.
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return nil
	}
	return fmt.Errorf("unsupported scheme: only http and https are allowed")
}

func formatBytes(s string) string {
	// Simple size formatting for display.
	if s == "" || s == "0" {
		return "-"
	}
	return s + " B"
}
