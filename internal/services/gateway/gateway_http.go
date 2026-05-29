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
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/protocol"
	"golang.org/x/time/rate"
)

const governanceEnvelopeRedirectError = "submit via POST /api/v1/governance/envelopes"

// HTTPHandlerDependencies groups all dependencies for HTTPHandler to reduce constructor bloat.
type HTTPHandlerDependencies struct {
	Cfg               *config.Config
	Logger            *slog.Logger
	DB                *GatewayDBService
	Pubsub            *PubSubBroker
	Auth              *AuthService
	PKI               *PKIAuthority
	SessionSvc        *SessionService
	Reg               *RegistrationService
	Passkey           *PasskeyService
	UserSvc           *UserService
	Responder         *responder.Responder
	MCPGateway        *mcp.GatewayService
	AppEnrollment     *AppEnrollmentService
	IsReady           func() bool
	IsGovernanceReady func() bool
	DocsFS            fs.FS
}

func (h *HTTPHandler) readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, h.cfg.Gateway.MaxPayloadBytes)
	return io.ReadAll(r.Body)
}

// HTTPHandler manages the web API for the gateway service.
type HTTPHandler struct {
	cfg               *config.Config
	logger            *slog.Logger
	db                *GatewayDBService
	pubsub            *PubSubBroker
	auth              *AuthService
	pki               *PKIAuthority
	sessionSvc        *SessionService
	reg               *RegistrationService
	passkey           *PasskeyService
	userSvc           *UserService
	responder         *responder.Responder
	mcp               *mcp.GatewayService
	appEnrollment     *AppEnrollmentService
	isReady           func() bool
	isGovernanceReady func() bool
	// envProc is the synchronous fail-closed Gateway mutation gate. It is
	// nil until SetEnvelopeProcessor is called by the boot sequence after
	// the in-process command service has initialized the verifier and
	// Actuator. While nil, /api/governance/envelope returns 503.
	envProc governance.EnvelopeProcessor

	// Controllers for domain-specific endpoints
	pkiController      *PKIController
	dbController       *DBController
	authController     *AuthController
	adminController    *AdminController
	operatorController *OperatorController

	// Rate limiting state
	muLimiters sync.Mutex
	limiters   map[string]*rate.Limiter

	// docsFS is the embedded documentation filesystem
	docsFS fs.FS
}

func newHTTPHandler(deps HTTPHandlerDependencies) *HTTPHandler {
	h := &HTTPHandler{
		cfg:               deps.Cfg,
		logger:            deps.Logger,
		db:                deps.DB,
		pubsub:            deps.Pubsub,
		auth:              deps.Auth,
		pki:               deps.PKI,
		sessionSvc:        deps.SessionSvc,
		reg:               deps.Reg,
		passkey:           deps.Passkey,
		userSvc:           deps.UserSvc,
		responder:         deps.Responder,
		mcp:               deps.MCPGateway,
		appEnrollment:     deps.AppEnrollment,
		isReady:           deps.IsReady,
		isGovernanceReady: deps.IsGovernanceReady,
		limiters:          make(map[string]*rate.Limiter),
		docsFS:            deps.DocsFS,
	}

	// Initialize controllers
	h.pkiController = newPKIController(deps.Cfg, deps.Logger, deps.DB, deps.PKI, deps.AppEnrollment, deps.Reg, deps.Responder)
	h.dbController = newDBController(deps.Cfg, deps.Logger, deps.DB, deps.Auth, deps.Pubsub, deps.UserSvc, deps.Responder)
	h.authController = newAuthController(deps.Cfg, deps.Logger, deps.DB, deps.Auth, deps.Passkey, deps.UserSvc, deps.Reg, deps.PKI, deps.SessionSvc, deps.MCPGateway, deps.Responder)
	h.adminController = newAdminController(deps.Cfg, deps.Logger, deps.DB, deps.UserSvc, deps.Responder)
	h.operatorController = newOperatorController(deps.Cfg, deps.Logger, deps.Reg, deps.Auth, deps.Responder)

	return h
}

