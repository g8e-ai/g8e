// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/spf13/cobra"
)

const (
	l3ApprovalMaxIterations = 30
	l3ApprovalPollInterval  = 10 * time.Second
	l3ApprovalTotalTimeout  = 5 * time.Minute
)

// mcpCmd implements the MCP stdio transport mode
func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP protocol operations (stdio transport)",
		Long:  `Run g8e as an MCP server using stdio transport for local agent integration. Exposes all native tools without requiring gateway mode.`,
	}

	cmd.AddCommand(
		mcpStdioCmd(),
		mcpStdioProxyCmd(),
		agentCmd(),
	)

	return cmd
}

func mcpStdioCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stdio",
		Short: "Run MCP stdio server with native tools only",
		Long:  `Run g8e as an MCP server using stdio transport for local agent integration. Exposes all native tools without requiring gateway mode.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPStdio(cmd, args)
		},
	}

	return cmd
}

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ToolsListResult represents the result of tools/list
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// CallToolRequest is the params for the "tools/call" method
type CallToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// runMCPStdio implements the MCP stdio transport
func runMCPStdio(cmd *cobra.Command, args []string) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	logger.Info("g8e MCP stdio server starting")

	// Initialize native tool handler
	nativeToolHandler, err := mcp.NewNativeToolHandler(logger)
	if err != nil {
		return fmt.Errorf("initialize native tool handler: %w", err)
	}

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			logger.Error("Failed to parse JSON-RPC request", "error", err)
			sendError(encoder, nil, -32700, "parse error")
			continue
		}

		logger.Info("Received MCP request", "method", req.Method, "id", req.ID)

		switch req.Method {
		case "tools/list":
			handleToolsList(encoder, req.ID, nativeToolHandler)
		case "tools/call":
			handleToolsCall(encoder, req.ID, req.Params, nativeToolHandler)
		case "initialize":
			handleInitialize(encoder, req.ID)
		case "ping":
			sendSuccess(encoder, req.ID, struct{}{})
		default:
			logger.Warn("Unknown MCP method", "method", req.Method)
			sendError(encoder, req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Error reading stdin", "error", err)
		return err
	}

	logger.Info("g8e MCP stdio server shutting down")
	return nil
}

func handleToolsList(encoder *json.Encoder, id interface{}, nativeToolHandler *mcp.NativeToolHandler) {
	nativeTools := nativeToolHandler.ListTools()
	tools := make([]Tool, 0, len(nativeTools))
	for _, nt := range nativeTools {
		tools = append(tools, Tool{
			Name:        nt.Name(),
			Description: nt.Description(),
			InputSchema: nt.InputSchema().ToMap(),
		})
	}

	result := ToolsListResult{
		Tools: tools,
	}

	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode tools/list response", "error", err)
	}
}

func handleToolsCall(encoder *json.Encoder, id interface{}, params json.RawMessage, nativeToolHandler *mcp.NativeToolHandler) {
	var callParams CallToolRequest
	if err := json.Unmarshal(params, &callParams); err != nil {
		sendError(encoder, id, -32600, fmt.Sprintf("invalid tools/call params: %v", err))
		return
	}

	if callParams.Name == "" {
		sendError(encoder, id, -32600, "tool name required")
		return
	}

	// Execute native tool
	result, err := nativeToolHandler.HandleTool(context.Background(), callParams.Name, callParams.Arguments)
	if err != nil {
		sendError(encoder, id, -32603, fmt.Sprintf("tool execution failed: %v", err))
		return
	}

	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode tools/call response", "error", err)
	}
}

func handleInitialize(encoder *json.Encoder, id interface{}) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "g8e",
				"version": "1.0.0",
			},
		},
	}

	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode initialize response", "error", err)
	}
}

func sendError(encoder *json.Encoder, id interface{}, code int, message string) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}

	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode error response", "error", err)
	}
}

func sendSuccess(encoder *json.Encoder, id interface{}, result interface{}) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode success response", "error", err)
	}
}

func mcpStdioProxyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gov",
		Short: "Proxy stdio MCP requests to the gateway HTTP endpoint",
		Long:  `Run as an MCP stdio server that proxies all requests to the running gateway's HTTP endpoint. This enables tools that only support stdio transport to use the full gateway governance layer.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPStdioProxy(cmd, args)
		},
	}

	return cmd
}

