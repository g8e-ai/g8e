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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/cli/serve"
	g8econfig "github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/pathutil"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/network"
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

// nativeToolsToDisable lists built-in tools that Claude Code and Codex must
// disable via --disallowed-tools to force all I/O through g8e's MCP gateway.
// Other agents use different mechanisms:
//   - Goose: --no-profile flag (zero extensions) + --with-extension for g8e MCP
//   - Gemini: tools.core: [] in settings.json (empty allowlist)
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

// MCPToolsCapability declares the tools capability for the MCP initialize handshake.
type MCPToolsCapability struct{}

// MCPCapabilities represents the capabilities object in the MCP initialize response.
type MCPCapabilities struct {
	Tools MCPToolsCapability `json:"tools"`
}

// InitializeResult is the result payload for initialize.
type InitializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    MCPCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo      `json:"serverInfo"`
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
	return mcpStdioCmdWithConfig(newFileSvc)
}

func mcpStdioCmdWithConfig(fileSvcFactory func() (fs.RuntimeFileService, error)) *cobra.Command {
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
			return runMCPStdioProxy(cmd, args, fileSvcFactory)
		},
	}
}

func handleInitialize(encoder *json.Encoder, id interface{}) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: MCPCapabilities{
				Tools: MCPToolsCapability{},
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

	// SSE fields for L3 approval notifications. Populated when CLI credentials
	// are available so the stdio proxy can subscribe to approval.completed events
	// instead of polling.
	sseBaseURL string
	userID     string
	sseClient  *http.Client
}

// buildGatewayConn constructs a gatewayConn. It reads the delegated credential
// from G8E_* environment variables injected by 'mcp agent run'. The delegated cert
// carries both the app SPIFFE ID and the requestor's user identity in its URI SANs.
func buildGatewayConn(fileSvc fs.RuntimeFileService, cfg *config.Config) (*gatewayConn, error) {
	// Use the delegated credential (app cert) for agent runs, or CLI cert for direct CLI usage
	certFile := envOr(envG8EAppCert, envOr(envG8EClientCert, cfg.CLICertFile()))
	keyFile := envOr(envG8EAppKey, envOr(envG8EClientKey, cfg.CLIKeyFile()))

	var caBundleBytes []byte
	var err error
	if envCA := os.Getenv(envG8ECABundle); envCA != "" {
		caBundleBytes, err = os.ReadFile(envCA)
	} else {
		caBundleBytes, err = auth.ReadTrustBundle(fileSvc, cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}

	// Try g8e.local first, fall back to IP if not set in env
	gatewayURL := os.Getenv(envG8EGatewayURL)
	if gatewayURL == "" {
		gatewayURL = fmt.Sprintf("https://%s:%d/mcp", constants.GatewayInternalHostname, constants.Ports.OperatorHttps)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
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

func runMCPStdioProxy(cmd *cobra.Command, _ []string, fileSvcFactory func() (fs.RuntimeFileService, error)) error {
	cfg, err := loadConfig("")
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}

	fileSvc, err := fileSvcFactory()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Build the mTLS gateway connection once. Identity is in the delegated cert's
	// URI SANs — no session object or headers. All proxy calls reuse this connection.
	conn, err := buildGatewayConn(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayNotReady, err)
	}

	logger.Info("g8e MCP governance proxy starting",
		"gateway_url", conn.gatewayURL,
	)

	// Populate SSE fields for L3 approval notifications. The SSE client uses
	// the CLI cert (not the delegated/app cert) because the gateway's SSE auth
	// middleware validates CLI session ownership. The gateway URL is stripped
	// of the /mcp suffix to get the base URL for SSE endpoints.
	if creds, err := auth.LoadCredentials(fileSvc, cfg); err == nil && creds != nil && creds.UserID != "" {
		if sseClient, err := auth.BuildMTLSClient(fileSvc, cfg, 0); err == nil {
			conn.sseClient = sseClient
			conn.userID = creds.UserID
			// Use OperatorPublicURL (g8e.local) for SSE to ensure TLS ServerName
			// matches the gateway cert SAN. Deriving from gatewayURL may produce
			// an IP-based URL that fails TLS verification.
			conn.sseBaseURL = strings.TrimSuffix(cfg.OperatorPublicURL(), "/")
			logger.Info("SSE approval notifications enabled", "user_id", creds.UserID)
		}
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

		resp, err := proxySessionToGatewayWithRetryContext(cmd.Context(), conn, req, logger)
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
		return fmt.Errorf("mcp: read stdin: %w", err)
	}

	logger.Info("g8e MCP governance proxy shutting down")
	return nil
}

// proxySessionToGateway posts a JSON-RPC request to the gateway over mTLS.
// In CLI mode, it attaches CLI session headers. In app mode, it relies purely on mTLS cert.
func proxySessionToGateway(session *gatewayConn, req JSONRPCRequest) (JSONRPCResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("mcp: marshal request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, session.gatewayURL, bytes.NewReader(reqBody))
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("mcp: create request: %w", err)
	}
	httpReq.Header.Set(constants.HeaderContentType, "application/json")

	// Identity is now carried in the delegated credential (mTLS cert), not in headers
	httpResp, err := session.client.Do(httpReq)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("mcp: execute request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return JSONRPCResponse{}, fmt.Errorf("%w: HTTP %d: %s", constants.ErrHTTPStatusError, httpResp.StatusCode, string(body))
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("mcp: decode response: %w", err)
	}
	return resp, nil
}

// proxySessionToGatewayWithRetryContext handles L3 approval responses by opening
// the browser for WebAuthn authorization and waiting for the approval.completed
// SSE event from the gateway. Once received, it re-sends the original request
// and returns the result. SSE credentials are required — there is no polling
// fallback.
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

	if session.sseClient == nil || session.sseBaseURL == "" || session.userID == "" {
		return resp, fmt.Errorf("L3 approval: %w", constants.ErrNotAuthenticated)
	}

	txHash := extractTxHashFromApprovalURL(approvalURL)
	if err := auth.WaitForApprovalSSE(ctx, session.sseClient, session.sseBaseURL, session.userID, txHash); err != nil {
		if logger != nil {
			logger.Warn("L3 approval SSE wait ended", "error", err)
		}
		return resp, err
	}

	retryResp, err := proxySessionToGateway(session, req)
	if err != nil {
		return resp, err
	}
	if logger != nil {
		logger.Info("L3 approval completed, proceeding with execution")
	}
	return retryResp, nil
}

