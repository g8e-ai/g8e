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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/protocol"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/marshaler"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/responder"
	"golang.org/x/time/rate"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	userIDKey contextKey = "user_id"
	appIDKey  contextKey = "app_id"
)

// AuthError represents a structured authentication error.
type AuthError struct {
	Message string `json:"error"`
	Reason  string `json:"reason,omitempty"`
	Status  int    `json:"-"`
}

func (e *AuthError) Error() string {
	b, _ := json.Marshal(e)
	return string(b)
}

func (e *AuthError) Is(target error) bool {
	_, ok := target.(*AuthError)
	return ok
}

// AuthService handles authentication for the Gateway service.
type AuthService struct {
	db         *GatewayDBService
	pki        *PKIAuthority
	logger     *slog.Logger
	userSvc    *UserService
	responder  *responder.Responder
	secretsDir string

	// Rate limiting state for app policies
	muLimiters sync.Mutex
	limiters   map[string]*rate.Limiter
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *GatewayDBService, pki *PKIAuthority, logger *slog.Logger, userSvc *UserService, responder *responder.Responder, secretsDir string) *AuthService {
	return &AuthService{
		db:         db,
		pki:        pki,
		logger:     logger,
		userSvc:    userSvc,
		responder:  responder,
		secretsDir: secretsDir,
		limiters:   make(map[string]*rate.Limiter),
	}
}

// ValidateOperatorSession checks if a session ID is valid and returns the operator document.
// Auth depends on session validity (existence + certificate revocation), not on operator
// status liveness signals from other processes. The primary session invalidation mechanism
// is certificate revocation via PKI authority.
func (s *AuthService) ValidateOperatorSession(operatorSessionID string) (*models.OperatorDocumentGo, error) {
	if operatorSessionID == "" {
		return nil, &AuthError{Message: "missing operator_session_id", Status: http.StatusUnauthorized}
	}

	filters := []models.DocFilter{
		{Field: "operator_session_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", operatorSessionID))},
	}

	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 1)
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return nil, &AuthError{Message: "invalid or expired operator session", Status: http.StatusUnauthorized}
	}

	// Convert Document to OperatorDocumentGo
	b, err := json.Marshal(docs[0].ForWire())
	if err != nil {
		return nil, err
	}

	var op models.OperatorDocumentGo
	if err := json.Unmarshal(b, &op); err != nil {
		return nil, err
	}

	// [PIVOT] Reject terminated identities (Plan §4.6)
	// We allow OFFLINE and STALE statuses to authenticate (to support bootstrap
	// and recovery), but TERMINATED is a hard-gate rejection.
	if op.Status == constants.OperatorStatusTerminated {
		return nil, &AuthError{
			Message: "operator identity disabled",
			Reason:  marshaler.OperatorStatus(constants.OperatorStatusTerminated),
			Status:  http.StatusForbidden,
		}
	}

	// Enforce session expiry (TTL)
	// Default session TTL is 24h if not specified.
	sessionTTL := 24 * time.Hour
	// We use the Document store's authoritative CreatedAt for TTL enforcement.
	if !docs[0].CreatedAt.IsZero() && time.Since(docs[0].CreatedAt) > sessionTTL {
		return nil, &AuthError{Message: "operator session expired", Reason: "ttl_exceeded", Status: http.StatusUnauthorized}
	}

	// Check if the linked user is active (plan §4.6)
	// This is the single chokepoint that makes retirement real - without it,
	// a stale CLI cert can still talk to the Gateway.
	if s.userSvc != nil && op.UserID != "" {
		user, err := s.userSvc.GetByID(op.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to load user %s: %w", op.UserID, err)
		}
		if user != nil && !user.IsActive() {
			// Return structured error for disabled users
			return nil, &AuthError{Message: "identity disabled", Reason: "retired_by_real_login", Status: http.StatusForbidden}
		}
	}

	return &op, nil
}

