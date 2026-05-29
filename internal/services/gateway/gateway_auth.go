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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/protocol"
	"golang.org/x/time/rate"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	userIDKey         contextKey = "user_id"
	appIDKey          contextKey = "app_id"
	tenantIDKey       contextKey = "tenant_id"
	bindingPersonaKey contextKey = "binding_persona"
)

// PublicRouteRegistry defines routes that bypass authentication.
// Exact paths are matched precisely. Prefixes allow any path under the prefix.
// This centralized registry eliminates fragile HasPrefix duplication.
type PublicRouteRegistry struct {
	exactPaths  map[string]struct{}
	prefixes    map[string]struct{}
	jwksEnabled bool
}

// NewPublicRouteRegistry creates a registry with the canonical public routes.
func NewPublicRouteRegistry(jwksEnabled bool) *PublicRouteRegistry {
	r := &PublicRouteRegistry{
		exactPaths:  make(map[string]struct{}),
		prefixes:    make(map[string]struct{}),
		jwksEnabled: jwksEnabled,
	}

	// Health check (always public)
	r.addExact("/health")

	// PKI bootstrap routes (public material only)
	r.addPrefix("/.well-known/g8e/pki/")

	// Protocol entry points (CSR enrollment, bootstrap)
	r.addExact("/api/v1/pki/csr/sign")
	r.addExact("/api/v1/pki/devices/enroll")
	r.addExact("/api/v1/auth/bootstrap")
	r.addExact("/api/v1/auth/bootstrap/status")
	r.addExact("/api/v1/auth/login/verify")
	r.addExact("/api/v1/auth/logout")
	r.addPrefix("/api/v1/approve/")

	// JIT passkey bootstrap (only when JWKS is configured)
	if jwksEnabled {
		r.addPrefix("/api/v1/auth/passkeys/jit-")
		r.addPrefix("/api/v1/mcp/")
		r.addPrefix("/api/v1/a2a/")
	}

	return r
}

func (r *PublicRouteRegistry) addExact(path string) {
	r.exactPaths[path] = struct{}{}
}

func (r *PublicRouteRegistry) addPrefix(prefix string) {
	r.prefixes[prefix] = struct{}{}
}

// IsPublic returns true if the path is registered as a public route.
func (r *PublicRouteRegistry) IsPublic(path string) bool {
	// Check exact matches first
	if _, ok := r.exactPaths[path]; ok {
		return true
	}

	// Check prefix matches
	for prefix := range r.prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

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
	personaSvc *PersonaService
	responder  *responder.Responder
	secretsDir string

	jwks        *JWKSProvider
	jwtRole     string
	jwtIssuer   string
	jwtAudience string

	publicRoutes *PublicRouteRegistry

	// Rate limiting state for app policies
	muLimiters sync.Mutex
	limiters   map[string]*rate.Limiter
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *GatewayDBService, pki *PKIAuthority, logger *slog.Logger, userSvc *UserService, personaSvc *PersonaService, responder *responder.Responder, secretsDir string, jwks *JWKSProvider, jwtRole, jwtIssuer, jwtAudience string) *AuthService {
	jwksEnabled := jwks != nil
	return &AuthService{
		db:           db,
		pki:          pki,
		logger:       logger,
		userSvc:      userSvc,
		personaSvc:   personaSvc,
		responder:    responder,
		secretsDir:   secretsDir,
		jwks:         jwks,
		jwtRole:      jwtRole,
		jwtIssuer:    jwtIssuer,
		jwtAudience:  jwtAudience,
		publicRoutes: NewPublicRouteRegistry(jwksEnabled),
		limiters:     make(map[string]*rate.Limiter),
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
// It chains multiple single-responsibility middlewares:
// 1. publicBypassMiddleware (unauthenticated routes)
// 2. mtlsMiddleware (mTLS enforcement and revocation)
// 3. authMiddleware (Operator, CLI, or App authentication)
func (s *AuthService) Middleware(next http.Handler) http.Handler {
	return s.publicBypassMiddleware(
		s.mtlsMiddleware(
			s.authMiddleware(next),
		),
	)
}

// publicBypassMiddleware handles routes that are accessible without any authentication.
func (s *AuthService) publicBypassMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.publicRoutes.IsPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// mtlsMiddleware enforces mTLS and verifies certificate revocation.
func (s *AuthService) mtlsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.publicRoutes.IsPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// [PIVOT] Enforce mTLS for all other routes (Phase 6)
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

		next.ServeHTTP(w, r)
	})
}