// extractTxHashFromApprovalURL extracts the transaction hash from an approval
// URL path (e.g., "https://g8e.local:8443/api/v1/approve/abc123" -> "abc123").
func extractTxHashFromApprovalURL(approvalURL string) string {
	if approvalURL == "" {
		return ""
	}
	parsed, err := url.Parse(approvalURL)
	if err != nil {
		return ""
	}
	path := strings.TrimPrefix(parsed.Path, constants.APIPaths.ApprovePagePrefix)
	// Remove any trailing query or fragment
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		path = path[:idx]
	}
	return path
}

// ─── createMCPClient: kept for tests and external callers ───────────────────

// createMCPClient builds a plain mTLS HTTP client from config paths.
// Most callers should use buildGatewayConn instead, which also loads the
// delegated credential required for mTLS gateway authentication.
func createMCPClient(fileSvc fs.RuntimeFileService, cfg *config.Config) (*http.Client, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	caCert, err := auth.ReadTrustBundle(fileSvc, cfg)
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
		return JSONRPCResponse{}, fmt.Errorf("mcp: marshal request: %w", err)
	}

	httpResp, err := client.Post(gatewayURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("mcp: post request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return JSONRPCResponse{}, fmt.Errorf("%w: HTTP %d: %s", constants.ErrHTTPStatusError, httpResp.StatusCode, string(body))
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("mcp: decode response: %w", err)
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
	cfg, err := loadConfig("")
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}

	externalIP := network.GetExternalInterfaceIP()
	cmd.Printf("# Add this entry to /etc/hosts to enable %s resolution:\n", constants.GatewayInternalHostname)
	cmd.Printf("%s %s\n\n", externalIP, constants.GatewayInternalHostname)

	gatewayURL := fmt.Sprintf("https://%s:%d/mcp", constants.GatewayInternalHostname, constants.Ports.OperatorHttps)

	actualCertPath := filepath.ToSlash(cfg.CLICertFile())
	actualKeyPath := filepath.ToSlash(cfg.CLIKeyFile())
	actualCAPath := filepath.ToSlash(cfg.ResolvedTrustBundlePath())

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
	cfg, err := loadConfig("")
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}

	externalIP := network.GetExternalInterfaceIP()
	gatewayURL := fmt.Sprintf("https://%s:%d/mcp", externalIP, constants.Ports.OperatorHttps)

	actualCertPath := filepath.ToSlash(cfg.CLICertFile())
	actualKeyPath := filepath.ToSlash(cfg.CLIKeyFile())
	actualCAPath := filepath.ToSlash(cfg.ResolvedTrustBundlePath())

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
		Long: `Configure and integrate g8e with popular AI agent binaries (Claude, Codex,
Cursor, Devin, etc.) for seamless MCP tool access.

Subcommands:
  list    List all supported agent binaries
  show    Print MCP client configuration for a specific agent
  run     Launch an agent or wrap an external MCP server with g8e governance

For tools that don't support the agent wrapper, use 'g8e mcp agent show <agent>'
to display MCP client configurations (g8e.local mTLS, IP Address mTLS, Stdio
Transport), then copy the generated JSON to your agent's MCP settings file.`,
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
	cmd.Println("│ Apps: Claude Code, Codex, Goose, Gemini CLI")
	cmd.Println("│ Requires: DNS or /etc/hosts entry for g8e.local resolution")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigLocal(cmd); err != nil {
		return fmt.Errorf("mcp: print local config: %w", err)
	}
	cmd.Println()

	cmd.Println("┌─ IP Address (mTLS) ───────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Environments without DNS or for direct IP access")
	cmd.Println("│ Apps: Claude Code, Codex, Goose, Gemini CLI")
	cmd.Println("│ Requires: No DNS setup, uses external interface IP")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigIP(cmd); err != nil {
		return fmt.Errorf("mcp: print IP config: %w", err)
	}
	cmd.Println()

	cmd.Println("┌─ Stdio Transport ────────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Direct native tool access without gateway")
	cmd.Println("│ Apps: Claude Code, Codex, Goose, Gemini CLI")
	cmd.Println("│ Requires: g8e binary in PATH or full path in config")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigStdio(cmd); err != nil {
		return fmt.Errorf("mcp: print stdio config: %w", err)
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
		{string(constants.AgentBinaryGemini), "Google Gemini CLI"},
		{string(constants.AgentBinaryGoose), "Goose AI coding assistant"},
	}
}

