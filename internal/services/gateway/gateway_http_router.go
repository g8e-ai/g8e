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
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/gateway/console"
)

func (h *HTTPHandler) buildPublicRouter() http.Handler {
	mux := http.NewServeMux()

	// Health endpoint (full health check: platform_settings + state root)
	mux.HandleFunc(constants.APIPaths.Health, h.handleHealth)

	// State endpoint (for envelope state root binding)
	mux.HandleFunc(constants.APIPaths.State, h.handleState)

	// Swagger UI documentation
	mux.HandleFunc("/swagger/", handleSwaggerUI)
	mux.HandleFunc("/swagger/index.html", handleSwaggerUI)
	mux.HandleFunc("/swagger/doc.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, paths.SwaggerFilePath)
	})

	// Bootstrap routes (CA discovery, trust scripts) - now on public HTTPS
	mux.HandleFunc(constants.APIPaths.WellKnownPKICABundle, h.pkiController.handlePKICABundle)
	mux.HandleFunc(constants.APIPaths.WellKnownPKIFingerprint, h.pkiController.handlePKIFingerprint)
	mux.HandleFunc(constants.APIPaths.PKICRL, h.pkiController.handlePKIRevocationBundle)
	mux.Handle(constants.APIPaths.DataBlobs, http.HandlerFunc(h.dbController.handleBlob))

	// Console SPA (public, no auth required)
	mux.Handle("/console/", http.StripPrefix("/console", console.Handler()))

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

	// MCP/A2A ingress (rate-limited, JWT when JWKS is configured, else mTLS via main middleware)
	mcpHandler := h.buildMCPHandler()
	registerMCPRoutes(mux, mcpHandler)

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
	// Browser-based passkey authenticate endpoints (create web session after auth)
	cliPasskeyMux.HandleFunc("/api/v1/auth/passkeys/browser/authenticate/challenge", h.authController.handleCLIBrowserPasskeyAuthenticateChallenge)
	cliPasskeyMux.HandleFunc("/api/v1/auth/passkeys/browser/authenticate/verify", h.authController.handleCLIBrowserPasskeyAuthenticateVerify)
	corsCLIPasskeyMux := h.corsMiddlewareForCLIPasskey(cliPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIRegisterChallenge, corsCLIPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIRegisterVerify, corsCLIPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIAuthenticateChallenge, corsCLIPasskeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysCLIAuthenticateVerify, corsCLIPasskeyMux)
	mux.Handle("/api/v1/auth/passkeys/cli-browser-register/challenge", corsCLIPasskeyMux)
	mux.Handle("/api/v1/auth/passkeys/cli-browser-register/verify", corsCLIPasskeyMux)
	mux.Handle("/api/v1/auth/passkeys/browser/authenticate/challenge", corsCLIPasskeyMux)
	mux.Handle("/api/v1/auth/passkeys/browser/authenticate/verify", corsCLIPasskeyMux)

	// mTLS-only routes (merged from buildRouter)
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
	mux.HandleFunc(constants.APIPaths.AdminTribunals, h.adminController.handleTribunals)
	mux.Handle(constants.APIPaths.AdminTribunalsByID, http.HandlerFunc(h.adminController.handleDeleteTribunal))

	// Tribunal deliberate endpoint (mTLS-guarded, enrolled principal)
	if h.tribunal != nil {
		mux.HandleFunc(constants.APIPaths.TribunalDeliberate, h.tribunal.HandleDeliberate)
	}

	// Rate-limited governance envelope
	govEnvMux := http.NewServeMux()
	govEnvMux.HandleFunc(constants.APIPaths.GovernanceEnvelopes, h.handleGovernanceEnvelope)
	govEnvHandler := h.rateLimitMiddleware(govEnvMux)
	mux.Handle(constants.APIPaths.GovernanceEnvelopes, govEnvHandler)

	mux.HandleFunc(constants.APIPaths.AuditReceipts, h.dbController.handleAuditReceipts)
	mux.HandleFunc(constants.APIPaths.AuditReceiptsExport, h.dbController.handleAuditReceiptsExport)
	mux.HandleFunc(constants.APIPaths.AuditEvents, h.dbController.handleAuditEvents)
	mux.HandleFunc(constants.APIPaths.AuditSummary, h.dbController.handleAuditSummary)
	mux.HandleFunc(constants.APIPaths.AuditReport, h.dbController.handleAuditReport)

	mux.HandleFunc(constants.APIPaths.SSEPush, h.handleInternalSSEPush)
	mux.HandleFunc(constants.APIPaths.SSEEvents, h.handleInternalSSEEvents)
	mux.HandleFunc(constants.APIPaths.SSEStream, h.handleInternalSSEStream)
	mux.Handle(constants.APIPaths.DataDB, http.HandlerFunc(h.dbController.handleDataDB))
	mux.Handle(constants.APIPaths.KV, http.HandlerFunc(h.dbController.handleKV))
	mux.HandleFunc(constants.APIPaths.PubSubPublish, h.dbController.handlePubSubPublish)
	mux.Handle(constants.APIPaths.PubSubStream, h.auth.WebSocketAuth(http.HandlerFunc(h.pubsub.HandleWebSocket)))

	// PKI management routes (require mTLS)
	mux.HandleFunc(constants.APIPaths.PKICSRSign, h.pkiController.handlePKICSRSign)
	mux.HandleFunc(constants.APIPaths.PKIAppsDelegated, h.pkiController.handlePKIAppsDelegated)
	mux.HandleFunc(constants.APIPaths.PKICertificatesRevoke, h.pkiController.handlePKICertificatesRevoke)
	mux.HandleFunc(constants.APIPaths.PKIRevocationBundle, h.pkiController.handlePKIRevocationBundle)

	// User management routes (require mTLS)
	mux.HandleFunc(constants.APIPaths.Users, h.authController.handleUsers)

	// Passkey / L3 Brokerage Routes (require mTLS) - register/challenge/verify variants
	mux.HandleFunc(constants.APIPaths.AuthPasskeysRegisterChallenge, h.authController.handleAuthPasskeysRegisterChallenge)
	mux.HandleFunc(constants.APIPaths.AuthPasskeysRegisterVerify, h.authController.handleAuthPasskeysRegisterVerify)
	mux.HandleFunc(constants.APIPaths.AuthPasskeysAuthenticateChallenge, h.authController.handleAuthPasskeysAuthenticateChallenge)
	mux.HandleFunc(constants.APIPaths.AuthPasskeysAuthenticateVerify, h.authController.handleAuthPasskeysAuthenticateVerify)

	// Browser-facing data routes (require web session cookie)
	authedMux := http.NewServeMux()
	authedMux.HandleFunc(constants.APIPaths.UsersMe, h.authController.handleUserMe)
	authedMux.HandleFunc(constants.APIPaths.AuthSessionsMe, h.authController.handleWebSession)

	// OOB Approval UI for suspended MCP/A2A transactions
	mux.HandleFunc(constants.APIPaths.ApprovePage, h.authController.handleApprovalPage)
	authedMux.Handle(constants.APIPaths.ApprovalsByID, http.HandlerFunc(h.authController.handleApprovalAction))
	authedMux.HandleFunc(constants.APIPaths.Approvals, h.authController.handleListSuspendedTransactions)

	// Passkey management (list, revoke) under WebSessionAuth
	authedMux.HandleFunc(constants.APIPaths.AuthPasskeys, h.authController.handleAuthPasskeys)
	authedMux.Handle(constants.APIPaths.AuthPasskeysByID, http.HandlerFunc(h.authController.handleAuthPasskeysRevoke))

	// Wrap authed routes in WebSessionAuth middleware
	mux.Handle("/api/v1/users/", h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle("/api/v1/auth/sessions/", h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle("/api/v1/approvals", h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle("/api/v1/approvals/", h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle("/api/v1/auth/passkeys", h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle("/api/v1/auth/passkeys/", h.auth.WebSessionAuth(authedMux, h.db))

	return h.pathTraversalGuard(h.auth.Middleware(mux))
}

// buildMCPHandler creates the rate-limited, auth-wrapped handler for all MCP/A2A ingress routes.
// When JWKS is configured, JWT middleware is applied for external IdP auth; otherwise MCP/A2A
// routes rely on mTLS + AppPolicy enforced by the main middleware at the router level.
func (h *HTTPHandler) buildMCPHandler() http.Handler {
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

	mcpRateLimited := h.rateLimitMiddleware(mcpMux)

	if h.auth != nil && h.auth.HasJWKS() {
		return h.auth.JWTAuthMiddleware(mcpRateLimited)
	}
	return mcpRateLimited
}

// registerMCPRoutes registers all MCP/A2A ingress paths on the given mux with the provided handler.
func registerMCPRoutes(mux *http.ServeMux, handler http.Handler) {
	mux.Handle(constants.APIPaths.MCPEndpoint, handler)
	mux.Handle(constants.APIPaths.MCPToolsList, handler)
	mux.Handle(constants.APIPaths.MCPToolsCall, handler)
	mux.Handle(constants.APIPaths.MCPToolsCallSSE, handler)
	mux.Handle(constants.APIPaths.MCPResourcesList, handler)
	mux.Handle(constants.APIPaths.MCPResourcesRead, handler)
	mux.Handle(constants.APIPaths.MCPPromptsList, handler)
	mux.Handle(constants.APIPaths.MCPPromptsGet, handler)
	mux.Handle(constants.APIPaths.A2ACall, handler)
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
	mux.HandleFunc(constants.APIPaths.PKICSRSign, h.pkiController.handlePKICSRSign)
	mux.HandleFunc(constants.APIPaths.WellKnownPKICABundle, h.pkiController.handlePKICABundle)
	mux.HandleFunc(constants.APIPaths.WellKnownPKIFingerprint, h.pkiController.handlePKIFingerprint)
	mux.HandleFunc(constants.APIPaths.BootstrapCALinux, h.pkiController.handleTrustScriptLinux)
	mux.HandleFunc(constants.APIPaths.BootstrapCAMacos, h.pkiController.handleTrustScriptMacos)
	mux.HandleFunc(constants.APIPaths.BootstrapCAWindows, h.pkiController.handleTrustScriptWindows)
	mux.HandleFunc("/.well-known/g8e/pki/trust-windows", h.pkiController.handleTrustScriptWindowsAlias)
	mux.HandleFunc("/.well-known/g8e/bin/", h.pkiController.handleNodeBinaryDownload)
	mux.HandleFunc(constants.APIPaths.DeployScriptLinux, h.pkiController.handleDeployScriptLinux)
	mux.HandleFunc(constants.APIPaths.DeployScriptWindows, h.pkiController.handleDeployScriptWindows)

	// Catch-all redirect to HTTPS for all other routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		if !isSafeHost(host, h.cfg) {
			if h.cfg != nil && h.cfg.Endpoint != "" {
				host = h.cfg.Endpoint
			} else {
				host = "localhost"
			}
		}

		var httpsPort int
		if h.cfg != nil && h.cfg.Gateway.HTTPSPort != 0 {
			httpsPort = h.cfg.Gateway.HTTPSPort
		} else {
			httpsPort = constants.Ports.OperatorHttps
		}

		var targetHost string
		if httpsPort == 443 {
			targetHost = host
		} else {
			targetHost = net.JoinHostPort(host, strconv.Itoa(httpsPort))
		}

		path := r.URL.Path
		if path == "" {
			path = "/"
		}
		for strings.HasPrefix(path, "//") {
			path = path[1:]
		}

		target := "https://" + targetHost + path
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}

		http.Redirect(w, r, target, http.StatusMovedPermanently) // #nosec G710
	})

	// Wrap with rate limiting
	return h.pathTraversalGuard(h.rateLimitMiddleware(mux))
}

