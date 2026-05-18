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

package listen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/marshaler"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sqliteutil"
	"github.com/google/uuid"
)

const governanceEnvelopeRedirectError = "submit via POST /api/governance/envelope"

func readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 50*1024*1024))
}

// HTTPHandler manages the web API for the listen service.
type HTTPHandler struct {
	cfg               *config.Config
	logger            *slog.Logger
	db                *ListenDBService
	pubsub            *PubSubBroker
	auth              *AuthService
	pki               *PKIAuthority
	reg               *RegistrationService
	passkey           *PasskeyService
	userSvc           *UserService
	apiKey            *ApiKeyService
	isReady           func() bool
	isGovernanceReady func() bool
	// envProc is the synchronous fail-closed substrate mutation gate. It is
	// nil until SetEnvelopeProcessor is called by the boot sequence after
	// the in-process command service has initialized the verifier and
	// Warden. While nil, /api/governance/envelope returns 503.
	envProc EnvelopeProcessor
}

func newHTTPHandler(cfg *config.Config, logger *slog.Logger, db *ListenDBService, pubsub *PubSubBroker, auth *AuthService, pki *PKIAuthority, reg *RegistrationService, passkey *PasskeyService, userSvc *UserService, apiKey *ApiKeyService, isReady func() bool, isGovernanceReady func() bool) *HTTPHandler {
	return &HTTPHandler{
		cfg:               cfg,
		logger:            logger,
		db:                db,
		pubsub:            pubsub,
		auth:              auth,
		pki:               pki,
		reg:               reg,
		passkey:           passkey,
		userSvc:           userSvc,
		apiKey:            apiKey,
		isReady:           isReady,
		isGovernanceReady: isGovernanceReady,
	}
}

func (h *HTTPHandler) buildBootstrapRouter() http.Handler {
	mux := http.NewServeMux()

	// Bootstrap routes that do not require client certificates
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/.well-known/g8e/pki/root.pem", h.handlePKIRoot)
	mux.HandleFunc("/.well-known/g8e/pki/hub-bundle.pem", h.handlePKIHubBundle)
	mux.HandleFunc("/.well-known/g8e/pki/fingerprint", h.handlePKIFingerprint)
	mux.HandleFunc("/api/auth/device-link/register", h.handleDeviceLinkRegister)
	mux.HandleFunc("/api/auth/device-link/request", h.handleDeviceLinkRequest)

	// Trust Portal
	mux.HandleFunc("/ca.crt", h.handlePKIRoot)
	mux.HandleFunc("/trust", h.handleTrustScript)
	mux.HandleFunc("/trust.sh", h.handleTrustScript)
	mux.HandleFunc("/trust.ps1", h.handleTrustScriptPS1)
	mux.HandleFunc("/trust.bat", h.handleTrustScriptBat)
	mux.HandleFunc("/g8e", h.handleG8eDeploy)

	return pathTraversalGuard(mux)
}

func (h *HTTPHandler) buildRouter() http.Handler {
	mux := http.NewServeMux()

	// Health check (available internally)
	mux.HandleFunc("/health", h.handleHealth)

	// Authenticated routes (require mTLS)
	mux.HandleFunc("/api/settings", h.handleSettings)
	mux.HandleFunc("/api/device-links", h.handleDeviceLinks)
	mux.HandleFunc("/api/device-links/", h.handleDeviceLinkByToken)
	mux.HandleFunc("/api/operators", h.handleOperators)
	mux.HandleFunc("/api/operators/rotate-api-key", h.handleRotateAPIKey)
	mux.HandleFunc("/api/operators/terminate", h.handleTerminateOperator)
	mux.HandleFunc("/api/operators/reauth", h.handleReauth)
	mux.HandleFunc("/api/operators/bind", h.handleBindOperators)
	mux.HandleFunc("/api/operators/unbind", h.handleUnbindOperators)
	mux.HandleFunc("/api/operators/target", h.handleSetTargetContext)
	mux.HandleFunc("/api/governance/signers", h.handleTrustedSigners)
	mux.HandleFunc("/api/governance/signers/", h.handleTrustedSignerByID)
	// Canonical synchronous fail-closed mutation entry. BYO clients submit
	// UAP JSON envelopes here to receive a signed ActionReceipt.
	mux.HandleFunc("/api/governance/envelope", h.handleGovernanceEnvelope)
	mux.HandleFunc("/api/audit/receipts", h.handleAuditReceipts)
	mux.HandleFunc("/api/audit/receipts/export", h.handleAuditReceiptsExport)

	// Internal SSE event bridge (used by g8ee Engine to publish typed events
	// for browser/CLI subscribers to consume). Producers are authenticated by
	// mTLS app identity; consumers poll /api/internal/sse/events or stream /api/internal/sse/stream.
	mux.HandleFunc("/api/internal/sse/push", h.handleInternalSSEPush)
	mux.HandleFunc("/api/internal/sse/events", h.handleInternalSSEEvents)
	mux.HandleFunc("/api/internal/sse/stream", h.handleInternalSSEStream)
	mux.HandleFunc("/db/", h.handleDB)
	mux.HandleFunc("/kv/", h.handleKV)
	mux.HandleFunc("/pubsub/publish", h.handlePubSubPublish)
	mux.Handle("/ws/pubsub", h.auth.WebSocketAuth(http.HandlerFunc(h.pubsub.HandleWebSocket)))
	mux.HandleFunc("/blob/", h.handleBlob)

	// PKI management routes (require mTLS)
	mux.HandleFunc("/api/pki/sign-csr", h.handlePKISignCSR)
	mux.HandleFunc("/api/pki/revoke", h.handlePKIRevoke)
	mux.HandleFunc("/api/pki/revocation-bundle", h.handlePKIRevocationBundle)

	// User management routes (require mTLS)
	mux.HandleFunc("/api/users", h.handleUsers)

	// Passkey / L3 Brokerage Routes (require mTLS)
	mux.HandleFunc("/api/auth/passkey/register-challenge", h.handlePasskeyRegisterChallenge)
	mux.HandleFunc("/api/auth/passkey/register-verify", h.handlePasskeyRegisterVerify)
	mux.HandleFunc("/api/auth/passkey/auth-challenge", h.handlePasskeyAuthChallenge)
	mux.HandleFunc("/api/auth/passkey/auth-verify", h.handlePasskeyAuthVerify)
	mux.HandleFunc("/api/auth/passkey/credentials", h.handlePasskeyCredentials)
	mux.HandleFunc("/api/auth/passkey/credentials/", h.handlePasskeyRevokeCredential)

	return pathTraversalGuard(h.auth.Middleware(mux))
}

