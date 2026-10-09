// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/agent"
	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/mcptransport"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	g8econfig "github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/mcp"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
)

// mcpCmd is the parent command for MCP stdio operations.
func Cmd() *cobra.Command {
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
type JSONRPCRequest = mcptransport.JSONRPCRequest

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse = mcptransport.JSONRPCResponse

// RPCError represents a JSON-RPC error object.
type RPCError = mcptransport.JSONRPCError

// ToolsListResult is the result payload for tools/list.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// Tool represents a single MCP tool descriptor.
type Tool struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	InputSchema *mcp.InputSchema `json:"inputSchema"`
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
	return McpStdioCmdWithConfig(shared.NewFileSvc)
}

func McpStdioCmdWithConfig(fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stdio",
		Short: "Run MCP stdio server with full L1-L5 governance (proxies to gateway)",
		Long: `Run as an MCP stdio server that proxies all requests to the running gateway over
mTLS with a bound CLI session. Every tool call passes through the L1-L5 governance
pipeline. HTTP is never used for proxy traffic — it is reserved for CA bundle
discovery and health checks only.

This command is launched automatically by 'g8e mcp agent run'. When invoked
directly (e.g. from an IDE MCP config), credentials resolve in order:
  1. --app <name>: the owner-approved application identity enrolled under that name
  2. --client-cert/--client-key flags
  3. Enrolled CLI credentials on disk

The CA bundle defaults to the managed trust bundle (override with --ca-bundle) and
the gateway URL defaults to the local gateway (override with --gateway-url).

This command is a credential CONSUMER, not an enrollment UI. It does NOT enroll,
open a browser, install OS trust, or run a passkey ceremony. If credentials are
absent, run 'g8e auth enroll user', 'g8e auth enroll app <name>', or
'g8e mcp agent run' first to obtain them.

Cert and key must be supplied as a pair per tier; supplying only one half fails closed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPStdioProxy(cmd, args, fileSvcFactory)
		},
	}
	cmd.Flags().String(constants.Flag.ClientCert, "", "Path to CLI client certificate (mTLS)")
	cmd.Flags().String(constants.Flag.ClientKey, "", "Path to CLI client key (mTLS)")
	cmd.Flags().String(constants.Flag.CABundle, "", "Path to gateway CA bundle PEM")
	cmd.Flags().String(constants.Flag.GatewayURL, "", "Gateway MCP endpoint URL (https only, e.g. https://g8e.local:8443/mcp)")
	cmd.Flags().String(constants.Flag.App, "", "Name of enrolled platform application to authenticate session")
	return cmd
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
// For platform-enrolled application credentials, identity is cryptographically
// bound in the cert's URI SANs — the cert IS the session. For CLI credentials (the enrolled
// CLI cert on disk), the cert's URI SAN is a CLI SPIFFE URI that the gateway
// validates against the CLI session ID, so cliSessionID must be sent as the
// X-G8E-CLI-Session-ID header on every proxied request.
type gatewayConn struct {
	client      *http.Client
	gatewayURL  string
	openBrowser func(string) error

	// cliSessionID is set when the resolved credential tier is a CLI cert (client
	// flags or enrolled CLI disk cert). When non-empty, it is attached
	// as X-G8E-CLI-Session-ID on every proxied request so the gateway routes the
	// request through handleCLIAuth instead of falling through to handleAppAuth
	// (which would reject a CLI cert SAN) and returning 401.
	cliSessionID string

	// SSE fields for L3 approval notifications. Populated when CLI credentials
	// are available so the stdio proxy can subscribe to approval.completed events
	// instead of polling. The cliSessionID is sent as the X-G8E-CLI-Session-ID
	// header on the SSE subscription; user_id is derived from the mTLS cert.
	sseBaseURL string
	sseClient  *http.Client
}

// stdioCredentialFlags holds the credential overrides parsed from 'mcp stdio' flags.
// Empty fields mean "not supplied" and fall through to the next tier, ending at the
// enrolled CLI credentials on disk.
type stdioCredentialFlags struct {
	ClientCert string
	ClientKey  string
	CABundle   string
	GatewayURL string
	App        string
}

// parseStdioCredentialFlags reads the credential flags from the cobra command.
// The zero value is valid (all fields empty), so tests that do not exercise flags
// pass stdioCredentialFlags{}.
func parseStdioCredentialFlags(cmd *cobra.Command) (stdioCredentialFlags, error) {
	var f stdioCredentialFlags
	var err error
	if f.ClientCert, err = cmd.Flags().GetString(constants.Flag.ClientCert); err != nil {
		return f, fmt.Errorf("mcp: get %s flag: %w", constants.Flag.ClientCert, err)
	}
	if f.ClientKey, err = cmd.Flags().GetString(constants.Flag.ClientKey); err != nil {
		return f, fmt.Errorf("mcp: get %s flag: %w", constants.Flag.ClientKey, err)
	}
	if f.CABundle, err = cmd.Flags().GetString(constants.Flag.CABundle); err != nil {
		return f, fmt.Errorf("mcp: get %s flag: %w", constants.Flag.CABundle, err)
	}
	if f.GatewayURL, err = cmd.Flags().GetString(constants.Flag.GatewayURL); err != nil {
		return f, fmt.Errorf("mcp: get %s flag: %w", constants.Flag.GatewayURL, err)
	}
	if f.App, err = cmd.Flags().GetString(constants.Flag.App); err != nil {
		return f, fmt.Errorf("mcp: get %s flag: %w", constants.Flag.App, err)
	}
	return f, nil
}

// resolveCredentialPair picks the first complete (cert+key) pair from the ordered
// tiers. Exactly one half of any tier present returns ErrIncompleteCredentialPair.
// The name of the winning tier is returned so callers can distinguish application
// credentials (which carry identity in the cert URI SANs) from CLI credentials
// (which require an X-G8E-CLI-Session-ID header for gateway auth).
func resolveCredentialPair(tiers []struct{ cert, key, name string }) (string, string, string, error) {
	for _, t := range tiers {
		switch {
		case t.cert != "" && t.key != "":
			return t.cert, t.key, t.name, nil
		case t.cert != "" || t.key != "":
			return "", "", "", fmt.Errorf("%w: tier %s", constants.ErrIncompleteCredentialPair, t.name)
		}
	}
	return "", "", "", nil
}

// isCLICredentialTier reports whether the resolved credential tier carries a CLI
// SPIFFE URI SAN (validated by the gateway via handleCLIAuth) rather than an
// application SAN (validated via handleAppAuth). CLI tiers require the
// X-G8E-CLI-Session-ID header; app tiers do not.
func isCLICredentialTier(tierName string) bool {
	switch tierName {
	case "client flags", "CLI disk":
		return true
	default:
		return false
	}
}

// buildGatewayConn constructs a gatewayConn. Credentials resolve in order:
// 1. --app <name>: the platform-enrolled application's managed cert/key
// 2. --client-cert/--client-key flags
// 3. enrolled CLI cert/key on disk (cfg.CLICertFile/cfg.CLIKeyFile)
// Cert and key are resolved as pairs per tier; supplying only one half fails closed.
// CA bundle resolves: --ca-bundle flag → auth.ReadTrustBundle.
// Gateway URL resolves: --gateway-url flag → default https://g8e.local:8443/mcp.
func buildGatewayConn(fileSvc fs.RuntimeFileService, cfg *config.Config, flags stdioCredentialFlags) (*gatewayConn, error) {
	var appCert, appKey string
	if flags.App != "" {
		appCert = cfg.AppCertFile(flags.App)
		appKey = cfg.AppKeyFile(flags.App)
	}

	certFile, keyFile, tierName, err := resolveCredentialPair([]struct{ cert, key, name string }{
		{appCert, appKey, "app"},
		{flags.ClientCert, flags.ClientKey, "client flags"},
		{cfg.CLICertFile(), cfg.CLIKeyFile(), "CLI disk"},
	})
	if err != nil {
		return nil, err
	}

	var caBundleBytes []byte
	if flags.CABundle != "" {
		caBundleBytes, err = readCABundle(fileSvc, flags.CABundle)
	} else {
		caBundleBytes, err = auth.ReadTrustBundle(fileSvc, cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}

	gatewayURL := flags.GatewayURL
	if gatewayURL == "" {
		gatewayURL = fmt.Sprintf("https://%s:%d/mcp", constants.GatewayInternalHostname, constants.Ports.OperatorHttps)
	} else {
		if u, perr := url.Parse(gatewayURL); perr != nil {
			return nil, fmt.Errorf("%w: %s", constants.ErrMCPConfigGatewayURLInvalidScheme, gatewayURL)
		} else if u.Scheme != "https" {
			return nil, fmt.Errorf("%w: %s", constants.ErrMCPConfigGatewayURLInvalidScheme, gatewayURL)
		} else if u.Host == "" {
			return nil, fmt.Errorf("%w: %s", constants.ErrMCPConfigGatewayURLHostEmpty, gatewayURL)
		}
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

	session := &gatewayConn{
		openBrowser: platform.OpenBrowser,
		client: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
			Timeout:   30 * time.Second,
		},
		gatewayURL: gatewayURL,
	}

	// CLI-tier certs (client flags or enrolled CLI disk cert) carry a
	// CLI SPIFFE URI SAN that the gateway validates against the CLI session ID via
	// handleCLIAuth. The gateway only routes to handleCLIAuth when the X-G8E-CLI-
	// Session-ID header is present; without it, the request falls through to
	// handleAppAuth (which rejects a CLI SAN) and returns 401. Platform-enrolled
	// application certs (--app) carry an app SAN and authenticate via handleAppAuth
	// without any header, so we only attach the session ID for CLI tiers.
	//
	// Best-effort: tests and some edge cases use synthetic certs without enrolled
	// credentials on disk. If LoadCredentials fails, leave cliSessionID empty and
	// let the gateway reject the request — this preserves existing behavior for
	// application certs and fails closed for CLI certs without a session.
	if isCLICredentialTier(tierName) {
		if creds, cerr := auth.LoadCredentials(fileSvc, cfg); cerr == nil && creds != nil && creds.CLISessionID != "" {
			session.cliSessionID = creds.CLISessionID
		}
	}

	if !strings.Contains(gatewayURL, constants.GatewayInternalHostname) {
		return session, nil
	}

	if g8econfig.GatewayDialHost(constants.GatewayInternalHostname) == constants.GatewayInternalHostname {
		return session, nil
	}

	// tlsCfg pins ServerName to g8e.local, so dialing the loopback IP still
	// verifies the Gateway certificate.
	gatewayURL = fmt.Sprintf("https://%s:%d/mcp", constants.LocalhostIP, constants.Ports.OperatorHttps)
	session.gatewayURL = gatewayURL
	slog.Info("g8e.local DNS resolution failed, falling back to loopback", "ip", constants.LocalhostIP)

	return session, nil
}

// readCABundle reads a CA bundle from a path, preferring fileSvc.ReadFile when the
// path is under the .g8e/ runtime root, falling back to os.ReadFile for external paths.
func readCABundle(fileSvc fs.RuntimeFileService, caPath string) ([]byte, error) {
	if rel, err := fileSvc.Rel(caPath); err == nil {
		return fileSvc.ReadFile(context.Background(), rel)
	}
	return os.ReadFile(caPath)
}

func runMCPStdioProxy(cmd *cobra.Command, _ []string, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) error {
	cfg, err := shared.LoadConfig("")
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}

	fileSvc, err := fileSvcFactory("", slog.Default())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Parse credential flags from the stdio subcommand. Empty fields fall through
	// to the enrolled CLI credentials on disk.
	credFlags, err := parseStdioCredentialFlags(cmd)
	if err != nil {
		return err
	}

	// Build the mTLS gateway connection once. Identity is in the application cert's
	// URI SANs — no session object or headers. All proxy calls reuse this connection.
	conn, err := buildGatewayConn(fileSvc, cfg, credFlags)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayNotReady, err)
	}

	logger.Info("g8e MCP governance proxy starting",
		"gateway_url", conn.gatewayURL,
	)

	// Populate SSE fields for L3 approval notifications. The SSE client uses
	// the CLI cert (not the application cert) because the gateway's SSE auth
	// middleware validates CLI session ownership. The gateway URL is stripped
	// of the /mcp suffix to get the base URL for SSE endpoints.
	if creds, err := auth.LoadCredentials(fileSvc, cfg); err == nil && creds != nil && creds.CLISessionID != "" {
		if sseClient, err := auth.BuildMTLSClient(fileSvc, cfg, 0); err == nil {
			conn.sseClient = sseClient
			// Use OperatorPublicURL (g8e.local) for SSE to ensure TLS ServerName
			// matches the gateway cert SAN. Deriving from gatewayURL may produce
			// an IP-based URL that fails TLS verification.
			conn.sseBaseURL = strings.TrimSuffix(cfg.OperatorPublicURL(), "/")
			logger.Info("SSE approval notifications enabled", "cli_session_id", creds.CLISessionID)
		}
	}

	handler := mcptransport.HandlerFunc(func(ctx context.Context, req mcptransport.JSONRPCRequest) (mcptransport.JSONRPCResponse, error) {
		if req.Method == "initialize" {
			return mcptransport.NewInitializeResponse(req.ID, "g8e", "dev"), nil
		}
		logger.Info("Proxying MCP request", "method", req.Method, "id", req.ID)
		return proxySessionToGatewayWithRetryContext(ctx, conn, req, logger)
	})

	if err := mcptransport.ServeStdio(cmd.Context(), os.Stdin, os.Stdout, logger, handler); err != nil {
		logger.Error("MCP stdio proxy terminated with error", "error", err)
		return fmt.Errorf("mcp: serve stdio: %w", err)
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

	// For CLI-tier credentials, the cert's URI SAN is a CLI SPIFFE URI that the
	// gateway validates against the CLI session ID in handleCLIAuth. That path is
	// only reached when X-G8E-CLI-Session-ID is present; without it the gateway
	// falls through to handleAppAuth (which rejects a CLI SAN) and returns 401.
	// Application certs carry identity in their URI SANs and need no header.
	if session.cliSessionID != "" {
		httpReq.Header.Set(constants.HeaderCLISessionID, session.cliSessionID)
	}

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

	if session.sseClient == nil || session.sseBaseURL == "" || session.cliSessionID == "" {
		return resp, fmt.Errorf("L3 approval: %w", constants.ErrNotAuthenticated)
	}
	if session.openBrowser == nil {
		return resp, fmt.Errorf("L3 approval: browser opener: %w", constants.ErrMissingRequiredField)
	}

	if err := session.openBrowser(approvalURL); err != nil {
		if logger != nil {
			logger.Warn("Failed to auto-open browser", "error", err)
		}
		fmt.Fprintf(os.Stderr, "\n[g8e] Please visit: %s\n", approvalURL)
	}

	txHash := extractTxHashFromApprovalURL(approvalURL)
	if err := auth.WaitForApprovalSSE(ctx, session.sseClient, session.sseBaseURL, session.cliSessionID, txHash); err != nil {
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
	cfg, err := shared.LoadConfig("")
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
	cfg, err := shared.LoadConfig("")
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

For tools the launcher does not support, use 'g8e mcp agent show <agent>'
to display MCP client configurations (g8e.local mTLS, IP Address mTLS, Stdio
Transport), then copy the generated JSON to your agent's MCP settings file.`,
	}

	cmd.AddCommand(
		agentListCmd(),
		agentShowCmd(),
		agentVerifyCmd(),
		agentRunCmd(),
	)

	return cmd
}

func agentVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <agent>",
		Short: "Verify an agent's launcher config and tool lockdown without starting it",
		Long: `Write the agent's MCP config into an isolated temporary home directory, compute
its launch arguments, and run the registry's tool-interception checks. The agent
binary is never started and your real agent config is never touched, so this is
safe to run in CI without the agent installed.`,
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentVerify(cmd, args[0])
		},
	}
}

func runAgentVerify(cmd *cobra.Command, agentID string) error {
	integration, err := agent.Lookup(agentID)
	if err != nil {
		return fmt.Errorf("mcp: agent verify: %w", err)
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	if err := integration.VerifyIsolated(binaryPath); err != nil {
		return fmt.Errorf("mcp: agent verify: %w", err)
	}

	cmd.Printf("PASS %s: g8e is the only MCP server (%s tool lockdown)\n", integration.ID, integration.ToolLockdown)
	return nil
}

func agentListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List supported agent binaries",
		Long:  `List all popular AI agent binaries that g8e supports for MCP integration.`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("Supported Agent Binaries:")
			cmd.Println()
			for _, integration := range agent.All() {
				cmd.Printf("  %-13s - %s\n", integration.ID, integration.DisplayName)
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
	integration, err := agent.Lookup(agentID)
	if err != nil {
		return fmt.Errorf("mcp: agent show: %w", err)
	}

	cmd.Println("╔═════════════════════════════════════════════════════════════════════════")
	cmd.Println("║           g8e Gateway MCP Configurations")
	cmd.Printf("║  Use these configs to connect %s \n", integration.DisplayName)
	cmd.Println("║  to the g8e Gateway for agent orchestration and tool execution.")
	cmd.Println("╚═════════════════════════════════════════════════════════════════════════")
	cmd.Println()

	cmd.Println("┌─ g8e.local (mTLS) ─────────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Production environments with DNS configured")
	cmd.Println("│ Apps: Claude Code, Codex, Goose, Gemini CLI")
	cmd.Println("│ Requires: DNS or /etc/hosts entry for g8e.local resolution; Gateway started with --listen-host 0.0.0.0")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigLocal(cmd); err != nil {
		return fmt.Errorf("mcp: print local config: %w", err)
	}
	cmd.Println()

	cmd.Println("┌─ IP Address (mTLS) ───────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Environments without DNS or for direct IP access")
	cmd.Println("│ Apps: Claude Code, Codex, Goose, Gemini CLI")
	cmd.Println("│ Requires: No DNS setup, uses external interface IP; Gateway started with --listen-host 0.0.0.0")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigIP(cmd); err != nil {
		return fmt.Errorf("mcp: print IP config: %w", err)
	}
	cmd.Println()

	cmd.Println("┌─ Stdio Transport ────────────────────────────────────────────────────────────")
	cmd.Println("│ Use: Stdio bridge to gateway with L1–L5; requires running gateway + enrolled credentials")
	cmd.Println("│ Apps: Claude Code, Codex, Goose, Gemini CLI")
	cmd.Println("│ Requires: g8e binary in PATH or full path in config")
	cmd.Println("└─────────────────────────────────────────────────────────────────────────────")
	if err := printMCPConfigStdio(cmd); err != nil {
		return fmt.Errorf("mcp: print stdio config: %w", err)
	}

	return nil
}

// ─── agent run ──────────────────────────────────────────────────────────────

func agentRunCmd() *cobra.Command {
	return agentRunCmdWithConfig(shared.NewFileSvc, authcmd.NewDefaultEnrollmentCoordinator)
}

func agentRunCmdWithConfig(
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	enrollerFactory authcmd.EnrollerFactory,
) *cobra.Command {
	var verify bool
	var posture string

	cmd := &cobra.Command{
		Use:   "run <agent> [-- <args...>]",
		Short: "Launch an AI agent with g8e governance",
		Long: `Launch an AI agent configured to use g8e as its governed MCP provider.

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

  g8e mcp agent run devin         Launch Devin CLI with g8e as the only MCP
                                  server in ~/.config/devin/config.json,
                                  forcing all I/O through g8e MCP.

  Extra args are forwarded to the agent:
    g8e mcp agent run claude -- -p "fix the failing tests"

EXTERNAL MCP SERVERS:
  To govern external third-party MCP servers (stdio subprocess or HTTP), attach
  them directly to the gateway via downstream egress flags:
    g8e serve gateway --mcp-downstream-cmd npx --mcp-downstream-args '-y,@modelcontextprotocol/server-filesystem,/path'
    g8e serve gateway --mcp-downstream-url http://localhost:3000

GATEWAY POSTURE:
  If the launcher has to start the gateway, it uses --posture, else the posture of
  the previous managed gateway (its launch profile), else doctrine. Posture is
  immutable on a running gateway; an explicit --posture that does not match it is
  an error. Use 'g8e gw restart' (or 'g8e gw start --posture <p>') to change it.

AUDIT TRAIL:
  When launching an agent, the agent is automatically enrolled as an external app
  identity (SPIFFE ID: spiffe://g8e.local/app/<agent-name>). All MCP tool calls
  are recorded in the audit vault with this app identity, enabling per-agent audit
  trails separate from human operator activity.

  Query audit events for a specific agent:
    g8e gw data audit list --operator-session-id spiffe://g8e.local/app/claude
    g8e gw data audit summary --operator-session-id spiffe://g8e.local/app/claude

APPLICATION ENROLLMENT MODEL:
  Each agent is an application enrolled through the owner-approved platform
  enrollment protocol (the same one g8ee uses). The first launch submits an
  enrollment request and waits; approve it in the Console or with
  'g8e auth enroll approve <request-id> --yes'. The launcher never approves its own
  request. The approved mTLS certificate (7-day validity, revocable by request ID)
  carries both identities:
  - App SPIFFE ID: spiffe://g8e.local/app/<agent-name> (the agent's policy identity)
  - Approving User ID: spiffe://g8e.local/user/<id> (the human who approved the agent)

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
			return runMCPAgentRun(args, verify, posture, fileSvcFactory, enrollerFactory)
		},
	}

	cmd.Flags().BoolVar(&verify, "verify", true, "Verify tool interception config before launching agent (use --verify=false to skip)")
	cmd.Flags().StringVar(&posture, "posture", "", "Governance posture for a gateway the launcher starts: doctrine, consensus, ratify, or notary (default: the previous managed gateway's posture, else doctrine). A running gateway keeps its posture; an explicit value that differs fails closed")
	return cmd
}

// validateRequestedPosture rejects an unrecognized --posture value. An empty
// value means "not requested".
func validateRequestedPosture(requested string) error {
	if requested == "" {
		return nil
	}
	if _, ok := constants.GetGovernancePostureRequirements(requested); !ok {
		return fmt.Errorf("%w: %q (must be one of: %s, %s, %s, %s)", constants.ErrInvalidPosture, requested,
			constants.PostureDoctrine, constants.PostureConsensus, constants.PostureRatify, constants.PostureNotary)
	}
	return nil
}

// resolveGatewayLaunchConfig builds the config for a gateway the launcher must
// start. The persisted launch profile of the previous managed gateway is the
// base so its ports, origins, and downstream settings survive; without one the
// launcher starts a localhost gateway. The posture is the requested one, else
// the profile's, else the doctrine default. A corrupt profile fails closed.
func resolveGatewayLaunchConfig(fileSvc fs.RuntimeFileService, requested string) (serve.GatewayConfig, error) {
	cfg := serve.GatewayConfig{LogLevel: "info", CertIdentityMode: "localhost"}
	profile, err := serve.ReadLaunchProfile(fileSvc)
	switch {
	case err == nil:
		cfg = profile.Config
	case !errors.Is(err, constants.ErrLaunchProfileMissing):
		return serve.GatewayConfig{}, fmt.Errorf("mcp: read launch profile: %w", err)
	}

	switch {
	case requested != "":
		cfg.Posture = g8econfig.GatewayPosture(requested)
	case cfg.Posture == "":
		cfg.Posture = g8econfig.PostureDoctrine
	}
	return cfg, nil
}

// confirmRunningGatewayPosture checks an explicit --posture against the running
// gateway. Posture is immutable at runtime, so a request that cannot be shown
// to match fails closed rather than launching an agent under a different
// posture than the user asked for.
func confirmRunningGatewayPosture(fileSvc fs.RuntimeFileService, requested string) error {
	if requested == "" {
		return nil
	}
	profile, err := serve.ReadLaunchProfile(fileSvc)
	if err != nil {
		return fmt.Errorf("%w: cannot confirm the running gateway's posture is %q: %w", constants.ErrGatewayPostureMismatch, requested, err)
	}
	if string(profile.Config.Posture) != requested {
		return fmt.Errorf("%w: running gateway posture is %q, requested %q; restart it with 'g8e gw restart' or omit --posture",
			constants.ErrGatewayPostureMismatch, profile.Config.Posture, requested)
	}
	return nil
}

// startGatewayIfNeeded starts the gateway if it is not already running and
// waits until it is healthy, then ensures CLI mTLS credentials exist.
// HTTP is only used here to poll the bootstrap health endpoint before mTLS
// certs have been issued — all subsequent traffic uses mTLS.
//
// requestedPosture is the --posture flag value ("" when unset). It selects the
// posture of a gateway the launcher starts; a gateway that is already running
// keeps the posture it was started with, which must match an explicit request.
func startGatewayIfNeeded(fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error), requestedPosture string) error {
	if err := validateRequestedPosture(requestedPosture); err != nil {
		return err
	}

	_, err := shared.LoadConfig("")
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}

	fileSvc, err := fileSvcFactory("", slog.Default())
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
		if err := confirmRunningGatewayPosture(fileSvc, requestedPosture); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "[g8e] Gateway already running (PID %d)\n", pid)
	} else {
		gatewayCfg, err := resolveGatewayLaunchConfig(fileSvc, requestedPosture)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "[g8e] Starting gateway (posture: %s)...\n", gatewayCfg.Posture)
		startOpts := platform.OperatorStartOptions{GatewayConfig: gatewayCfg}
		if err := pm.StartOperator(&startOpts); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
		}
		if err := serve.WriteLaunchProfile(fileSvc, startOpts.GatewayConfig); err != nil {
			if stopErr := pm.StopOperator(); stopErr != nil {
				return fmt.Errorf("%w: persist launch profile: %w; stop untracked gateway: %w", constants.ErrInternal, err, stopErr)
			}
			return fmt.Errorf("%w: persist launch profile: %w", constants.ErrInternal, err)
		}

		// Poll plain HTTP health until the gateway is ready.
		// mTLS certs do not exist yet at this stage, so HTTP is the only
		// option. HTTP is only ever used here for this bootstrap health check.
		healthURL := network.LocalhostHTTPURL(startOpts.HTTPPort) + constants.APIPaths.Health
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

// launchAgentWithGovernance starts the gateway if needed, performs CLI auth,
// then launches the requested agent with 'g8e mcp stdio --app <agent>' as its sole
// MCP server. The stdio bridge loads the agent's managed application credentials by
// name; no credential material or paths are passed through the environment.
func launchAgentWithGovernance(agentID string, extraArgs []string, verify bool, posture string, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error), enrollerFactory authcmd.EnrollerFactory) error {
	if err := startGatewayIfNeeded(fileSvcFactory, posture); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayNotReady, err)
	}

	cfg, err := shared.LoadConfig("")
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}

	fileSvc, err := fileSvcFactory("", slog.Default())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}

	integration, err := agent.Lookup(agentID)
	if err != nil {
		return fmt.Errorf("mcp: launch agent: %w", err)
	}

	// Use the shared interactive enrollment coordinator. mcp agent run is an
	// interactive user-facing caller, so it uses the same trust and passkey
	// policy as `auth enroll user`. The coordinator inspects local state and
	// bootstraps, recovers, rotates, or reuses as needed. If a passkey
	// already exists, the coordinator's passkey ceremony is still run (it
	// is idempotent for an existing passkey); if no passkey exists, the
	// browser ceremony runs after system trust is installed.
	fmt.Fprintf(os.Stderr, "[g8e] Ensuring CLI credentials and passkey...\n")
	coordinator, err := enrollerFactory(func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}, fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}
	enrollResult, err := coordinator.Enroll(context.Background(), auth.EnrollmentOptions{})
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}
	if enrollResult.UserID == "" || enrollResult.CLISessionID == "" {
		return fmt.Errorf("%w: enrollment returned empty identity", constants.ErrEnrollmentFailed)
	}

	// Ensure the agent application is enrolled with the gateway
	agentAppName := strings.ToLower(agentID)
	appClient, err := auth.NewAppPlatformEnrollmentClient(agentAppName, fileSvc, cfg, slog.Default())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}

	if !appClient.HasValidIdentity() {
		fmt.Fprintf(os.Stderr, "[g8e] Enrolling application %q...\n", agentAppName)
		if _, err := appClient.Enroll(context.Background(), os.Stderr); err != nil {
			return fmt.Errorf("%w: failed to enroll agent app %q: %w", constants.ErrEnrollmentFailed, agentAppName, err)
		}
	}

	_, cleanup, launchArgs, err := prepareAgentLaunch(agentID, agentAppName, verify)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	return launchAgentProcess(integration, extraArgs, launchArgs)
}

// prepareAgentLaunch validates the agent binary, writes the agent config, computes launch
// args, and optionally verifies tool interception. The stdio bridge the agent spawns runs
// as application appName. Returns configPath, cleanup func, launchArgs.
func prepareAgentLaunch(agentID, appName string, verify bool) (string, func(), []string, error) {
	integration, err := agent.Lookup(agentID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("mcp: prepare launch: %w", err)
	}

	if _, err := exec.LookPath(integration.BinaryName); err != nil {
		return "", nil, nil, fmt.Errorf("%w: %q not found in PATH — is it installed?", constants.ErrAgentNotInPath, integration.BinaryName)
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return "", nil, nil, fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	homeDir, err := agent.ResolveHomeDir()
	if err != nil {
		return "", nil, nil, fmt.Errorf("mcp: prepare launch: %w", err)
	}

	prepared, err := integration.Prepare(homeDir, binaryPath, appName, verify)
	if err != nil {
		return "", nil, nil, fmt.Errorf("mcp: prepare launch: %w", err)
	}
	if verify {
		fmt.Fprintf(os.Stderr, "[g8e] Tool interception verified — g8e is the only MCP server (%s tool lockdown)\n", integration.ToolLockdown)
	}

	return prepared.ConfigPath, prepared.Cleanup, prepared.LaunchArgs, nil
}

// launchAgentProcess spawns the agent binary. The agent inherits the caller's
// environment unchanged; governance identity is carried by the generated MCP config.
func launchAgentProcess(integration agent.Integration, extraArgs, launchArgs []string) error {
	agentBin, err := exec.LookPath(integration.BinaryName)
	if err != nil {
		return fmt.Errorf("%w: %q not found in PATH — is it installed?", constants.ErrAgentNotInPath, integration.BinaryName)
	}

	if integration.ToolLockdown == agent.LockdownPartial {
		fmt.Fprintf(os.Stderr, "[g8e] WARNING: %s cannot disable its native tools; actions it takes without MCP bypass g8e governance\n", integration.ID)
	}
	fmt.Fprintf(os.Stderr, "[g8e] Launching %s with L1-L5 governance via gateway\n", integration.ID)

	agentCmd := exec.Command(agentBin, append(launchArgs, extraArgs...)...) //nolint:gosec
	agentCmd.Stdin = os.Stdin
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr
	// Don't set process group for interactive agents - it breaks terminal handling

	return agentCmd.Run()
}

func runMCPAgentRun(args []string, verify bool, posture string, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error), enrollerFactory authcmd.EnrollerFactory) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: specify an agent name to launch with g8e governance\n\nUsage:\n  g8e mcp agent run <agent> [-- <args...>]\n\nUse 'g8e mcp agent list' to see supported agents", constants.ErrAgentNotFound)
	}

	integration, err := agent.Lookup(args[0])
	if err != nil {
		return fmt.Errorf("mcp: agent run: %w", err)
	}
	return launchAgentWithGovernance(string(integration.ID), args[1:], verify, posture, fileSvcFactory, enrollerFactory)
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
