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
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/pathutil"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/spf13/cobra"
)

// G8E environment variables injected by 'agent run' and consumed by 'mcp stdio'.
// Identity is carried in the delegated cert's URI SANs — no session header env vars.
const (
	envG8EClientCert = "G8E_CLIENT_CERT"
	envG8EClientKey  = "G8E_CLIENT_KEY"
	envG8ECABundle   = "G8E_CA_BUNDLE"
	envG8EGatewayURL = "G8E_GATEWAY_URL"
	envG8EAppID      = "G8E_APP_ID"
	envG8EAppCert    = "G8E_APP_CERT"
	envG8EAppKey     = "G8E_APP_KEY"
)

var (
	l3ApprovalMaxIterations = 30
	l3ApprovalPollInterval  = 10 * time.Second
)

// nativeToolsToDisable are Claude/Codex built-in tools that bypass MCP governance.
// Disabling them forces all I/O through g8e's MCP tools so every action is audited.
var nativeToolsToDisable = []string{
	"Bash", "Read", "Write", "Edit", "Glob", "Grep", "WebSearch", "WebFetch",
}

// mcpCmd is the parent command for MCP stdio operations.
func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP protocol operations (stdio transport with full governance)",
		Long:  `Run g8e as an MCP server using stdio transport for local agent integration. All MCP calls are proxied through the gateway with full L1-L5 governance enforcement.`,
	}

	cmd.AddCommand(
		mcpStdioCmd(),
		agentCmd(),
	)

	return cmd
}

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error object.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ToolsListResult is the result payload for tools/list.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// Tool represents a single MCP tool descriptor.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// CallToolRequest is the params object for tools/call.
type CallToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// InitializeResult is the result payload for initialize.
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
}

// ServerInfo contains server information for initialize.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ApprovalResult represents a tool call result requiring L3 approval.
type ApprovalResult struct {
	ApprovalURL string    `json:"approval_url,omitempty"`
	Content     []Content `json:"content,omitempty"`
}

// Content represents a content item in an MCP response.
type Content struct {
	Type string      `json:"type"`
	Text string      `json:"text,omitempty"`
	Data interface{} `json:"data,omitempty"`
}

// ─── stdio: governed proxy (the only supported mode) ────────────────────────

func mcpStdioCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stdio",
		Short: "Run MCP stdio server with full L1-L5 governance (proxies to gateway)",
		Long: `Run as an MCP stdio server that proxies all requests to the running gateway over
mTLS with a bound CLI session. Every tool call passes through the L1-L5 governance
pipeline. HTTP is never used for proxy traffic — it is reserved for CA bundle
discovery and health checks only.

This command is launched automatically by 'g8e mcp agent run'. When invoked
directly the CLI session is loaded from disk (bootstrapping enrollment if needed).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPStdioProxy(cmd, args)
		},
	}
}

func handleInitialize(encoder *json.Encoder, id interface{}) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			ServerInfo: ServerInfo{
				Name:    "g8e",
				Version: "1.0.0",
			},
		},
	}
	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode initialize response", "error", err)
	}
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
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  ToolsListResult{Tools: tools},
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

func sendError(encoder *json.Encoder, id interface{}, code int, message string) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
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

// ─── stdio: governed proxy, full mTLS + CLI session to gateway ────────────────

// gatewayConn is the mTLS connection to the gateway established at startup.
// Identity is cryptographically bound in the delegated cert's URI SANs — the cert
// IS the session. No server-side session object or identity headers are used.
type gatewayConn struct {
	client     *http.Client
	gatewayURL string
}

// buildGatewayConn constructs a gatewayConn. It reads the delegated credential
// from G8E_* environment variables injected by 'mcp agent run'. The delegated cert
// carries both the app SPIFFE ID and the requestor's user identity in its URI SANs.
func buildGatewayConn(cfg *config.Config) (*gatewayConn, error) {
	// Use the delegated credential (app cert) for agent runs, or CLI cert for direct CLI usage
	certFile := envOr(envG8EAppCert, envOr(envG8EClientCert, cfg.CLICertFile()))
	keyFile := envOr(envG8EAppKey, envOr(envG8EClientKey, cfg.CLIKeyFile()))
	caFile := envOr(envG8ECABundle, cfg.TrustBundlePath())

	// Try g8e.local first, fall back to IP if not set in env
	gatewayURL := os.Getenv(envG8EGatewayURL)
	if gatewayURL == "" {
		gatewayURL = fmt.Sprintf("https://%s:%d/mcp", constants.GatewayInternalHostname, constants.Ports.OperatorHttps)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}
	caBundleBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caBundleBytes)

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
		ServerName:   constants.GatewayInternalHostname,
	}

	// Try to connect with the current gatewayURL
	session := &gatewayConn{
		client: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
			Timeout:   30 * time.Second,
		},
		gatewayURL: gatewayURL,
	}

	// Test the connection - if it fails due to DNS, fall back to IP
	if !strings.Contains(gatewayURL, constants.GatewayInternalHostname) {
		// Already using IP or custom URL, return as-is
		return session, nil
	}

	// Try to resolve g8e.local via DNS
	_, err = net.LookupHost(constants.GatewayInternalHostname)
	if err == nil {
		// g8e.local resolves, use it
		return session, nil
	}

	// DNS failed, fall back to IP
	externalIP := network.GetExternalInterfaceIP()
	gatewayURL = fmt.Sprintf("https://%s:%d/mcp", externalIP, constants.Ports.OperatorHttps)
	session.gatewayURL = gatewayURL
	slog.Info("g8e.local DNS resolution failed, falling back to direct IP", "ip", externalIP)

	return session, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runMCPStdioProxy(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Build the mTLS gateway connection once. Identity is in the delegated cert's
	// URI SANs — no session object or headers. All proxy calls reuse this connection.
	conn, err := buildGatewayConn(cfg)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayNotReady, err)
	}

	logger.Info("g8e MCP governance proxy starting",
		"gateway_url", conn.gatewayURL,
	)

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

		// MCP notifications are fire-and-forget. They must not receive a
		// response — drop them silently.
		if req.ID == nil && req.Method != "" {
			logger.Debug("Dropping MCP notification", "method", req.Method)
			continue
		}

		// The initialize handshake is answered locally so the agent gets an
		// immediate response without a gateway round-trip.
		if req.Method == "initialize" {
			handleInitialize(encoder, req.ID)
			continue
		}

		logger.Info("Proxying MCP request", "method", req.Method, "id", req.ID)

		resp, err := proxySessionToGatewayWithRetry(conn, req, logger)
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

	logger.Info("g8e MCP governance proxy shutting down")
	return nil
}

// proxySessionToGateway posts a JSON-RPC request to the gateway over mTLS.
// In CLI mode, it attaches CLI session headers. In app mode, it relies purely on mTLS cert.
func proxySessionToGateway(session *gatewayConn, req JSONRPCRequest) (JSONRPCResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return JSONRPCResponse{}, err
	}

	httpReq, err := http.NewRequest(http.MethodPost, session.gatewayURL, bytes.NewReader(reqBody))
	if err != nil {
		return JSONRPCResponse{}, err
	}
	httpReq.Header.Set(constants.HeaderContentType, "application/json")

	// Identity is now carried in the delegated credential (mTLS cert), not in headers
	httpResp, err := session.client.Do(httpReq)
	if err != nil {
		return JSONRPCResponse{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return JSONRPCResponse{}, fmt.Errorf("%w: HTTP %d: %s", constants.ErrHTTPStatusError, httpResp.StatusCode, string(body))
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return JSONRPCResponse{}, err
	}
	return resp, nil
}

func proxySessionToGatewayWithRetry(session *gatewayConn, req JSONRPCRequest, logger *slog.Logger) (JSONRPCResponse, error) {
	return proxySessionToGatewayWithRetryContext(context.Background(), session, req, logger)
}

// proxySessionToGatewayWithRetryContext performs L3 approval polling with context support.
func proxySessionToGatewayWithRetryContext(ctx context.Context, session *gatewayConn, req JSONRPCRequest, logger *slog.Logger) (JSONRPCResponse, error) {
	resp, err := proxySessionToGateway(session, req)
	if err != nil {
		return resp, err
	}

	if !isL3ApprovalResponse(resp) {
		return resp, nil
	}

	approvalURL := extractApprovalURL(resp)
	if logger != nil {
		logger.Info("L3 approval required, waiting for user to authorize...", "url", approvalURL)
	}

	if err := platform.OpenBrowser(approvalURL); err != nil {
		if logger != nil {
			logger.Warn("Failed to auto-open browser", "error", err)
		}
		fmt.Fprintf(os.Stderr, "\n[g8e] Please visit: %s\n", approvalURL)
	}

	ticker := time.NewTicker(l3ApprovalPollInterval)
	defer ticker.Stop()

	for i := 0; i < l3ApprovalMaxIterations; i++ {
		select {
		case <-ctx.Done():
			if logger != nil {
				logger.Warn("L3 approval polling cancelled by context")
			}
			return resp, ctx.Err()
		case <-ticker.C:
			retryResp, err := proxySessionToGateway(session, req)
			if err != nil {
				continue
			}
			if !isL3ApprovalResponse(retryResp) {
				if logger != nil {
					logger.Info("L3 approval completed, proceeding with execution")
				}
				return retryResp, nil
			}
		}
	}

	if logger != nil {
		logger.Warn("L3 approval timeout, returning original response")
	}
	return resp, nil
}

// ─── createMCPClient: kept for tests and external callers ───────────────────

// createMCPClient builds a plain mTLS HTTP client from config paths.
// Most callers should use buildGatewayConn instead, which also loads the
// delegated credential required for mTLS gateway authentication.
func createMCPClient(cfg *config.Config) (*http.Client, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	caCert, err := os.ReadFile(cfg.TrustBundlePath())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
		ServerName:   constants.GatewayInternalHostname,
	}

	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
		Timeout:   30 * time.Second,
	}, nil
}

// proxyToGateway is a low-level helper used by the L1-only governance proxy
// and test code. It does not attach CLI session headers; use
// proxySessionToGateway when a bound session is available.
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

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return JSONRPCResponse{}, fmt.Errorf("%w: HTTP %d: %s", constants.ErrHTTPStatusError, httpResp.StatusCode, string(body))
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return JSONRPCResponse{}, err
	}
	return resp, nil
}

func isL3ApprovalResponse(resp JSONRPCResponse) bool {
	if resp.Result == nil {
		return false
	}
	// Try to unmarshal as ApprovalResult
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return false
	}
	var approvalResult ApprovalResult
	if err := json.Unmarshal(resultBytes, &approvalResult); err != nil {
		return false
	}
	return approvalResult.ApprovalURL != ""
}

func extractApprovalURL(resp JSONRPCResponse) string {
	if resp.Result == nil {
		return ""
	}

	// Try to unmarshal as ApprovalResult
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return ""
	}
	var approvalResult ApprovalResult
	if err := json.Unmarshal(resultBytes, &approvalResult); err == nil {
		if approvalResult.ApprovalURL != "" {
			return approvalResult.ApprovalURL
		}
		// Check content array for approval URL
		for _, item := range approvalResult.Content {
			if item.Text != "" {
				if url := extractURLFromText(item.Text); url != "" {
					return url
				}
			}
		}
	}

	// Fallback to text extraction from entire result
	return extractURLFromText(string(resultBytes))
}

// ─── agent show config printers ─────────────────────────────────────────────

func printMCPConfigLocal(cmd *cobra.Command) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
	}

	externalIP := network.GetExternalInterfaceIP()
	cmd.Printf("# Add this entry to /etc/hosts to enable %s resolution:\n", constants.GatewayInternalHostname)
	cmd.Printf("%s %s\n\n", externalIP, constants.GatewayInternalHostname)

	gatewayURL := fmt.Sprintf("https://%s:%d/mcp", constants.GatewayInternalHostname, constants.Ports.OperatorHttps)

	actualCertPath := filepath.ToSlash(cfg.CLICertFile())
	actualKeyPath := filepath.ToSlash(cfg.CLIKeyFile())
	actualCAPath := filepath.ToSlash(cfg.TrustBundlePath())

	mcpConfig, err := mcp.NewGatewayConfig(gatewayURL, actualCertPath, actualKeyPath, actualCAPath)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayURLRequired, err)
	}

	configJSON, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}

	cmd.Println(string(configJSON))
	return nil
}

func printMCPConfigIP(cmd *cobra.Command) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
	}

	externalIP := network.GetExternalInterfaceIP()
	gatewayURL := fmt.Sprintf("https://%s:%d/mcp", externalIP, constants.Ports.OperatorHttps)

	actualCertPath := filepath.ToSlash(cfg.CLICertFile())
	actualKeyPath := filepath.ToSlash(cfg.CLIKeyFile())
	actualCAPath := filepath.ToSlash(cfg.TrustBundlePath())

	// Use constants.GatewayInternalHostname for hostname verification even when connecting via IP
	// The certificate has constants.GatewayInternalHostname in its SAN, so verification will succeed
	mcpConfig, err := mcp.NewGatewayConfigWithHostname(gatewayURL, actualCertPath, actualKeyPath, actualCAPath, constants.GatewayInternalHostname)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayURLRequired, err)
	}

	configJSON, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}

	cmd.Println(string(configJSON))
	return nil
}

func printMCPConfigStdio(cmd *cobra.Command) error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	mcpConfig, err := mcp.NewStdioConfigSimple(binaryPath)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayURLRequired, err)
	}

	configJSON, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}

	cmd.Println(string(configJSON))
	return nil
}

// ─── agent subcommands ───────────────────────────────────────────────────────

func agentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Agent integration commands for popular AI coding tools",
		Long:  `Configure and integrate g8e with popular AI agent binaries (Claude, Codex, Cursor, Devin, etc.) for seamless MCP tool access.`,
	}

	cmd.AddCommand(
		agentListCmd(),
		agentShowCmd(),
		agentRunCmd(),
	)

	return cmd
}

func agentListCmd() *cobra.Command {
	return &cobra.Command{
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
}

func agentShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <agent>",
		Short: "Print MCP client configuration for the Gateway",
		Long:  `Print MCP client configuration for connecting to the g8e Gateway from local coding tools. Displays configurations for g8e.local (mTLS), IP Address (mTLS), and Stdio Transport.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return printAgentShow(cmd, args[0])
		},
	}
}