func (h *HTTPHandler) buildPublicRouter() http.Handler {
	mux := http.NewServeMux()

	// Public auth routes (browser-reachable, no mTLS, uses session cookies)
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/api/auth/login/challenge", h.handleAuthLoginChallenge)
	mux.HandleFunc("/api/auth/login/verify", h.handleAuthLoginVerify)
	mux.HandleFunc("/api/auth/logout", h.handleAuthLogout)
	mux.HandleFunc("/api/auth/bootstrap", h.handleBootstrap)
	mux.HandleFunc("/api/auth/bootstrap/status", h.handleBootstrapStatus)

	// PKI discovery also available on public port for BYO bootstrap
	mux.HandleFunc("/.well-known/g8e/pki/root.pem", h.handlePKIRoot)
	mux.HandleFunc("/.well-known/g8e/pki/hub-bundle.pem", h.handlePKIHubBundle)
	mux.HandleFunc("/.well-known/g8e/pki/fingerprint", h.handlePKIFingerprint)

	// Browser-facing data routes (require web session cookie)
	authedMux := http.NewServeMux()
	authedMux.HandleFunc("/api/user/me", h.handleUserMe)
	authedMux.HandleFunc("/api/auth/web-session", h.handleWebSession)

	// [PIVOT] Move passkey registration to public authed mux so bootstrapped users can register
	authedMux.HandleFunc("/api/auth/passkey/register-challenge", h.handlePasskeyRegisterChallenge)
	authedMux.HandleFunc("/api/auth/passkey/register-verify", h.handlePasskeyRegisterVerify)

	// Wrap authed routes in WebSessionAuth middleware
	mux.Handle("/api/user/", h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle("/api/auth/web-session", h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle("/api/auth/passkey/", h.auth.WebSessionAuth(authedMux, h.db))

	return pathTraversalGuard(mux)
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.buildRouter().ServeHTTP(w, r)
}

// pathTraversalGuard rejects any request whose raw URL path contains a ".."
// segment before Go's ServeMux can normalize the path and issue a 301 redirect.
func pathTraversalGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path to handle multiple slashes, etc.
		cleaned := filepath.ToSlash(filepath.Clean(r.URL.Path))
		if containsTraversal(r.URL.Path) || (cleaned != r.URL.Path && cleaned != r.URL.Path+"/" && r.URL.Path != "/") {
			if containsTraversal(r.URL.Path) || strings.Contains(cleaned, "..") {
				jsonError(w, http.StatusBadRequest, "invalid path")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func containsTraversal(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

func isDirectDBMutationAllowed(collection string) bool {
	switch constants.CollectionName(collection) {
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

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set(constants.HeaderContentType, "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, struct {
		Error string `json:"error"`
	}{Error: msg})
}

func (h *HTTPHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if h.isReady != nil && !h.isReady() {
		jsonError(w, http.StatusServiceUnavailable, "service initializing")
		return
	}

	doc, err := h.db.DocGet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings))
	if err != nil {
		h.logger.Error("Health check failed to query platform_settings", "error", err)
		jsonError(w, http.StatusServiceUnavailable, "platform_settings not ready")
		return
	}
	if doc == nil {
		h.logger.Warn("Health check: platform_settings not found")
		jsonError(w, http.StatusServiceUnavailable, "platform_settings not ready")
		return
	}

	root, err := h.db.GetCurrentStateRoot()
	if err != nil {
		h.logger.Error("Health check failed to get state root", "error", err)
	}

	jsonResponse(w, http.StatusOK, models.HealthResponse{
		Status:          constants.Status.ListenMode.StatusOK,
		Mode:            constants.Status.ListenMode.Mode,
		Version:         h.cfg.Version,
		GovernanceReady: h.isGovernanceReady != nil && h.isGovernanceReady(),
		StateMerkleRoot: root,
	})
}

func (h *HTTPHandler) handlePKIRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	pem, err := os.ReadFile(filepath.Join(h.pki.PKIDir(), "root", "root_ca.crt"))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to read root CA")
		return
	}
	w.Header().Set(constants.HeaderContentType, "application/x-pem-file")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	w.Write(pem)
}

func (h *HTTPHandler) handlePKIHubBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	pem, err := h.pki.HubTrustBundle()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to read hub bundle")
		return
	}
	w.Header().Set(constants.HeaderContentType, "application/x-pem-file")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	w.Write(pem)
}

func (h *HTTPHandler) handlePKIFingerprint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// Return the SHA-256 fingerprint of the root CA
	pemData, err := os.ReadFile(filepath.Join(h.pki.PKIDir(), "root", "root_ca.crt"))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to read root CA")
		return
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		jsonError(w, http.StatusInternalServerError, "invalid root CA PEM")
		return
	}

	hash := sha256.Sum256(block.Bytes)
	fingerprint := hex.EncodeToString(hash[:])

	jsonResponse(w, http.StatusOK, map[string]string{
		"root_ca": "sha256:" + fingerprint,
	})
}

func (h *HTTPHandler) handlePKISignCSR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		CSR               string `json:"csr_pem"`
		LeafType          string `json:"leaf_type"`
		OrganizationID    string `json:"organization_id"`
		OperatorID        string `json:"operator_id"`
		UserID            string `json:"user_id"`
		WorkloadSessionID string `json:"workload_session_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	certPEM, chainPEM, err := h.pki.SignCSR(req.CSR, req.LeafType, req.OrganizationID, req.OperatorID, req.UserID, req.WorkloadSessionID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{
		"certificate_pem":       certPEM,
		"certificate_chain_pem": chainPEM,
	})
}

func (h *HTTPHandler) handlePKIRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		Serial string `json:"serial"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Serial == "" {
		jsonError(w, http.StatusBadRequest, "serial required")
		return
	}

	if err := h.pki.RevokeCertificate(req.Serial, req.Reason); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})
}

func (h *HTTPHandler) handlePKIRevocationBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	bundleJSON, signature, err := h.pki.GenerateRevocationBundle()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{
		"bundle_json": bundleJSON,
		"signature":   signature,
	})
}

func (h *HTTPHandler) handleDeviceLinkRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.CreateDeviceLinkRequest
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// For bootstrap/public requests, we enforce some constraints
	// e.g. cannot specify a custom UserID directly if we want to be safe,
	// but CreateDeviceLink handles email-to-UserID resolution.
	// We might want to flag these as 'CLI' or 'Bootstrap' requests.

	resp, err := h.reg.CreateDeviceLink(req)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonResponse(w, http.StatusCreated, resp)
}

func (h *HTTPHandler) handleDeviceLinkRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	// We'll reuse the existing RegisterDevice logic but expose it via this new route
	// The token usually comes from the URL in handleDeviceLink, but here we might
	// want it in the body or header for the new protocol.
	// For now, let's just forward to handleDeviceLink style if we have a token.
	token := r.Header.Get(constants.HeaderDeviceToken)
	if token == "" {
		jsonError(w, http.StatusBadRequest, "missing X-G8E-Device-Token header")
		return
	}

	var req models.OperatorRegistrationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	resp, err := h.reg.RegisterDevice(token, req)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, resp)
}

// =============================================================================
// /db/{collection}/{id} — Document Store
//
// GET    /db/{collection}/{id}       → get document
// PUT    /db/{collection}/{id}       → set (create/replace) document
// PATCH  /db/{collection}/{id}       → update (merge) document
// DELETE /db/{collection}/{id}       → delete document
// POST   /db/{collection}/_query     → query documents
// =============================================================================