// ─── agent run ──────────────────────────────────────────────────────────────

func agentRunCmd() *cobra.Command {
	return agentRunCmdWithConfig(newFileSvc)
}

func agentRunCmdWithConfig(fileSvcFactory func() (fs.RuntimeFileService, error)) *cobra.Command {
	var downstreamURL string
	var verify bool

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

  g8e mcp agent run codex         Launch OpenAI Codex with native tools disabled
                                  via --disallowed-tools, forcing all I/O through
                                  g8e MCP governance.

  g8e mcp agent run goose         Launch Goose with --no-profile flag (zero
                                  extensions), forcing all I/O through g8e MCP.

  g8e mcp agent run gemini        Launch Gemini CLI with tools.core set to an
                                  empty allowlist in settings.json, forcing all
                                  I/O through g8e MCP.

  Extra args are forwarded to the agent:
    g8e mcp agent run claude -- -p "fix the failing tests"

WRAP AN EXTERNAL MCP SERVER (governance reverse proxy):

  g8e mcp agent run -- npx -y @modelcontextprotocol/server-filesystem /home/user
  g8e mcp agent run --url http://localhost:3000

  Intercepts all tools/call requests, screens them through L1 doctrine
  (MITRE ATT&CK threat detection), and blocks violations before forwarding.

AUDIT TRAIL:
  When launching an agent, the agent is automatically enrolled as an external app
  identity (SPIFFE ID: spiffe://g8e.local/app/<agent-name>). All MCP tool calls
  are recorded in the audit vault with this app identity, enabling per-agent audit
  trails separate from human operator activity.

  Query audit events for a specific agent:
    g8e gw data audit list --operator-session-id spiffe://g8e.local/app/claude
    g8e gw data audit summary --operator-session-id spiffe://g8e.local/app/claude

DELEGATED CREDENTIAL MODEL:
  g8e uses a delegated credential model for agent identity. When an agent is
  launched, it receives a short-lived mTLS certificate that carries both
  identities:
  - App SPIFFE ID: spiffe://g8e.local/app/<agent-name> (the agent's policy identity)
  - Requestor User ID: spiffe://g8e.local/user/<id> (the human who launched the agent)

  Both identities are cryptographically bound in the certificate's URI SANs and
  presented at the TLS handshake. No trusted identity headers are used; the
  certificate IS the session. Every governed transaction includes both identities
  in the signed hash, ensuring end-to-end identity correctness and auditability.

L3 APPROVAL FLOW:
  When a tool requires L3 approval, g8e will:
  1. Automatically open your browser to the approval URL
  2. Wait for you to authorize via WebAuthn
  3. Retry the tool call automatically
  4. Return the result to the tool

For full L1-L5 governance (L2 consensus, L3 human approval via WebAuthn), start
the gateway and use 'g8e mcp stdio'.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPAgentRun(args, downstreamURL, verify, fileSvcFactory)
		},
	}

	cmd.Flags().StringVar(&downstreamURL, "url", "", "URL of the downstream MCP server")
	cmd.Flags().BoolVar(&verify, "verify", true, "Verify tool interception config before launching agent (use --verify=false to skip)")
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