// isSafeHost checks if the requested host is a recognized local, private, or configured endpoint.
func isSafeHost(host string, cfg *config.Config) bool {
	if host == "" {
		return false
	}

	for i := 0; i < len(host); i++ {
		c := host[i]
		isAlphanumeric := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		isSpecial := c == '.' || c == '-' || c == '[' || c == ']' || c == ':'
		if !isAlphanumeric && !isSpecial {
			return false
		}
	}

	if host == "localhost" {
		return true
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return true
		}
		if isPrivateIP(ip) {
			return true
		}
	}

	if cfg != nil {
		if cfg.Endpoint != "" {
			endpointHost := cfg.Endpoint
			if h, _, err := net.SplitHostPort(endpointHost); err == nil {
				endpointHost = h
			}
			if strings.EqualFold(host, endpointHost) {
				return true
			}
		}

		if cfg.Gateway.PublicBaseURL != "" {
			if u, err := url.Parse(cfg.Gateway.PublicBaseURL); err == nil {
				publicHost := u.Hostname()
				if strings.EqualFold(host, publicHost) {
					return true
				}
			}
		}
	}

	return false
}

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>g8e Gateway API - Swagger UI</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
<style>body{margin:0}</style>
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
window.onload=function(){
  SwaggerUIBundle({
    url:"/swagger/doc.json",
    dom_id:"#swagger-ui",
    docExpansion:"none",
    deepLinking:true
  });
};
</script>
</body>
</html>`

func handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIHTML))
}