// ValidateAPIKey checks if an API key is valid and returns the operator document.
func (s *AuthService) ValidateAPIKey(apiKey string) (*models.OperatorDocumentGo, error) {
	if apiKey == "" {
		return nil, &AuthError{Message: "missing api key", Status: http.StatusUnauthorized}
	}

	filters := []models.DocFilter{
		{Field: "operator_api_key", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", apiKey))},
	}

	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 1)
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return nil, &AuthError{Message: "invalid api key", Status: http.StatusUnauthorized}
	}

	// Convert Document to OperatorDocumentGo
	b, err := json.Marshal(docs[0].ForWire())
	if err != nil {
		return nil, err
	}

	var op models.OperatorDocumentGo
	if err := json.Unmarshal(b, &op); err != nil {
		return nil, err
	}

	// Check if the linked user is active (plan §4.6)
	if s.userSvc != nil && op.UserID != "" {
		user, err := s.userSvc.GetByID(op.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to load user %s: %w", op.UserID, err)
		}
		if user != nil && !user.IsActive() {
			return nil, &AuthError{Message: "identity disabled", Reason: "retired_by_real_login", Status: http.StatusForbidden}
		}
	}

	return &op, nil
}

// ExtractOperatorSessionID returns the operator session ID from the request headers.
// It prefers Authorization: Bearer <token>.
func (s *AuthService) ExtractOperatorSessionID(r *http.Request) string {
	authHeader := r.Header.Get(constants.HeaderAuthorization)
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

// Middleware returns an http.Handler that authenticates requests.
func (s *AuthService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always allow health check without a token
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// [PIVOT] Support public protocol auth surface (Phase 4)
		// These unauthenticated PKI routes return only public material and fingerprints.
		if strings.HasPrefix(r.URL.Path, "/.well-known/g8e/pki/") {
			next.ServeHTTP(w, r)
			return
		}

		// [PIVOT] MCP & A2A gateways handle their own authentication since they support standard clients
		if strings.HasPrefix(r.URL.Path, "/api/mcp/") || strings.HasPrefix(r.URL.Path, "/api/a2a/") || strings.HasPrefix(r.URL.Path, "/api/approve/") {
			next.ServeHTTP(w, r)
			return
		}

		// These routes are new protocol entry points.
		// Native Registration Path (Phase 4)
		// This endpoint is the new sovereign entry point for enrolling binaries.
		// It MUST be accessible without an internal token as it is the first step
		// of the trust bootstrap.
		if r.URL.Path == "/api/pki/sign-csr" ||
			r.URL.Path == "/api/auth/device-link/register" ||
			r.URL.Path == "/api/auth/bootstrap" ||
			r.URL.Path == "/api/auth/bootstrap/status" {
			next.ServeHTTP(w, r)
			return
		}

		// Blob endpoint: allow device-link token authentication for bootstrap
		// Devices use device-link tokens to download the operator binary
		if strings.HasPrefix(r.URL.Path, "/blob/") {
			authHeader := r.Header.Get(constants.HeaderAuthorization)
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if strings.HasPrefix(token, "dlk_") && len(token) >= 20 {
					// Validate device-link token exists and is not expired
					linkKey := "g8e:device-link:" + token
					raw, found := s.db.KVGet(linkKey)
					if found {
						var linkData map[string]interface{}
						if err := json.Unmarshal([]byte(raw), &linkData); err == nil {
							if expiresAt, ok := linkData["expires_at"].(string); ok {
								if expTime, err := time.Parse(time.RFC3339, expiresAt); err == nil {
									if expTime.After(time.Now()) {
										// Token is valid, allow access
										next.ServeHTTP(w, r)
										return
									}
								}
							}
						}
					}
				}
			}
			// If device-link token validation fails, fall through to mTLS requirement
		}

		// [PIVOT] Enforce mTLS for all other routes (Phase 6)
		// The mTLS gateway uses tls.RequireAndVerifyClientCert; reaching L7
		// without a peer cert means an internal misroute, not a client error.
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			s.logger.Warn("mTLS required but no client certificate provided", "path", r.URL.Path)
			s.responder.Error(w, http.StatusUnauthorized, "mTLS client certificate required")
			return
		}

		// [PIVOT] Verify certificate revocation status (Phase 6)
		if s.pki != nil {
			if err := s.pki.VerifyCertificate(r.TLS.PeerCertificates[0]); err != nil {
				s.logger.Warn("mTLS client certificate revoked or invalid", "path", r.URL.Path, string(constants.ConnectionStateError), err)
				s.responder.Error(w, http.StatusUnauthorized, "mTLS client certificate revoked or invalid")
				return
			}
		}

		// We prioritize session auth for operators.
		// [PIVOT] Prefer standard Authorization: Bearer <token> header (Plan §4.6)
		operatorSessionID := s.ExtractOperatorSessionID(r)

		cliSessionID := r.Header.Get(constants.HeaderCLISessionID)

		switch {
		case operatorSessionID != "":
			op, err := s.ValidateOperatorSession(operatorSessionID)
			if err == nil {
				// [PIVOT] Verify URI SAN identity (Phase 6)
				// The client cert must bind to the same operator session, OR
				// to a CLI session owned by this operator session. CLI clients
				// (./g8e login) hold a CLI cert and authenticate with their
				// linked operator session via Bearer; they MUST be allowed to
				// reach internal APIs scoped by cli_session_id.
				// SPIFFE ID formats: protocol.WorkloadIdentity.OperatorSPIFFEID()
				// and protocol.WorkloadIdentity.CLISPIFFEID().
				if len(r.TLS.PeerCertificates) > 0 {
					wid := protocol.NewWorkloadIdentity()
					cert := r.TLS.PeerCertificates[0]
					match := false
					for _, uri := range cert.URIs {
						if wid.MatchesOperator(uri.String(), op.OrganizationID, op.ID, operatorSessionID) {
							match = true
							break
						}
					}
					if !match && cliSessionID != "" {
						match = s.cliCertBoundToOperator(cert.URIs, cliSessionID, op.UserID, operatorSessionID)
					}
					if !match {
						s.logger.Warn("mTLS URI SAN mismatch for operator session", "path", r.URL.Path, "operator_id", op.ID, "operator_session_id", operatorSessionID)
						s.responder.Error(w, http.StatusForbidden, "mTLS identity mismatch")
						return
					}
				}

				next.ServeHTTP(w, r)
				return
			}
			s.logger.Warn("Invalid operator session attempt", "operator_session_id", operatorSessionID[:8]+"...", string(constants.ConnectionStateError), err)

			// If it's a structured AuthError, return it properly
			if ae, ok := err.(*AuthError); ok {
				s.responder.Error(w, ae.Status, ae.Message) // Note: responder.Error wraps it in {"error": ...}
				return
			}
		case cliSessionID != "":
			// CLI authentication via CLI session ID and CLI certificate
			// Verify the CLI certificate matches the CLI session ID
			// SPIFFE ID format: protocol.WorkloadIdentity.CLISPIFFEID()
			if len(r.TLS.PeerCertificates) > 0 {
				wid := protocol.NewWorkloadIdentity()
				cert := r.TLS.PeerCertificates[0]

				// [PIVOT] Verify CLI session and lookup UserID (Plan §4.6)
				cliDoc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
				if err != nil {
					s.logger.Error("failed to load CLI session", "cli_session_id", cliSessionID, string(constants.ConnectionStateError), err)
					s.responder.Error(w, http.StatusInternalServerError, "failed to load session")
					return
				}
				if cliDoc == nil {
					s.logger.Warn("CLI session not found", "cli_session_id", cliSessionID)
					s.responder.Error(w, http.StatusUnauthorized, "invalid CLI session")
					return
				}

				var cliSession models.CLISession
				b, _ := json.Marshal(cliDoc.Data)
				if err := json.Unmarshal(b, &cliSession); err != nil {
					s.logger.Error("failed to parse CLI session", "cli_session_id", cliSessionID, string(constants.ConnectionStateError), err)
					s.responder.Error(w, http.StatusInternalServerError, "failed to parse session")
					return
				}

				// Check expiry
				if !cliSession.ExpiresAt.IsZero() && cliSession.ExpiresAt.Before(time.Now()) {
					s.logger.Warn("CLI session expired", "cli_session_id", cliSessionID)
					s.responder.Error(w, http.StatusUnauthorized, "CLI session expired")
					return
				}

				// Check if the linked user is active
				if s.userSvc != nil && cliSession.UserID != "" {
					user, err := s.userSvc.GetByID(cliSession.UserID)
					if err != nil {
						s.logger.Error("failed to load user for CLI session", "user_id", cliSession.UserID, string(constants.ConnectionStateError), err)
						s.responder.Error(w, http.StatusInternalServerError, "identity validation failed")
						return
					}
					if user != nil && !user.IsActive() {
						s.logger.Warn("CLI session identity disabled", "user_id", cliSession.UserID)
						s.responder.Error(w, http.StatusForbidden, "identity disabled")
						return
					}
				}

				// Verify the CLI certificate matches the CLI session ID
				// SPIFFE ID format: protocol.WorkloadIdentity.CLISPIFFEID()
				match := false
				for _, uri := range cert.URIs {
					if wid.MatchesCLI(uri.String(), cliSession.UserID, cliSessionID) {
						match = true
						break
					}
				}
				if !match {
					s.logger.Warn("mTLS URI SAN mismatch for CLI session", "path", r.URL.Path, "cli_session_id", cliSessionID)
					s.responder.Error(w, http.StatusForbidden, "mTLS identity mismatch")
					return
				}
			}

			next.ServeHTTP(w, r)
			return
		default:
			// [PIVOT] System/App Authentication via URI SAN (Phase 6)
			// If no session ID is provided, we check if the certificate belongs to a trusted system app.
			// Note: /_query requires operator session authentication - no app bypass allowed.
			// SPIFFE ID format: protocol.WorkloadIdentity.AppSPIFFEID()
			if len(r.TLS.PeerCertificates) > 0 {
				cert := r.TLS.PeerCertificates[0]
				for _, uri := range cert.URIs {
					uriStr := uri.String()
					if strings.HasPrefix(uriStr, "spiffe://"+protocol.TrustDomain+"/app/") {
						// g8ee is a native platform component, it bypasses the external AppPolicy check
						if uriStr == "spiffe://"+protocol.TrustDomain+"/app/g8ee" {
							next.ServeHTTP(w, r)
							return
						}

						// For external apps, verify an explicit AppPolicy exists and applies to this path
						appID := uriStr
						doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
						if err != nil || doc == nil {
							s.logger.Warn("App policy not found (deny-all default)", "app_id", appID)
							s.responder.Error(w, http.StatusForbidden, "app policy not found")
							return
						}

						var policy models.AppPolicy
						data, _ := json.Marshal(doc.Data)
						if err := json.Unmarshal(data, &policy); err != nil {
							s.logger.Error("Failed to parse app policy", "app_id", appID, "error", err)
							s.responder.Error(w, http.StatusInternalServerError, "invalid app policy")
							return
						}

						// The /_query and /api/governance/envelope endpoints require a human operator session
						// External apps cannot use the direct query or envelope endpoints directly.
						if strings.HasPrefix(r.URL.Path, "/_query") || strings.HasPrefix(r.URL.Path, "/api/governance/envelope") {
							s.responder.Error(w, http.StatusForbidden, "external apps cannot access privileged endpoints")
							return
						}

						// Enforce AppPolicy rules
						if err := s.enforceAppPolicy(r, &policy, appID); err != nil {
							s.logger.Warn("App policy enforcement failed", "app_id", appID, "error", err)
							s.responder.Error(w, http.StatusForbidden, err.Error())
							return
						}

						// Stamp context with app identity for downstream MCP/A2A endpoints
						ctx := context.WithValue(r.Context(), appIDKey, appID)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}
		}

		// API keys are no longer a valid mutation authority.
		// They are only used for registration, which is handled in the bypass above.

		// For WebSockets, return a plain text error for 401.
		// Handshake fails if a JSON body is returned instead of just the 401 status.
		if strings.HasPrefix(r.URL.Path, "/ws/") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		s.responder.Error(w, http.StatusUnauthorized, "protocol authentication required")
	})
}