// authMiddleware handles authentication for Operator, CLI, and App identities.
func (s *AuthService) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth check for bypass routes. These routes either need no auth
		// or have their own auth middleware (like JWTAuthMiddleware for MCP/A2A).
		if s.publicRoutes.IsPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// We prioritize session auth for operators.
		operatorSessionID := s.ExtractOperatorSessionID(r)
		cliSessionID := r.Header.Get(constants.HeaderCLISessionID)

		switch {
		case operatorSessionID != "":
			if s.handleOperatorAuth(w, r, operatorSessionID, cliSessionID, next) {
				return
			}
		case cliSessionID != "":
			if s.handleCLIAuth(w, r, cliSessionID, next) {
				return
			}
		default:
			if s.handleAppAuth(w, r, next) {
				return
			}
		}

		// For WebSockets, return a plain text error for 401.
		if strings.HasPrefix(r.URL.Path, "/ws/") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		s.responder.Error(w, http.StatusUnauthorized, "protocol authentication required")
	})
}

// handleOperatorAuth handles authentication for operator sessions.
// Returns true if the request was handled (either succeeded or failed with error).
func (s *AuthService) handleOperatorAuth(w http.ResponseWriter, r *http.Request, operatorSessionID, cliSessionID string, next http.Handler) bool {
	op, err := s.ValidateOperatorSession(operatorSessionID)
	if err == nil {
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
				return true
			}
		}

		next.ServeHTTP(w, r)
		return true
	}

	s.logger.Warn("Invalid operator session attempt", "operator_session_id", operatorSessionID[:8]+"...", string(constants.ConnectionStateError), err)

	if ae, ok := err.(*AuthError); ok {
		s.responder.Error(w, ae.Status, ae.Message)
		return true
	}
	return false
}

// handleCLIAuth handles authentication for CLI sessions.
func (s *AuthService) handleCLIAuth(w http.ResponseWriter, r *http.Request, cliSessionID string, next http.Handler) bool {
	if len(r.TLS.PeerCertificates) > 0 {
		wid := protocol.NewWorkloadIdentity()
		cert := r.TLS.PeerCertificates[0]

		cliDoc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		if err != nil {
			s.logger.Error("failed to load CLI session", "cli_session_id", cliSessionID, string(constants.ConnectionStateError), err)
			s.responder.Error(w, http.StatusInternalServerError, "failed to load session")
			return true
		}
		if cliDoc == nil {
			s.logger.Warn("CLI session not found", "cli_session_id", cliSessionID)
			s.responder.Error(w, http.StatusUnauthorized, "invalid CLI session")
			return true
		}

		var cliSession models.CLISession
		b, _ := json.Marshal(cliDoc.Data)
		if err := json.Unmarshal(b, &cliSession); err != nil {
			s.logger.Error("failed to parse CLI session", "cli_session_id", cliSessionID, string(constants.ConnectionStateError), err)
			s.responder.Error(w, http.StatusInternalServerError, "failed to parse session")
			return true
		}

		if !cliSession.ExpiresAt.IsZero() && cliSession.ExpiresAt.Before(time.Now()) {
			s.logger.Warn("CLI session expired", "cli_session_id", cliSessionID)
			s.responder.Error(w, http.StatusUnauthorized, "CLI session expired")
			return true
		}

		if s.userSvc != nil && cliSession.UserID != "" {
			user, err := s.userSvc.GetByID(cliSession.UserID)
			if err != nil {
				s.logger.Error("failed to load user for CLI session", "user_id", cliSession.UserID, string(constants.ConnectionStateError), err)
				s.responder.Error(w, http.StatusInternalServerError, "identity validation failed")
				return true
			}
			if user != nil && !user.IsActive() {
				s.logger.Warn("CLI session identity disabled", "user_id", cliSession.UserID)
				s.responder.Error(w, http.StatusForbidden, "identity disabled")
				return true
			}
		}

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
			return true
		}
	}

	next.ServeHTTP(w, r)
	return true
}