func printAgentShow(cmd *cobra.Command, agentID string) error {
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
		return fmt.Errorf("%w: %s. Use 'g8e mcp agent list' to see supported agents", constants.ErrAgentNotFound, agentID)
	}

	cmd.Println("╔═════════════════════════════════════════════════════════════════════════")
	cmd.Println("║           g8e Gateway MCP Configurations")
	cmd.Printf("║  Use these configs to connect %s \n", description)
	cmd.Println("║  to the g8e Gateway for agent orchestration and tool execution.")
	cmd.Println("╚═════════════════════════════════════════════════════════════════════════")
	cmd.Println()

	cmd.Println("┌─ g8e.local (mTLS) ─────────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Production environments with DNS configured")
	cmd.Println("│ Apps: Cursor, Devin, VS Code MCP clients")
	cmd.Println("│ Requires: DNS or /etc/hosts entry for g8e.local resolution")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigLocal(cmd); err != nil {
		return err
	}
	cmd.Println()

	cmd.Println("┌─ IP Address (mTLS) ───────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Environments without DNS or for direct IP access")
	cmd.Println("│ Apps: Cursor, Devin, VS Code MCP clients")
	cmd.Println("│ Requires: No DNS setup, uses external interface IP")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigIP(cmd); err != nil {
		return err
	}
	cmd.Println()

	cmd.Println("┌─ Stdio Transport ────────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Direct native tool access without gateway")
	cmd.Println("│ Apps: Claude Code, Cursor, Devin, VS Code MCP clients")
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
		{string(constants.AgentBinaryClaude), "Anthropic Claude Desktop / Claude Code"},
		{string(constants.AgentBinaryCodex), "OpenAI Codex AI coding assistant"},
		{string(constants.AgentBinaryCursor), "Cursor AI IDE"},
		{string(constants.AgentBinaryDevin), "Devin AI IDE (formerly Windsurf)"},
		{string(constants.AgentBinaryVSCode), "Visual Studio Code with MCP extension"},
		{string(constants.AgentBinaryContinue), "Continue.dev AI coding assistant"},
		{string(constants.AgentBinaryContinueAlias), "Continue.dev AI coding assistant (alias)"},
		{string(constants.AgentBinaryAider), "Aider AI pair programmer"},
		{string(constants.AgentBinaryCodeium), "Codeium AI assistant"},
		{string(constants.AgentBinaryTabby), "Tabby AI autocomplete"},
		{string(constants.AgentBinaryOllama), "Ollama local LLM runner"},
		{string(constants.AgentBinaryGemini), "Google Gemini CLI"},
		{string(constants.AgentBinaryGoose), "Goose AI coding assistant"},
		{string(constants.AgentBinaryGeneric), "Generic MCP-compatible agent"},
	}
}