func runMCPStdioProxy(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if gateway is running via HTTP
	apiClient, err := api.NewClient(cfg)
	if err == nil {
		_, err = apiClient.Get("/api/v1/health")
		if err == nil {
			// Gateway is running
		} else {
			return fmt.Errorf("gateway is not running. Start it with: ./g8e gw start")
		}
	} else {
		// Fallback to ProcessManager check
		pm, err := platform.NewProcessManager(cfg.ProjectRoot)
		if err != nil {
			return fmt.Errorf("failed to create process manager: %w", err)
		}

		running, _, err := pm.OperatorStatus()
		if err != nil {
			return fmt.Errorf("failed to check Operator status: %w", err)
		}
		if !running {
			return fmt.Errorf("gateway is not running. Start it with: ./g8e gw start")
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	logger.Info("g8e MCP stdio proxy starting")

	client, err := createMCPClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create MCP client: %w", err)
	}

	gatewayURL := fmt.Sprintf("https://g8e.local:%d/mcp", constants.Ports.OperatorHttps)

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			logger.Error("Failed to parse JSON-RPC request", "error", err)
			sendError(encoder, nil, -32700, "parse error")
			continue
		}

		logger.Info("Received MCP request", "method", req.Method, "id", req.ID)

		resp, err := proxyToGatewayWithRetry(client, gatewayURL, req, logger)
		if err != nil {
			logger.Error("Failed to proxy to gateway", "error", err)
			sendError(encoder, req.ID, -32603, fmt.Sprintf("gateway proxy error: %v", err))
			continue
		}

		if err := encoder.Encode(resp); err != nil {
			logger.Error("Failed to encode response", "error", err)
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Error reading stdin", "error", err)
		return err
	}

	logger.Info("g8e MCP stdio proxy shutting down")
	return nil
}

func createMCPClient(cfg *config.Config) (*http.Client, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	caCert, err := os.ReadFile(cfg.TrustBundlePath())
	if err != nil {
		return nil, fmt.Errorf("failed to read CA bundle: %w", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
		ServerName:   "g8e.local",
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

func proxyToGateway(client *http.Client, gatewayURL string, req JSONRPCRequest) (JSONRPCResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return JSONRPCResponse{}, err
	}

	httpResp, err := client.Post(gatewayURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return JSONRPCResponse{}, err
	}
	defer httpResp.Body.Close()

	var resp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return JSONRPCResponse{}, err
	}

	return resp, nil
}

func proxyToGatewayWithRetry(client *http.Client, gatewayURL string, req JSONRPCRequest, logger *slog.Logger) (JSONRPCResponse, error) {
	resp, err := proxyToGateway(client, gatewayURL, req)
	if err != nil {
		return resp, err
	}

	if isL3ApprovalResponse(resp) {
		approvalURL := extractApprovalURL(resp)
		logger.Info("L3 approval required, waiting for user to authorize...", "url", approvalURL)

		if err := platform.OpenBrowser(approvalURL); err != nil {
			logger.Warn("Failed to auto-open browser", "error", err)
			fmt.Fprintf(os.Stderr, "\n[g8e] Please visit: %s\n", approvalURL)
		}

		for i := 0; i < l3ApprovalMaxIterations; i++ {
			time.Sleep(l3ApprovalPollInterval)

			retryResp, err := proxyToGateway(client, gatewayURL, req)
			if err != nil {
				continue
			}

			if !isL3ApprovalResponse(retryResp) {
				logger.Info("L3 approval completed, proceeding with execution")
				return retryResp, nil
			}
		}

		logger.Warn("L3 approval timeout, returning original response")
	}

	return resp, nil
}

func isL3ApprovalResponse(resp JSONRPCResponse) bool {
	if resp.Result == nil {
		return false
	}

	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return false
	}

	resultStr := string(resultBytes)
	return strings.Contains(resultStr, "Execution paused") &&
		strings.Contains(resultStr, "approve/")
}