func (h *HTTPHandler) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		h.muLimiters.Lock()
		limiter, ok := h.limiters[ip]
		if !ok {
			limiter = rate.NewLimiter(rate.Limit(h.cfg.Gateway.RateLimitRPS), h.cfg.Gateway.RateLimitBurst)
			h.limiters[ip] = limiter
		}
		h.muLimiters.Unlock()

		if !limiter.Allow() {
			h.logger.Warn("Rate limit exceeded", "ip", ip, "path", r.URL.Path)
			h.responder.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (h *HTTPHandler) buildRouter() http.Handler {
	mux := http.NewServeMux()

	// MCP Ingress routes with rate limiting
	mcpMux := http.NewServeMux()
	mcpMux.HandleFunc("/api/v1/mcp/tools/list", h.mcp.HandleToolsList)
	mcpMux.HandleFunc("/api/v1/mcp/tools/call", h.mcp.HandleToolsCall)
	mcpMux.HandleFunc("/api/v1/mcp/tools/call/sse", h.mcp.HandleToolsCallSSE)
	mcpMux.HandleFunc("/api/v1/mcp/resources/list", h.mcp.HandleResourcesList)
	mcpMux.HandleFunc("/api/v1/mcp/resources/read", h.mcp.HandleResourcesRead)
	mcpMux.HandleFunc("/api/v1/mcp/prompts/list", h.mcp.HandlePromptsList)
	mcpMux.HandleFunc("/api/v1/mcp/prompts/get", h.mcp.HandlePromptsGet)
	mcpMux.HandleFunc("/api/v1/a2a/call", h.mcp.HandleA2aCall)

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
	govEnvMux.HandleFunc("/api/v1/governance/envelopes", h.handleGovernanceEnvelope)
	govEnvHandler := h.rateLimitMiddleware(govEnvMux)

	// Health check (available internally)
	mux.HandleFunc("/health", h.handleHealth)

	// Documentation endpoint (embedded docs)
	mux.HandleFunc("/docs/", h.handleDocs)

	// Authenticated routes (require mTLS)
	mux.HandleFunc("/api/v1/data/settings", h.dbController.handleDataSettings)
	mux.HandleFunc("/api/v1/operators", h.operatorController.handleListOperators)
	mux.HandleFunc("/api/v1/operators/", h.operatorController.handleTerminateOperator)
	mux.HandleFunc("/api/v1/operators/bind", h.operatorController.handleBindOperators)
	mux.HandleFunc("/api/v1/operators/unbind", h.operatorController.handleUnbindOperators)
	mux.HandleFunc("/api/v1/operators/target", h.operatorController.handleSetTargetContext)
	mux.HandleFunc("/api/v1/operators/reauth", h.operatorController.handleReauth)
	mux.HandleFunc("/api/v1/governance/signers", h.dbController.handleGovernanceSigners)
	mux.HandleFunc("/api/v1/governance/signers/", h.dbController.handleGovernanceSignerByID)
	mux.HandleFunc("/api/v1/admin/app-policies/", h.adminController.handleAppPolicySigner)
	mux.HandleFunc("/api/v1/admin/apps/revoke", h.adminController.handleRevokeApp)

	// Register rate-limited MCP routes
	mux.Handle("/api/v1/governance/envelopes", govEnvHandler)
	mux.Handle("/api/v1/mcp/", mcpHandler)
	mux.Handle("/api/v1/a2a/", mcpHandler)

	mux.HandleFunc("/api/v1/audit/receipts", h.dbController.handleAuditReceipts)
	mux.HandleFunc("/api/v1/audit/receipts/export", h.dbController.handleAuditReceiptsExport)

	// Internal SSE event bridge (used by g8e-compatible agentic ensembles to publish typed events
	// for browser/CLI subscribers to consume). Producers are authenticated by
	// mTLS app identity; consumers poll /api/v1/sse/events or stream /api/v1/sse/stream.
	mux.HandleFunc("/api/v1/sse/push", h.handleInternalSSEPush)
	mux.HandleFunc("/api/v1/sse/events", h.handleInternalSSEEvents)
	mux.HandleFunc("/api/v1/sse/stream", h.handleInternalSSEStream)
	mux.HandleFunc("/api/v1/data/", h.dbController.handleDataDB)
	mux.HandleFunc("/api/v1/kv/", h.dbController.handleKV)
	mux.HandleFunc("/api/v1/pubsub/publish", h.dbController.handlePubSubPublish)
	mux.Handle("/api/v1/pubsub/stream", h.auth.WebSocketAuth(http.HandlerFunc(h.pubsub.HandleWebSocket)))
	mux.HandleFunc("/api/v1/blobs/", h.dbController.handleBlob)

	// PKI management routes (require mTLS)
	mux.HandleFunc("/api/v1/pki/csr/sign", h.pkiController.handlePKICSRSign)
	mux.HandleFunc("/api/v1/pki/devices/enroll", h.pkiController.handlePKIDevicesEnroll)
	mux.HandleFunc("/api/v1/pki/certificates/revoke", h.pkiController.handlePKICertificatesRevoke)
	mux.HandleFunc("/api/v1/pki/revocation-bundle", h.pkiController.handlePKIRevocationBundle)

	// User management routes (require mTLS)
	mux.HandleFunc("/api/v1/users", h.authController.handleUsers)

	// Passkey / L3 Brokerage Routes (require mTLS)
	mux.HandleFunc("/api/v1/auth/passkeys/register/challenge", h.authController.handleAuthPasskeysRegisterChallenge)
	mux.HandleFunc("/api/v1/auth/passkeys/register/verify", h.authController.handleAuthPasskeysRegisterVerify)
	mux.HandleFunc("/api/v1/auth/passkeys/authenticate/challenge", h.authController.handleAuthPasskeysAuthenticateChallenge)
	mux.HandleFunc("/api/v1/auth/passkeys/authenticate/verify", h.authController.handleAuthPasskeysAuthenticateVerify)
	mux.HandleFunc("/api/v1/auth/passkeys", h.authController.handleAuthPasskeys)
	mux.HandleFunc("/api/v1/auth/passkeys/", h.authController.handleAuthPasskeysRevoke)

	// Approval routes (require mTLS)
	mux.HandleFunc("/api/v1/approvals/", h.authController.handleApprovalAction)

	return h.pathTraversalGuard(h.auth.Middleware(mux))
}

func (h *HTTPHandler) buildPublicRouter() http.Handler {
	mux := http.NewServeMux()

	// Bootstrap routes (CA discovery, trust scripts) - now on public HTTPS
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/.well-known/g8e/pki/ca-bundle", h.pkiController.handlePKICABundle)
	mux.HandleFunc("/.well-known/g8e/pki/fingerprint", h.pkiController.handlePKIFingerprint)
	mux.HandleFunc("/api/v1/blobs/", h.dbController.handleBlob)
	mux.HandleFunc("/docs/", h.handleDocs)

	// Landing page and health
	mux.HandleFunc("/", h.handleLandingPage)
	mux.HandleFunc("/public/auth/login/verify", h.authController.handlePublicAuthLoginVerify)
	mux.HandleFunc("/public/auth/logout", h.authController.handlePublicAuthLogout)
	mux.HandleFunc("/public/auth/bootstrap", h.authController.handlePublicAuthBootstrap)
	mux.HandleFunc("/public/auth/bootstrap/status", h.authController.handleBootstrapStatus)

	// MCP/A2A Ingress routes with JWT authentication for remote clients
	mcpMux := http.NewServeMux()
	mcpMux.HandleFunc("/api/v1/mcp/tools/list", h.mcp.HandleToolsList)
	mcpMux.HandleFunc("/api/v1/mcp/tools/call", h.mcp.HandleToolsCall)
	mcpMux.HandleFunc("/api/v1/mcp/tools/call/sse", h.mcp.HandleToolsCallSSE)
	mcpMux.HandleFunc("/api/v1/mcp/resources/list", h.mcp.HandleResourcesList)
	mcpMux.HandleFunc("/api/v1/mcp/resources/read", h.mcp.HandleResourcesRead)
	mcpMux.HandleFunc("/api/v1/mcp/prompts/list", h.mcp.HandlePromptsList)
	mcpMux.HandleFunc("/api/v1/mcp/prompts/get", h.mcp.HandlePromptsGet)
	mcpMux.HandleFunc("/api/v1/a2a/call", h.mcp.HandleA2aCall)

	// Wrap MCP/A2A with Rate Limiting
	mcpRateLimited := h.rateLimitMiddleware(mcpMux)

	// Apply JWT middleware only when JWKS is configured (for external IdP auth)
	// When JWKS is not configured, MCP/A2A routes are not available on public port
	var mcpHandler http.Handler
	if h.auth != nil && h.auth.HasJWKS() {
		mcpHandler = h.auth.JWTAuthMiddleware(mcpRateLimited)
		mux.Handle("/api/v1/mcp/", mcpHandler)
		mux.Handle("/api/v1/a2a/", mcpHandler)

		// JIT passkey bootstrap: allow first-credential registration via JWT
		// This unblocks OIDC/JIT users who have zero credentials and cannot reach WebSessionAuth
		jwtPasskeyMux := http.NewServeMux()
		jwtPasskeyMux.HandleFunc("/api/v1/auth/passkeys/jit-register/challenge", h.authController.handleAuthPasskeysRegisterChallenge)
		jwtPasskeyMux.HandleFunc("/api/v1/auth/passkeys/jit-register/verify", h.authController.handleAuthPasskeysRegisterVerify)
		mux.Handle("/api/v1/auth/passkeys/jit-register/challenge", h.auth.JWTAuthMiddleware(jwtPasskeyMux))
		mux.Handle("/api/v1/auth/passkeys/jit-register/verify", h.auth.JWTAuthMiddleware(jwtPasskeyMux))
	}

	// Browser-facing data routes (require web session cookie)
	authedMux := http.NewServeMux()
	authedMux.HandleFunc("/api/v1/users/me", h.authController.handleUserMe)
	authedMux.HandleFunc("/api/v1/auth/sessions/me", h.authController.handleWebSession)

	// OOB Approval UI for suspended MCP/A2A transactions
	mux.HandleFunc("/public/approve/", h.authController.handleApprovalPage)
	authedMux.HandleFunc("/api/v1/approvals/", h.authController.handleApprovalAction)
	authedMux.HandleFunc("/api/v1/approvals", h.authController.handleListSuspendedTransactions)

	// Wrap authed routes in WebSessionAuth middleware
	mux.Handle("/api/v1/users/", h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle("/api/v1/auth/sessions/", h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle("/api/v1/approvals", h.auth.WebSessionAuth(authedMux, h.db))

	return h.pathTraversalGuard(h.auth.Middleware(mux))
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.buildRouter().ServeHTTP(w, r)
}

// pathTraversalGuard rejects any request whose raw URL path contains a ".."
// segment before Go's ServeMux can normalize the path and issue a 301 redirect.
func (h *HTTPHandler) pathTraversalGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path to handle multiple slashes, etc.
		cleaned := filepath.ToSlash(filepath.Clean(r.URL.Path))
		if h.containsTraversal(r.URL.Path) || (cleaned != r.URL.Path && cleaned != r.URL.Path+"/" && r.URL.Path != "/") {
			if h.containsTraversal(r.URL.Path) || strings.Contains(cleaned, "..") {
				h.responder.Error(w, http.StatusBadRequest, "invalid path")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *HTTPHandler) containsTraversal(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

func isDirectDBMutationAllowed(collection string) bool {
	switch constants.CollectionName(collection) {
	// Platform infrastructure collections (internal use, no governance required)
	case constants.CollectionSettings,
		constants.CollectionUsers,
		constants.CollectionOperators,
		constants.CollectionOperatorSessions,
		constants.CollectionBoundSessions,
		constants.CollectionPasskeyChallenges,
		constants.CollectionRevokedCertificates,
		constants.CollectionTrustedSigners,
		constants.CollectionConsoleAudit:
		return true
	// Governed collections must use POST /api/governance/envelope
	case constants.CollectionCases,
		constants.CollectionInvestigations,
		constants.CollectionTasks,
		constants.CollectionMemories,
		constants.CollectionReputationState,
		constants.CollectionReputationCommitments,
		constants.CollectionAgentActivityMetadata,
		constants.CollectionStakeResolutions:
		return false
	default:
		return false
	}
}

func isMutationPubSubChannelAllowed(channel string) bool {
	for _, prefix := range []string{"heartbeat:", "results:", "sse:", "ws_session:", "internal:"} {
		if strings.HasPrefix(channel, prefix) {
			return true
		}
	}
	return false
}

func (h *HTTPHandler) GetMCPGateway() *mcp.GatewayService {
	return h.mcp
}

func (h *HTTPHandler) GetPasskeyService() *PasskeyService {
	return h.passkey
}

func (h *HTTPHandler) GetPubSubBroker() *PubSubBroker {
	return h.pubsub
}

func (h *HTTPHandler) handleLandingPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}
	if host == "" {
		host = "localhost"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <title>g8e Operator - Public Entry</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 800px; margin: 40px auto; padding: 20px; line-height: 1.6; }
        .container { border: 1px solid #ddd; border-radius: 8px; padding: 30px; box-shadow: 0 2px 10px rgba(0,0,0,0.05); }
        h1 { color: #2c3e50; margin-top: 0; }
        .section { margin-bottom: 30px; }
        .label { font-weight: bold; color: #34495e; }
        code { background: #f8f9fa; padding: 2px 5px; border-radius: 4px; border: 1px solid #e9ecef; }
        .btn { display: inline-block; background: #3498db; color: white; padding: 10px 20px; text-decoration: none; border-radius: 4px; margin-top: 10px; }
        .btn:hover { background: #2980b9; }
        .footer { margin-top: 40px; font-size: 0.9em; color: #7f8c8d; border-top: 1px solid #eee; padding-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>g8e Operator</h1>
        <p>You have reached the public entry point for the g8e Operator Gateway.</p>

        <div class="section">
            <div class="label">Trust & Security</div>
            <p>To use this operator from your browser or as a BYO client, you must first install the platform's root certificate. If you see a "Not Secure" warning, please provide your own valid client certificate for mTLS operations.</p>
        </div>

        <div class="section">
            <div class="label">Next Steps</div>
            <ul>
                <li><a href="/api/auth/login/challenge">Check Login Capabilities</a></li>
                <li><a href="https://github.com/g8e-ai/g8e/docs" target="_blank">Read Documentation</a></li>
            </ul>
        </div>

        <div class="footer">
            Sovereign Governance Gateway &copy; 2026 Lateralus Labs, LLC.
        </div>
    </div>
</body>
</html>
`, html.EscapeString(host))
}

func (h *HTTPHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if h.isReady != nil && !h.isReady() {
		h.responder.Error(w, http.StatusServiceUnavailable, "service initializing")
		return
	}

	doc, err := h.db.DocGet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings))
	if err != nil {
		h.logger.Error("Health check failed to query platform_settings", string(constants.ConnectionStateError), err)
		h.responder.Error(w, http.StatusServiceUnavailable, "platform_settings not ready")
		return
	}
	if doc == nil {
		h.logger.Warn("Health check: platform_settings not found")
		h.responder.Error(w, http.StatusServiceUnavailable, "platform_settings not ready")
		return
	}

	root, err := h.db.GetCurrentStateRoot()
	if err != nil {
		h.logger.Error("Health check failed to get state root", string(constants.ConnectionStateError), err)
	}

	h.responder.JSON(w, http.StatusOK, models.HealthResponse{
		Status:          constants.GatewayModeStatusOK,
		Mode:            constants.GatewayModeMode,
		Version:         h.cfg.Version,
		GovernanceReady: h.isGovernanceReady != nil && h.isGovernanceReady(),
		StateMerkleRoot: root,
	})
}

func (h *HTTPHandler) handleDocs(w http.ResponseWriter, r *http.Request) {
	if h.docsFS == nil {
		h.responder.Error(w, http.StatusServiceUnavailable, "documentation not available")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/docs/")
	if path == "" {
		path = "index.md"
	}

	if !strings.HasSuffix(path, ".md") {
		path = path + ".md"
	}

	file, err := h.docsFS.Open("docs/" + path)
	if err != nil {
		h.responder.Error(w, http.StatusNotFound, "document not found")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		h.responder.Error(w, http.StatusInternalServerError, "failed to read document")
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// =============================================================================
// /api/v1/sse/push, /api/v1/sse/events - Internal SSE event bridge
//
// POST /api/v1/sse/push     → Producer (g8e-compatible agentic ensemble) appends an event.
//                            Body MUST set exactly one of
//                            web_session_id, cli_session_id, user_id.
// GET  /api/v1/sse/events   → Consumer (CLI / dashboard) polls events.
//                            Query string MUST set exactly one of
//                            web_session_id, cli_session_id, user_id,
//                            plus since_id=N and limit=K.
//
// The Gateway refuses to talk about a bare session id - every routing
// target is tagged at the type level so a web_session_id can never be
// mis-delivered as a cli_session_id (or vice versa).
// =============================================================================

// internalSSEPushPayload mirrors the wire shape produced by g8e-compatible agentic ensembles
// (SessionEventWire | BackgroundEventWire). Producers MUST set exactly one of
// web_session_id (web UI session), cli_session_id (CLI / BYO session), or
// user_id (background fan-out across every session a user owns).
type internalSSEPushPayload struct {
	WebSessionID string          `json:"web_session_id"`
	CliSessionID string          `json:"cli_session_id"`
	UserID       string          `json:"user_id"`
	Event        json.RawMessage `json:"event"`
}

func (h *HTTPHandler) handleInternalSSEPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Strictly verify that the caller is an app workload via mTLS peer certificate URI SAN
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		h.logger.Warn("Unauthorized SSE push attempt: missing mTLS client certificate", "path", r.URL.Path)
		h.responder.Error(w, http.StatusUnauthorized, "mTLS client certificate required")
		return
	}

	cert := r.TLS.PeerCertificates[0]
	appID := ""
	isAppWorkload := false
	for _, uri := range cert.URIs {
		// Only g8e-compatible agentic ensembles are authorized to push SSE events, as they act as the centralized event broker
		// between LLM generations and the end user. Accept any app workload identity (SPIFFE ID with /app/ prefix)
		// except operator identities (g8eo, g8eg).
		if strings.HasPrefix(uri.Path, "/app/") && uri.Path != "/app/g8eo" && uri.Path != "/app/g8eg" {
			isAppWorkload = true
			appID = uri.String()
			break
		}
	}
	if !isAppWorkload {
		h.logger.Warn("Unauthorized SSE push attempt: not app workload identity", "path", r.URL.Path, "uris", cert.URIs)
		h.responder.Error(w, http.StatusForbidden, "unauthorized client identity")
		return
	}

	body, err := h.readBody(r)
	if err != nil {
		h.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var p internalSSEPushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		h.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(p.Event) == 0 {
		h.responder.Error(w, http.StatusBadRequest, "event field is required")
		return
	}

	route := SSERoute{
		WebSessionID: strings.TrimSpace(p.WebSessionID),
		CLISessionID: strings.TrimSpace(p.CliSessionID),
		UserID:       strings.TrimSpace(p.UserID),
	}

	// Extract event.type for indexing/filtering. Store the full envelope as the payload.
	var inner struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(p.Event, &inner)
	if inner.Type == "" {
		inner.Type = string(constants.SystemHealthUnknown)
	}

	if err := h.db.SSEEventsAppend(route, inner.Type, string(body), appID); err != nil {
		h.logger.Error("SSE push: failed to append event", string(constants.ConnectionStateError), err, "type", inner.Type)
		h.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	// Authorization: Enforce producer-to-target ownership.
	// The app identity extracted from the peer certificate must be associated with the target.
	if route.WebSessionID != "" {
		webBindKey := sessionWebBindKey(route.WebSessionID)
		raw, found := h.db.KVGet(webBindKey)
		if !found {
			h.logger.Warn("SSE push: target web session has no bound operators", "web_session_id", route.WebSessionID, "app_id", appID)
			h.responder.Error(w, http.StatusForbidden, "target session not found or not bound")
			return
		}
		var operatorSessionIDs []string
		if err := json.Unmarshal([]byte(raw), &operatorSessionIDs); err != nil {
			h.logger.Error("SSE push: failed to parse web session bindings", "web_session_id", route.WebSessionID, "error", err)
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify session ownership")
			return
		}

		// Check if any bound operator session is associated with this appID
		authorized := false
		for _, opSessID := range operatorSessionIDs {
			opDoc, err := h.db.DocGet(marshaler.CollectionName(constants.CollectionOperators), opSessID)
			if err != nil || opDoc == nil {
				continue
			}
			// AppID format in this context is the SPIFFE ID string
			if opDoc.ID == appID || strings.HasSuffix(appID, "/app/"+opSessID) {
				authorized = true
				break
			}
			// Alternative: check if the app is explicitly allowed by the operator's policy or if it's the engine
			// For now, we'll keep it simple: if the app is spiffe://g8e.local/app/<operator_id>, it's authorized.
			// MatchesApp(spiffeID, operatorID) from workload_identity.go
			wid := protocol.NewWorkloadIdentity()
			if wid.MatchesApp(appID, opDoc.ID) {
				authorized = true
				break
			}
		}

		if !authorized {
			h.logger.Warn("SSE push: app not authorized for target web session", "app_id", appID, "web_session_id", route.WebSessionID)
			h.responder.Error(w, http.StatusForbidden, "unauthorized for target session")
			return
		}
	} else if route.CLISessionID != "" {
		doc, err := h.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), route.CLISessionID)
		if err != nil || doc == nil {
			h.logger.Warn("SSE push: target CLI session not found", "cli_session_id", route.CLISessionID, "app_id", appID)
			h.responder.Error(w, http.StatusForbidden, "target session not found")
			return
		}
		var cliSess models.CLISession
		b, _ := json.Marshal(doc.Data)
		if err := json.Unmarshal(b, &cliSess); err != nil {
			h.logger.Error("SSE push: failed to parse CLI session", "cli_session_id", route.CLISessionID, "error", err)
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify session ownership")
			return
		}

		// Verify app owns the operator session bound to this CLI session
		opDoc, err := h.db.DocGet(marshaler.CollectionName(constants.CollectionOperators), cliSess.OperatorSessionID)
		if err != nil || opDoc == nil {
			h.logger.Warn("SSE push: operator session for CLI session not found", "operator_session_id", cliSess.OperatorSessionID, "cli_session_id", route.CLISessionID)
			h.responder.Error(w, http.StatusForbidden, "operator session not found")
			return
		}

		wid := protocol.NewWorkloadIdentity()
		if !wid.MatchesApp(appID, opDoc.ID) {
			h.logger.Warn("SSE push: app not authorized for target CLI session", "app_id", appID, "cli_session_id", route.CLISessionID)
			h.responder.Error(w, http.StatusForbidden, "unauthorized for target session")
			return
		}
	} else if route.UserID != "" {
		// User-scoped pushes: app must be authorized for AT LEAST ONE session belonging to the user.
		// We check if the app identity corresponds to an operator owned by this user.
		filters := []models.DocFilter{
			{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", route.UserID))},
		}
		docs, err := h.db.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 100)
		if err != nil || len(docs) == 0 {
			h.logger.Warn("SSE push: user has no operators", "user_id", route.UserID, "app_id", appID)
			h.responder.Error(w, http.StatusForbidden, "unauthorized for target user")
			return
		}

		// Check if the app is authorized for any of the user's operators
		authorized := false
		wid := protocol.NewWorkloadIdentity()
		for _, doc := range docs {
			if wid.MatchesApp(appID, doc.ID) {
				authorized = true
				break
			}
		}

		if !authorized {
			h.logger.Warn("SSE push: app not authorized for target user", "app_id", appID, "user_id", route.UserID)
			h.responder.Error(w, http.StatusForbidden, "unauthorized for target user")
			return
		}
	}

	// Publish to pub/sub for real-time streaming
	// We use the same routing logic: exactly one of web_session_id, cli_session_id, or user_id.
	var channel string
	switch {
	case route.CLISessionID != "":
		channel = "sse:cli:" + route.CLISessionID
	case route.WebSessionID != "":
		channel = "sse:web:" + route.WebSessionID
	case route.UserID != "":
		channel = "sse:user:" + route.UserID
	}

	if channel != "" {
		// We publish the full body which is the internalSSEPushPayload JSON.
		// The streamer will wrap this in SSE format.
		h.pubsub.Publish(channel, body)
	}

	h.responder.JSON(w, http.StatusOK, models.SSEPushResponse{
		Success:   true,
		Delivered: 1,
	})
}

func (h *HTTPHandler) handleInternalSSEEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := r.URL.Query()
	route := SSERoute{
		WebSessionID: strings.TrimSpace(q.Get("web_session_id")),
		CLISessionID: strings.TrimSpace(q.Get("cli_session_id")),
		UserID:       strings.TrimSpace(q.Get("user_id")),
	}
	sinceID, _ := strconv.ParseInt(q.Get("since_id"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))

	// Authorization: ensure the authenticated operator session has the right
	// to access the requested routing buffer. Without this check, any operator
	// could drain any other client's event buffer, creating a multi-tenant
	// data leak.
	operatorSessionID := h.auth.ExtractOperatorSessionID(r)
	if operatorSessionID == "" {
		h.responder.Error(w, http.StatusUnauthorized, "missing operator session id")
		return
	}

	// Consumers MUST declare exactly one routing target. The Gateway refuses
	// to fall back to a single shared namespace because that is precisely the
	// conflation we are eliminating.
	switch {
	case route.CLISessionID != "" && route.WebSessionID == "" && route.UserID == "":
		// Verify operator_session_id is bound to this cli_session_id.
		doc, err := h.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), route.CLISessionID)
		if err != nil {
			h.logger.Error("Failed to fetch CLI session", string(constants.ConnectionStateError), err, "cli_session_id", route.CLISessionID)
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if doc == nil {
			h.responder.Error(w, http.StatusForbidden, "cli session not found")
			return
		}
		var cliSess models.CLISession
		b, _ := json.Marshal(doc.ForWire())
		if err := json.Unmarshal(b, &cliSess); err != nil {
			h.logger.Error("Failed to unmarshal CLI session", string(constants.ConnectionStateError), err)
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if cliSess.OperatorSessionID != operatorSessionID {
			h.responder.Error(w, http.StatusForbidden, "operator session does not own this cli session")
			return
		}
	case route.WebSessionID != "" && route.CLISessionID == "" && route.UserID == "":
		// Verify operator_session_id is bound to this web_session_id.
		operatorBindKey := sessionOperatorBindKey(operatorSessionID)
		boundWebSessionID, found := h.db.KVGet(operatorBindKey)
		if !found || boundWebSessionID != route.WebSessionID {
			h.responder.Error(w, http.StatusForbidden, "operator session does not own this web session")
			return
		}
	case route.UserID != "" && route.WebSessionID == "" && route.CLISessionID == "":
		// User-scoped events are accessible to any operator owned by that user.
		op, err := h.auth.ValidateOperatorSession(operatorSessionID)
		if err != nil {
			h.responder.Error(w, http.StatusUnauthorized, "invalid operator session")
			return
		}
		if op.UserID != route.UserID {
			h.responder.Error(w, http.StatusForbidden, "operator does not belong to this user")
			return
		}
	default:
		h.responder.Error(w, http.StatusBadRequest, "exactly one of web_session_id, cli_session_id, user_id is required")
		return
	}

	rows, err := h.db.SSEEventsListSince(route, sinceID, limit)
	if err != nil {
		h.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	h.responder.JSON(w, http.StatusOK, models.SSEEventsResponse{
		Events: rows,
		Count:  len(rows),
	})
}

func (h *HTTPHandler) handleInternalSSEStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	route := SSERoute{
		WebSessionID: strings.TrimSpace(q.Get("web_session_id")),
		CLISessionID: strings.TrimSpace(q.Get("cli_session_id")),
		UserID:       strings.TrimSpace(q.Get("user_id")),
	}
	sinceIDStr := q.Get("since_id")
	if lastEventID := r.Header.Get("Last-Event-ID"); lastEventID != "" {
		sinceIDStr = lastEventID
	}
	sinceID, _ := strconv.ParseInt(sinceIDStr, 10, 64)

	// 1. Authorization (re-use logic from handleInternalSSEEvents)
	operatorSessionID := h.auth.ExtractOperatorSessionID(r)
	if operatorSessionID == "" {
		h.responder.Error(w, http.StatusUnauthorized, "missing operator session id")
		return
	}

	var channel string
	switch {
	case route.CLISessionID != "" && route.WebSessionID == "" && route.UserID == "":
		doc, err := h.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), route.CLISessionID)
		if err != nil || doc == nil {
			h.responder.Error(w, http.StatusForbidden, "not authorized for this cli session")
			return
		}
		var cliSess models.CLISession
		b, _ := json.Marshal(doc.ForWire())
		if err := json.Unmarshal(b, &cliSess); err != nil {
			h.responder.Error(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if cliSess.OperatorSessionID != operatorSessionID {
			h.responder.Error(w, http.StatusForbidden, "not authorized for this cli session")
			return
		}
		channel = "sse:cli:" + route.CLISessionID
	case route.WebSessionID != "" && route.CLISessionID == "" && route.UserID == "":
		operatorBindKey := sessionOperatorBindKey(operatorSessionID)
		boundWebSessionID, found := h.db.KVGet(operatorBindKey)
		if !found || boundWebSessionID != route.WebSessionID {
			h.responder.Error(w, http.StatusForbidden, "not authorized for this web session")
			return
		}
		channel = "sse:web:" + route.WebSessionID
	case route.UserID != "" && route.WebSessionID == "" && route.CLISessionID == "":
		op, err := h.auth.ValidateOperatorSession(operatorSessionID)
		if err != nil || op.UserID != route.UserID {
			h.responder.Error(w, http.StatusForbidden, "not authorized for this user")
			return
		}
		channel = "sse:user:" + route.UserID
	default:
		h.responder.Error(w, http.StatusBadRequest, "exactly one routing target required")
		return
	}

	// 2. Set SSE Headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no") // For Nginx

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// 3. Subscribe to real-time events FIRST to avoid missing any during replay
	eventCh := make(chan []byte, 100)
	unregister := h.pubsub.RegisterHandler(channel, func(ch string, data []byte) {
		select {
		case eventCh <- data:
		default:
			h.logger.Warn("SSE Stream: back-pressure dropping event", "channel", channel)
		}
	})
	defer unregister()

	// 4. Replay from DB if sinceID is provided
	if sinceID > 0 {
		rows, err := h.db.SSEEventsListSince(route, sinceID, 1000)
		if err == nil {
			for _, row := range rows {
				fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", row.ID, row.EventType, row.Payload)
			}
			flusher.Flush()
		}
	}

	// 5. Stream from pubsub
	ctx := r.Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	h.logger.Info("SSE Stream: client connected", "channel", channel, "operator_session_id", operatorSessionID)

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("SSE Stream: client disconnected", "channel", channel)
			return
		case <-ticker.C:
			// Heartbeat
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case raw := <-eventCh:
			// The raw data from internalSSEPush is the full JSON payload
			var p internalSSEPushPayload
			if err := json.Unmarshal(raw, &p); err == nil {
				// We need the ID from the DB append, but we don't have it here easily
				// without doing another query. For now, we use a 0 or skip ID for real-time.
				// Actually, we can just omit 'id:' for real-time pushes and let the client
				// rely on the sequence. Or we can have SSEEventsAppend return the ID and
				// pass it through pubsub.

				// Re-extract type
				var inner struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal(p.Event, &inner)
				if inner.Type == "" {
					inner.Type = string(constants.SystemHealthUnknown)
				}

				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", inner.Type, string(raw))
				flusher.Flush()
			}
		}
	}
}

// =============================================================================
// /kv/{key} - KV Store
//
// GET    /kv/{key}           → get value
// PUT    /kv/{key}           → set value (body: {"value":"...", "ttl": seconds})
// DELETE /kv/{key}           → delete key
// POST   /kv/_keys           → list all keys matching pattern (body: {"pattern":"..."})
// POST   /kv/_scan           → paginated key scan (body: {"pattern":"...", "cursor": N, "count": N})
// POST   /kv/_delete_pattern → delete keys matching pattern (body: {"pattern":"..."})
// GET    /kv/{key}/_ttl      → get TTL
// PUT    /kv/{key}/_expire   → set TTL (body: {"ttl": seconds})
// =============================================================================

// =============================================================================
// /blob/{namespace}/{id}      - Blob Store
// /blob/{namespace}/{id}/meta - Blob metadata
// /blob/{namespace}           - Namespace-level delete
//
// PUT    /blob/{namespace}/{id}       → store blob (raw bytes, Content-Type header required, optional X-Blob-TTL seconds)
// GET    /blob/{namespace}/{id}       → retrieve blob (streams raw bytes with original Content-Type)
// DELETE /blob/{namespace}/{id}       → delete single blob
// GET    /blob/{namespace}/{id}/meta  → metadata only (no data)
// DELETE /blob/{namespace}            → delete all blobs in namespace
// =============================================================================
// Note: blob handler moved to db_controller.go

// =============================================================================
// /pubsub/publish - HTTP-based publish (for components that don't hold a WS)
//
// POST /pubsub/publish  body: {"channel":"...", "data": {...}}
// =============================================================================