// ─── agent run ──────────────────────────────────────────────────────────────

func agentRunCmd() *cobra.Command {
	var downstreamURL string

	cmd := &cobra.Command{
		Use:   "run [--url <url>] [-- <command> [args...]]",
		Short: "Govern any MCP server via g8e reverse proxy",
		Long: `Launch an AI agent or wrap an MCP server with g8e governance.

LAUNCH AN AGENT (one command does everything):

  g8e mcp agent run claude       Start the g8e gateway (if not already running),
                                  perform CLI auth, then launch Claude with native
                                  tools disabled so ALL I/O must go through g8e MCP
                                  — every action is audited at L1-L5. No other MCP
                                  servers are reachable.

  g8e mcp agent run cursor        Launch Cursor IDE with g8e MCP config written
                                  to ~/.cursor/mcp.json

  g8e mcp agent run devin         Launch Devin IDE with g8e MCP config written
                                  to ~/.codeium/windsurf/mcp_config.json

  g8e mcp agent run aider          Launch Aider with g8e MCP config written to
                                  .aider.conf.yml in the current directory

  g8e mcp agent run continue      Launch Continue CLI with g8e MCP config

  Extra args are forwarded to the agent:
    g8e mcp agent run claude -- -p "fix the failing tests"

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

	cmd.Flags().StringVar(&downstreamURL, "url", "", "URL of the downstream MCP server")
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
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}
	d.stdin = stdin

	stdout, err := d.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	d.scanner = scanner
	d.cmd.Stderr = os.Stderr

	if err := d.cmd.Start(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
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
		return JSONRPCResponse{}, fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}
	if _, err := fmt.Fprintf(d.stdin, "%s\n", reqBytes); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}

	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return JSONRPCResponse{}, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
		}
		return JSONRPCResponse{}, fmt.Errorf("%w: subprocess closed", constants.ErrProcessInterrupted)
	}
	var resp JSONRPCResponse
	if err := json.Unmarshal(d.scanner.Bytes(), &resp); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return resp, nil
}

// ensureGatewayRunning starts the gateway if it is not already running and
// waits until it is healthy, then ensures CLI mTLS credentials exist.
// HTTP is only used here to poll the bootstrap health endpoint before mTLS
// certs have been issued — all subsequent traffic uses mTLS.
func ensureGatewayRunning() error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
	}

	pm, err := platform.NewProcessManager(cfg.ProjectRoot)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}

	running, pid, err := pm.OperatorStatus()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}

	if running {
		fmt.Fprintf(os.Stderr, "[g8e] Gateway already running (PID %d)\n", pid)
	} else {
		fmt.Fprintf(os.Stderr, "[g8e] Starting gateway...\n")
		if err := pm.StartOperator(platform.OperatorStartOptions{
			Posture:            "doctrine",
			HTTPPort:           0,
			HTTPSPort:          0,
			DataDir:            "",
			PKIDir:             "",
			SecretsDir:         "",
			VaultDir:           "",
			VaultKeyPath:       "",
			VaultRequireUnlock: false,
			PasskeyRpID:        "",
			PasskeyRpName:      "",
			RateLimitRPS:       0,
			RateLimitBurst:     0,
			LogLevel:           "info",
			CertIdentityMode:   "localhost",
			IdentityData:       nil,
		}); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
		}

		// Poll plain HTTP health until the gateway is ready.
		// mTLS certs do not exist yet at this stage, so HTTP is the only
		// option. HTTP is only ever used here for this bootstrap health check.
		healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", constants.Ports.OperatorHttp)
		plainClient := &http.Client{Timeout: 2 * time.Second}
		const (
			maxAttempts  = 30
			pollInterval = 500 * time.Millisecond
		)
		for i := 0; i < maxAttempts; i++ {
			resp, err := plainClient.Get(healthURL) //nolint:gosec,noctx
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					break
				}
			}
			if i == maxAttempts-1 {
				return fmt.Errorf("%w: gateway did not become healthy after %v",
					constants.ErrGatewayNotReady, time.Duration(maxAttempts)*pollInterval)
			}
			time.Sleep(pollInterval)
		}
	}

	fmt.Fprintf(os.Stderr, "[g8e] Gateway ready (L1-L5 governance active)\n")
	return nil
}

// agentMCPConfig represents the MCP configuration structure for agents.
type agentMCPConfig struct {
	MCPServers   map[string]agentMCPServer `json:"mcpServers"`
	ExcludeTools []string                  `json:"excludeTools"`
}

// agentMCPServer represents a single MCP server configuration.
type agentMCPServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// geminiSettings represents the Gemini settings.json structure.
type geminiSettings struct {
	MCPServers   map[string]agentMCPServer `json:"mcpServers,omitempty"`
	ExcludeTools []string                  `json:"excludeTools,omitempty"`
}

// WriteAgentConfig writes the appropriate MCP config file for the agent.
// Returns the path to the config file and a cleanup function (if any).
func WriteAgentConfig(agentID, binaryPath string) (string, func(), error) {
	config := agentMCPConfig{
		MCPServers: map[string]agentMCPServer{
			"g8e": {
				Command: binaryPath,
				Args:    []string{"mcp", "stdio"},
			},
		},
		ExcludeTools: nativeToolsToDisable,
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
	}

	// Get home directory with cross-platform fallback
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrMCPGetHomeDirectory, err)
		}
	}

	// Precompute all agent config paths to avoid repeated filepath.Join calls
	agentPaths := paths.GetAgentConfigPaths(homeDir)

	switch agentID {
	case string(constants.AgentBinaryCursor):
		// Cursor does not support native tool disabling via config
		// Governance enforced by making g8e the only MCP server
		if err := os.MkdirAll(agentPaths.CursorConfigDir, 0755); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if err := BackupConfigFile(agentPaths.CursorConfigPath); err != nil {
			return "", nil, err
		}
		displayPath := pathutil.ToSlash(agentPaths.CursorConfigPath)
		fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s (g8e as only MCP server for governance)\n", displayPath)
		if err := os.WriteFile(agentPaths.CursorConfigPath, configJSON, 0644); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		return agentPaths.CursorConfigPath, nil, nil

	case string(constants.AgentBinaryDevin):
		// Devin does not support native tool disabling via config
		// Governance enforced by making g8e the only MCP server
		if err := os.MkdirAll(agentPaths.DevinConfigDir, 0755); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if err := BackupConfigFile(agentPaths.DevinConfigPath); err != nil {
			return "", nil, err
		}
		displayPath := pathutil.ToSlash(agentPaths.DevinConfigPath)
		fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s (g8e as only MCP server for governance)\n", displayPath)
		if err := os.WriteFile(agentPaths.DevinConfigPath, configJSON, 0644); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		return agentPaths.DevinConfigPath, nil, nil

	case string(constants.AgentBinaryAider):
		// Aider does not support native tool disabling via config
		// Governance enforced by making g8e the only MCP server
		configPath := constants.AgentConfigFileAider
		// Check if config already exists to avoid silent overwrite
		if _, err := os.Stat(configPath); err == nil {
			return "", nil, fmt.Errorf("%w: %s in current directory - please back it up before running g8e", constants.ErrConfigFileExists, constants.AgentConfigFileAider)
		}
		displayPath := pathutil.ToSlash(configPath)
		fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s (g8e as only MCP server for governance)\n", displayPath)
		configYAML := fmt.Sprintf("mcp-server:\n  - name: g8e\n    command: %s\n    args:\n      - mcp\n      - stdio\n", binaryPath)
		if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		return configPath, nil, nil

	case string(constants.AgentBinaryGemini):
		// Gemini uses settings.json for configuration
		// We need to add g8e MCP server and exclude native tools to force governance
		if err := os.MkdirAll(agentPaths.GeminiConfigDir, 0755); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}

		// Read existing settings if present
		var settings geminiSettings
		if existingData, err := os.ReadFile(agentPaths.GeminiConfigPath); err == nil {
			if err := json.Unmarshal(existingData, &settings); err != nil {
				return "", nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}
		}

		// Add mcpServers configuration with g8e
		if settings.MCPServers == nil {
			settings.MCPServers = make(map[string]agentMCPServer)
		}
		settings.MCPServers["g8e"] = agentMCPServer{
			Command: binaryPath,
			Args:    []string{"mcp", "stdio"},
		}

		// Exclude native tools to force governance through g8e MCP
		// This forces the agent to use only g8e's governed tools
		settings.ExcludeTools = nativeToolsToDisable

		configJSON, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
		}

		displayPath := pathutil.ToSlash(agentPaths.GeminiConfigPath)
		fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s with native tools disabled\n", displayPath)
		if err := os.WriteFile(agentPaths.GeminiConfigPath, configJSON, 0644); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		return agentPaths.GeminiConfigPath, nil, nil

	case string(constants.AgentBinaryGoose):
		// Goose does not support native tool disabling via config
		// Governance enforced by making g8e the only MCP server
		if err := os.MkdirAll(agentPaths.GooseConfigDir, 0755); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if err := BackupConfigFile(agentPaths.GooseConfigPath); err != nil {
			return "", nil, err
		}
		displayPath := pathutil.ToSlash(agentPaths.GooseConfigPath)
		fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s (g8e as only MCP server for governance)\n", displayPath)
		if err := os.WriteFile(agentPaths.GooseConfigPath, configJSON, 0644); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		return agentPaths.GooseConfigPath, nil, nil

	case string(constants.AgentBinaryVSCode):
		// VS Code does not support native tool disabling via config
		// Governance enforced by making g8e the only MCP server
		if err := os.MkdirAll(agentPaths.VSCodeConfigDir, 0755); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if err := BackupConfigFile(agentPaths.VSCodeConfigPath); err != nil {
			return "", nil, err
		}
		displayPath := pathutil.ToSlash(agentPaths.VSCodeConfigPath)
		fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s (g8e as only MCP server for governance)\n", displayPath)
		if err := os.WriteFile(agentPaths.VSCodeConfigPath, configJSON, 0644); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		return agentPaths.VSCodeConfigPath, nil, nil

	case string(constants.AgentBinaryCodeium):
		// Codeium does not support native tool disabling via config
		// Governance enforced by making g8e the only MCP server
		if err := os.MkdirAll(agentPaths.CodeiumConfigDir, 0755); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if err := BackupConfigFile(agentPaths.CodeiumConfigPath); err != nil {
			return "", nil, err
		}
		displayPath := pathutil.ToSlash(agentPaths.CodeiumConfigPath)
		fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s (g8e as only MCP server for governance)\n", displayPath)
		if err := os.WriteFile(agentPaths.CodeiumConfigPath, configJSON, 0644); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		return agentPaths.CodeiumConfigPath, nil, nil

	case string(constants.AgentBinaryTabby):
		// Tabby does not support native tool disabling via config
		// Governance enforced by making g8e the only MCP server
		if err := os.MkdirAll(agentPaths.TabbyConfigDir, 0755); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if err := BackupConfigFile(agentPaths.TabbyConfigPath); err != nil {
			return "", nil, err
		}
		displayPath := pathutil.ToSlash(agentPaths.TabbyConfigPath)
		fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s (g8e as only MCP server for governance)\n", displayPath)
		if err := os.WriteFile(agentPaths.TabbyConfigPath, configJSON, 0644); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		return agentPaths.TabbyConfigPath, nil, nil

	case string(constants.AgentBinaryContinue), string(constants.AgentBinaryContinueAlias):
		// Continue does not support native tool disabling via config
		// Governance enforced by making g8e the only MCP server
		if err := os.MkdirAll(agentPaths.ContinueConfigDir, 0755); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if err := BackupConfigFile(agentPaths.ContinueConfigPath); err != nil {
			return "", nil, err
		}
		displayPath := pathutil.ToSlash(agentPaths.ContinueConfigPath)
		fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s (g8e as only MCP server for governance)\n", displayPath)
		if err := os.WriteFile(agentPaths.ContinueConfigPath, configJSON, 0644); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		return agentPaths.ContinueConfigPath, nil, nil

	case string(constants.AgentBinaryOllama):
		// Ollama doesn't use a local config file - it's typically configured via CLI args
		// or third-party MCP clients. Return a temp file for reference.
		// Governance enforced by making g8e the only MCP server in the reference config
		tmpFile, err := os.CreateTemp("", "g8e-mcp-ollama-*.json")
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if _, err := tmpFile.Write(configJSON); err != nil {
			tmpFile.Close()
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		tmpFile.Close()
		tmpPath := tmpFile.Name()
		displayPath := pathutil.ToSlash(tmpPath)
		fmt.Fprintf(os.Stderr, "[g8e] Ollama requires manual MCP configuration. Reference config at: %s (g8e as only MCP server for governance)\n", displayPath)
		return tmpPath, func() {
			if err := os.Remove(tmpPath); err != nil {
				slog.Warn("Failed to cleanup temp MCP config file", "path", tmpPath, "error", err)
			}
		}, nil

	default:
		// For agents that use CLI flags or temp files
		tmpFile, err := os.CreateTemp("", "g8e-mcp-*.json")
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if _, err := tmpFile.Write(configJSON); err != nil {
			tmpFile.Close()
			return "", nil, fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		tmpFile.Close()
		tmpPath := tmpFile.Name()
		return tmpPath, func() {
			if err := os.Remove(tmpPath); err != nil {
				slog.Warn("Failed to cleanup temp MCP config file", "path", tmpPath, "error", err)
			}
		}, nil
	}
}

// launchAgentWithGovernance starts the gateway if needed, performs CLI auth,
// then launches the requested agent with 'g8e mcp stdio' as its sole MCP server.
// The authenticated CLI session is propagated to the stdio subprocess via G8E_*
// environment variables so it never needs to re-read credentials from disk.
func launchAgentWithGovernance(agentID string, extraArgs []string) error {
	if err := ensureGatewayRunning(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayNotReady, err)
	}

	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
	}

	// Ensure CLI credentials exist, auto-enroll if needed
	creds, err := auth.LoadCredentials(cfg)
	if err != nil || creds == nil {
		fmt.Fprintf(os.Stderr, "[g8e] No CLI credentials found, enrolling...\n")
		if err := auth.EnrollCLI(cfg); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
		}
		fmt.Fprintf(os.Stderr, "[g8e] CLI enrolled successfully\n")
		creds, err = auth.LoadCredentials(cfg)
		if err != nil || creds == nil {
			return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
		}
	}

	// Enroll the agent as an external app for audit trail attribution
	appID, appCert, appKey, err := auth.EnrollAgentApp(cfg, strings.ToLower(agentID))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}

	// Require an authenticated human with passkey registration; auto-register if missing
	hasPasskey, err := auth.VerifyPasskeyRegistration(cfg, creds.UserID)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrNoPasskeysRegistered, err)
	}
	if !hasPasskey {
		fmt.Fprintf(os.Stderr, "[g8e] No passkey registered, starting passkey enrollment...\n")
		if err := auth.RegisterPasskeyViaLocalhost(cfg, creds.UserID, creds.CLISessionID); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrPasskeyRegistrationFailed, err)
		}
	}

	// Validate agent binary exists before writing any config files
	agentBin, err := exec.LookPath(agentID)
	if err != nil {
		return fmt.Errorf("%w: %q not found in PATH — is it installed?", constants.ErrAgentNotInPath, agentID)
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	configPath, cleanup, err := WriteAgentConfig(agentID, binaryPath)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	launchArgs, err := agentLaunchArgs(agentID, configPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "[g8e] Launching %s with L1-L5 governance via gateway\n", agentID)

	agentCmd := exec.Command(agentBin, append(launchArgs, extraArgs...)...) //nolint:gosec
	agentCmd.Stdin = os.Stdin
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr
	// Don't set process group for interactive agents - it breaks terminal handling

	// Propagate the delegated credential to the 'g8e mcp stdio' subprocess that
	// the agent will spawn. Identity (both app and human) is cryptographically bound
	// in the delegated cert's URI SANs — no session headers needed.
	// The stdio proxy will automatically fall back to IP if g8e.local DNS fails.
	gatewayURL := fmt.Sprintf("https://%s:%d/mcp", constants.GatewayInternalHostname, constants.Ports.OperatorHttps)
	agentCmd.Env = append(os.Environ(),
		envG8EClientCert+"="+cfg.CLICertFile(),
		envG8EClientKey+"="+cfg.CLIKeyFile(),
		envG8ECABundle+"="+cfg.TrustBundlePath(),
		envG8EGatewayURL+"="+gatewayURL,
		envG8EAppID+"="+appID,
		envG8EAppCert+"="+appCert,
		envG8EAppKey+"="+appKey,
	)

	return agentCmd.Run()
}

// agentLaunchArgs returns the argv to pass to the agent binary for a governed session.
// Governance is enforced by making g8e the only MCP server in the agent's config.
// For agents that support native tool disabling via CLI flags, those are added here.
func agentLaunchArgs(agentID, mcpConfigPath string) ([]string, error) {
	switch strings.ToLower(agentID) {
	case "claude", "codex":
		// --mcp-config          load g8e as the only MCP server
		// --strict-mcp-config   ignore all other configured MCP servers
		// --disallowed-tools    disable native tools so every I/O action must
		//                       go through g8e MCP tools and is therefore audited
		return []string{
			"--mcp-config", mcpConfigPath,
			"--strict-mcp-config",
			"--disallowed-tools", strings.Join(nativeToolsToDisable, ","),
		}, nil
	case "cursor":
		// Cursor reads from ~/.cursor/mcp.json written by WriteAgentConfig
		// Governance enforced by g8e being the only MCP server
		// Cursor does not support CLI flags to disable native tools
		return []string{}, nil
	case "devin":
		// Devin reads from ~/.codeium/windsurf/mcp_config.json written by WriteAgentConfig
		// Governance enforced by g8e being the only MCP server
		// Devin does not support CLI flags to disable native tools
		return []string{}, nil
	case "continue", "cn":
		// Continue reads from ~/.continue/config.json written by WriteAgentConfig
		// Governance enforced by g8e being the only MCP server
		// Continue does not support CLI flags to disable native tools
		return []string{}, nil
	case "aider":
		// Aider reads from .aider.conf.yml in current directory written by WriteAgentConfig
		// Governance enforced by g8e being the only MCP server
		return []string{}, nil
	case "goose":
		// Goose reads from ~/.goose/config.json written by WriteAgentConfig
		// Governance enforced by g8e being the only MCP server
		return []string{}, nil
	case "vscode":
		// VS Code reads from ~/.vscode/mcp.json written by WriteAgentConfig
		// Governance enforced by g8e being the only MCP server
		// VS Code does not support CLI flags to disable native tools
		return []string{}, nil
	case "codeium":
		// Codeium reads from ~/.codeium/mcp.json written by WriteAgentConfig
		// Governance enforced by g8e being the only MCP server
		// Codeium does not support CLI flags to disable native tools
		return []string{}, nil
	case "tabby":
		// Tabby reads from ~/.tabby/mcp.json written by WriteAgentConfig
		// Governance enforced by g8e being the only MCP server
		// Tabby does not support CLI flags to disable native tools
		return []string{}, nil
	case "ollama":
		// Ollama requires manual MCP configuration via third-party clients
		// Return error to guide user to manual setup
		return nil, fmt.Errorf("%w: ollama requires manual MCP configuration via third-party clients. Run 'g8e mcp agent show ollama' to see the required configuration, then use it with an MCP-compatible ollama client", constants.ErrAgentNotSupported)
	case "gemini":
		// Gemini uses `gemini mcp add` to register servers, no config file needed
		// Governance enforced by g8e being the only MCP server
		return []string{}, nil
	default:
		return nil, fmt.Errorf("%w: auto-launch not yet supported for %q. To configure manually: g8e mcp agent show %s", constants.ErrAgentNotSupported, agentID, agentID)
	}
}

func runMCPAgentRun(args []string, downstreamURL string) error {
	if downstreamURL == "" && len(args) == 0 {
		return fmt.Errorf("specify an agent name or MCP server\n\nLaunch an agent with governance:\n  g8e mcp agent run claude\n\nWrap an MCP server subprocess:\n  g8e mcp agent run -- npx -y @modelcontextprotocol/server-filesystem /\n\nWrap an HTTP MCP server:\n  g8e mcp agent run --url http://localhost:3000")
	}

	// Named agent → launch it with g8e as its governed MCP provider.
	if downstreamURL == "" && len(args) > 0 {
		firstArg := strings.ToLower(args[0])
		for _, a := range getSupportedAgents() {
			if strings.ToLower(a.ID) == firstArg {
				return launchAgentWithGovernance(a.ID, args[1:])
			}
		}
	}

	// MCP server (--url or -- command) → run as L1 governance reverse proxy.
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
			return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
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

		// Drop notifications.
		if req.ID == nil && req.Method != "" {
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
					violations = append(violations, fmt.Sprintf("%s [%s, MITRE: %s]",
						sig.Indicator, sig.Category, sig.MitreAttack))
				}
			}

			if len(violations) > 0 {
				logger.Warn("g8e L1 BLOCKED", "tool", callParams.Name, "violations", violations)
				sendSuccess(encoder, req.ID, mcp.CallToolResult{
					IsError: true,
					Content: []mcp.TextContent{{
						Type: "text",
						Text: fmt.Sprintf("g8e governance blocked tool call %q:\n- %s",
							callParams.Name, strings.Join(violations, "\n- ")),
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
	urlPattern := regexp.MustCompile(`https://[^\s"']+` + regexp.QuoteMeta(constants.APIPaths.ApprovePagePrefix) + `[^\s"']*`)
	if matches := urlPattern.FindStringSubmatch(text); len(matches) > 0 {
		return matches[0]
	}
	genericURLPattern := regexp.MustCompile(`https://[^\s"']+`)
	if matches := genericURLPattern.FindStringSubmatch(text); len(matches) > 0 {
		return matches[0]
	}
	return ""
}