func extractApprovalURL(resp JSONRPCResponse) string {
	if resp.Result == nil {
		return ""
	}

	// First try to extract approval_url from structured JSON response
	resultMap, ok := resp.Result.(map[string]interface{})
	if ok {
		if approvalURL, exists := resultMap["approval_url"]; exists {
			if urlStr, ok := approvalURL.(string); ok && urlStr != "" {
				return urlStr
			}
		}

		// Check if content array exists and extract URL from text content
		if content, exists := resultMap["content"]; exists {
			if contentArray, ok := content.([]interface{}); ok {
				for _, item := range contentArray {
					if itemMap, ok := item.(map[string]interface{}); ok {
						if text, exists := itemMap["text"]; exists {
							if textStr, ok := text.(string); ok {
								if url := extractURLFromText(textStr); url != "" {
									return url
								}
							}
						}
					}
				}
			}
		}
	}

	// Fallback: marshal and use regex extraction
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return ""
	}

	return extractURLFromText(string(resultBytes))
}

func printMCPConfigLocal(cmd *cobra.Command) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	externalIP := config.GetExternalInterfaceIP()
	cmd.Printf("# Add this entry to /etc/hosts to enable g8e.local resolution:\n")
	cmd.Printf("%s g8e.local\n\n", externalIP)

	// Use the canonical g8e.local internal hostname with unified /mcp endpoint
	gatewayURL := fmt.Sprintf("https://g8e.local:%d/mcp", constants.Ports.OperatorHttps)

	// Get actual resolved cert paths (absolute paths)
	actualCertPath := cfg.CLICertFile()
	actualKeyPath := cfg.CLIKeyFile()
	actualCAPath := cfg.TrustBundlePath()

	// Normalize to forward slashes for JSON (cross-platform compatibility)
	actualCertPath = filepath.ToSlash(actualCertPath)
	actualKeyPath = filepath.ToSlash(actualKeyPath)
	actualCAPath = filepath.ToSlash(actualCAPath)

	mcpConfig, err := mcp.NewGatewayConfig(gatewayURL, actualCertPath, actualKeyPath, actualCAPath)
	if err != nil {
		return fmt.Errorf("failed to create MCP config: %w", err)
	}

	configJSON, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MCP config: %w", err)
	}

	cmd.Println(string(configJSON))
	return nil
}

func printMCPConfigIP(cmd *cobra.Command) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Use the external IP address instead of g8e.local
	externalIP := config.GetExternalInterfaceIP()
	gatewayURL := fmt.Sprintf("https://%s:%d/mcp", externalIP, constants.Ports.OperatorHttps)

	// Get actual resolved cert paths (absolute paths)
	actualCertPath := cfg.CLICertFile()
	actualKeyPath := cfg.CLIKeyFile()
	actualCAPath := cfg.TrustBundlePath()

	// Normalize to forward slashes for JSON (cross-platform compatibility)
	actualCertPath = filepath.ToSlash(actualCertPath)
	actualKeyPath = filepath.ToSlash(actualKeyPath)
	actualCAPath = filepath.ToSlash(actualCAPath)

	// Use IP address as hostname for verification
	mcpConfig, err := mcp.NewGatewayConfigWithHostname(gatewayURL, actualCertPath, actualKeyPath, actualCAPath, externalIP)
	if err != nil {
		return fmt.Errorf("failed to create MCP config: %w", err)
	}

	configJSON, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MCP config: %w", err)
	}

	cmd.Println(string(configJSON))
	return nil
}

