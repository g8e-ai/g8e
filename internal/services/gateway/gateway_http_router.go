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

package gateway

import (
	"net/http"

	"github.com/g8e-ai/g8e/internal/constants"
	httpSwagger "github.com/swaggo/http-swagger"
)

func (h *HTTPHandler) buildRouter() http.Handler {
	mux := http.NewServeMux()

	// Health endpoint (available on mTLS surface for state root queries)
	mux.HandleFunc(constants.APIPaths.Health, h.handleHealth)

	// State endpoint (for envelope state root binding)
	mux.HandleFunc(constants.APIPaths.State, h.handleState)

	// MCP Ingress routes with rate limiting
	mcpMux := http.NewServeMux()
	mcpMux.HandleFunc(constants.APIPaths.MCPEndpoint, h.mcp.HandleMCP)
	mcpMux.HandleFunc(constants.APIPaths.MCPToolsList, h.mcp.HandleToolsList)
	mcpMux.HandleFunc(constants.APIPaths.MCPToolsCall, h.mcp.HandleToolsCall)
	mcpMux.HandleFunc(constants.APIPaths.MCPToolsCallSSE, h.mcp.HandleToolsCallSSE)
	mcpMux.HandleFunc(constants.APIPaths.MCPResourcesList, h.mcp.HandleResourcesList)
	mcpMux.HandleFunc(constants.APIPaths.MCPResourcesRead, h.mcp.HandleResourcesRead)
	mcpMux.HandleFunc(constants.APIPaths.MCPPromptsList, h.mcp.HandlePromptsList)
	mcpMux.HandleFunc(constants.APIPaths.MCPPromptsGet, h.mcp.HandlePromptsGet)
	mcpMux.HandleFunc(constants.APIPaths.A2ACall, h.mcp.HandleA2aCall)

	// Wrap MCP/A2A with Rate Limiting
	mcpRateLimited := h.rateLimitMiddleware(mcpMux)

	// Apply JWT middleware only when JWKS is configured (for external IdP auth)
	// When JWKS is not configured, MCP/A2A routes go through main middleware which enforces mTLS + AppPolicy
	var mcpHandler http.Handler
	if h.auth != nil && h.auth.HasJWKS() {
		mcpHandler = h.auth.JWTAuthMiddleware(mcpRateLimited)
	} else {
		// When JWKS is not configured, MCP/A2A must use mTLS + AppPolicy via main middleware
		// The main middleware (enforced at router level) handles mTLS, identity binding, and AppPolicy
		mcpHandler = mcpRateLimited
	}

	// Rate-limited mux for core governance envelope (uses mTLS via main middleware)
	govEnvMux := http.NewServeMux()
	govEnvMux.HandleFunc(constants.APIPaths.GovernanceEnvelopes, h.handleGovernanceEnvelope)
	govEnvHandler := h.rateLimitMiddleware(govEnvMux)

	// Authenticated routes (require mTLS)
	mux.HandleFunc(constants.APIPaths.DataSettings, h.dbController.handleDataSettings)
	mux.HandleFunc(constants.APIPaths.Operators, h.operatorController.handleListOperators)
	mux.Handle(constants.APIPaths.OperatorsByID, http.HandlerFunc(h.operatorController.handleTerminateOperator))
	mux.HandleFunc(constants.APIPaths.OperatorsBind, h.operatorController.handleBindOperators)
	mux.HandleFunc(constants.APIPaths.OperatorsUnbind, h.operatorController.handleUnbindOperators)
	mux.HandleFunc(constants.APIPaths.OperatorsTarget, h.operatorController.handleSetTargetContext)
	mux.HandleFunc(constants.APIPaths.OperatorsReauth, h.operatorController.handleReauth)
	mux.HandleFunc(constants.APIPaths.GovernanceSigners, h.dbController.handleGovernanceSigners)
	mux.Handle(constants.APIPaths.GovernanceSignersByID, http.HandlerFunc(h.dbController.handleGovernanceSignerByID))
	mux.Handle(constants.APIPaths.AdminAppPoliciesBySigner, http.HandlerFunc(h.adminController.handleAppPolicySigner))
	mux.HandleFunc(constants.APIPaths.AdminAppsRevoke, h.adminController.handleRevokeApp)

	// Register rate-limited MCP routes with full paths
	mux.Handle(constants.APIPaths.GovernanceEnvelopes, govEnvHandler)
	mux.Handle(constants.APIPaths.MCPEndpoint, mcpHandler)
	mux.Handle(constants.APIPaths.MCPToolsList, mcpHandler)
	mux.Handle(constants.APIPaths.MCPToolsCall, mcpHandler)
	mux.Handle(constants.APIPaths.MCPToolsCallSSE, mcpHandler)
	mux.Handle(constants.APIPaths.MCPResourcesList, mcpHandler)
	mux.Handle(constants.APIPaths.MCPResourcesRead, mcpHandler)
	mux.Handle(constants.APIPaths.MCPPromptsList, mcpHandler)
	mux.Handle(constants.APIPaths.MCPPromptsGet, mcpHandler)
	mux.Handle(constants.APIPaths.A2ACall, mcpHandler)

	mux.HandleFunc(constants.APIPaths.AuditReceipts, h.dbController.handleAuditReceipts)
	mux.HandleFunc(constants.APIPaths.AuditReceiptsExport, h.dbController.handleAuditReceiptsExport)
	mux.HandleFunc(constants.APIPaths.AuditEvents, h.dbController.handleAuditEvents)
	mux.HandleFunc(constants.APIPaths.AuditSummary, h.dbController.handleAuditSummary)
	mux.HandleFunc(constants.APIPaths.AuditReport, h.dbController.handleAuditReport)

	// Internal SSE event bridge (used by g8e-compatible agentic ensembles to publish typed events
	// for browser/CLI subscribers to consume). Producers are authenticated by
	// mTLS app identity; consumers poll /api/v1/sse/events or stream /api/v1/sse/stream.
	mux.HandleFunc(constants.APIPaths.SSEPush, h.handleInternalSSEPush)
	mux.HandleFunc(constants.APIPaths.SSEEvents, h.handleInternalSSEEvents)
	mux.HandleFunc(constants.APIPaths.SSEStream, h.handleInternalSSEStream)
	mux.Handle(constants.APIPaths.DataDB, http.HandlerFunc(h.dbController.handleDataDB))
	mux.Handle(constants.APIPaths.KV, http.HandlerFunc(h.dbController.handleKV))
	mux.HandleFunc(constants.APIPaths.PubSubPublish, h.dbController.handlePubSubPublish)
	mux.Handle(constants.APIPaths.PubSubStream, h.auth.WebSocketAuth(http.HandlerFunc(h.pubsub.HandleWebSocket)))

	// PKI management routes (require mTLS)
	mux.HandleFunc(constants.APIPaths.PKICSRSign, h.pkiController.handlePKICSRSign)
	mux.HandleFunc(constants.APIPaths.PKIDevicesEnroll, h.pkiController.handlePKIDevicesEnroll)
	mux.HandleFunc(constants.APIPaths.PKIAppsDelegated, h.pkiController.handlePKIAppsDelegated)
	mux.HandleFunc(constants.APIPaths.PKICertificatesRevoke, h.pkiController.handlePKICertificatesRevoke)
	mux.HandleFunc(constants.APIPaths.PKIRevocationBundle, h.pkiController.handlePKIRevocationBundle)

	// User management routes (require mTLS)
	mux.HandleFunc(constants.APIPaths.Users, h.authController.handleUsers)

	// Passkey / L3 Brokerage Routes (require mTLS)
	mux.HandleFunc(constants.APIPaths.AuthPasskeysRegisterChallenge, h.authController.handleAuthPasskeysRegisterChallenge)
	mux.HandleFunc(constants.APIPaths.AuthPasskeysRegisterVerify, h.authController.handleAuthPasskeysRegisterVerify)
	mux.HandleFunc(constants.APIPaths.AuthPasskeysAuthenticateChallenge, h.authController.handleAuthPasskeysAuthenticateChallenge)
	mux.HandleFunc(constants.APIPaths.AuthPasskeysAuthenticateVerify, h.authController.handleAuthPasskeysAuthenticateVerify)
	mux.HandleFunc(constants.APIPaths.AuthPasskeys, h.authController.handleAuthPasskeys)
	mux.Handle(constants.APIPaths.AuthPasskeysByID, http.HandlerFunc(h.authController.handleAuthPasskeysRevoke))

	// Approval routes (require mTLS)
	mux.Handle(constants.APIPaths.ApprovalsByID, http.HandlerFunc(h.authController.handleApprovalAction))

	return h.pathTraversalGuard(h.auth.Middleware(mux))
}