func (h *HTTPHandler) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		doc, err := h.db.DocGet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings))
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if doc == nil {
			jsonError(w, http.StatusNotFound, "settings not found")
			return
		}
		jsonResponse(w, http.StatusOK, doc.ForWire())
	case http.MethodPut, http.MethodPatch:
		body, err := readBody(r)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid body")
			return
		}
		var err2 error
		if r.Method == http.MethodPut {
			err2 = h.db.DocSet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings), json.RawMessage(body))
		} else {
			_, err2 = h.db.DocUpdate(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings), json.RawMessage(body))
		}
		if err2 != nil {
			jsonError(w, http.StatusInternalServerError, err2.Error())
			return
		}
		jsonResponse(w, http.StatusOK, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})
	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *HTTPHandler) handleDeviceLinks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		body, err := readBody(r)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		var req models.CreateDeviceLinkRequest
		if err := json.Unmarshal(body, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		resp, err := h.reg.CreateDeviceLink(req)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, resp)
	case http.MethodGet:
		userID := r.URL.Query().Get("user_id")
		links, err := h.reg.ListDeviceLinks(userID)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, models.DeviceLinkListResponse{Success: true, Links: links})
	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *HTTPHandler) handleDeviceLinkByToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/api/device-links/")
	if token == "" || strings.Contains(token, "/") {
		jsonError(w, http.StatusBadRequest, "token required")
		return
	}
	userID := r.URL.Query().Get("user_id")
	if err := h.reg.DeleteDeviceLink(token, userID); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})
}

func (h *HTTPHandler) handleOperators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		jsonError(w, http.StatusBadRequest, "user_id required")
		return
	}
	slots, err := h.reg.ListOperatorSlots(userID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, models.OperatorSlotResponse{Success: true, Operators: slots})
}

func (h *HTTPHandler) handleG8eDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
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
	script := G8eDeployScript(host, h.cfg.Listen.WSSPort, h.cfg.Listen.BootstrapPort)
	w.Header().Set("Content-Type", "text/x-shellscript")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}

func (h *HTTPHandler) handleTrustScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
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
	script := UniversalTrustScript(host, h.cfg.Listen.BootstrapPort)
	w.Header().Set("Content-Type", "text/x-shellscript")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}

func (h *HTTPHandler) handleTrustScriptPS1(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
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
	script := WindowsPowerShellTrustScript(host, h.cfg.Listen.BootstrapPort)
	w.Header().Set("Content-Type", "text/plain") // PowerShell scripts often served as text or application/powershell
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}

func (h *HTTPHandler) handleTrustScriptBat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
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
	script := WindowsTrustScriptBat(host, h.cfg.Listen.BootstrapPort)
	w.Header().Set("Content-Type", "text/plain") // Batch scripts often served as text
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}

func (h *HTTPHandler) handleRotateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req models.RotateAPIKeyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		jsonError(w, http.StatusBadRequest, "user_id required")
		return
	}
	if err := h.reg.RotateOperatorAPIKey(req.OperatorID, userID); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, models.RotateAPIKeyResponse{Success: true})
}

func (h *HTTPHandler) handleTerminateOperator(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req models.TerminateOperatorRequest
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.OperatorID == "" {
		jsonError(w, http.StatusBadRequest, "operator_id required")
		return
	}
	if req.UserID == "" {
		jsonError(w, http.StatusBadRequest, "user_id required")
		return
	}
	if err := h.reg.TerminateOperator(req.OperatorID, req.UserID, req.Reason); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, models.TerminateOperatorResponse{Success: true, Message: "Operator terminated"})
}

func (h *HTTPHandler) handleBindOperators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req models.BindOperatorsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate ownership
	userID := r.URL.Query().Get("user_id")
	if userID != "" && req.UserID != userID {
		jsonError(w, http.StatusForbidden, "user_id mismatch")
		return
	}

	resp, err := h.reg.BindOperators(req)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, resp)
}

func (h *HTTPHandler) handleUnbindOperators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req models.UnbindOperatorsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate ownership
	userID := r.URL.Query().Get("user_id")
	if userID != "" && req.UserID != userID {
		jsonError(w, http.StatusForbidden, "user_id mismatch")
		return
	}

	resp, err := h.reg.UnbindOperators(req)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, resp)
}

func (h *HTTPHandler) handleSetTargetContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var req models.SetTargetContextRequest
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate ownership
	userID := r.URL.Query().Get("user_id")
	if userID != "" && req.UserID != userID {
		jsonError(w, http.StatusForbidden, "user_id mismatch")
		return
	}

	resp, err := h.reg.SetTargetContext(req)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, resp)
}

func (h *HTTPHandler) handleReauth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// Reauth is basically a session refresh. For now, we validate the current session.
	sessionID := h.auth.ExtractOperatorSessionID(r)
	if sessionID == "" {
		jsonError(w, http.StatusUnauthorized, "missing session id")
		return
	}
	op, err := h.auth.ValidateOperatorSession(sessionID)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"operator": op,
	})
}