// handleAppAuth handles authentication for system and external apps via URI SAN.
func (s *AuthService) handleAppAuth(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	if len(r.TLS.PeerCertificates) > 0 {
		cert := r.TLS.PeerCertificates[0]
		for _, uri := range cert.URIs {
			uriStr := uri.String()
			if strings.HasPrefix(uriStr, "spiffe://"+protocol.TrustDomain+"/app/") {
				appID := uriStr
				doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
				if err != nil || doc == nil {
					s.logger.Warn("App policy not found (deny-all default)", "app_id", appID)
					s.responder.Error(w, http.StatusForbidden, "app policy not found")
					return true
				}

				var policy models.AppPolicy
				data, _ := json.Marshal(doc.Data)
				if err := json.Unmarshal(data, &policy); err != nil {
					s.logger.Error("Failed to parse app policy", "app_id", appID, "error", err)
					s.responder.Error(w, http.StatusInternalServerError, "invalid app policy")
					return true
				}

				if strings.HasPrefix(r.URL.Path, "/_query") || strings.HasPrefix(r.URL.Path, "/api/governance/envelope") {
					s.responder.Error(w, http.StatusForbidden, "external apps cannot access privileged endpoints")
					return true
				}

				if err := s.enforceAppPolicy(r, &policy, appID); err != nil {
					s.logger.Warn("App policy enforcement failed", "app_id", appID, "error", err)
					s.responder.Error(w, http.StatusForbidden, err.Error())
					return true
				}

				ctx := context.WithValue(r.Context(), appIDKey, appID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return true
			}
		}
	}
	return false
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

// HasJWKS returns true if JWT authentication is configured.
func (s *AuthService) HasJWKS() bool {
	return s.jwks != nil
}

// JWTAuthMiddleware validates JWT tokens and performs JIT user provisioning.
// This is for external IdP authentication on MCP/A2A endpoints.
func (s *AuthService) JWTAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.jwks == nil {
			s.logger.Warn("JWT authentication requested but JWKS provider not configured")
			s.responder.Error(w, http.StatusServiceUnavailable, "JWT authentication not configured")
			return
		}

		authHeader := r.Header.Get(constants.HeaderAuthorization)
		if !strings.HasPrefix(authHeader, "Bearer ") {
			s.responder.Error(w, http.StatusUnauthorized, "missing JWT bearer token")
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == "" {
			s.responder.Error(w, http.StatusUnauthorized, "missing JWT token")
			return
		}

		jwt, err := ParseAndVerifyJWT(tokenString, s.jwks, s.jwtRole, s.jwtIssuer, s.jwtAudience)
		if err != nil {
			s.logger.Warn("JWT validation failed", "error", err)
			s.responder.Error(w, http.StatusUnauthorized, "invalid JWT token")
			return
		}

		if jwt.Claims.Sub == "" {
			s.responder.Error(w, http.StatusUnauthorized, "JWT missing subject claim")
			return
		}

		// JIT User Provisioning: get or create user by subject
		user, err := s.userSvc.GetOrCreateBySub(jwt.Claims.Sub)
		if err != nil {
			s.logger.Error("JIT user provisioning failed", "sub", jwt.Claims.Sub, "error", err)
			s.responder.Error(w, http.StatusInternalServerError, "user provisioning failed")
			return
		}

		if !user.IsActive() {
			s.responder.Error(w, http.StatusForbidden, "identity disabled")
			return
		}

		// Persona Mapping: map JWT roles to binding persona
		bindingPersona, err := s.personaSvc.MapRolesToPersona(jwt.Roles)
		if err != nil {
			s.logger.Warn("Failed to map roles to persona, using default", "error", err)
			bindingPersona = "default"
		}

		// Extract tenant_id from JWT claims (if present)
		tenantID := jwt.Claims.TenantID
		if tenantID == "" {
			tenantID = "default"
		}

		// Stamp context with identity and persona
		ctx := context.WithValue(r.Context(), userIDKey, user.ID)
		ctx = context.WithValue(ctx, constants.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, constants.ContextKeyBindingPersona, bindingPersona)

		s.logger.Debug("JWT authentication successful", "user_id", user.ID, "tenant_id", tenantID, "persona", bindingPersona)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