func (h *HTTPHandler) buildPublicRouter() http.Handler {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc(constants.APIPaths.Health, h.handleBootstrapHealth)

	// State endpoint (for envelope state root binding)
	mux.HandleFunc(constants.APIPaths.State, h.handleState)

	// Swagger UI documentation
	mux.Handle("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.DocExpansion("none"),
	))
	mux.HandleFunc("/swagger/doc.json", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, constants.SwaggerFilePath)
	})

	// Bootstrap routes (CA discovery, trust scripts) - now on public HTTPS
	mux.HandleFunc(constants.APIPaths.WellKnownPKICABundle, h.pkiController.handlePKICABundle)
	mux.HandleFunc(constants.APIPaths.WellKnownPKIFingerprint, h.pkiController.handlePKIFingerprint)
	mux.HandleFunc(constants.APIPaths.PKICRL, h.pkiController.handlePKIRevocationBundle)
	mux.Handle(constants.APIPaths.DataBlobs, http.HandlerFunc(h.dbController.handleBlob))

	// Landing page and health
	mux.HandleFunc(constants.APIPaths.Landing, h.handleLandingPage)
	mux.HandleFunc(constants.APIPaths.AuthLoginVerify, h.authController.handlePublicAuthLoginVerify)
	mux.HandleFunc(constants.APIPaths.AuthLogout, h.authController.handlePublicAuthLogout)
	mux.HandleFunc(constants.APIPaths.AuthBootstrap, h.authController.handleLocalBootstrapWithURL)
	mux.HandleFunc(constants.APIPaths.AuthBootstrapStatus, h.authController.handleBootstrapStatus)
	mux.HandleFunc(constants.APIPaths.AuthCLIEnroll, h.authController.handleCLIEnrollment)
	mux.HandleFunc(constants.APIPaths.AuthDeviceEnroll, h.authController.handleDeviceEnrollment)
	mux.HandleFunc(constants.APIPaths.PKIAppsEnroll, h.pkiController.handlePKIAppsEnroll)
	mux.HandleFunc(constants.APIPaths.PKIDevicesEnroll, h.pkiController.handlePKIDevicesEnroll)

	// MCP/A2A Ingress routes with JWT authentication for remote clients
	mcpMux := http.NewServeMux()
	mcpMux.HandleFunc(constants.APIPaths.MCPEndpoint, h.mcp.HandleMCP)
	mcpMux.HandleFunc(constants.APIPaths.MCPToolsList, h.mcp.HandleToolsList)
	mcpMux.HandleFunc(constants.APIPaths.MCPToolsCall, h.mcp.HandleToolsCall)
	mcpMux.HandleFunc(constants.APIPaths.MCPToolsCallSSE, h.mcp.HandleToolsCallSSE)
	mcpMux.HandleFunc(constants.APIPaths.MCPResourcesList, h.mcp.HandleResourcesList)
	mcpMux.HandleFunc(constants.APIPaths.MCPResourcesRead, h.mcp.HandleResourcesRead)
	mcpMux.HandleFunc(constants.APIPaths.MCPPromptsList, h.mcp.HandlePromptsList)
	mcpMux.HandleFunc(constants.APIPaths.MCPPromptsGet, h.mcp.HandlePromptsGet)
	mcpMux.HandleFunc(constants.APIPaths.A2ACall, h.mcp.HandleA2aCall)

	// Wrap MCP/A2A with Rate Limiting
	mcpRateLimited := h.rateLimitMiddleware(mcpMux)

	// Apply JWT middleware when JWKS is configured (for external IdP auth)
	// When JWKS is not configured, MCP/A2A routes use mTLS via main middleware
	var mcpHandler http.Handler
	if h.auth != nil && h.auth.HasJWKS() {
		mcpHandler = h.auth.JWTAuthMiddleware(mcpRateLimited)
	} else {
		// When JWKS is not configured, MCP/A2A must use mTLS via main middleware
		mcpHandler = mcpRateLimited
	}

	// Register MCP routes unconditionally - they are protected by auth.Middleware (mTLS) or JWTAuthMiddleware
	mux.Handle(constants.APIPaths.MCPEndpoint, mcpHandler)
	mux.Handle(constants.APIPaths.MCPToolsList, mcpHandler)
	mux.Handle(constants.APIPaths.MCPToolsCall, mcpHandler)
	mux.Handle(constants.APIPaths.MCPToolsCallSSE, mcpHandler)
	mux.Handle(constants.APIPaths.MCPResourcesList, mcpHandler)
	mux.Handle(constants.APIPaths.MCPResourcesRead, mcpHandler)
	mux.Handle(constants.APIPaths.MCPPromptsList, mcpHandler)
	mux.Handle(constants.APIPaths.MCPPromptsGet, mcpHandler)
	mux.Handle(constants.APIPaths.A2ACall, mcpHandler)

	// JIT passkey bootstrap: allow first-credential registration via JWT
	// This unblocks OIDC/JIT users who have zero credentials and cannot reach WebSessionAuth
	if h.auth != nil && h.auth.HasJWKS() {
		jwtPasskeyMux := http.NewServeMux()
		jwtPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysJITRegisterChallenge, h.authController.handleAuthPasskeysRegisterChallenge)
		jwtPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysJITRegisterVerify, h.authController.handleAuthPasskeysRegisterVerify)
		mux.Handle(constants.APIPaths.AuthPasskeysJITRegisterChallenge, h.auth.JWTAuthMiddleware(jwtPasskeyMux))
		mux.Handle(constants.APIPaths.AuthPasskeysJITRegisterVerify, h.auth.JWTAuthMiddleware(jwtPasskeyMux))
	}

	// CLI passkey bootstrap: allow first-credential registration for CLI bootstrap flow
	// This is a public endpoint (no auth) for the initial bootstrap where no credentials exist yet
	cliPasskeyMux := http.NewServeMux()
	cliPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysCLIRegisterChallenge, h.authController.handleCLIPasskeyRegisterChallenge)
	cliPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysCLIRegisterVerify, h.authController.handleCLIPasskeyRegisterVerify)
	cliPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysCLIAuthenticateChallenge, h.authController.handleCLIPasskeyAuthenticateChallenge)
	cliPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysCLIAuthenticateVerify, h.authController.handleCLIPasskeyAuthenticateVerify)
	// Browser-based CLI bootstrap endpoints (create web session after registration)
	cliPasskeyMux.HandleFunc("/api/v1/auth/passkeys/cli-browser-register/challenge", h.authController.handleCLIBrowserPasskeyRegisterChallenge)
	cliPasskeyMux.HandleFunc("/api/v1/auth/passkeys/cli-browser-register/verify", h.authController.handleCLIBrowserPasskeyRegisterVerify)
	corsCLIPasskeyMux := h.corsMiddlewareForCLIPasskey(cliPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIRegisterChallenge, corsCLIPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIRegisterVerify, corsCLIPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIAuthenticateChallenge, corsCLIPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIAuthenticateVerify, corsCLIPasskeyMux)
	mux.Handle("/api/v1/auth/passkeys/cli-browser-register/challenge", corsCLIPasskeyMux)
	mux.Handle("/api/v1/auth/passkeys/cli-browser-register/verify", corsCLIPasskeyMux)

	// Browser-facing data routes (require web session cookie)
	authedMux := http.NewServeMux()
	authedMux.HandleFunc(constants.APIPaths.UsersMe, h.authController.handleUserMe)
	authedMux.HandleFunc(constants.APIPaths.AuthSessionsMe, h.authController.handleWebSession)

	// OOB Approval UI for suspended MCP/A2A transactions
	mux.HandleFunc(constants.APIPaths.ApprovePage, h.authController.handleApprovalPage)
	authedMux.Handle(constants.APIPaths.ApprovalsByID, http.HandlerFunc(h.authController.handleApprovalAction))
	authedMux.HandleFunc(constants.APIPaths.Approvals, h.authController.handleListSuspendedTransactions)

	// Wrap authed routes in WebSessionAuth middleware
	mux.Handle(constants.APIPaths.Users[:len(constants.APIPaths.Users)-1], h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle(constants.APIPaths.AuthSessionsMe[:len(constants.APIPaths.AuthSessionsMe)-len("/me")], h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle(constants.APIPaths.Approvals, h.auth.WebSessionAuth(authedMux, h.db))

	return h.pathTraversalGuard(h.auth.Middleware(mux))
}

func (h *HTTPHandler) buildHTTPRouter() http.Handler {
	mux := http.NewServeMux()

	// Health check - available on HTTP port for initialization monitoring
	mux.HandleFunc(constants.APIPaths.Health, h.handleBootstrapHealth)

	// State endpoint (for envelope state root binding)
	mux.HandleFunc(constants.APIPaths.State, h.handleState)

	// Bootstrap routes - plain HTTP for initial CA discovery and bootstrap
	mux.HandleFunc(constants.APIPaths.AuthBootstrap, h.authController.handleLocalBootstrapWithURL)
	mux.HandleFunc(constants.APIPaths.AuthBootstrapStatus, h.authController.handleBootstrapStatus)
	mux.HandleFunc(constants.APIPaths.AuthCLIEnroll, h.authController.handleCLIEnrollment)
	mux.HandleFunc(constants.APIPaths.AuthDeviceEnroll, h.authController.handleDeviceEnrollment)
	mux.HandleFunc(constants.APIPaths.PKIAppsEnroll, h.pkiController.handlePKIAppsEnroll)
	mux.HandleFunc(constants.APIPaths.WellKnownPKICABundle, h.pkiController.handlePKICABundle)
	mux.HandleFunc(constants.APIPaths.WellKnownPKIFingerprint, h.pkiController.handlePKIFingerprint)
	mux.HandleFunc(constants.APIPaths.BootstrapCALinux, h.pkiController.handleTrustScriptLinux)
	mux.HandleFunc(constants.APIPaths.BootstrapCAMacos, h.pkiController.handleTrustScriptMacos)
	mux.HandleFunc(constants.APIPaths.BootstrapCAWindows, h.pkiController.handleTrustScriptWindows)
	mux.HandleFunc("/.well-known/g8e/pki/trust-windows", h.pkiController.handleTrustScriptWindowsAlias)
	mux.HandleFunc("/.well-known/g8e/bin/", h.pkiController.handleNodeBinaryDownload)
	mux.HandleFunc(constants.APIPaths.DeployScriptLinux, h.pkiController.handleDeployScriptLinux)
	mux.HandleFunc(constants.APIPaths.DeployScriptWindows, h.pkiController.handleDeployScriptWindows)

	// CLI passkey bootstrap: allow first-credential registration for CLI bootstrap flow
	// This is a public endpoint (no auth) for the initial bootstrap where no credentials exist yet
	cliPasskeyMux := http.NewServeMux()
	cliPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysCLIRegisterChallenge, h.authController.handleCLIPasskeyRegisterChallenge)
	cliPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysCLIRegisterVerify, h.authController.handleCLIPasskeyRegisterVerify)
	cliPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysCLIAuthenticateChallenge, h.authController.handleCLIPasskeyAuthenticateChallenge)
	cliPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysCLIAuthenticateVerify, h.authController.handleCLIPasskeyAuthenticateVerify)
	// Browser-based CLI bootstrap endpoints (create web session after registration)
	cliPasskeyMux.HandleFunc("/api/v1/auth/passkeys/cli-browser-register/challenge", h.authController.handleCLIBrowserPasskeyRegisterChallenge)
	cliPasskeyMux.HandleFunc("/api/v1/auth/passkeys/cli-browser-register/verify", h.authController.handleCLIBrowserPasskeyRegisterVerify)
	corsCLIPasskeyMux := h.corsMiddlewareForCLIPasskey(cliPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIRegisterChallenge, corsCLIPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIRegisterVerify, corsCLIPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIAuthenticateChallenge, corsCLIPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIAuthenticateVerify, corsCLIPasskeyMux)
	mux.Handle("/api/v1/auth/passkeys/cli-browser-register/challenge", corsCLIPasskeyMux)
	mux.Handle("/api/v1/auth/passkeys/cli-browser-register/verify", corsCLIPasskeyMux)

	// Wrap with rate limiting
	return h.pathTraversalGuard(h.rateLimitMiddleware(mux))
}