// startGatewayIfNeeded starts the gateway if it is not already running and
// waits until it is healthy, then ensures CLI mTLS credentials exist.
// HTTP is only used here to poll the bootstrap health endpoint before mTLS
// certs have been issued — all subsequent traffic uses mTLS.
func startGatewayIfNeeded(fileSvcFactory func() (fs.RuntimeFileService, error)) error {
	_, err := loadConfig("")
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}

	fileSvc, err := fileSvcFactory()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}

	pm, err := platform.NewProcessManager(fileSvc)
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
			GatewayConfig: serve.GatewayConfig{
				Posture:          g8econfig.GatewayPosture("doctrine"),
				LogLevel:         "info",
				CertIdentityMode: "localhost",
			},
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
	MCPServers map[string]agentMCPServer `json:"mcpServers,omitempty"`
	Tools      *geminiToolsConfig        `json:"tools,omitempty"`
}

// gooseConfig represents the Goose config.yaml structure.
// Goose uses ~/.config/goose/config.yaml with an extensions map.
type gooseConfig struct {
	Extensions map[string]gooseExtension `yaml:"extensions"`
}

// gooseExtension represents a single extension entry in Goose's config.yaml.
type gooseExtension struct {
	Enabled bool           `yaml:"enabled"`
	Config  gooseExtConfig `yaml:"config"`
}

// gooseExtConfig holds the stdio transport configuration for a Goose extension.
type gooseExtConfig struct {
	Type        string   `yaml:"type"`
	Name        string   `yaml:"name"`
	Cmd         string   `yaml:"cmd"`
	Args        []string `yaml:"args"`
	Description string   `yaml:"description"`
	Timeout     int      `yaml:"timeout"`
}

// geminiToolsConfig controls Gemini's built-in tool enablement.
// When tools.core is set to any value, only the listed tools are enabled.
// An empty array means zero built-in tools — all actions must go through MCP.
type geminiToolsConfig struct {
	Core    []string `json:"core"`
	Exclude []string `json:"exclude,omitempty"`
}