func printMCPConfigHTTP(cmd *cobra.Command) error {
	staticConfig := fmt.Sprintf(`{
  "mcpServers": {
    "g8e-gateway": {
      "disabled": true,
      "serverUrl": "http://127.0.0.1:%d/mcp",
      "note": "Must use explicit 127.0.0.1 for HTTP (localhost may resolve to IPv6 ::1)"
    }
  }
}`, constants.Ports.OperatorHttp)
	cmd.Println(staticConfig)
	return nil
}

func printMCPConfigStdio(cmd *cobra.Command) error {
	// Get the full path to the current binary
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get binary path: %w", err)
	}

	// Use the new simple config format
	mcpConfig, err := mcp.NewStdioConfigSimple(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to create MCP stdio config: %w", err)
	}

	configJSON, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MCP config: %w", err)
	}

	cmd.Println(string(configJSON))
	return nil
}

func agentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Agent integration commands for popular AI coding tools",
		Long:  `Configure and integrate g8e with popular AI agent binaries (Claude, Cursor, Windsurf, etc.) for seamless MCP tool access.`,
	}

	cmd.AddCommand(
		agentListCmd(),
		agentShowCmd(),
		agentRunCmd(),
	)

	return cmd
}

func agentListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List supported agent binaries",
		Long:  `List all popular AI agent binaries that g8e supports for MCP integration.`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("Supported Agent Binaries:")
			cmd.Println()
			for _, agent := range getSupportedAgents() {
				cmd.Printf("  %-13s - %s\n", agent.ID, agent.Description)
			}
			cmd.Println()
			cmd.Println("Use 'g8e mcp agent show <agent>' to show configuration for a specific agent.")
		},
	}

	return cmd
}

func agentShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <agent>",
		Short: "Print MCP client configuration for the Gateway",
		Long:  `Print MCP client configuration for connecting to the g8e Gateway from local coding tools. Displays configurations side-by-side for g8e.local (mTLS), IP Address (mTLS), Plain HTTP, and Stdio Transport.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agent := args[0]
			return printAgentShow(cmd, agent)
		},
	}

	return cmd
}

func printAgentShow(cmd *cobra.Command, agentID string) error {
	// Validate agent exists
	var description string
	found := false
	for _, a := range getSupportedAgents() {
		if strings.EqualFold(a.ID, agentID) {
			description = a.Description
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("unknown agent: %s. Use 'g8e mcp agent list' to see supported agents", agentID)
	}

	cmd.Println("╔═════════════════════════════════════════════════════════════════════════")
	cmd.Println("║           g8e Gateway MCP Configurations")
	cmd.Printf("║  Use these configs to connect %s \n", description)
	cmd.Println("║  to the g8e Gateway for agent orchestration and tool execution.")
	cmd.Println("╚═════════════════════════════════════════════════════════════════════════")

	cmd.Println()


	cmd.Println("┌─ g8e.local (mTLS) ─────────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Production environments with DNS configured")
	cmd.Println("│ Apps: Cursor, Windsurf, VS Code MCP clients")
	cmd.Println("│ Requires: DNS or /etc/hosts entry for g8e.local resolution")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigLocal(cmd); err != nil {
		return err
	}
	cmd.Println()

	cmd.Println("┌─ IP Address (mTLS) ───────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Environments without DNS or for direct IP access")
	cmd.Println("│ Apps: Cursor, Windsurf, VS Code MCP clients")
	cmd.Println("│ Requires: No DNS setup, uses external interface IP")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigIP(cmd); err != nil {
		return err
	}
	cmd.Println()

	cmd.Println("┌─ Plain HTTP ────────────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Local development only (localhost access)")
	cmd.Println("│ Apps: Local MCP clients, testing")
	cmd.Println("│ Requires: No mTLS, uses 127.0.0.1 explicitly")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigHTTP(cmd); err != nil {
		return err
	}
	cmd.Println()

	cmd.Println("┌─ Stdio Transport ────────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Direct native tool access without gateway")
	cmd.Println("│ Apps: Cursor, Windsurf, VS Code MCP clients")
	cmd.Println("│ Requires: g8e binary in PATH or full path in config")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigStdio(cmd); err != nil {
		return err
	}

	return nil
}

type agentInfo struct {
	ID          string
	Description string
}

func getSupportedAgents() []agentInfo {
	return []agentInfo{
		{"claude", "Anthropic Claude Desktop / Claude Code"},
		{"cursor", "Cursor AI IDE"},
		{"windsurf", "Windsurf AI IDE"},
		{"vscode", "Visual Studio Code with MCP extension"},
		{"continue", "Continue.dev AI coding assistant"},
		{"aider", "Aider AI pair programmer"},
		{"codeium", "Codeium AI assistant"},
		{"tabby", "Tabby AI autocomplete"},
		{"generic", "Generic MCP-compatible agent"},
	}
}

// agentRunCmd implements 'g8e mcp agent run' - a governance reverse proxy for any MCP server.
func agentRunCmd() *cobra.Command {
	var downstreamURL string

	cmd := &cobra.Command{
		Use:   "run [--url <url>] [-- <command> [args...]]",
		Short: "Govern any MCP server via g8e reverse proxy",
		Long: `Launch an AI agent or wrap an MCP server with g8e governance.