func (h *HTTPHandler) buildMCPHttpRouter() http.Handler {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc(constants.APIPaths.Health, h.handleBootstrapHealth)

	// Unified MCP Streamable HTTP endpoint for standard MCP clients (e.g.
	// Claude Code custom connectors). This is the canonical single-URL
	// JSON-RPC surface; the per-method routes below remain for compatibility.
	mux.HandleFunc(constants.APIPaths.MCPEndpoint, h.mcp.HandleMCP)

	// MCP-only routes on plain HTTP for HTTP MCP calls
	mux.HandleFunc(constants.APIPaths.MCPToolsList, h.mcp.HandleToolsList)
	mux.HandleFunc(constants.APIPaths.MCPToolsCall, h.mcp.HandleToolsCall)
	mux.HandleFunc(constants.APIPaths.MCPToolsCallSSE, h.mcp.HandleToolsCallSSE)
	mux.HandleFunc(constants.APIPaths.MCPResourcesList, h.mcp.HandleResourcesList)
	mux.HandleFunc(constants.APIPaths.MCPResourcesRead, h.mcp.HandleResourcesRead)
	mux.HandleFunc(constants.APIPaths.MCPPromptsList, h.mcp.HandlePromptsList)
	mux.HandleFunc(constants.APIPaths.MCPPromptsGet, h.mcp.HandlePromptsGet)
	mux.HandleFunc(constants.APIPaths.A2ACall, h.mcp.HandleA2aCall)

	// Wrap with Origin validation (DNS-rebinding protection per the MCP
	// Streamable HTTP transport spec) and rate limiting.
	return h.pathTraversalGuard(h.mcpOriginGuard(h.rateLimitMiddleware(mux)))
}
