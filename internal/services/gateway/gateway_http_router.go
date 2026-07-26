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
	"strings"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/gateway/console"
)

func (h *HTTPHandler) buildPublicRouter() http.Handler {
	mux := http.NewServeMux()

	// Health endpoint (full health check: platform_settings + state root)
	mux.HandleFunc(constants.APIPaths.Health, h.healthController.handleHealth)

	// State endpoint (for envelope state root binding)
	mux.HandleFunc(constants.APIPaths.State, h.healthController.handleState)

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
	consoleHandler := console.Handler()
	mux.Handle(constants.APIPaths.ConsolePrefix, http.StripPrefix(strings.TrimSuffix(constants.APIPaths.ConsolePrefix, "/"), consoleHandler))

	// Landing page and health
	mux.HandleFunc(constants.APIPaths.Landing, h.healthController.handleLandingPage)
	mux.HandleFunc(constants.APIPaths.AuthLogout, h.sessionController.handlePublicAuthLogout)
	mux.HandleFunc(constants.APIPaths.AuthBootstrap, h.bootstrapController.handleLocalBootstrapWithURL)
	mux.HandleFunc(constants.APIPaths.AuthBootstrapStatus, h.bootstrapController.handleBootstrapStatus)
	mux.HandleFunc(constants.APIPaths.AuthCLIEnroll, h.bootstrapController.handleCLIEnrollment)
	mux.HandleFunc(constants.APIPaths.AuthDeviceEnroll, h.bootstrapController.handleDeviceEnrollment)
	mux.HandleFunc(constants.APIPaths.PKIAppsEnroll, h.pkiController.handlePKIAppsEnroll)
	mux.HandleFunc(constants.APIPaths.PKIDevicesEnroll, h.pkiController.handlePKIDevicesEnroll)

	// Enrollment token validation (public — the token itself is the credential)
	mux.HandleFunc(constants.APIPaths.AuthEnrollmentTokenValidate, h.enrollmentTokenController.handleEnrollmentTokenValidate)

	// MCP/A2A ingress (rate-limited, JWT when JWKS is configured, else mTLS via main middleware)
	mcpHandler := h.buildMCPHandler()
	registerMCPRoutes(mux, mcpHandler)

	jitCfg := passkeyHandlerConfig{source: sourceJWT, enforceFirstCredentialOnly: true, requireAuthenticatedUser: true, enforceSessionUserBinding: true}
	browserBootstrapRegisterCfg := passkeyHandlerConfig{source: sourceBrowserBootstrap, enforceFirstCredentialOnly: true, createWebSession: true, setCookie: true, createUserOnBootstrap: true}
	browserBootstrapAuthCfg := passkeyHandlerConfig{source: sourceBrowserBootstrap, createWebSession: true, setCookie: true}

	// JIT passkey bootstrap: allow first-credential registration via JWT
	// This unblocks OIDC/JIT users who have zero credentials and cannot reach RouteAuthWebSession routes
	if h.auth != nil && h.auth.HasJWKS() {
		jwtPasskeyMux := http.NewServeMux()
		jwtPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysJITRegisterChallenge, h.passkey.RegisterChallenge(jitCfg))
		jwtPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysJITRegisterVerify, h.passkey.RegisterVerify(jitCfg))
		mux.Handle(constants.APIPaths.AuthPasskeysJITRegisterChallenge, h.auth.JWTAuthMiddleware(jwtPasskeyMux))
		mux.Handle(constants.APIPaths.AuthPasskeysJITRegisterVerify, h.auth.JWTAuthMiddleware(jwtPasskeyMux))
	}

	// Passkey console routes (public, no auth required).
	// console/*  — Browser-facing passkey registration and authentication (creates web session, sets cookie).
	passkeyMux := http.NewServeMux()
	passkeyMux.HandleFunc(constants.APIPaths.AuthPasskeysConsoleRegisterChallenge, h.passkey.RegisterChallenge(browserBootstrapRegisterCfg))
	passkeyMux.HandleFunc(constants.APIPaths.AuthPasskeysConsoleRegisterVerify, h.passkey.RegisterVerify(browserBootstrapRegisterCfg))
	passkeyMux.HandleFunc(constants.APIPaths.AuthPasskeysConsoleAuthenticateChallenge, h.passkey.AuthenticateChallenge(browserBootstrapAuthCfg))
	passkeyMux.HandleFunc(constants.APIPaths.AuthPasskeysConsoleAuthenticateVerify, h.passkey.AuthenticateVerify(browserBootstrapAuthCfg))

	mux.Handle(constants.APIPaths.AuthPasskeysConsoleRegisterChallenge, passkeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysConsoleRegisterVerify, passkeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysConsoleAuthenticateChallenge, passkeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysConsoleAuthenticateVerify, passkeyMux)

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

	// Tribunal deliberate endpoint (mTLS-guarded, enrolled principal).
	// Always registered — the handler checks the atomic pointer and returns
	// 503 if tribunal is not yet wired, eliminating the need for a router
	// rebuild when SetTribunal is called later in the boot sequence.
	mux.HandleFunc(constants.APIPaths.TribunalDeliberate, h.governanceController.handleTribunalDeliberate)

	// Rate-limited governance envelope
	govEnvMux := http.NewServeMux()
	govEnvMux.HandleFunc(constants.APIPaths.GovernanceEnvelopes, h.governanceController.handleGovernanceEnvelope)
	govEnvHandler := h.rateLimitMiddleware(govEnvMux)
	mux.Handle(constants.APIPaths.GovernanceEnvelopes, govEnvHandler)

	mux.HandleFunc(constants.APIPaths.AuditReceipts, h.dbController.handleAuditReceipts)
	mux.HandleFunc(constants.APIPaths.AuditReceiptsExport, h.dbController.handleAuditReceiptsExport)
	mux.HandleFunc(constants.APIPaths.AuditEvents, h.dbController.handleAuditEvents)
	mux.HandleFunc(constants.APIPaths.AuditSummary, h.dbController.handleAuditSummary)
	mux.HandleFunc(constants.APIPaths.AuditReport, h.dbController.handleAuditReport)

	mux.HandleFunc(constants.APIPaths.SSEPush, h.sseController.handleInternalSSEPush)
	mux.HandleFunc(constants.APIPaths.SSEEvents, h.sseController.handleInternalSSEEvents)
	mux.HandleFunc(constants.APIPaths.SSEStream, h.sseController.handleInternalSSEStream)
	mux.Handle(constants.APIPaths.DataDB, http.HandlerFunc(h.dbController.handleDataDB))
	mux.Handle(constants.APIPaths.KV, http.HandlerFunc(h.dbController.handleKV))
	mux.HandleFunc(constants.APIPaths.PubSubPublish, h.dbController.handlePubSubPublish)
	mux.Handle(constants.APIPaths.PubSubStream, h.auth.Middleware(http.HandlerFunc(h.pubsub.HandleWebSocket)))

	// PKI management routes (require mTLS)
	mux.HandleFunc(constants.APIPaths.PKICSRSign, h.pkiController.handlePKICSRSign)
	mux.HandleFunc(constants.APIPaths.PKIAppsDelegated, h.pkiController.handlePKIAppsDelegated)
	mux.HandleFunc(constants.APIPaths.PKICertificatesRevoke, h.pkiController.handlePKICertificatesRevoke)
	mux.HandleFunc(constants.APIPaths.PKIRevocationBundle, h.pkiController.handlePKIRevocationBundle)

	// User management routes (require mTLS)
	mux.HandleFunc(constants.APIPaths.Users, h.userController.handleUsers)

	// Passkey CLI status (require mTLS)
	mux.HandleFunc(constants.APIPaths.AuthPasskeysCLIStatus, h.passkey.CLIStatus)

	// Enrollment token generation (require mTLS CLI session)
	mux.HandleFunc(constants.APIPaths.AuthEnrollmentTokenGenerate, h.enrollmentTokenController.handleEnrollmentTokenGenerate)

	// OOB Approval UI for suspended MCP/A2A transactions
	mux.HandleFunc(constants.APIPaths.ApprovePage, h.passkey.handleApprovalPage)
	mux.HandleFunc(constants.APIPaths.ApprovalsCLIStatus, h.passkey.handleCLIApprovalStatus)
	mux.HandleFunc(constants.APIPaths.ApprovalsCLIList, h.passkey.handleCLIListSuspended)

	// Browser-facing routes (RouteAuthWebSession — unified middleware validates cookie)
	mux.HandleFunc(constants.APIPaths.UsersMe, h.userController.handleUserMe)
	mux.HandleFunc(constants.APIPaths.AuthSessionsMe, h.sessionController.handleWebSession)
	mux.Handle(constants.APIPaths.ApprovalsByID, http.HandlerFunc(h.passkey.handleApprovalAction))
	mux.HandleFunc(constants.APIPaths.Approvals, h.passkey.handleListSuspendedTransactions)
	mux.HandleFunc(constants.APIPaths.AuthPasskeys, h.passkey.ListCredentials)
	mux.Handle(constants.APIPaths.AuthPasskeysByID, http.HandlerFunc(h.passkey.RevokeCredential))

	return h.corsMiddleware(h.pathTraversalGuard(h.auth.Middleware(mux)))
}