LAUNCH AN AGENT (one command does everything):

  g8e mcp agent run claude       Start Claude with g8e as its governed MCP provider.
                                  Uses 'mcp gov' if the gateway is running (L1-L5),
                                  or 'mcp stdio' otherwise (L1 native tools).
                                  All MCP tool calls are routed exclusively through
                                  g8e — no other MCP servers are reachable.

  Extra args are forwarded to the agent:
    g8e mcp agent run claude -p "fix the failing tests"

WRAP AN EXTERNAL MCP SERVER (governance reverse proxy):

  g8e mcp agent run -- npx -y @modelcontextprotocol/server-filesystem /home/user
  g8e mcp agent run --url http://localhost:3000

  Intercepts all tools/call requests, screens them through L1 doctrine
  (MITRE ATT&CK threat detection), and blocks violations before forwarding.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPAgentRun(args, downstreamURL)
		},
	}

	cmd.Flags().StringVar(&downstreamURL, "url", "", "URL of the downstream HTTP MCP server")
	return cmd
}

// mcpDownstreamProxy abstracts the downstream MCP server (HTTP or subprocess).
type mcpDownstreamProxy interface {
	forward(req JSONRPCRequest) (JSONRPCResponse, error)
	stop()
}

// httpMCPProxy forwards MCP requests to an HTTP downstream server.
type httpMCPProxy struct {
	url    string
	client *http.Client
}

func (d *httpMCPProxy) forward(req JSONRPCRequest) (JSONRPCResponse, error) {
	return proxyToGateway(d.client, d.url, req)
}

func (d *httpMCPProxy) stop() {}

// subprocessMCPProxy manages an MCP subprocess connected via stdio.
type subprocessMCPProxy struct {
	command string
	args    []string
	logger  *slog.Logger
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	mu      sync.Mutex
}

func (d *subprocessMCPProxy) start() error {
	d.cmd = exec.Command(d.command, d.args...) //nolint:gosec
	setSysProcAttr(d.cmd)

	stdin, err := d.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	d.stdin = stdin

	stdout, err := d.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	d.scanner = scanner
	d.cmd.Stderr = os.Stderr

	if err := d.cmd.Start(); err != nil {
		return fmt.Errorf("start subprocess: %w", err)
	}
	d.logger.Info("Downstream MCP subprocess started", "command", d.command, "pid", d.cmd.Process.Pid)
	return nil
}