func (h *HTTPHandler) handleDB(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/db/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		jsonError(w, http.StatusBadRequest, "collection required")
		return
	}

	collection := parts[0]
	id := ""
	if len(parts) > 1 {
		id = parts[1]
	}

	if id == "_query" && r.Method == http.MethodPost {
		h.handleDBQuery(w, r, collection)
		return
	}

	if collection == "_sse_events" {
		h.handleSSEEvents(w, r, id)
		return
	}

	if id == "" {
		jsonError(w, http.StatusBadRequest, "document id required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		doc, err := h.db.DocGet(collection, id)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if doc == nil {
			jsonError(w, http.StatusNotFound, fmt.Sprintf("document %s/%s not found", collection, id))
			return
		}
		jsonResponse(w, http.StatusOK, doc.ForWire())

	case http.MethodPut:
		if !isDirectDBMutationAllowed(collection) {
			jsonError(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		body, err := readBody(r)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if !json.Valid(body) {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := h.db.DocSet(collection, id, json.RawMessage(body)); err != nil {
			if strings.Contains(err.Error(), "locked") {
				jsonError(w, http.StatusServiceUnavailable, "database is locked")
			} else {
				jsonError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		jsonResponse(w, http.StatusOK, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})

	case http.MethodPatch:
		if !isDirectDBMutationAllowed(collection) {
			jsonError(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		body, err := readBody(r)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if !json.Valid(body) {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		doc, err := h.db.DocUpdate(collection, id, json.RawMessage(body))
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				jsonError(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "constraint") {
				jsonError(w, http.StatusConflict, "database constraint violation")
			} else if strings.Contains(err.Error(), "locked") {
				jsonError(w, http.StatusServiceUnavailable, "database is locked")
			} else {
				jsonError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		jsonResponse(w, http.StatusOK, doc.ForWire())

	case http.MethodDelete:
		if !isDirectDBMutationAllowed(collection) {
			jsonError(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		deleted, err := h.db.DocDelete(collection, id)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			jsonError(w, http.StatusNotFound, "document not found")
			return
		}
		jsonResponse(w, http.StatusOK, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// =============================================================================
// /db/_sse_events — SSE Event Buffer management
//
// DELETE /db/_sse_events         → wipe all SSE events
// GET    /db/_sse_events/count   → count rows
// =============================================================================

func (h *HTTPHandler) handleAuditReceipts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	txID := r.URL.Query().Get("tx_id")
	if txID != "" {
		receipt, err := h.db.AuditVault.GetActionReceipt(txID)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if receipt == nil {
			jsonError(w, http.StatusNotFound, "receipt not found")
			return
		}
		jsonResponse(w, http.StatusOK, receipt)
		return
	}

	operatorSessionID := r.URL.Query().Get("operator_session_id")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	receipts, err := h.db.AuditVault.ListActionReceipts(operatorSessionID, limit, offset)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"receipts": receipts,
	})
}

func (h *HTTPHandler) handleAuditReceiptsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sinceStr := r.URL.Query().Get("since")
	limitStr := r.URL.Query().Get("limit")

	since := time.Time{}
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		} else if t, err := sqliteutil.ParseTimestamp(sinceStr); err == nil {
			since = t
		}
	}

	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	receipts, err := h.db.AuditVault.ListActionReceiptsSince(since, limit)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	for _, r := range receipts {
		if err := encoder.Encode(r); err != nil {
			h.logger.Error("Failed to encode audit receipt for export", "transaction_id", r.TransactionID, "error", err)
			break
		}
	}
}

func (h *HTTPHandler) handleTrustedSigners(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		signers, err := h.db.ListTrustedSigners()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"signers": signers,
		})

	case http.MethodPost:
		body, err := readBody(r)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		var signer models.TrustedSigner
		if err := json.Unmarshal(body, &signer); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if signer.ID == "" || signer.PublicKey == "" {
			jsonError(w, http.StatusBadRequest, "id and public_key_hex required")
			return
		}
		if err := h.db.AddTrustedSigner(signer); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *HTTPHandler) handleTrustedSignerByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/governance/signers/")
	if id == "" || strings.Contains(id, "/") {
		jsonError(w, http.StatusBadRequest, "invalid signer id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		pubKey, err := h.db.GetTrustedSigner(id)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if pubKey == nil {
			jsonError(w, http.StatusNotFound, "signer not found")
			return
		}
		// Return metadata rather than raw bytes
		doc, _ := h.db.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), id)
		jsonResponse(w, http.StatusOK, doc.ForWire())

	case http.MethodDelete:
		deleted, err := h.db.DeleteTrustedSigner(id)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			jsonError(w, http.StatusNotFound, "signer not found")
			return
		}
		jsonResponse(w, http.StatusOK, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *HTTPHandler) handleSSEEvents(w http.ResponseWriter, r *http.Request, id string) {
	if id == "count" && r.Method == http.MethodGet {
		count, err := h.db.SSEEventsCount()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]int64{"count": count})
		return
	}

	if id == "" && r.Method == http.MethodDelete {
		deleted, err := h.db.SSEEventsWipe()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]int64{"deleted": deleted})
		return
	}

	jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// =============================================================================
// /api/internal/sse/push, /api/internal/sse/events — Internal SSE event bridge
//
// POST /api/internal/sse/push     → Producer (g8ee Engine) appends an event.
//                                   Body MUST set exactly one of
//                                   web_session_id, cli_session_id, user_id.
// GET  /api/internal/sse/events   → Consumer (CLI / dashboard) polls events.
//                                   Query string MUST set exactly one of
//                                   web_session_id, cli_session_id, user_id,
//                                   plus since_id=N and limit=K.
//
// The substrate refuses to talk about a bare session id — every routing
// target is tagged at the type level so a web_session_id can never be
// mis-delivered as a cli_session_id (or vice versa).
// =============================================================================

// internalSSEPushPayload mirrors the wire shape produced by g8ee
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
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var p internalSSEPushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(p.Event) == 0 {
		jsonError(w, http.StatusBadRequest, "event field is required")
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
		inner.Type = "unknown"
	}

	if err := h.db.SSEEventsAppend(route, inner.Type, string(body)); err != nil {
		h.logger.Error("SSE push: failed to append event", "error", err, "type", inner.Type)
		jsonError(w, http.StatusBadRequest, err.Error())
		return
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

	jsonResponse(w, http.StatusOK, map[string]any{
		"success":   true,
		"delivered": 1,
	})
}

func (h *HTTPHandler) handleInternalSSEEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
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
		jsonError(w, http.StatusUnauthorized, "missing operator session id")
		return
	}

	// Consumers MUST declare exactly one routing target. The substrate refuses
	// to fall back to a single shared namespace because that is precisely the
	// conflation we are eliminating.
	switch {
	case route.CLISessionID != "" && route.WebSessionID == "" && route.UserID == "":
		// Verify operator_session_id is bound to this cli_session_id.
		doc, err := h.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), route.CLISessionID)
		if err != nil {
			h.logger.Error("Failed to fetch CLI session", "error", err, "cli_session_id", route.CLISessionID)
			jsonError(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if doc == nil {
			jsonError(w, http.StatusForbidden, "cli session not found")
			return
		}
		var cliSess models.CLISession
		b, _ := json.Marshal(doc.ForWire())
		if err := json.Unmarshal(b, &cliSess); err != nil {
			h.logger.Error("Failed to unmarshal CLI session", "error", err)
			jsonError(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if cliSess.OperatorSessionID != operatorSessionID {
			jsonError(w, http.StatusForbidden, "operator session does not own this cli session")
			return
		}
	case route.WebSessionID != "" && route.CLISessionID == "" && route.UserID == "":
		// Verify operator_session_id is bound to this web_session_id.
		operatorBindKey := sessionOperatorBindKey(operatorSessionID)
		boundWebSessionID, found := h.db.KVGet(operatorBindKey)
		if !found || boundWebSessionID != route.WebSessionID {
			jsonError(w, http.StatusForbidden, "operator session does not own this web session")
			return
		}
	case route.UserID != "" && route.WebSessionID == "" && route.CLISessionID == "":
		// User-scoped events are accessible to any operator owned by that user.
		op, err := h.auth.ValidateOperatorSession(operatorSessionID)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid operator session")
			return
		}
		if op.UserID != route.UserID {
			jsonError(w, http.StatusForbidden, "operator does not belong to this user")
			return
		}
	default:
		jsonError(w, http.StatusBadRequest, "exactly one of web_session_id, cli_session_id, user_id is required")
		return
	}

	rows, err := h.db.SSEEventsListSince(route, sinceID, limit)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"events": rows,
		"count":  len(rows),
	})
}