// writeConfigWithBackup creates the config dir, backs up an existing config,
// and writes the new config JSON. Used by agents that follow the standard
// MCP-only governance pattern.
func writeConfigWithBackup(configDir, configPath string, configJSON []byte) (string, func(), error) {
	if err := os.MkdirAll(configDir, constants.PermDirStandard); err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	if _, err := os.Stat(configPath); err == nil {
		existing, err := os.ReadFile(configPath)
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrFileReadFailed, err)
		}
		if err := os.WriteFile(configPath+".bak", existing, constants.PermFilePublic); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
		}
		fmt.Fprintf(os.Stderr, "[g8e] Backing up existing config to %s\n", pathutil.ToSlash(configPath+".bak"))
	}
	displayPath := pathutil.ToSlash(configPath)
	fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s (g8e as only MCP server for governance)\n", displayPath)
	if err := os.WriteFile(configPath, configJSON, constants.PermFilePublic); err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}
	return configPath, nil, nil
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

	// Get home directory: prefer os.UserHomeDir() (cross-platform), fall back to HOME env
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		homeDir = os.Getenv("HOME")
		if homeDir == "" {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrMCPGetHomeDirectory, err)
		}
	}

	// Precompute all agent config paths to avoid repeated filepath.Join calls
	agentPaths := paths.GetAgentConfigPaths(homeDir)

	switch agentID {
	case string(constants.AgentBinaryGemini):
		// Gemini uses settings.json for configuration.
		// We add g8e as the MCP server and disable all built-in tools via tools.core.
		if err := os.MkdirAll(agentPaths.GeminiConfigDir, constants.PermDirStandard); err != nil {
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

		// Disable ALL built-in tools by setting tools.core to an empty array.
		// When tools.core is set to any value, only the listed tools are enabled.
		// An empty array means zero built-in tools — all actions must go through MCP.
		settings.Tools = &geminiToolsConfig{
			Core: []string{},
		}

		configJSON, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
		}

		displayPath := pathutil.ToSlash(agentPaths.GeminiConfigPath)
		fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s with native tools disabled\n", displayPath)
		if err := os.WriteFile(agentPaths.GeminiConfigPath, configJSON, constants.PermFilePublic); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
		}
		return agentPaths.GeminiConfigPath, nil, nil

	case string(constants.AgentBinaryGoose):
		// Goose uses ~/.config/goose/config.yaml with an extensions map.
		// We merge g8e into the existing config, preserving provider and other
		// settings. The --no-profile flag in agentLaunchArgs skips all profile
		// extensions (including the developer extension that provides shell/file
		// tools), and --with-extension on the command line loads g8e as the
		// sole MCP server for the session.
		g8eExt := map[string]any{
			"enabled": true,
			"config": map[string]any{
				"type":        "stdio",
				"name":        "g8e",
				"cmd":         binaryPath,
				"args":        []string{"mcp", "stdio"},
				"description": "g8e governance gateway",
				"timeout":     300,
			},
		}

		// Read existing config if present, preserving all fields (provider, etc.)
		var rawConfig map[string]any
		if existingData, err := os.ReadFile(agentPaths.GooseYAMLConfigPath); err == nil {
			if err := yaml.Unmarshal(existingData, &rawConfig); err != nil {
				return "", nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}
		}
		if rawConfig == nil {
			rawConfig = make(map[string]any)
		}

		// Get or create extensions map and add g8e
		extMap, _ := rawConfig["extensions"].(map[string]any)
		if extMap == nil {
			extMap = make(map[string]any)
		}
		extMap["g8e"] = g8eExt
		rawConfig["extensions"] = extMap

		configYAML, err := yaml.Marshal(rawConfig)
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
		}
		return writeConfigWithBackup(agentPaths.GooseYAMLConfigDir, agentPaths.GooseYAMLConfigPath, configYAML)

	default:
		// For agents that use CLI flags (claude, codex), write a temp config file.
		// The config is passed via --mcp-config and --strict-mcp-config CLI flags.
		tmpFile, err := os.CreateTemp("", "g8e-mcp-*.json")
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if _, err := tmpFile.Write(configJSON); err != nil {
			tmpFile.Close()
			return "", nil, fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
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
func launchAgentWithGovernance(agentID string, extraArgs []string, verify bool, fileSvcFactory func() (fs.RuntimeFileService, error)) error {
	if err := startGatewayIfNeeded(fileSvcFactory); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayNotReady, err)
	}

	cfg, err := loadConfig("")
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}

	fileSvc, err := fileSvcFactory()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}

	creds, err := ensureCLIAuth(fileSvc, cfg)
	if err != nil {
		return err
	}

	// Enroll the agent as an external app for audit trail attribution
	appID, appCert, appKey, err := auth.EnrollAgentApp(fileSvc, cfg, strings.ToLower(agentID))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}

	if err := ensurePasskeyRegistration(fileSvc, cfg, creds); err != nil {
		return err
	}

	_, cleanup, launchArgs, err := prepareAgentLaunch(agentID, verify)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return launchAgentProcess(agentID, extraArgs, launchArgs, cfg, appID, appCert, appKey)
}

// ensureCLIAuth loads existing CLI credentials or auto-enrolls if none exist.
func ensureCLIAuth(fileSvc fs.RuntimeFileService, cfg *config.Config) (*auth.Credentials, error) {
	creds, err := auth.LoadCredentials(fileSvc, cfg)
	if err != nil || creds == nil {
		fmt.Fprintf(os.Stderr, "[g8e] No CLI credentials found, enrolling...\n")
		if err := auth.EnrollCLI(fileSvc, cfg, false); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
		}
		fmt.Fprintf(os.Stderr, "[g8e] CLI enrolled successfully\n")
		creds, err = auth.LoadCredentials(fileSvc, cfg)
		if err != nil || creds == nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
		}
	}
	return creds, nil
}

