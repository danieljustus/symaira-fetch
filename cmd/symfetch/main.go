package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/danieljustus/symaira-fetch/internal/config"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/mcp"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

var version = "0.1.0-dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		flagFormat      string
		flagRaw         bool
		flagProfile     string
		flagProxy       string
		flagTimeout     string
		flagMaxChars    int
		flagLinks       bool
		flagSession     string
		flagNoCache     bool
		flagCacheTTL    string
		flagHeaders     []string
		flagMethod      string
		flagData        string
		flagConcurrency int
		flagAllowPriv   bool
	)

	root := &cobra.Command{
		Use:   "symfetch [url...]",
		Short: "AI-native web fetch engine for LLM agents",
		Long: `symfetch fetches web pages using browser-impersonating TLS and
returns LLM-optimized Markdown, JSON, or plain text.

Multiple URLs are fetched sequentially; output is separated by --- delimiters
in Markdown mode, or as a JSON array in --format json mode.`,
		Args:          cobra.MinimumNArgs(0),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}

			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: config error: %v\n", err)
				cfg = config.Defaults()
			}

			// Flag overrides config
			profile := cfg.HTTP.Profile
			if cmd.Flags().Changed("profile") {
				profile = flagProfile
			}
			proxy := cfg.HTTP.Proxy
			if cmd.Flags().Changed("proxy") {
				proxy = flagProxy
			}
			format := cfg.HTTP.DefaultFormat
			if cmd.Flags().Changed("format") {
				format = flagFormat
			}
			maxChars := cfg.HTTP.MaxChars
			if cmd.Flags().Changed("max-chars") {
				maxChars = flagMaxChars
			}

			noCache := flagNoCache
			cacheTTL, err := time.ParseDuration(flagCacheTTL)
			if err != nil {
				return fmt.Errorf("invalid cache-ttl: %w", err)
			}

			timeoutSec := cfg.HTTP.TimeoutSeconds
			if cmd.Flags().Changed("timeout") {
				d, err := time.ParseDuration(flagTimeout)
				if err != nil {
					return fmt.Errorf("invalid timeout: %w", err)
				}
				timeoutSec = int(d.Seconds())
			}

			extraHeaders := parseHeaders(flagHeaders)

			p := fetch.ParseProfile(profile)
			client, err := fetch.New(p,
				fetch.WithProxy(proxy),
				fetch.WithTimeout(timeoutSec),
				fetch.WithMaxBody(cfg.HTTP.MaxBodyMB),
			)
			if err != nil {
				return fmt.Errorf("init client: %w", err)
			}
			defer client.Close()

			eng := pipeline.StaticEngine{}
			opts := pipeline.Options{
				Format:       pipeline.ParseFormat(format),
				MaxChars:     maxChars,
				IncludeLinks: flagLinks,
				NoCache:      noCache,
				CacheTTL:     cacheTTL,
				Profile:      profile,
				Session:      flagSession,
			}

			ctx := context.Background()
			allowPrivate := flagAllowPriv || cfg.Security.AllowPrivate

			if flagRaw {
				return runRaw(ctx, client, args, flagMethod, extraHeaders, flagData, allowPrivate)
			}

			if opts.Format == pipeline.FormatJSON && len(args) > 1 {
				return runMultiJSON(ctx, client, eng, args, opts)
			}

			for i, rawURL := range args {
				if i > 0 {
					fmt.Print("\n---\n\n")
				}
				res, err := pipeline.Run(ctx, client, eng, rawURL, opts)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", rawURL, err)
					continue
				}
				if opts.Format == pipeline.FormatMarkdown {
					printMarkdownResult(res)
				} else {
					fmt.Print(res.Output)
					if !strings.HasSuffix(res.Output, "\n") {
						fmt.Println()
					}
				}
			}
			return nil
		},
	}

	root.Flags().StringVarP(&flagFormat, "format", "f", "markdown", "Output format: markdown, json, text, html")
	root.Flags().BoolVar(&flagRaw, "raw", false, "Return raw decoded response body without semantic processing")
	root.Flags().StringVar(&flagProfile, "profile", "chrome", "Browser profile: chrome, firefox, honest")
	root.Flags().StringVar(&flagProxy, "proxy", "", "Proxy URL (http/https/socks5)")
	root.Flags().StringVar(&flagTimeout, "timeout", "30s", "Request timeout (e.g. 30s, 1m)")
	root.Flags().IntVar(&flagMaxChars, "max-chars", 20000, "Maximum characters in semantic output")
	root.Flags().BoolVar(&flagLinks, "links", false, "Append Links section with all hrefs")
	root.Flags().StringVar(&flagSession, "session", "", "Named persistent cookie jar")
	root.Flags().BoolVar(&flagNoCache, "no-cache", false, "Disable response caching")
	root.Flags().StringVar(&flagCacheTTL, "cache-ttl", "15m", "Cache TTL (e.g. 15m, 1h)")
	root.Flags().StringArrayVarP(&flagHeaders, "header", "H", nil, "Extra request header (\"Key: Value\")")
	root.Flags().StringVarP(&flagMethod, "request", "X", "GET", "HTTP method")
	root.Flags().StringVar(&flagData, "data", "", "Request body data")
	root.Flags().IntVar(&flagConcurrency, "concurrency", 4, "Parallel fetch workers for multiple URLs")
	root.Flags().BoolVar(&flagAllowPriv, "allow-private", false, "Allow fetching private/loopback addresses")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newConfigCmd())

	return root
}

