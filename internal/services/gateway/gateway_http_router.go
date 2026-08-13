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

	// Bootstrap routes (CA discovery) - now on public HTTPS
	mux.HandleFunc(constants.APIPaths.WellKnownPKICABundle, h.pkiController.handlePKICABundle)
	mux.HandleFunc(constants.APIPaths.WellKnownPKIFingerprint, h.pkiController.handlePKIFingerprint)
	mux.HandleFunc(constants.APIPaths.PKICRL, h.pkiController.handlePKIRevocationBundle)
	mux.Handle(constants.APIPaths.DataBlobs, http.HandlerFunc(h.dataController.handleBlob))

	// Console SPA (public, no auth required)
	consoleHandler := console.Handler()
	mux.Handle(constants.APIPaths.ConsolePrefix, http.StripPrefix(strings.TrimSuffix(constants.APIPaths.ConsolePrefix, "/"), consoleHandler))

	// Landing page and health
	mux.HandleFunc(constants.APIPaths.Landing, h.healthController.handleLandingPage)
	mux.HandleFunc(constants.APIPaths.AuthLogout, h.sessionController.handlePublicAuthLogout)
	mux.HandleFunc(constants.APIPaths.AuthBootstrap, h.bootstrapController.handleLocalBootstrapWithURL)
	mux.HandleFunc(constants.APIPaths.AuthBootstrapStatus, h.bootstrapController.handleBootstrapStatus)
	mux.HandleFunc(constants.APIPaths.AuthDeviceEnroll, h.bootstrapController.handleDeviceEnrollment)
	mux.HandleFunc(constants.APIPaths.PKIAppsEnroll, h.pkiController.handlePKIAppsEnroll)
	mux.HandleFunc(constants.APIPaths.PKIDevicesEnroll, h.pkiController.handlePKIDevicesEnroll)

	// CLI recovery flow — request/status/complete are public (token-scoped);
	// approve is web-session protected (browser console only).
	mux.HandleFunc(constants.APIPaths.AuthCLIRecoveryRequest, h.cliRecoveryController.handleRecoveryRequest)
	mux.HandleFunc(constants.APIPaths.AuthCLIRecoveryStatus, h.cliRecoveryController.handleRecoveryStatus)
	mux.HandleFunc(constants.APIPaths.AuthCLIRecoveryApprove, h.cliRecoveryController.handleRecoveryApprove)
	mux.HandleFunc(constants.APIPaths.AuthCLIRecoveryComplete, h.cliRecoveryController.handleRecoveryComplete)

	// CLI rotation — mTLS-protected; the caller's identity is derived from
	// the verified CLI certificate. NOT registered on buildHTTPRouter
	// (plain HTTP) because rotation requires mTLS, which the plain router
	// does not provide.
	mux.HandleFunc(constants.APIPaths.AuthCLIRotate, h.cliRotationController.handleRotate)

	// Enrollment token validation (public — the token itself is the credential)
	mux.HandleFunc(constants.APIPaths.AuthEnrollmentTokenValidate, h.enrollmentTokenController.handleEnrollmentTokenValidate)

	// MCP/A2A ingress (rate-limited, JWT when JWKS is configured, else mTLS via main middleware)
	mcpHandler := h.buildMCPHandler()
	registerMCPRoutes(mux, mcpHandler)

	jitCfg := passkeyHandlerConfig{source: sourceJWT, enforceFirstCredentialOnly: true, requireAuthenticatedUser: true, enforceSessionUserBinding: true}
	browserBootstrapRegisterCfg := passkeyHandlerConfig{source: sourceBrowserBootstrap, enforceFirstCredentialOnly: true, createWebSession: true, setCookie: true, createUserOnBootstrap: true}
	browserBootstrapAuthCfg := passkeyHandlerConfig{source: sourceBrowserBootstrap, createWebSession: true, setCookie: true}
	// CLI-initiated enrollment: the enrollment token is the single
	// authorization primitive. No enforceFirstCredentialOnly (the token
	// already vouches for the user), no createUserOnBootstrap (the user
	// exists — the CLI created it via `auth enroll`), no
	// requireAuthenticatedUser (the token is the credential).
	enrollmentRegisterCfg := passkeyHandlerConfig{source: sourceEnrollmentToken, requireEnrollmentToken: true, createWebSession: true, setCookie: true}

	// JIT passkey bootstrap: allow first-credential registration via JWT
	// This unblocks OIDC/JIT users who have zero credentials and cannot reach RouteAuthWebSession routes
	if h.authMiddleware != nil && h.authMiddleware.HasJWKS() {
		jwtPasskeyMux := http.NewServeMux()
		jwtPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysJITRegisterChallenge, h.passkeyController.registerChallenge(jitCfg))
		jwtPasskeyMux.HandleFunc(constants.APIPaths.AuthPasskeysJITRegisterVerify, h.passkeyController.registerVerify(jitCfg))
		mux.Handle(constants.APIPaths.AuthPasskeysJITRegisterChallenge, h.authMiddleware.JWTAuthMiddleware(jwtPasskeyMux))
		mux.Handle(constants.APIPaths.AuthPasskeysJITRegisterVerify, h.authMiddleware.JWTAuthMiddleware(jwtPasskeyMux))
	}

	// Passkey console routes (public, no auth required).
	// console/*  — Browser-facing passkey registration and authentication (creates web session, sets cookie).
	passkeyMux := http.NewServeMux()
	passkeyMux.HandleFunc(constants.APIPaths.AuthPasskeysConsoleRegisterChallenge, h.passkeyController.registerChallenge(browserBootstrapRegisterCfg))
	passkeyMux.HandleFunc(constants.APIPaths.AuthPasskeysConsoleRegisterVerify, h.passkeyController.registerVerify(browserBootstrapRegisterCfg))
	passkeyMux.HandleFunc(constants.APIPaths.AuthPasskeysConsoleAuthenticateChallenge, h.passkeyController.authenticateChallenge(browserBootstrapAuthCfg))
	passkeyMux.HandleFunc(constants.APIPaths.AuthPasskeysConsoleAuthenticateVerify, h.passkeyController.authenticateVerify(browserBootstrapAuthCfg))

	mux.Handle(constants.APIPaths.AuthPasskeysConsoleRegisterChallenge, passkeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysConsoleRegisterVerify, passkeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysConsoleAuthenticateChallenge, passkeyMux)
	mux.Handle(constants.APIPaths.AuthPasskeysConsoleAuthenticateVerify, passkeyMux)

	// CLI-initiated enrollment routes (public, no auth required — the
	// enrollment token is the credential). These are separate from the
	// console bootstrap routes above so the two ceremonies do not share
	// a config or a JS code path. See plan
	// passkey-enrollment-console-400.md.
	enrollmentMux := http.NewServeMux()
	enrollmentMux.HandleFunc(constants.APIPaths.AuthPasskeysEnrollmentRegisterChallenge, h.passkeyController.registerChallenge(enrollmentRegisterCfg))
	enrollmentMux.HandleFunc(constants.APIPaths.AuthPasskeysEnrollmentRegisterVerify, h.passkeyController.registerVerify(enrollmentRegisterCfg))
	mux.Handle(constants.APIPaths.AuthPasskeysEnrollmentRegisterChallenge, enrollmentMux)
	mux.Handle(constants.APIPaths.AuthPasskeysEnrollmentRegisterVerify, enrollmentMux)

	// mTLS-only routes (merged from buildRouter)
	mux.HandleFunc(constants.APIPaths.DataSettings, h.dataController.handleDataSettings)
	mux.HandleFunc(constants.APIPaths.Operators, h.operatorController.handleListOperators)
	mux.Handle(constants.APIPaths.OperatorsByID, http.HandlerFunc(h.operatorController.handleTerminateOperator))
	mux.HandleFunc(constants.APIPaths.OperatorsBind, h.operatorController.handleBindOperators)
	mux.HandleFunc(constants.APIPaths.OperatorsUnbind, h.operatorController.handleUnbindOperators)
	mux.HandleFunc(constants.APIPaths.OperatorsTarget, h.operatorController.handleSetTargetContext)
	mux.HandleFunc(constants.APIPaths.OperatorsReauth, h.operatorController.handleReauth)
	mux.HandleFunc(constants.APIPaths.GovernanceSigners, h.signerController.handleGovernanceSigners)
	mux.Handle(constants.APIPaths.GovernanceSignersByID, http.HandlerFunc(h.signerController.handleGovernanceSignerByID))
	mux.Handle(constants.APIPaths.AdminAppPoliciesBySigner, http.HandlerFunc(h.adminController.handleAppPolicySigner))
	mux.HandleFunc(constants.APIPaths.AdminAppsRevoke, h.adminController.handleRevokeApp)
	mux.HandleFunc(constants.APIPaths.AdminConsensus, h.adminController.handleConsensus)
	mux.Handle(constants.APIPaths.AdminConsensusByID, http.HandlerFunc(h.adminController.handleDeleteConsensus))

	// Consensus deliberate endpoint (mTLS-guarded, enrolled principal).
	// Returns 503 if consensus is not configured for the current posture.
	mux.HandleFunc(constants.APIPaths.ConsensusDeliberate, h.governanceController.handleConsensusDeliberate)

	// Rate-limited governance envelope
	govEnvMux := http.NewServeMux()
	govEnvMux.HandleFunc(constants.APIPaths.GovernanceEnvelopes, h.governanceController.handleGovernanceEnvelope)
	govEnvHandler := h.rateLimitMiddleware(govEnvMux)
	mux.Handle(constants.APIPaths.GovernanceEnvelopes, govEnvHandler)

	mux.HandleFunc(constants.APIPaths.AuditReceipts, h.auditController.handleAuditReceipts)
	mux.HandleFunc(constants.APIPaths.AuditReceiptsExport, h.auditController.handleAuditReceiptsExport)
	mux.HandleFunc(constants.APIPaths.AuditEvents, h.auditController.handleAuditEvents)
	mux.HandleFunc(constants.APIPaths.AuditSummary, h.auditController.handleAuditSummary)
	mux.HandleFunc(constants.APIPaths.AuditReport, h.auditController.handleAuditReport)

	mux.HandleFunc(constants.APIPaths.SSEPush, h.sseController.handleInternalSSEPush)
	mux.HandleFunc(constants.APIPaths.SSEEvents, h.sseController.handleInternalSSEEvents)
	mux.HandleFunc(constants.APIPaths.SSEStream, h.sseController.handleInternalSSEStream)
	mux.Handle(constants.APIPaths.DataDB, http.HandlerFunc(h.dataController.handleDataDB))
	mux.Handle(constants.APIPaths.KV, http.HandlerFunc(h.dataController.handleKV))
	mux.HandleFunc(constants.APIPaths.PubSubPublish, h.dataController.handlePubSubPublish)
	mux.Handle(constants.APIPaths.PubSubStream, h.authMiddleware.Middleware(http.HandlerFunc(h.pubsubController.handleWebSocket)))

	// PKI management routes (require mTLS)
	mux.HandleFunc(constants.APIPaths.PKICSRSign, h.pkiController.handlePKICSRSign)
	mux.HandleFunc(constants.APIPaths.PKIAppsDelegated, h.pkiController.handlePKIAppsDelegated)
	mux.HandleFunc(constants.APIPaths.PKICertificatesRevoke, h.pkiController.handlePKICertificatesRevoke)
	mux.HandleFunc(constants.APIPaths.PKIRevocationBundle, h.pkiController.handlePKIRevocationBundle)

	// User management routes (require mTLS)
	mux.HandleFunc(constants.APIPaths.Users, h.userController.handleUsers)

	// Passkey CLI status (require mTLS)
	mux.HandleFunc(constants.APIPaths.AuthPasskeysCLIStatus, h.passkeyController.cliStatus)

	// Enrollment token generation (require mTLS CLI session)
	mux.HandleFunc(constants.APIPaths.AuthEnrollmentTokenGenerate, h.enrollmentTokenController.handleEnrollmentTokenGenerate)

	// OOB Approval UI for suspended MCP/A2A transactions
	mux.HandleFunc(constants.APIPaths.ApprovePage, h.passkeyController.handleApprovalPage)
	mux.HandleFunc(constants.APIPaths.ApprovalsCLIStatus, h.passkeyController.handleCLIApprovalStatus)
	mux.HandleFunc(constants.APIPaths.ApprovalsCLIList, h.passkeyController.handleCLIListSuspended)

	// Browser-facing routes (RouteAuthWebSession — unified middleware validates cookie)
	mux.HandleFunc(constants.APIPaths.UsersMe, h.userController.handleUserMe)
	mux.HandleFunc(constants.APIPaths.AuthSessionsMe, h.sessionController.handleWebSession)
	mux.Handle(constants.APIPaths.ApprovalsByID, http.HandlerFunc(h.passkeyController.handleApprovalAction))
	mux.HandleFunc(constants.APIPaths.Approvals, h.passkeyController.handleListSuspendedTransactions)
	mux.HandleFunc(constants.APIPaths.AuthPasskeys, h.passkeyController.listCredentials)
	mux.Handle(constants.APIPaths.AuthPasskeysByID, http.HandlerFunc(h.passkeyController.revokeCredential))

	return h.corsMiddleware(h.pathTraversalGuard(h.authMiddleware.Middleware(mux)))
}

