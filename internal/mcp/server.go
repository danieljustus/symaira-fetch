package mcp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/danieljustus/symaira-corekit/mcpserver"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

// ServerVersion is reported in the MCP initialize handshake.
// Set from main before starting the server.
var ServerVersion = "dev"

// StartServer starts the MCP server over stdio with graceful shutdown.
// All logging goes to stderr; only JSON-RPC frames go to stdout.
func StartServer(profile fetch.Profile, proxy string) error {
	client, err := fetch.New(profile, fetch.WithProxy(proxy))
	if err != nil {
		return fmt.Errorf("init fetch client: %w", err)
	}
	defer client.Close()
	eng := pipeline.StaticEngine{}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := mcpserver.New("symfetch", ServerVersion)
	registerTools(srv, client, eng)
	return srv.ServeStdio(ctx)
}

// ServeIO runs the MCP JSON-RPC loop reading from r and writing to w.
// Exposed for testing; use StartServer for production.
func ServeIO(ctx context.Context, r io.Reader, w io.Writer, client fetch.Client, eng pipeline.Engine) error {
	srv := mcpserver.New("symfetch", "test")
	registerTools(srv, client, eng)
	return srv.ServeIO(ctx, r, w)
}