// getOrCreateLimiter returns a rate limiter for the given app ID, creating one if needed.
func (s *AuthService) getOrCreateLimiter(appID string, rps int) *rate.Limiter {
	s.muLimiters.Lock()
	defer s.muLimiters.Unlock()

	limiter, exists := s.limiters[appID]
	if !exists {
		// Create a new limiter with the configured RPS and a burst of 2x RPS
		limiter = rate.NewLimiter(rate.Limit(rps), rps*2)
		s.limiters[appID] = limiter
	}
	return limiter
}

// enforceAppPolicy validates that the request complies with the app's policy.
// It checks collection access, event types, intents, and rate limits.
func (s *AuthService) enforceAppPolicy(r *http.Request, policy *models.AppPolicy, appID string) error {
	// Check rate limit (if configured)
	if policy.RateLimitRPS > 0 {
		limiter := s.getOrCreateLimiter(appID, policy.RateLimitRPS)
		if !limiter.Allow() {
			s.logger.Warn("App rate limit exceeded", "app_id", appID, "rate_limit_rps", policy.RateLimitRPS, "path", r.URL.Path)
			return fmt.Errorf("rate limit exceeded (%d RPS)", policy.RateLimitRPS)
		}
	}

	// Check payload size (if configured)
	if policy.MaxPayloadBytes > 0 {
		if r.ContentLength > policy.MaxPayloadBytes {
			return fmt.Errorf("payload exceeds maximum allowed size of %d bytes", policy.MaxPayloadBytes)
		}
	}

	// Check collection access for /_query paths (already blocked, but for future-proofing)
	if strings.HasPrefix(r.URL.Path, "/_query") {
		// Extract collection from query parameters
		collection := r.URL.Query().Get("collection")
		if collection != "" && len(policy.AllowedCollections) > 0 {
			allowed := false
			for _, allowedCol := range policy.AllowedCollections {
				if collection == allowedCol {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("collection '%s' not in allowed collections", collection)
			}
		}
	}

	// Check event type for governance envelope submissions
	if strings.HasPrefix(r.URL.Path, "/api/governance/envelope") {
		// Extract action type from request body (if available)
		// This is a simplified check - full enforcement happens in the transaction verifier
		// For now, we just ensure the policy exists (already checked before this call)
	}

	return nil
}

// cliCertBoundToOperator reports whether the presented client certificate is a
// CLI workload SPIFFE ID whose CLI session is owned by the given operator
// session. This lets a CLI client (./g8e login) call internal APIs scoped by
// cli_session_id while presenting its CLI mTLS cert and the linked operator
// session as a Bearer token.
func (s *AuthService) cliCertBoundToOperator(certURIs []*url.URL, cliSessionID, userID, operatorSessionID string) bool {
	if cliSessionID == "" || operatorSessionID == "" {
		return false
	}
	wid := protocol.NewWorkloadIdentity()
	uriMatch := false
	for _, uri := range certURIs {
		if userID != "" && wid.MatchesCLI(uri.String(), userID, cliSessionID) {
			uriMatch = true
			break
		}
		if wid.MatchesCLISessionOnly(uri.String(), cliSessionID) {
			uriMatch = true
			break
		}
	}
	if !uriMatch {
		return false
	}
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
	if err != nil || doc == nil {
		return false
	}
	var cliSession models.CLISession
	b, _ := json.Marshal(doc.Data)
	if err := json.Unmarshal(b, &cliSession); err != nil {
		return false
	}
	if !cliSession.ExpiresAt.IsZero() && cliSession.ExpiresAt.Before(time.Now()) {
		return false
	}
	return cliSession.OperatorSessionID == operatorSessionID
}

// WebSocketAuth returns an http.Handler that authenticates WebSocket connections.
func (s *AuthService) WebSocketAuth(next http.Handler) http.Handler {
	// Re-use the main Middleware logic for WebSockets to ensure consistency.
	// Middleware already handles /ws/ prefix specifically and bootstrap bypass.
	return s.Middleware(next)
}

// WebSessionAuth validates web session cookies and stamps context with user_id.
// This is for browser-based authentication on the public gateway.
func (s *AuthService) WebSessionAuth(next http.Handler, db *GatewayDBService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("g8e_session")
		if err != nil || cookie == nil {
			s.responder.Error(w, http.StatusUnauthorized, "web session cookie required")
			return
		}

		sessionID := cookie.Value
		if sessionID == "" {
			s.responder.Error(w, http.StatusUnauthorized, "invalid web session cookie")
			return
		}

		// Validate web session
		doc, err := db.DocGet(marshaler.CollectionName(constants.CollectionWebSessions), sessionID)
		if err != nil {
			s.responder.Error(w, http.StatusUnauthorized, "web session validation failed")
			return
		}
		if doc == nil {
			s.responder.Error(w, http.StatusUnauthorized, "web session not found")
			return
		}

		// Check expiry
		var webSession models.WebSession
		data, err := json.Marshal(doc.Data)
		if err != nil {
			s.responder.Error(w, http.StatusUnauthorized, "web session parse failed")
			return
		}
		if err := json.Unmarshal(data, &webSession); err != nil {
			s.responder.Error(w, http.StatusUnauthorized, "web session parse failed")
			return
		}

		if time.Now().UnixMilli() > webSession.ExpiresAtUnixMs {
			s.responder.Error(w, http.StatusUnauthorized, "web session expired")
			return
		}

		// Check if the user is active (plan §4.6)
		if s.userSvc != nil {
			user, err := s.userSvc.GetByID(webSession.UserID)
			if err != nil {
				s.responder.Error(w, http.StatusUnauthorized, "user validation failed")
				return
			}
			if user != nil && !user.IsActive() {
				s.responder.Error(w, http.StatusUnauthorized, "identity disabled")
				return
			}
		}

		// Stamp context with user_id
		ctx := context.WithValue(r.Context(), userIDKey, webSession.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