func (h *HTTPHandler) handleInternalSSEStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	route := SSERoute{
		WebSessionID: strings.TrimSpace(q.Get("web_session_id")),
		CLISessionID: strings.TrimSpace(q.Get("cli_session_id")),
		UserID:       strings.TrimSpace(q.Get("user_id")),
	}
	sinceID, _ := strconv.ParseInt(q.Get("since_id"), 10, 64)

	// 1. Authorization (re-use logic from handleInternalSSEEvents)
	operatorSessionID := h.auth.ExtractOperatorSessionID(r)
	if operatorSessionID == "" {
		jsonError(w, http.StatusUnauthorized, "missing operator session id")
		return
	}

	var channel string
	switch {
	case route.CLISessionID != "" && route.WebSessionID == "" && route.UserID == "":
		doc, err := h.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), route.CLISessionID)
		if err != nil || doc == nil {
			jsonError(w, http.StatusForbidden, "not authorized for this cli session")
			return
		}
		var cliSess models.CLISession
		b, _ := json.Marshal(doc.ForWire())
		if err := json.Unmarshal(b, &cliSess); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to verify cli session")
			return
		}
		if cliSess.OperatorSessionID != operatorSessionID {
			jsonError(w, http.StatusForbidden, "not authorized for this cli session")
			return
		}
		channel = "sse:cli:" + route.CLISessionID
	case route.WebSessionID != "" && route.CLISessionID == "" && route.UserID == "":
		operatorBindKey := sessionOperatorBindKey(operatorSessionID)
		boundWebSessionID, found := h.db.KVGet(operatorBindKey)
		if !found || boundWebSessionID != route.WebSessionID {
			jsonError(w, http.StatusForbidden, "not authorized for this web session")
			return
		}
		channel = "sse:web:" + route.WebSessionID
	case route.UserID != "" && route.WebSessionID == "" && route.CLISessionID == "":
		op, err := h.auth.ValidateOperatorSession(operatorSessionID)
		if err != nil || op.UserID != route.UserID {
			jsonError(w, http.StatusForbidden, "not authorized for this user")
			return
		}
		channel = "sse:user:" + route.UserID
	default:
		jsonError(w, http.StatusBadRequest, "exactly one routing target required")
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
					inner.Type = "unknown"
				}

				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", inner.Type, string(raw))
				flusher.Flush()
			}
		}
	}
}

func (h *HTTPHandler) handleDBQuery(w http.ResponseWriter, r *http.Request, collection string) {
	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var req models.DocQueryRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	docs, err := h.db.DocQuery(collection, req.Filters, req.OrderBy, req.Limit)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	wire := make([]map[string]json.RawMessage, 0, len(docs))
	for _, d := range docs {
		wire = append(wire, d.ForWire())
	}
	jsonResponse(w, http.StatusOK, wire)
}

// =============================================================================
// /kv/{key} — KV Store
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

func (h *HTTPHandler) handleKV(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/kv/")
	if path == "" {
		jsonError(w, http.StatusBadRequest, "key required")
		return
	}

	if path == "_keys" && r.Method == http.MethodPost {
		h.handleKVKeys(w, r)
		return
	}
	if path == "_scan" && r.Method == http.MethodPost {
		h.handleKVScan(w, r)
		return
	}
	if path == "_delete_pattern" && r.Method == http.MethodPost {
		h.handleKVDeletePattern(w, r)
		return
	}

	if strings.HasSuffix(path, "/_ttl") {
		key := strings.TrimSuffix(path, "/_ttl")
		ttl := h.db.KVTTL(key)
		jsonResponse(w, http.StatusOK, models.KVTTLResponse{TTL: ttl})
		return
	}
	if strings.HasSuffix(path, "/_expire") && r.Method == http.MethodPut {
		key := strings.TrimSuffix(path, "/_expire")
		body, err := readBody(r)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		var req models.KVExpireRequest
		if err := json.Unmarshal(body, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.TTL <= 0 {
			jsonError(w, http.StatusBadRequest, "ttl required and must be > 0")
			return
		}
		ok := h.db.KVExpire(key, req.TTL)
		if !ok {
			jsonError(w, http.StatusNotFound, "key not found")
			return
		}
		jsonResponse(w, http.StatusOK, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})
		return
	}

	key := path

	switch r.Method {
	case http.MethodGet:
		value, found := h.db.KVGet(key)
		if !found {
			jsonError(w, http.StatusNotFound, "key not found")
			return
		}
		jsonResponse(w, http.StatusOK, models.KVGetResponse{Value: value})

	case http.MethodPut:
		body, err := readBody(r)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		var req models.KVSetRequest
		if err := json.Unmarshal(body, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := h.db.KVSet(key, req.Value, req.TTL); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})

	case http.MethodDelete:
		if err := h.db.KVDelete(key); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *HTTPHandler) handleKVKeys(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var req models.KVPatternRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if req.Pattern == "" {
		req.Pattern = "*"
	}
	keys, err := h.db.KVKeys(req.Pattern)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []string{}
	}
	jsonResponse(w, http.StatusOK, models.KVKeysResponse{Keys: keys})
}

func (h *HTTPHandler) handleKVScan(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var req models.KVPatternRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if req.Pattern == "" {
		req.Pattern = "*"
	}
	if req.Count <= 0 {
		req.Count = 100
	}
	nextCursor, keys, err := h.db.KVScan(req.Pattern, req.Cursor, req.Count)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []string{}
	}
	jsonResponse(w, http.StatusOK, models.KVScanResponse{Cursor: nextCursor, Keys: keys})
}

func (h *HTTPHandler) handleKVDeletePattern(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var req models.KVPatternRequest
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Pattern == "" {
		jsonError(w, http.StatusBadRequest, "pattern required")
		return
	}
	count, err := h.db.KVDeletePattern(req.Pattern)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, models.KVDeletePatternResponse{Deleted: count})
}

// =============================================================================
// /blob/{namespace}/{id}      — Blob Store
// /blob/{namespace}/{id}/meta — Blob metadata
// /blob/{namespace}           — Namespace-level delete
//
// PUT    /blob/{namespace}/{id}       → store blob (raw bytes, Content-Type header required, optional X-Blob-TTL seconds)
// GET    /blob/{namespace}/{id}       → retrieve blob (streams raw bytes with original Content-Type)
// DELETE /blob/{namespace}/{id}       → delete single blob
// GET    /blob/{namespace}/{id}/meta  → metadata only (no data)
// DELETE /blob/{namespace}            → delete all blobs in namespace
// =============================================================================

// blobSegmentValid returns false if s contains characters that could be used
// for path traversal or injection: forward slash, backslash, dot-dot, null byte.
func blobSegmentValid(s string) bool {
	if s == "" || s == ".." {
		return false
	}
	for _, c := range s {
		if c == '/' || c == '\\' || c == 0 {
			return false
		}
	}
	return true
}

const maxBlobBodySize = 15 * 1024 * 1024 // 15 MB hard cap at the transport layer