// ensurePasskeyRegistration verifies a passkey is registered and auto-registers if missing.
func ensurePasskeyRegistration(fileSvc fs.RuntimeFileService, cfg *config.Config, creds *auth.Credentials) error {
	hasPasskey, err := auth.VerifyPasskeyRegistration(fileSvc, cfg, creds.UserID, creds.CLISessionID)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrNoPasskeysRegistered, err)
	}
	if !hasPasskey {
		fmt.Fprintf(os.Stderr, "[g8e] No passkey registered, starting passkey enrollment...\n")
		if err := auth.RegisterPasskeyViaBrowser(fileSvc, cfg, creds.UserID, creds.CLISessionID); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrPasskeyRegistrationFailed, err)
		}
	}
	return nil
}

// prepareAgentLaunch validates the agent binary, writes the agent config, computes launch
// args, and optionally verifies tool interception. Returns configPath, cleanup func, launchArgs.
func prepareAgentLaunch(agentID string, verify bool) (string, func(), []string, error) {
	agentBin, err := exec.LookPath(agentID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("%w: %q not found in PATH — is it installed?", constants.ErrAgentNotInPath, agentID)
	}
	_ = agentBin // used by caller via launchAgentProcess

	binaryPath, err := os.Executable()
	if err != nil {
		return "", nil, nil, fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	configPath, cleanup, err := WriteAgentConfig(agentID, binaryPath)
	if err != nil {
		return "", nil, nil, fmt.Errorf("mcp: write agent config: %w", err)
	}

	launchArgs, err := agentLaunchArgs(agentID, configPath, binaryPath)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return "", nil, nil, fmt.Errorf("mcp: get launch args: %w", err)
	}

	if verify {
		if err := verifyToolInterception(agentID, configPath, launchArgs); err != nil {
			if cleanup != nil {
				cleanup()
			}
			return "", nil, nil, fmt.Errorf("%w: %w", constants.ErrToolInterceptionVerification, err)
		}
		fmt.Fprintf(os.Stderr, "[g8e] Tool interception verified — native tools disabled, all I/O routed through g8e MCP\n")
	}

	return configPath, cleanup, launchArgs, nil
}

// launchAgentProcess spawns the agent binary with governance environment variables.
func launchAgentProcess(agentID string, extraArgs, launchArgs []string, cfg *config.Config, appID, appCert, appKey string) error {
	agentBin, err := exec.LookPath(agentID)
	if err != nil {
		return fmt.Errorf("%w: %q not found in PATH — is it installed?", constants.ErrAgentNotInPath, agentID)
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
		envG8ECABundle+"="+cfg.ResolvedTrustBundlePath(),
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
func agentLaunchArgs(agentID, mcpConfigPath, binaryPath string) ([]string, error) {
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
	case "goose":
		// --no-profile starts goose with zero profile extensions (no developer
		// extension, which provides shell/file tools). --with-extension loads
		// g8e as the sole MCP server for the session, since --no-profile also
		// skips extensions defined in config.yaml.
		return []string{"session", "--no-profile", "--with-extension", binaryPath + " mcp stdio"}, nil
	case "gemini":
		// Gemini uses `gemini mcp add` to register servers, no config file needed
		// Governance enforced by g8e being the only MCP server
		return []string{}, nil
	default:
		return nil, fmt.Errorf("%w: agent %q does not support full tool interception. g8e requires agents that can disable all built-in tools so every action routes through the governance gateway. Supported agents: claude, codex, goose, gemini", constants.ErrAgentNotSupported, agentID)
	}
}

// verifyToolInterception checks that each agent's tool-disabling mechanism was
// correctly applied before the agent process is launched. This catches config
// write failures, missing CLI flags, and config format drift.
func verifyToolInterception(agentID, configPath string, launchArgs []string) error {
	switch strings.ToLower(agentID) {
	case "claude", "codex":
		return verifyClaudeCodexInterception(configPath, launchArgs)
	case "goose":
		return verifyGooseInterception(configPath, launchArgs)
	case "gemini":
		return verifyGeminiInterception(configPath)
	default:
		return fmt.Errorf("%w: agent %q", constants.ErrAgentNotSupported, agentID)
	}
}

// verifyClaudeCodexInterception verifies that the temp MCP config file exists
// and contains the g8e MCP server, and that launch args include the required
// --disallowed-tools and --strict-mcp-config flags.
func verifyClaudeCodexInterception(configPath string, launchArgs []string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read mcp config %q: %w", configPath, err)
	}

	var cfg agentMCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse mcp config: %w", err)
	}

	if _, ok := cfg.MCPServers["g8e"]; !ok {
		return fmt.Errorf("mcp config missing g8e server entry")
	}

	hasStrict := false
	hasDisallowed := false
	for i, arg := range launchArgs {
		if arg == "--strict-mcp-config" {
			hasStrict = true
		}
		if arg == "--disallowed-tools" && i+1 < len(launchArgs) {
			hasDisallowed = true
		}
	}
	if !hasStrict {
		return fmt.Errorf("launch args missing --strict-mcp-config flag")
	}
	if !hasDisallowed {
		return fmt.Errorf("launch args missing --disallowed-tools flag")
	}

	return nil
}