// buildMCPHandler creates the rate-limited, auth-wrapped handler for all MCP/A2A ingress routes.
// When JWKS is configured, JWT middleware is applied for external IdP auth; otherwise MCP/A2A
// routes rely on mTLS + AppPolicy enforced by the main middleware at the router level.
func (h *HTTPHandler) buildMCPHandler() http.Handler {
	mcpMux := http.NewServeMux()
	mcpMux.HandleFunc(constants.APIPaths.MCPEndpoint, h.mcpController.handleMCP)
	mcpMux.HandleFunc(constants.APIPaths.A2ACall, h.mcpController.handleA2aCall)

	mcpRateLimited := h.rateLimitMiddleware(mcpMux)

	if h.authMiddleware != nil && h.authMiddleware.HasJWKS() {
		return h.authMiddleware.JWTAuthMiddleware(mcpRateLimited)
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
	mux.HandleFunc(constants.APIPaths.AuthDeviceEnroll, h.bootstrapController.handleDeviceEnrollment)
	mux.HandleFunc(constants.APIPaths.PKIAppsEnroll, h.pkiController.handlePKIAppsEnroll)
	mux.HandleFunc(constants.APIPaths.PKICSRSign, h.pkiController.handlePKICSRSign)
	mux.HandleFunc(constants.APIPaths.WellKnownPKICABundle, h.pkiController.handlePKICABundle)
	mux.HandleFunc(constants.APIPaths.WellKnownPKIFingerprint, h.pkiController.handlePKIFingerprint)

	// CLI recovery discovery surface — request/status/complete are reachable
	// over plain HTTP so a new CLI without trusted TLS can initiate recovery.
	// The approve endpoint is intentionally NOT registered here: approval
	// requires a web-session cookie, which is only set over HTTPS.
	mux.HandleFunc(constants.APIPaths.AuthCLIRecoveryRequest, h.cliRecoveryController.handleRecoveryRequest)
	mux.HandleFunc(constants.APIPaths.AuthCLIRecoveryStatus, h.cliRecoveryController.handleRecoveryStatus)
	mux.HandleFunc(constants.APIPaths.AuthCLIRecoveryComplete, h.cliRecoveryController.handleRecoveryComplete)

	mux.HandleFunc("/.well-known/g8e/bin/", h.pkiController.handleNodeBinaryDownload)
	mux.HandleFunc(constants.APIPaths.DeployScriptLinux, h.pkiController.handleDeployScriptLinux)
	mux.HandleFunc(constants.APIPaths.DeployScriptWindows, h.pkiController.handleDeployScriptWindows)

	// Catch-all: redirect all non-bootstrap requests to HTTPS. The console,
	// SSE stream, and all API routes are served exclusively via TLS on the
	// HTTPS port. Without this redirect, a browser that lands on the plain
	// HTTP port (e.g. http://host:8080/console/) gets a 404 and the Secure
	// web-session cookie is never set, so SSE EventSource auth fails silently.
	//
	// The Host header is validated against localhost, loopback, RFC 1918
	// private IPs, and the configured Endpoint/PublicBaseURL before being
	// reflected into the redirect target. Unrecognized hosts fall back to a
	// safe default to prevent open-redirect abuse.
	mux.HandleFunc("/{path...}", func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if parsed, _, err := net.SplitHostPort(host); err == nil {
			host = parsed
		}

		if !isSafeHost(host, h.cfg) {
			if h.cfg != nil && h.cfg.Endpoint != "" {
				host = h.cfg.Endpoint
				if parsed, _, err := net.SplitHostPort(host); err == nil {
					host = parsed
				}
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

		// Collapse leading double slashes to prevent path-injection redirects
		// (e.g. //evil.com/x -> /evil.com/x, which stays on this host).
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

		http.Redirect(w, r, target, http.StatusMovedPermanently) // #nosec G710 -- host validated by isSafeHost
	})

	// Wrap with rate limiting
	return h.pathTraversalGuard(h.rateLimitMiddleware(mux))
}

// isSafeHost checks if the requested host is a recognized local, private, or
// configured endpoint. Unrecognized public hosts (e.g. attacker-controlled
// domains reflected via the Host header) return false so the catch-all
// redirect falls back to a safe default instead of creating an open redirect.
func isSafeHost(host string, cfg *config.Config) bool {
	if host == "" {
		return false
	}

	// Reject any host containing characters that are unsafe to reflect into a
	// Location header (path separators, query delimiters, CRLF, etc.).
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

// isPrivateIP reports whether ip is an RFC 1918 private IPv4 address.
// IPv6 addresses are not handled here and return false.
func isPrivateIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
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