func runRaw(ctx context.Context, client fetch.Client, urls []string, method string, headers map[string]string, data string, allowPrivate bool) error {
	for _, rawURL := range urls {
		req := fetch.Request{
			URL:          rawURL,
			Method:       strings.ToUpper(method),
			Headers:      headers,
			AllowPrivate: allowPrivate,
		}
		if data != "" {
			req.Body = []byte(data)
		}
		resp, err := client.Fetch(ctx, req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", rawURL, err)
			continue
		}
		fmt.Print(string(resp.Body))
	}
	return nil
}

func runMultiJSON(ctx context.Context, client fetch.Client, eng pipeline.Engine, urls []string, opts pipeline.Options) error {
	type jsonOut struct {
		URL    string `json:"url"`
		OK     bool   `json:"ok"`
		Output string `json:"output,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	results := make([]jsonOut, 0, len(urls))
	for _, rawURL := range urls {
		res, err := pipeline.Run(ctx, client, eng, rawURL, opts)
		if err != nil {
			results = append(results, jsonOut{URL: rawURL, OK: false, Error: err.Error()})
		} else {
			results = append(results, jsonOut{URL: rawURL, OK: true, Output: res.Output})
		}
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printMarkdownResult(res *pipeline.Result) {
	m := res.Meta
	fmt.Printf("> **%s** · %d · ~%d tokens", m.Title, m.StatusCode, m.EstTokens)
	if m.Truncated {
		fmt.Print(" · ⚠ truncated")
	}
	fmt.Println()
	fmt.Printf("> %s\n\n", m.FinalURL)
	fmt.Print(res.Output)
	if !strings.HasSuffix(res.Output, "\n") {
		fmt.Println()
	}
}

func parseHeaders(raw []string) map[string]string {
	m := make(map[string]string, len(raw))
	for _, h := range raw {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return m
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("symfetch", version)
		},
	}
}

func newMCPCmd() *cobra.Command {
	var flagProfile string
	var flagProxy string

	cmd := &cobra.Command{
		Use:          "mcp",
		Aliases:      []string{"serve"},
		Short:        "Start the MCP stdio server",
		Long:         "Start a JSON-RPC 2.0 MCP server over stdin/stdout for use with AI agents.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			mcp.ServerVersion = version
			p := fetch.ParseProfile(flagProfile)
			return mcp.StartServer(p, flagProxy)
		},
	}

	cmd.Flags().StringVar(&flagProfile, "profile", "chrome", "Browser profile: chrome, firefox, honest")
	cmd.Flags().StringVar(&flagProxy, "proxy", "", "Proxy URL")
	return cmd
}

func newConfigCmd() *cobra.Command {
	cfg := &cobra.Command{
		Use:   "config",
		Short: "Manage symfetch configuration",
	}

	initCmd := &cobra.Command{
		Use:          "init",
		Short:        "Write default config to ~/.config/symfetch/config.toml",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			dir := home + "/.config/symfetch"
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			path := dir + "/config.toml"
			if _, err := os.Stat(path); err == nil {
				fmt.Fprintf(os.Stderr, "config already exists at %s\n", path)
				return nil
			}
			if err := os.WriteFile(path, []byte(config.DefaultConfigTOML()), 0644); err != nil {
				return err
			}
			fmt.Printf("Config written to %s\n", path)
			return nil
		},
	}

	cfg.AddCommand(initCmd)
	return cfg
}
