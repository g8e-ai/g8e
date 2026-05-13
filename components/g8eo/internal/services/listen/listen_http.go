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

	"github.com/g8e-ai/g8e/components/g8eo/internal/config"
	"github.com/g8e-ai/g8e/components/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/components/g8eo/internal/models"
)

func readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 50*1024*1024))
}

// HTTPHandler manages the web API for the listen service.
type HTTPHandler struct {
	cfg     *config.Config
	logger  *slog.Logger
	db      *ListenDBService
	pubsub  *PubSubBroker
	auth    *AuthService
	pki     *PKIAuthority
	reg     *RegistrationService
	passkey *PasskeyService
	userSvc *UserService
	apiKey  *ApiKeyService
	isReady func() bool
}

func newHTTPHandler(cfg *config.Config, logger *slog.Logger, db *ListenDBService, pubsub *PubSubBroker, auth *AuthService, pki *PKIAuthority, reg *RegistrationService, passkey *PasskeyService, userSvc *UserService, apiKey *ApiKeyService, isReady func() bool) *HTTPHandler {
	return &HTTPHandler{
		cfg:     cfg,
		logger:  logger,
		db:      db,
		pubsub:  pubsub,
		auth:    auth,
		pki:     pki,
		reg:     reg,
		passkey: passkey,
		userSvc: userSvc,
		apiKey:  apiKey,
		isReady: isReady,
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
	mux.HandleFunc("/api/auth/login/challenge", h.handleAuthLoginChallenge)
	mux.HandleFunc("/api/auth/login/verify", h.handleAuthLoginVerify)
	mux.HandleFunc("/api/auth/logout", h.handleAuthLogout)

	// PKI discovery also available on public port for BYO bootstrap
	mux.HandleFunc("/.well-known/g8e/pki/root.pem", h.handlePKIRoot)
	mux.HandleFunc("/.well-known/g8e/pki/hub-bundle.pem", h.handlePKIHubBundle)
	mux.HandleFunc("/.well-known/g8e/pki/fingerprint", h.handlePKIFingerprint)

	// Browser-facing data routes (require session cookie)
	authedMux := http.NewServeMux()
	authedMux.HandleFunc("/api/user/me", h.handleUserMe)
	authedMux.HandleFunc("/api/auth/web-session", h.handleWebSession)

	// Wrap authed routes in WebSessionAuth middleware
	mux.Handle("/api/user/", h.auth.WebSessionAuth(authedMux, h.db))
	mux.Handle("/api/auth/web-session", h.auth.WebSessionAuth(authedMux, h.db))

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

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set(constants.HeaderContentType, "application/json")
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

	doc, err := h.db.DocGet(string(constants.CollectionSettings), string(constants.DocIDPlatformSettings))
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
		CSR            string `json:"csr_pem"`
		LeafType       string `json:"leaf_type"`
		OrganizationID string `json:"organization_id"`
		OperatorID     string `json:"operator_id"`
		SessionID      string `json:"session_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	certPEM, chainPEM, err := h.pki.SignCSR(req.CSR, req.LeafType, req.OrganizationID, req.OperatorID, req.SessionID)
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
	token := r.Header.Get("X-G8E-Device-Token")
	if token == "" {
		jsonError(w, http.StatusBadRequest, "missing X-G8E-Device-Token")
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
		doc, err := h.db.DocGet(string(constants.CollectionSettings), string(constants.DocIDPlatformSettings))
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
			err2 = h.db.DocSet(string(constants.CollectionSettings), string(constants.DocIDPlatformSettings), json.RawMessage(body))
		} else {
			_, err2 = h.db.DocUpdate(string(constants.CollectionSettings), string(constants.DocIDPlatformSettings), json.RawMessage(body))
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
	script := WindowsTrustScript(host, h.cfg.Listen.BootstrapPort)
	w.Header().Set("Content-Type", "text/plain") // Batch scripts often served as text
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
	newKey, err := h.reg.RotateOperatorAPIKey(req.OperatorID, userID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, models.RotateAPIKeyResponse{Success: true, APIKey: newKey})
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
	sessionID := r.Header.Get(constants.HeaderOperatorSessionID)
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
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
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

	// If Data is a JSON string, it might be base64-encoded binary (e.g. UniversalEnvelope).
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

	session, err := h.passkey.CreateSession(userID)
	if err != nil {
		h.logger.Error("Failed to create session after auth", "error", err, "userID", userID)
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
		"session": map[string]interface{}{
			"id":                 session.ID,
			"expires_at_unix_ms": session.ExpiresAtUnixMs,
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
		Email string   `json:"email"`
		Name  string   `json:"name"`
		Roles []string `json:"roles,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Email == "" {
		jsonError(w, http.StatusBadRequest, "email required")
		return
	}

	user, err := h.userSvc.CreateUser(req.Email, req.Name, req.Roles)
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

// handleAuthLoginVerify verifies an auth assertion and sets a session cookie.
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

	session, err := h.passkey.CreateSession(req.UserID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	// Set HttpOnly Secure SameSite=Lax cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "g8e_session",
		Value:    session.ID,
		Path:     "/",
		Expires:  time.Unix(session.ExpiresAtUnixMs/1000, 0),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"user_id": req.UserID,
		"session": session,
	})
}

// handleAuthLogout clears the session cookie and deletes the session.
func (h *HTTPHandler) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("g8e_session")
	if err == nil {
		// Best effort delete session from DB
		_, _ = h.db.DocDelete(string(constants.CollectionWebSessions), cookie.Value)
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
	sessionID := ""
	if cookie != nil {
		sessionID = cookie.Value
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"user_id":    userID,
		"session_id": sessionID,
	})
}
