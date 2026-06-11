package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

const jsonRPCMarshalFailureFrame = `{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal error: failed to marshal response"}}`

// mcpOut is the writer for JSON-RPC output. Defaults to os.Stdout; overridden in tests.
var mcpOut io.Writer = os.Stdout

// ServerVersion is reported in the MCP initialize handshake.
// Set from main before starting the server.
var ServerVersion = "dev"

// JSONRPCRequest represents an incoming JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents an outgoing JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// StartServer starts the MCP server over stdio.
// All logging goes to stderr; only JSON-RPC frames go to stdout.
func StartServer(profile fetch.Profile, proxy string) error {
	client, err := fetch.New(profile, fetch.WithProxy(proxy))
	if err != nil {
		return fmt.Errorf("init fetch client: %w", err)
	}
	defer client.Close()
	eng := pipeline.StaticEngine{}
	return ServeIO(os.Stdin, os.Stdout, client, eng)
}

// ServeIO runs the MCP JSON-RPC loop reading from r and writing to w.
// Exposed for testing; use StartServer for production.
func ServeIO(r io.Reader, w io.Writer, client fetch.Client, eng pipeline.Engine) error {
	mcpOut = w
	defer func() { mcpOut = os.Stdout }()

	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("error reading stdin: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			sendError(nil, -32700, "Parse error: "+err.Error())
			continue
		}

		handleRequest(&req, client, eng)
	}
}

func handleRequest(req *JSONRPCRequest, client fetch.Client, eng pipeline.Engine) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("MCP handler panicked", "panic", r)
			sendError(req.ID, -32603, "Internal error: handler panicked")
		}
	}()

	switch req.Method {
	case "initialize":
		sendResponse(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]string{
				"name":    "symfetch",
				"version": ServerVersion,
			},
		})

	case "notifications/initialized":
		// No-op

	case "tools/list":
		sendResponse(req.ID, map[string]interface{}{
			"tools": toolDefinitions(),
		})

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			sendError(req.ID, -32602, "Invalid params: "+err.Error())
			return
		}
		handleToolCall(req.ID, params.Name, params.Arguments, client, eng)

	default:
		sendError(req.ID, -32601, "Method not found: "+req.Method)
	}
}

func sendResponse(id interface{}, result interface{}) {
	writeFrame(JSONRPCResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func sendToolResponse(id interface{}, text string) {
	sendResponse(id, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
		"isError": false,
	})
}

func sendToolError(id interface{}, text string) {
	sendResponse(id, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
		"isError": true,
	})
}

func sendError(id interface{}, code int, message string) {
	writeFrame(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   map[string]interface{}{"code": code, "message": message},
	})
}

func writeFrame(resp JSONRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("mcp: failed to marshal JSON-RPC response", "err", err)
		mcpOut.Write([]byte(jsonRPCMarshalFailureFrame + "\n"))
		return
	}
	mcpOut.Write(append(data, '\n'))
}