func (h *HTTPHandler) handleBlob(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/blob/")
	if path == "" {
		jsonError(w, http.StatusBadRequest, "namespace required")
		return
	}

	parts := strings.SplitN(path, "/", 3)
	namespace := parts[0]
	if !blobSegmentValid(namespace) {
		jsonError(w, http.StatusBadRequest, "invalid namespace")
		return
	}

	// DELETE /blob/{namespace} — delete entire namespace
	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		count, err := h.db.BlobDeleteNamespace(namespace)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, models.BlobDeleteResponse{Deleted: count})
		return
	}

	blobID := parts[1]
	if !blobSegmentValid(blobID) {
		jsonError(w, http.StatusBadRequest, "invalid blob id")
		return
	}

	// GET /blob/{namespace}/{id}/meta
	if len(parts) == 3 {
		if parts[2] != "meta" {
			jsonError(w, http.StatusBadRequest, "invalid path")
			return
		}
		if r.Method != http.MethodGet {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		rec, ok := h.db.BlobMeta(namespace, blobID)
		if !ok {
			jsonError(w, http.StatusNotFound, "blob not found")
			return
		}
		jsonResponse(w, http.StatusOK, models.BlobMetaResponse{
			ID:          rec.ID,
			Namespace:   rec.Namespace,
			Size:        rec.Size,
			ContentType: rec.ContentType,
			CreatedAt:   rec.CreatedAt.UTC(),
		})
		return
	}

	switch r.Method {
	case http.MethodPut:
		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			jsonError(w, http.StatusBadRequest, "Content-Type header required")
			return
		}

		ttl := 0
		if v := r.Header.Get("X-Blob-TTL"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				jsonError(w, http.StatusBadRequest, "X-Blob-TTL must be a non-negative integer")
				return
			}
			ttl = n
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxBlobBodySize+1))
		if err != nil {
			jsonError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		if int64(len(body)) > maxBlobBodySize {
			jsonError(w, http.StatusRequestEntityTooLarge, "blob exceeds maximum size")
			return
		}
		if len(body) == 0 {
			jsonError(w, http.StatusBadRequest, "body must not be empty")
			return
		}

		if err := h.db.BlobPut(namespace, blobID, body, contentType, ttl); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, models.StatusResponse{Status: constants.Status.ListenMode.StatusOK})

	case http.MethodGet:
		data, contentType, ok := h.db.BlobGet(namespace, blobID)
		if !ok {
			jsonError(w, http.StatusNotFound, "blob not found")
			return
		}

		// SECURITY: Sanitize content type to prevent XSS.
		// Only allow safe, well-known types or default to application/octet-stream.
		safeContentType := "application/octet-stream"
		allowedTypes := map[string]bool{
			"application/json":       true,
			"application/pdf":        true,
			"image/png":              true,
			"image/jpeg":             true,
			"image/gif":              true,
			"text/plain":             true,
			"application/x-ndjson":   true,
			"application/x-pem-file": true,
		}
		if allowedTypes[contentType] {
			safeContentType = contentType
		}

		w.Header().Set("Content-Type", safeContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.WriteHeader(http.StatusOK)
		w.Write(data) //nolint:errcheck

	case http.MethodDelete:
		deleted, err := h.db.BlobDelete(namespace, blobID)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			jsonError(w, http.StatusNotFound, "blob not found")
			return
		}
		jsonResponse(w, http.StatusOK, models.BlobDeleteResponse{Deleted: 1})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// =============================================================================
// /pubsub/publish — HTTP-based publish (for components that don't hold a WS)
//
// POST /pubsub/publish  body: {"channel":"...", "data": {...}}
// =============================================================================

func (h *HTTPHandler) handlePubSubPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var req models.PubSubPublishRequest
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Channel == "" {
		jsonError(w, http.StatusBadRequest, "channel required")
		return
	}
	if !isMutationPubSubChannelAllowed(req.Channel) {
		jsonError(w, http.StatusConflict, governanceEnvelopeRedirectError)
		return
	}

	// If Data is a JSON string, it might be base64-encoded binary (e.g. GovernanceEnvelope).
	// Try to unmarshal it as []byte first (which handles base64 strings).
	var binData []byte
	var receivers int
	if err := json.Unmarshal(req.Data, &binData); err == nil {
		receivers = h.pubsub.Publish(req.Channel, binData)
	} else {
		// Not a string or not base64, treat as raw JSON fragment
		receivers = h.pubsub.Publish(req.Channel, req.Data)
	}
	jsonResponse(w, http.StatusOK, models.PubSubPublishResponse{Receivers: receivers})
}

// =============================================================================
// Passkey / L3 Brokerage Handlers
// =============================================================================

// handlePasskeyRegisterChallenge generates a WebAuthn registration challenge.
func (h *HTTPHandler) handlePasskeyRegisterChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var req struct {
		UserID   string `json:"user_id"`
		Email    string `json:"email"`
		UserName string `json:"user_name"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// [PIVOT] Enforce session-to-user binding for public browser registration
	if ctxUserID, ok := r.Context().Value("user_id").(string); ok {
		if req.UserID != "" && req.UserID != ctxUserID {
			jsonError(w, http.StatusForbidden, "user_id mismatch with session")
			return
		}
		req.UserID = ctxUserID
	}

	if req.UserID == "" {
		jsonError(w, http.StatusBadRequest, "user_id required")
		return
	}

	options, err := h.passkey.GenerateRegistrationChallenge(req.UserID, req.Email, req.UserName)
	if err != nil {
		h.logger.Warn("Passkey register challenge failed", "error", err, "userID", req.UserID)
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"options": options,
	})
}

// handlePasskeyRegisterVerify verifies a WebAuthn registration attestation.
func (h *HTTPHandler) handlePasskeyRegisterVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var req struct {
		UserID              string               `json:"user_id"`
		AttestationResponse *AttestationResponse `json:"attestation_response"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// [PIVOT] Enforce session-to-user binding for public browser registration
	if ctxUserID, ok := r.Context().Value("user_id").(string); ok {
		if req.UserID != "" && req.UserID != ctxUserID {
			jsonError(w, http.StatusForbidden, "user_id mismatch with session")
			return
		}
		req.UserID = ctxUserID
	}

	if req.UserID == "" {
		jsonError(w, http.StatusBadRequest, "user_id required")
		return
	}

	cred, err := h.passkey.VerifyRegistration(req.UserID, r)
	if err != nil {
		h.logger.Warn("Passkey register verify failed", "error", err, "userID", req.UserID)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"credential": cred,
	})
}

// handlePasskeyAuthChallenge generates a WebAuthn authentication challenge.
func (h *HTTPHandler) handlePasskeyAuthChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var req struct {
		Email  string `json:"email"`
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	userID := req.UserID
	if ctxUserID, ok := r.Context().Value("user_id").(string); ok {
		if userID != "" && userID != ctxUserID {
			jsonError(w, http.StatusForbidden, "user_id mismatch with session")
			return
		}
		userID = ctxUserID
	}

	if userID == "" {
		jsonError(w, http.StatusBadRequest, "user_id required")
		return
	}

	options, err := h.passkey.GenerateAuthenticationChallenge(userID)
	if err != nil {
		h.logger.Warn("Passkey auth challenge failed", "error", err, "userID", userID)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success":     false,
			"error":       err.Error(),
			"needs_setup": err.Error() == "no passkeys registered",
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"options": options,
	})
}