// buildMCPHandler creates the rate-limited, auth-wrapped handler for all MCP/A2A ingress routes.
// When JWKS is configured, JWT middleware is applied for external IdP auth; otherwise MCP/A2A
// routes rely on mTLS + AppPolicy enforced by the main middleware at the router level.
func (h *HTTPHandler) buildMCPHandler() http.Handler {
	mcpMux := http.NewServeMux()
	mcpMux.HandleFunc(constants.APIPaths.MCPEndpoint, h.mcp.HandleMCP)
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
	mux.Handle(constants.APIPaths.A2ACall, handler)
}

func (h *HTTPHandler) buildHTTPRouter() http.Handler {
	mux := http.NewServeMux()

	// Health check - available on HTTP port for initialization monitoring
	mux.HandleFunc(constants.APIPaths.Health, h.healthController.handleBootstrapHealth)

	// State endpoint (for envelope state root binding)
	mux.HandleFunc(constants.APIPaths.State, h.healthController.handleState)

	// Bootstrap routes - plain HTTP for initial CA discovery and bootstrap
	mux.HandleFunc(constants.APIPaths.AuthBootstrap, h.bootstrapController.handleLocalBootstrapWithURL)
	mux.HandleFunc(constants.APIPaths.AuthBootstrapStatus, h.bootstrapController.handleBootstrapStatus)
	mux.HandleFunc(constants.APIPaths.AuthCLIEnroll, h.bootstrapController.handleCLIEnrollment)
	mux.HandleFunc(constants.APIPaths.AuthDeviceEnroll, h.bootstrapController.handleDeviceEnrollment)
	mux.HandleFunc(constants.APIPaths.PKIAppsEnroll, h.pkiController.handlePKIAppsEnroll)
	mux.HandleFunc(constants.APIPaths.PKICSRSign, h.pkiController.handlePKICSRSign)
	mux.HandleFunc(constants.APIPaths.WellKnownPKICABundle, h.pkiController.handlePKICABundle)
	mux.HandleFunc(constants.APIPaths.WellKnownPKIFingerprint, h.pkiController.handlePKIFingerprint)

	mux.HandleFunc(constants.APIPaths.WebCertLinux, h.pkiController.handleTrustScriptLinux)
	mux.HandleFunc(constants.APIPaths.WebCertWindows, h.pkiController.handleTrustScriptWindows)
	mux.HandleFunc("/.well-known/g8e/pki/trust-windows", h.pkiController.handleTrustScriptWindowsAlias)
	mux.HandleFunc("/.well-known/g8e/bin/", h.pkiController.handleNodeBinaryDownload)
	mux.HandleFunc(constants.APIPaths.DeployScriptLinux, h.pkiController.handleDeployScriptLinux)
	mux.HandleFunc(constants.APIPaths.DeployScriptWindows, h.pkiController.handleDeployScriptWindows)

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