func (d *subprocessMCPProxy) stop() {
	if d.stdin != nil {
		_ = d.stdin.Close()
	}
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	}
}

func (d *subprocessMCPProxy) forward(req JSONRPCRequest) (JSONRPCResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("marshal request: %w", err)
	}
	if _, err := fmt.Fprintf(d.stdin, "%s\n", reqBytes); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("write to subprocess: %w", err)
	}

	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return JSONRPCResponse{}, fmt.Errorf("read from subprocess: %w", err)
		}
		return JSONRPCResponse{}, fmt.Errorf("subprocess closed")
	}
	var resp JSONRPCResponse
	if err := json.Unmarshal(d.scanner.Bytes(), &resp); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("decode subprocess response: %w", err)
	}
	return resp, nil
}

// gatewayAvailable returns true if the local g8e gateway is reachable.
func gatewayAvailable() bool {
	cfg, err := config.Load("")
	if err != nil {
		return false
	}
	client, err := createMCPClient(cfg)
	if err != nil {
		return false
	}
	client.Timeout = 2 * time.Second
	gatewayURL := fmt.Sprintf("https://g8e.local:%d/api/v1/health", constants.Ports.OperatorHttps)
	resp, err := client.Get(gatewayURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// launchAgentWithGovernance launches a supported AI agent with g8e configured as its
// sole MCP provider. All MCP tool calls from the agent pass through g8e governance.
//
// If the g8e gateway is running, uses 'mcp gov' for full L1-L5 governance.
// Otherwise falls back to 'mcp stdio' for L1 inline governance with native tools.
func launchAgentWithGovernance(agentID string, extraArgs []string) error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve g8e binary path: %w", err)
	}

	mcpArgs := []string{"mcp", "stdio"}
	modeLabel := "stdio (L1 native tools)"
	if gatewayAvailable() {
		mcpArgs = []string{"mcp", "gov"}
		modeLabel = "gov (L1-L5 full governance via gateway)"
	}

	configJSON, err := json.Marshal(map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"g8e": map[string]interface{}{
				"command": binaryPath,
				"args":    mcpArgs,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("build MCP config: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "g8e-mcp-*.json")
	if err != nil {
		return fmt.Errorf("create temp MCP config: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write(configJSON); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write MCP config: %w", err)
	}
	tmpFile.Close()

	agentBin, err := exec.LookPath(agentID)
	if err != nil {
		return fmt.Errorf("%q not found in PATH — is it installed?", agentID)
	}

	launchArgs, err := agentLaunchArgs(agentID, tmpFile.Name())
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "[g8e] Launching %s with governance mode: %s\n", agentID, modeLabel)

	agentCmd := exec.Command(agentBin, append(launchArgs, extraArgs...)...) //nolint:gosec
	agentCmd.Stdin = os.Stdin
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr
	setSysProcAttr(agentCmd)

	return agentCmd.Run()
}

// agentLaunchArgs returns the argv to pass to the agent binary for a governed session.
func agentLaunchArgs(agentID, mcpConfigPath string) ([]string, error) {
	switch strings.ToLower(agentID) {
	case "claude":
		// --mcp-config   loads servers from the JSON file
		// --strict-mcp-config  ignores all other configured MCP servers so every
		//                      MCP call is routed exclusively through g8e governance
		return []string{"--mcp-config", mcpConfigPath, "--strict-mcp-config"}, nil
	default:
		return nil, fmt.Errorf("auto-launch not yet supported for %q\n\nTo configure manually:\n  g8e mcp agent show %s", agentID, agentID)
	}
}

func runMCPAgentRun(args []string, downstreamURL string) error {
	if downstreamURL == "" && len(args) == 0 {
		return fmt.Errorf("specify an agent name or MCP server\n\nLaunch an agent with governance:\n  g8e mcp agent run claude\n\nWrap an MCP server subprocess:\n  g8e mcp agent run -- npx -y @modelcontextprotocol/server-filesystem /\n\nWrap an HTTP MCP server:\n  g8e mcp agent run --url http://localhost:3000")
	}

	// Named agent (e.g. 'claude', 'cursor') → launch it with g8e as its governed MCP provider.
	// Any args after the agent name are forwarded to the agent binary unchanged.
	if downstreamURL == "" && len(args) > 0 {
		firstArg := strings.ToLower(args[0])
		for _, a := range getSupportedAgents() {
			if strings.ToLower(a.ID) == firstArg {
				return launchAgentWithGovernance(a.ID, args[1:])
			}
		}
	}

	// MCP server (--url or -- command) → run as governance reverse proxy.
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	var ds mcpDownstreamProxy
	if downstreamURL != "" {
		ds = &httpMCPProxy{
			url:    downstreamURL,
			client: &http.Client{Timeout: 30 * time.Second},
		}
	} else {
		proc := &subprocessMCPProxy{
			command: args[0],
			args:    args[1:],
			logger:  logger,
		}
		if err := proc.start(); err != nil {
			return fmt.Errorf("start downstream MCP server: %w", err)
		}
		ds = proc
	}
	defer ds.stop()

	l1 := governance.NewL1Doctrine()
	logger.Info("g8e MCP governance proxy started")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			sendError(encoder, nil, -32700, "parse error")
			continue
		}

		logger.Info("g8e intercepted", "method", req.Method, "id", req.ID)

		if req.Method == "tools/call" {
			var callParams CallToolRequest
			if err := json.Unmarshal(req.Params, &callParams); err != nil {
				sendError(encoder, req.ID, -32600, "invalid tools/call params")
				continue
			}

			argsJSON := "{}"
			if len(callParams.Arguments) > 0 {
				argsJSON = string(callParams.Arguments)
			}

			signals, err := l1.AnalyzeMCPArguments(argsJSON)
			if err != nil {
				logger.Warn("L1 analysis error", "tool", callParams.Name, "error", err)
			}

			var violations []string
			for _, sig := range signals {
				if sig.BlockRecommended {
					violations = append(violations, fmt.Sprintf("%s [%s, MITRE: %s]", sig.Indicator, sig.Category, sig.MitreAttack))
				}
			}

			if len(violations) > 0 {
				logger.Warn("g8e L1 BLOCKED", "tool", callParams.Name, "violations", violations)
				sendSuccess(encoder, req.ID, mcp.CallToolResult{
					IsError: true,
					Content: []mcp.TextContent{{
						Type: "text",
						Text: fmt.Sprintf("g8e governance blocked tool call %q:\n- %s", callParams.Name, strings.Join(violations, "\n- ")),
					}},
				})
				continue
			}

			logger.Info("g8e L1 approved", "tool", callParams.Name)
		}

		resp, err := ds.forward(req)
		if err != nil {
			if req.Method == "initialize" {
				handleInitialize(encoder, req.ID)
				continue
			}
			sendError(encoder, req.ID, -32603, fmt.Sprintf("downstream error: %v", err))
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			logger.Error("Failed to encode response", "error", err)
		}
	}

	return scanner.Err()
}

func extractURLFromText(text string) string {
	// Use regex to extract HTTPS URLs that contain the approval path
	urlPattern := regexp.MustCompile(`https://[^\s"']+` + regexp.QuoteMeta(constants.APIPaths.ApprovePagePrefix) + `[^\s"']*`)
	matches := urlPattern.FindStringSubmatch(text)
	if len(matches) > 0 {
		return matches[0]
	}

	// Fallback to any HTTPS URL if approval path not found
	genericURLPattern := regexp.MustCompile(`https://[^\s"']+`)
	matches = genericURLPattern.FindStringSubmatch(text)
	if len(matches) > 0 {
		return matches[0]
	}

	return ""
}