// handlePasskeyAuthVerify verifies a WebAuthn authentication assertion.
func (h *HTTPHandler) handlePasskeyAuthVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var req struct {
		Email             string             `json:"email"`
		UserID            string             `json:"user_id"`
		AssertionResponse *AssertionResponse `json:"assertion_response"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	userID := req.UserID
	if ctxUserID, ok := r.Context().Value("user_id").(string); ok {
		if userID != "" && userID != ctxUserID {
			jsonError(w, http.StatusForbidden, "user_id mismatch with session")
			return
		}
		userID = ctxUserID
	}

	if userID == "" {
		jsonError(w, http.StatusBadRequest, "user_id required")
		return
	}

	cred, err := h.passkey.VerifyAuthentication(userID, r)
	if err != nil {
		h.logger.Warn("Passkey auth verify failed", "error", err, "userID", userID)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	webSession, err := h.passkey.CreateWebSession(userID)
	if err != nil {
		h.logger.Error("Failed to create web session after auth", "error", err, "userID", userID)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "authentication succeeded but session creation failed",
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"user_id":    userID,
		"credential": cred,
		"web_session": map[string]interface{}{
			"id":                 webSession.ID,
			"expires_at_unix_ms": webSession.ExpiresAtUnixMs,
		},
	})
}

// handlePasskeyCredentials lists passkey credentials for a user.
func (h *HTTPHandler) handlePasskeyCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		jsonError(w, http.StatusBadRequest, "user_id required")
		return
	}

	creds, err := h.passkey.ListCredentials(userID)
	if err != nil {
		h.logger.Error("Failed to list credentials", "error", err, "userID", userID)
		jsonError(w, http.StatusInternalServerError, "failed to list credentials")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"credentials": creds,
	})
}

// handlePasskeyRevokeCredential revokes a specific passkey credential.
func (h *HTTPHandler) handlePasskeyRevokeCredential(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		jsonError(w, http.StatusBadRequest, "user_id required")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/auth/passkey/credentials/")
	if path == "" {
		jsonError(w, http.StatusBadRequest, "credential_id required")
		return
	}

	found, remaining, err := h.passkey.RevokeCredential(userID, path)
	if err != nil {
		h.logger.Error("Failed to revoke credential", "error", err, "userID", userID)
		jsonError(w, http.StatusInternalServerError, "failed to revoke credential")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"found":     found,
		"remaining": remaining,
	})
}

// handleUsers handles user management (POST to create a user).
// Requires mTLS — only CLI/internal callers with a signed certificate can create users.
func (h *HTTPHandler) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Email == "" {
		jsonError(w, http.StatusBadRequest, "email required")
		return
	}

	user, err := h.userSvc.CreateUser(req.Email, req.Name)
	if err != nil {
		h.logger.Warn("Failed to create user", "error", err, "email", req.Email)
		jsonError(w, http.StatusConflict, err.Error())
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"user":    user,
	})
}

// =============================================================================
// Browser Auth Handlers (Public Router)
// =============================================================================

// handleAuthLoginChallenge generates an auth challenge for a user email.
func (h *HTTPHandler) handleAuthLoginChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	user, err := h.userSvc.FindByEmail(req.Email)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to find user")
		return
	}
	if user == nil {
		jsonError(w, http.StatusNotFound, "user not found")
		return
	}

	options, err := h.passkey.GenerateAuthenticationChallenge(user.ID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"user_id": user.ID,
		"options": options,
	})
}

// handleAuthLoginVerify verifies an auth assertion and sets a web session cookie.
func (h *HTTPHandler) handleAuthLoginVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		UserID            string             `json:"user_id"`
		AssertionResponse *AssertionResponse `json:"assertion_response"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	_, err = h.passkey.VerifyAuthentication(req.UserID, r)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, err.Error())
		return
	}

	webSession, err := h.passkey.CreateWebSession(req.UserID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create web session")
		return
	}

	// Set HttpOnly Secure SameSite=Lax cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "g8e_session",
		Value:    webSession.ID,
		Path:     "/",
		Expires:  time.Unix(webSession.ExpiresAtUnixMs/1000, 0),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"user_id":     req.UserID,
		"web_session": webSession,
	})
}