// verifyGooseInterception verifies that launch args include --no-profile and
// --with-extension, and that the goose config.yaml file exists with the g8e
// extension entry.
func verifyGooseInterception(configPath string, launchArgs []string) error {
	hasNoProfile := false
	hasWithExtension := false
	for i, arg := range launchArgs {
		if arg == "--no-profile" {
			hasNoProfile = true
		}
		if arg == "--with-extension" && i+1 < len(launchArgs) {
			hasWithExtension = true
		}
	}
	if !hasNoProfile {
		return fmt.Errorf("launch args missing --no-profile flag")
	}
	if !hasWithExtension {
		return fmt.Errorf("launch args missing --with-extension flag")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read goose config %q: %w", configPath, err)
	}

	var cfg gooseConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse goose config: %w", err)
	}

	if _, ok := cfg.Extensions["g8e"]; !ok {
		return fmt.Errorf("goose config missing g8e extension entry")
	}

	return nil
}

// verifyGeminiInterception verifies that the Gemini settings.json file contains
// tools.core set to an empty array (disabling all built-in tools) and the g8e
// MCP server entry.
func verifyGeminiInterception(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read gemini settings %q: %w", configPath, err)
	}

	var settings geminiSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parse gemini settings: %w", err)
	}

	if settings.Tools == nil {
		return fmt.Errorf("gemini settings missing tools.core configuration")
	}
	if settings.Tools.Core == nil {
		return fmt.Errorf("gemini settings tools.core is null (expected empty array)")
	}
	if len(settings.Tools.Core) != 0 {
		return fmt.Errorf("gemini settings tools.core has %d entries (expected empty array to disable all built-in tools)", len(settings.Tools.Core))
	}

	if _, ok := settings.MCPServers["g8e"]; !ok {
		return fmt.Errorf("gemini settings missing g8e MCP server entry")
	}

	return nil
}

func runMCPAgentRun(args []string, downstreamURL string, verify bool, fileSvcFactory func() (fs.RuntimeFileService, error)) error {
	if downstreamURL == "" && len(args) == 0 {
		return fmt.Errorf("specify an agent name or MCP server\n\nLaunch an agent with governance:\n  g8e mcp agent run claude\n\nWrap an MCP server subprocess:\n  g8e mcp agent run -- npx -y @modelcontextprotocol/server-filesystem /\n\nWrap an HTTP MCP server:\n  g8e mcp agent run --url http://localhost:3000")
	}

	// Named agent → launch it with g8e as its governed MCP provider.
	if downstreamURL == "" && len(args) > 0 {
		firstArg := strings.ToLower(args[0])
		for _, a := range getSupportedAgents() {
			if strings.ToLower(a.ID) == firstArg {
				return launchAgentWithGovernance(a.ID, args[1:], verify, fileSvcFactory)
			}
		}
	}

	// MCP server (--url or -- command) → run as L1 governance reverse proxy.
	return runMCPProxy(args, downstreamURL)
}

// runMCPProxy runs an L1 governance reverse proxy in front of a downstream MCP
// server (HTTP via --url, or subprocess via -- command).
func runMCPProxy(args []string, downstreamURL string) error {
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