// handleAuthLogout clears the web session cookie and deletes the web session.
func (h *HTTPHandler) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("g8e_session")
	if err == nil {
		// Best effort delete web session from DB
		_, _ = h.db.DocDelete(marshaler.CollectionName(constants.CollectionWebSessions), cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "g8e_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

// handleBootstrap creates the first user in the system and optionally issues a CLI mTLS cert.
// Rejects with 403 Forbidden if any users already exist (unless rotating an active bootstrap user over loopback).
func (h *HTTPHandler) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		CSRPEM            string `json:"csr_pem"`
		CLICSRPEM         string `json:"cli_csr_pem,omitempty"`
		SystemFingerprint string `json:"system_fingerprint"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Email == "" {
		jsonError(w, http.StatusBadRequest, "email required")
		return
	}

	// Check if CSR signing is requested
	csrRequested := req.CSRPEM != ""

	// If CSR is requested, enforce loopback gate (plan §4.2)
	if csrRequested {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			h.logger.Warn("Bootstrap CSR request rejected: not from loopback", "remote_addr", r.RemoteAddr)
			jsonError(w, http.StatusForbidden, "CSR auto-issue only available over loopback")
			return
		}
	}

	// Check for existing bootstrap user (plan §4.2, §9.1 rotation carve-out)
	bootstrapUser, err := h.userSvc.FindBootstrapUser()
	if err != nil {
		h.logger.Error("Failed to check for existing bootstrap user", "error", err)
		jsonError(w, http.StatusInternalServerError, "bootstrap check failed")
		return
	}

	var user *models.User
	if bootstrapUser != nil {
		// Bootstrap user exists - check if rotation is allowed
		if !bootstrapUser.IsActive() {
			h.logger.Warn("Bootstrap user is disabled, refusing rotation", "user_id", bootstrapUser.ID)
			jsonError(w, http.StatusConflict, "bootstrap user is disabled, cannot rotate")
			return
		}
		if !csrRequested {
			h.logger.Warn("Bootstrap user exists but no CSR requested", "user_id", bootstrapUser.ID)
			jsonError(w, http.StatusForbidden, "bootstrap already exists, CSR required for rotation")
			return
		}
		// Rotation allowed: active bootstrap user + CSR + loopback
		user = bootstrapUser
		h.logger.Info("[BOOTSTRAP] Rotating existing bootstrap user", "user_id", user.ID, "email", user.Email)
	} else {
		// No bootstrap user exists - create one.
		// Defense-in-depth: refuse if any user already exists, so bootstrap can
		// only run on a genuinely empty system.
		hasUsers, err := h.userSvc.HasAnyUsers()
		if err != nil {
			h.logger.Error("Failed to check for existing users during bootstrap", "error", err)
			jsonError(w, http.StatusInternalServerError, "bootstrap check failed")
			return
		}
		if hasUsers {
			h.logger.Warn("Bootstrap attempted on non-empty system", "remote_addr", r.RemoteAddr)
			jsonError(w, http.StatusForbidden, "bootstrap only available for initial setup")
			return
		}

		// Create the bootstrap user
		user, err = h.userSvc.CreateBootstrapUser(req.Email, req.Name)
		if err != nil {
			h.logger.Error("Failed to create bootstrap user", "error", err, "email", req.Email)
			jsonError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	}

	// Issue a web session cookie for passkey registration
	webSession, err := h.passkey.CreateWebSession(user.ID)
	if err != nil {
		h.logger.Error("Failed to create web session for bootstrap user", "error", err, "user_id", user.ID)
		jsonError(w, http.StatusInternalServerError, "user created but web session failed")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "g8e_session",
		Value:    webSession.ID,
		Path:     "/",
		Expires:  time.Unix(webSession.ExpiresAtUnixMs/1000, 0),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	response := map[string]interface{}{
		"success":     true,
		"user":        user,
		"web_session": webSession,
	}

	// If CSR is requested and loopback, sign and return cert (plan §4.2)
	if csrRequested {
		// Create operator slot for the bootstrap user
		operatorID := uuid.NewString()
		sessionID := uuid.NewString()
		cliSessionID := uuid.NewString()
		orgID := user.ID // Use user ID as org ID for bootstrap
		now := time.Now().UTC()

		operator := &models.OperatorDocumentGo{
			ID:                operatorID,
			UserID:            user.ID,
			OrganizationID:    orgID,
			Component:         constants.Status.ComponentName.G8EO,
			Name:              "bootstrap-operator",
			Status:            constants.Status.OperatorStatus.Active,
			OperatorSessionID: sessionID,
			OperatorType:      constants.Status.OperatorType.System,
			SystemFingerprint: req.SystemFingerprint,
			Claimed:           true,
			ClaimedAt:         &now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		// Sign the CSR
		certPEM, chainPEM, err := h.pki.SignCSR(req.CSRPEM, constants.LeafTypeOperator, orgID, operatorID, user.ID, sessionID)
		if err != nil {
			h.logger.Error("Failed to sign bootstrap CSR", "error", err, "user_id", user.ID)
			jsonError(w, http.StatusInternalServerError, "failed to sign CSR")
			return
		}

		operator.OperatorCert = certPEM

		// CLI certificate generation (optional)
		var cliCertPEM, cliCertChainPEM string
		if req.CLICSRPEM != "" {
			cliCertPEM, cliCertChainPEM, err = h.pki.SignCSR(req.CLICSRPEM, constants.LeafTypeCLI, "", "", user.ID, cliSessionID)
			if err != nil {
				h.logger.Error("Failed to sign bootstrap CLI CSR", "error", err, "user_id", user.ID)
				jsonError(w, http.StatusInternalServerError, "failed to sign CLI CSR")
				return
			}
		} else {
			// [SPIFFE-DRIFT] Fallback: If no CLI CSR provided, the CLI cert returned MUST be
			// the operator cert for backwards compatibility with older binaries, even though
			// they will fail modern /cli/ path checks.
			cliCertPEM = certPEM
			cliCertChainPEM = chainPEM
		}

		// Persist operator document
		opBytes, err := json.Marshal(operator)
		if err != nil {
			h.logger.Error("Failed to marshal operator document", "error", err)
			jsonError(w, http.StatusInternalServerError, "failed to create operator")
			return
		}
		if err := h.db.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes); err != nil {
			h.logger.Error("Failed to persist operator document", "error", err)
			jsonError(w, http.StatusInternalServerError, "failed to create operator")
			return
		}

		// Fetch trust bundle
		hubBundle, err := h.pki.HubTrustBundle()
		if err != nil {
			h.logger.Warn("Failed to fetch hub trust bundle", "error", err)
			// Non-fatal - continue without bundle
		}

		// CLI session id is a first-class session type, strictly disjoint from
		// operator_session_id. The operator_session_id authenticates the host
		// agent (mTLS URI SAN); the cli_session_id is the routing namespace
		// the BYO/CLI client uses to receive SessionEvents (SSE) and embed in
		// outbound request bodies. Conflating the two would let an operator
		// session drain another client's event stream — and vice versa.

		// Store the binding between operator_session_id and cli_session_id in a first-class
		// collection to support metadata, expiry, and revocation. Without this binding,
		// any authenticated operator could drain any cli_session_id's event buffer.
		cliSession := models.CLISession{
			ID:                cliSessionID,
			UserID:            user.ID,
			OperatorSessionID: sessionID,
			SystemFingerprint: req.SystemFingerprint,
			CreatedAt:         time.Now().UTC(),
			ExpiresAt:         time.Now().UTC().Add(24 * time.Hour), // Match operator session expiry
		}
		cliSessionBytes, _ := json.Marshal(cliSession)
		if err := h.db.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes); err != nil {
			h.logger.Error("Failed to persist CLI session during bootstrap", "error", err)
			jsonError(w, http.StatusInternalServerError, "failed to bind CLI session")
			return
		}

		response["operator_cert"] = certPEM
		response["operator_cert_chain"] = chainPEM
		response["hub_trust_bundle"] = string(hubBundle)
		response["operator_session_id"] = sessionID
		response["operator_id"] = operatorID
		response["cli_session_id"] = cliSessionID
		response["cli_cert"] = cliCertPEM
		response["cli_cert_chain"] = cliCertChainPEM

		h.logger.Info("[BOOTSTRAP] System initialized with bootstrap user and CLI cert", "user_id", user.ID, "email", user.Email, "operator_id", operatorID, "cli_session_id_prefix", cliSessionID[:8])
	} else {
		h.logger.Info("[BOOTSTRAP] System initialized with bootstrap user (no CSR)", "user_id", user.ID, "email", user.Email)
	}

	jsonResponse(w, http.StatusCreated, response)
}

// handleBootstrapStatus returns whether the system has been bootstrapped (at least one user exists).
func (h *HTTPHandler) handleBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	hasUsers, err := h.userSvc.HasAnyUsers()
	if err != nil {
		h.logger.Error("Failed to check for existing users", "error", err)
		jsonError(w, http.StatusInternalServerError, "status check failed")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"bootstrapped": hasUsers,
	})
}

// handleUserMe returns the current authenticated user.
func (h *HTTPHandler) handleUserMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("user_id").(string)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.userSvc.GetByID(userID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to get user")
		return
	}
	if user == nil {
		jsonError(w, http.StatusNotFound, "user not found")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"user":    user,
	})
}

// handleWebSession returns current session info.
func (h *HTTPHandler) handleWebSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("user_id").(string)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cookie, _ := r.Cookie("g8e_session")
	webSessionID := ""
	if cookie != nil {
		webSessionID = cookie.Value
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":        true,
		"user_id":        userID,
		"web_session_id": webSessionID,
	})
}
