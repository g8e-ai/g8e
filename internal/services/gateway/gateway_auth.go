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
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/protocol"
)

// PublicRouteRegistry defines routes that bypass authentication.
// Exact paths are matched precisely. Prefixes allow any path under the prefix.
// Excluded prefixes protect mTLS-only sub-paths that share a prefix with WebSessionAuth routes.
// This centralized registry eliminates fragile HasPrefix duplication.
type PublicRouteRegistry struct {
	exactPaths       map[string]struct{}
	prefixes         map[string]struct{}
	excludedPrefixes map[string]struct{}
	jwksEnabled      bool
}

// NewPublicRouteRegistry creates a registry with the canonical public routes.
func NewPublicRouteRegistry(jwksEnabled bool) *PublicRouteRegistry {
	r := &PublicRouteRegistry{
		exactPaths:       make(map[string]struct{}),
		prefixes:         make(map[string]struct{}),
		excludedPrefixes: make(map[string]struct{}),
		jwksEnabled:      jwksEnabled,
	}

	// Health check (always public)
	r.addExact(constants.APIPaths.Health)

	// Landing page (always public to allow redirecting to /console)
	r.addExact(constants.APIPaths.Landing)

	// State endpoint (always public for envelope binding)
	r.addExact(constants.APIPaths.State)

	// PKI bootstrap routes (public material only)
	r.addPrefix(constants.APIPaths.WellKnownPKIPrefix)
	r.addPrefix(constants.APIPaths.WellKnownBinPrefix)

	// Trust script endpoints (public for initial bootstrap)
	r.addExact(constants.APIPaths.BootstrapCALinux)
	r.addExact(constants.APIPaths.BootstrapCAMacos)
	r.addExact(constants.APIPaths.BootstrapCAWindows)
	r.addExact(constants.APIPaths.WellKnownTrustWindows)

	// Deploy script endpoints (public for initial deployment)
	r.addExact(constants.APIPaths.DeployScriptLinux)
	r.addExact(constants.APIPaths.DeployScriptWindows)

	// Protocol entry points (CSR enrollment, bootstrap)
	r.addExact(constants.APIPaths.PKICSRSign)
	r.addExact(constants.APIPaths.PKIDevicesEnroll)
	r.addExact(constants.APIPaths.AuthBootstrap)
	r.addExact(constants.APIPaths.AuthBootstrapStatus)
	r.addExact(constants.APIPaths.AuthLogout)
	r.addPrefix(constants.APIPaths.ApprovePage)

	// Console SPA (public, no auth required)
	r.addPrefix("/console/")

	// Passkey console routes (public, no mTLS for browser access)
	// console/*  — Browser-facing passkey registration and authentication
	r.addPrefix(constants.APIPaths.AuthPasskeysConsolePrefix)

	// SSE consumer endpoints (dual auth: mTLS for CLI/operator, web session cookie for browser)
	// These bypass mTLS middleware; the handlers perform their own auth check.
	r.addExact(constants.APIPaths.SSEStream)
	r.addExact(constants.APIPaths.SSEEvents)

	// WebSessionAuth-protected routes (browser-facing, no client cert)
	// These bypass mTLS middleware; WebSessionAuth provides the auth gate
	// (cookie validation, session expiry, user active check — fail-closed).
	r.addPrefix("/api/v1/users/")
	r.addPrefix("/api/v1/auth/sessions/")
	r.addPrefix("/api/v1/approvals")
	r.addPrefix("/api/v1/auth/passkeys")

	// Exclude mTLS-protected sub-paths under the passkeys prefix.
	// These routes require client certificates and must NOT bypass mTLS.
	r.addExcludedPrefix("/api/v1/auth/passkeys/cli/")

	// Exclude mTLS-protected CLI approval status endpoint.
	// The CLI polls this with mTLS; it must NOT bypass mTLS via the
	// WebSessionAuth public prefix for /api/v1/approvals.
	r.addExcludedPrefix("/api/v1/approvals/status/")

	// Exclude JIT passkey sub-paths when JWKS is not configured.
	// When JWKS is enabled, the JIT prefix is added below as a public prefix,
	// and exact paths are checked before exclusions in IsPublic.
	if !jwksEnabled {
		r.addExcludedPrefix(constants.APIPaths.AuthPasskeysJITPrefix)
	}

	// JIT passkey bootstrap (only when JWKS is configured)
	if jwksEnabled {
		r.addPrefix(constants.APIPaths.AuthPasskeysJITPrefix)
		// MCP endpoint is public when JWKS is enabled for BYO clients
		r.addExact(constants.APIPaths.MCPEndpoint)
		// A2A endpoints are public when JWKS is enabled
		r.addPrefix(constants.APIPaths.A2APrefix)
	}

	return r
}

func (r *PublicRouteRegistry) addExact(path string) {
	r.exactPaths[path] = struct{}{}
}

func (r *PublicRouteRegistry) addPrefix(prefix string) {
	r.prefixes[prefix] = struct{}{}
}

func (r *PublicRouteRegistry) addExcludedPrefix(prefix string) {
	r.excludedPrefixes[prefix] = struct{}{}
}

// IsPublic returns true if the path is registered as a public route.
func (r *PublicRouteRegistry) IsPublic(path string) bool {
	// Check exact matches first (highest priority)
	if _, ok := r.exactPaths[path]; ok {
		return true
	}

	// Check excluded prefixes (mTLS-protected sub-paths under WebSessionAuth prefixes)
	for prefix := range r.excludedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return false
		}
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
	Message string                    `json:"error"`
	Reason  constants.AuthErrorReason `json:"reason,omitempty"`
	Status  int                       `json:"-"`
}

// PrivilegedRouteRegistry defines routes that require operator or CLI auth.
// App certificates are blocked from these routes. This centralized registry
// eliminates fragile HasPrefix duplication in handleAppAuth.
type PrivilegedRouteRegistry struct {
	prefixes map[string]struct{}
}

// NewPrivilegedRouteRegistry creates a registry with the canonical privileged routes.
func NewPrivilegedRouteRegistry() *PrivilegedRouteRegistry {
	r := &PrivilegedRouteRegistry{
		prefixes: make(map[string]struct{}),
	}

	// Governance envelope submission requires operator/CLI auth
	r.addPrefix(constants.APIPaths.GovernanceEnvelopes)

	// Query endpoints require operator/CLI auth
	r.addPrefix(constants.APIPaths.QueryPrefix)

	return r
}

func (r *PrivilegedRouteRegistry) addPrefix(prefix string) {
	r.prefixes[prefix] = struct{}{}
}

// IsPrivileged returns true if the path matches a privileged route prefix.
func (r *PrivilegedRouteRegistry) IsPrivileged(path string) bool {
	for prefix := range r.prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func (e *AuthError) Error() string {
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Sprintf("{\"error\":\"%s\"}", e.Message)
	}
	return string(b)
}

func (e *AuthError) Is(target error) bool {
	_, ok := target.(*AuthError)
	return ok
}

// cacheEntry represents a cached value with expiration time.
type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// AuthService handles authentication for the Gateway service.
type AuthService struct {
	db         *CanonicalDBService
	pki        *PKIAuthority
	logger     *slog.Logger
	userSvc    *UserService
	personaSvc *PersonaService
	responder  *response.Writer
	secretsDir string

	jwks        *JWKSProvider
	jwtRole     string
	jwtIssuer   string
	jwtAudience string

	publicRoutes     *PublicRouteRegistry
	privilegedRoutes *PrivilegedRouteRegistry

	// Rate limiting state for app policies
	muLimiters sync.Mutex
	limiters   map[string]*tokenBucket

	// Auth caching (5-minute TTL)
	userCache sync.Map // userID -> *cacheEntry[*models.User]
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *CanonicalDBService, pki *PKIAuthority, logger *slog.Logger, userSvc *UserService, personaSvc *PersonaService, responder *response.Writer, secretsDir string, jwks *JWKSProvider, jwtRole, jwtIssuer, jwtAudience string) *AuthService {
	jwksEnabled := jwks != nil
	return &AuthService{
		db:               db,
		pki:              pki,
		logger:           logger,
		userSvc:          userSvc,
		personaSvc:       personaSvc,
		responder:        responder,
		secretsDir:       secretsDir,
		jwks:             jwks,
		jwtRole:          jwtRole,
		jwtIssuer:        jwtIssuer,
		jwtAudience:      jwtAudience,
		publicRoutes:     NewPublicRouteRegistry(jwksEnabled),
		privilegedRoutes: NewPrivilegedRouteRegistry(),
		limiters:         make(map[string]*tokenBucket),
	}
}

// getCachedUser retrieves a user from cache if valid.
func (s *AuthService) getCachedUser(userID string) *models.User {
	if userID == "" {
		return nil
	}
	entry, ok := s.userCache.Load(userID)
	if !ok {
		return nil
	}
	ce := entry.(*cacheEntry)
	if time.Now().After(ce.expiresAt) {
		s.userCache.Delete(userID)
		return nil
	}
	return ce.value.(*models.User)
}

// cacheUser stores a user in cache with 5-minute TTL.
func (s *AuthService) cacheUser(userID string, user *models.User) {
	if userID == "" || user == nil {
		return
	}
	s.userCache.Store(userID, &cacheEntry{
		value:     user,
		expiresAt: time.Now().Add(5 * time.Minute),
	})
}

// invalidateUserCache removes a user from cache.
func (s *AuthService) invalidateUserCache(userID string) {
	if userID != "" {
		s.userCache.Delete(userID)
	}
}

// InvalidateUserCache is a public method for explicit cache invalidation.
// Call this after user status changes (disable, delete, etc.) to ensure cache consistency.
func (s *AuthService) InvalidateUserCache(userID string) {
	s.invalidateUserCache(userID)
}

// ValidateOperatorSession checks if a session ID is valid and returns the Operator document.
// Auth depends on session validity (existence + certificate revocation), not on operator
// status liveness signals from other processes. The primary session invalidation mechanism
// is certificate revocation via PKI authority.
func (s *AuthService) ValidateOperatorSession(operatorSessionID string) (*models.OperatorDocumentGo, error) {
	if operatorSessionID == "" {
		return nil, &AuthError{Message: constants.ErrGatewayOperatorSessionIDRequired.Error(), Status: http.StatusUnauthorized}
	}

	filters := []models.DocFilter{
		{Field: "operator_session_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", operatorSessionID))},
	}

	docs, err := s.db.DocStore.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 1)
	if err != nil {
		return nil, fmt.Errorf("gateway: auth: query operator session: %w", err)
	}

	if len(docs) == 0 {
		return nil, &AuthError{Message: constants.ErrGatewayOperatorSessionInvalid.Error(), Status: http.StatusUnauthorized}
	}

	// Convert Document to OperatorDocumentGo
	b, err := json.Marshal(docs[0].ForWire())
	if err != nil {
		return nil, fmt.Errorf("gateway: auth: marshal operator document: %w", constants.ErrRequestMarshalFailed)
	}

	var op models.OperatorDocumentGo
	if err := json.Unmarshal(b, &op); err != nil {
		return nil, fmt.Errorf("gateway: auth: unmarshal operator document: %w", constants.ErrResponseParseFailed)
	}

	// [PIVOT] Reject terminated identities (Plan §4.6)
	// We allow OFFLINE and STALE statuses to authenticate (to support bootstrap
	// and recovery), but TERMINATED is a hard-gate rejection.
	if op.Status == constants.OperatorStatusTerminated {
		return nil, &AuthError{Message: constants.ErrOperatorIdentityDisabled.Error(), Status: http.StatusUnauthorized}
	}

	// Enforce session expiry (TTL)
	// Default session TTL is 24h if not specified.
	sessionTTL := 24 * time.Hour
	// We use the Document store's authoritative CreatedAt for TTL enforcement.
	if !docs[0].CreatedAt.IsZero() && time.Since(docs[0].CreatedAt) > sessionTTL {
		return nil, &AuthError{Message: constants.ErrOperatorSessionExpired.Error(), Status: http.StatusUnauthorized}
	}

	// Check if the linked user is active (plan §4.6)
	// This is the single chokepoint that makes retirement real - without it,
	// a stale CLI cert can still talk to the Gateway.
	if s.userSvc != nil && op.UserID != "" {
		// Try cache first
		user := s.getCachedUser(op.UserID)
		if user == nil {
			var err error
			user, err = s.userSvc.GetByID(op.UserID)
			if err != nil {
				return nil, fmt.Errorf("gateway: auth: load user %s: %w", op.UserID, constants.ErrUserNotFound)
			}
			if user != nil {
				s.cacheUser(op.UserID, user)
			}
		}
		if user != nil && !user.IsActive() {
			// Return structured error for disabled users
			return nil, &AuthError{Message: constants.ErrIdentityDisabled.Error(), Reason: constants.AuthErrorReasonIdentityDisabled, Status: http.StatusForbidden}
		}
	}

	return &op, nil
}

// extractOperatorSessionIDFromMTLS extracts the operator session ID from the mTLS certificate's SPIFFE URI SAN.
// This enables mTLS-only authentication without requiring Bearer tokens.
func (s *AuthService) extractOperatorSessionIDFromMTLS(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return ""
	}

	cert := r.TLS.PeerCertificates[0]
	wid := protocol.NewWorkloadIdentity()

	for _, uri := range cert.URIs {
		spiffeID := uri.String()
		if sessionID, ok := wid.ExtractOperatorSessionID(spiffeID); ok {
			return sessionID
		}
	}

	return ""
}

// Middleware returns an http.Handler that authenticates requests.
// It chains multiple single-responsibility middlewares:
// 1. publicBypassMiddleware (unauthenticated routes - bypasses entire chain)
// 2. mtlsMiddleware (mTLS enforcement and revocation)
// 3. authMiddleware (Operator, CLI, or App authentication)
func (s *AuthService) Middleware(next http.Handler) http.Handler {
	return s.publicBypassMiddleware(
		s.mtlsMiddleware(
			s.authMiddleware(next),
		),
		next,
	)
}

// publicBypassMiddleware handles routes that are accessible without any authentication.
// For public routes, it bypasses the entire middleware chain and calls the final handler directly.
func (s *AuthService) publicBypassMiddleware(middlewareChain, finalHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.publicRoutes.IsPublic(r.URL.Path) {
			finalHandler.ServeHTTP(w, r)
			return
		}

		middlewareChain.ServeHTTP(w, r)
	})
}

// mtlsMiddleware enforces mTLS and verifies certificate revocation.
func (s *AuthService) mtlsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// [PIVOT] Enforce mTLS for all routes (Phase 6)
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			s.logger.Warn("gateway: auth: mTLS required but no client certificate provided", "path", r.URL.Path)
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrMTLSCertRequired.Error())
			return
		}

		// [PIVOT] Verify certificate revocation status (Phase 6)
		if s.pki != nil {
			if err := s.pki.VerifyCertificate(r.TLS.PeerCertificates[0]); err != nil {
				s.logger.Warn("gateway: auth: mTLS certificate revoked", "path", r.URL.Path, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: verify certificate: %w", constants.ErrCertParseFailed))
				s.responder.Error(w, http.StatusUnauthorized, constants.ErrMTLSCertRevoked.Error())
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware handles authentication for Operator, CLI, and App identities.
func (s *AuthService) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract operator session ID from mTLS certificate SPIFFE URI SAN (mTLS-only auth)
		operatorSessionID := s.extractOperatorSessionIDFromMTLS(r)
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

		s.responder.Error(w, http.StatusUnauthorized, constants.ErrProtocolAuthRequired.Error())
	})
}

// handleOperatorAuth handles authentication for Operator sessions.
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
				var err error
				match, err = s.cliCertBoundToOperator(cert.URIs, cliSessionID, op.UserID, operatorSessionID)
				if err != nil {
					s.logger.Error("gateway: auth: CLI cert binding check failed", "operator_session_id", operatorSessionID, "cli_session_id", cliSessionID, string(constants.ConnectionStateError), err)
					s.responder.Error(w, http.StatusInternalServerError, constants.ErrCLICertBindingCheckFailed.Error())
					return true
				}
			}
			if !match {
				s.logger.Warn("gateway: auth: mTLS URI SAN mismatch for Operator session", "path", r.URL.Path, "operator_id", op.ID, "operator_session_id", operatorSessionID)
				s.responder.Error(w, http.StatusForbidden, constants.ErrMTLSIdentityMismatch.Error())
				return true
			}
		}

		// Stamp context with user_id and operator session info
		ctx := context.WithValue(r.Context(), constants.ContextKeyUserID, op.UserID)
		ctx = context.WithValue(ctx, constants.ContextKeyTenantID, op.OrganizationID)
		ctx = context.WithValue(ctx, constants.ContextKeyOperatorID, op.ID)
		ctx = context.WithValue(ctx, constants.ContextKeyOperatorSessionID, operatorSessionID)
		next.ServeHTTP(w, r.WithContext(ctx))
		return true
	}

	s.logger.Warn("gateway: auth: Invalid Operator session attempt", "operator_session_id", safeTruncateID(operatorSessionID, 8), string(constants.ConnectionStateError), err)

	if ae, ok := err.(*AuthError); ok {
		s.responder.Error(w, ae.Status, ae.Message)
		return true
	}
	s.logger.Error("gateway: auth: operator session validation failed", "operator_session_id", safeTruncateID(operatorSessionID, 8), string(constants.ConnectionStateError), err)
	s.responder.Error(w, http.StatusInternalServerError, constants.ErrInternal.Error())
	return true
}

// handleCLIAuth handles authentication for CLI sessions.
func (s *AuthService) handleCLIAuth(w http.ResponseWriter, r *http.Request, cliSessionID string, next http.Handler) bool {
	if len(r.TLS.PeerCertificates) > 0 {
		wid := protocol.NewWorkloadIdentity()
		cert := r.TLS.PeerCertificates[0]

		cliDoc, err := s.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		if err != nil {
			s.logger.Error("gateway: auth: load CLI session", "cli_session_id", cliSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: load CLI session %s: %w", cliSessionID, constants.ErrNotFound))
			s.responder.Error(w, http.StatusInternalServerError, constants.ErrSessionLoadFailed.Error())
			return true
		}
		if cliDoc == nil {
			s.logger.Warn("gateway: auth: CLI session not found", "cli_session_id", cliSessionID)
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrCLISessionInvalid.Error())
			return true
		}

		var cliSession models.CLISession
		b, err := json.Marshal(cliDoc.Data)
		if err != nil {
			s.logger.Error("gateway: auth: marshal CLI session", "cli_session_id", cliSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: marshal CLI session %s: %w", cliSessionID, constants.ErrRequestMarshalFailed))
			s.responder.Error(w, http.StatusInternalServerError, constants.ErrSessionParseFailed.Error())
			return true
		}
		if err := json.Unmarshal(b, &cliSession); err != nil {
			s.logger.Error("gateway: auth: unmarshal CLI session", "cli_session_id", cliSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: unmarshal CLI session %s: %w", cliSessionID, constants.ErrResponseParseFailed))
			s.responder.Error(w, http.StatusInternalServerError, constants.ErrSessionParseFailed.Error())
			return true
		}

		if !cliSession.ExpiresAt.IsZero() && cliSession.ExpiresAt.Before(time.Now()) {
			s.logger.Warn("gateway: auth: CLI session expired", "cli_session_id", cliSessionID)
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrCLISessionExpired.Error())
			return true
		}

		if s.userSvc != nil && cliSession.UserID != "" {
			// Try cache first
			user := s.getCachedUser(cliSession.UserID)
			if user == nil {
				var err error
				user, err = s.userSvc.GetByID(cliSession.UserID)
				if err != nil {
					s.logger.Error("gateway: auth: load user for CLI session", "user_id", cliSession.UserID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: load user %s for CLI session: %w", cliSession.UserID, constants.ErrUserNotFound))
					s.responder.Error(w, http.StatusInternalServerError, constants.ErrIdentityValidationFailed.Error())
					return true
				}
				if user != nil {
					s.cacheUser(cliSession.UserID, user)
				}
			}
			if user != nil && !user.IsActive() {
				s.logger.Warn("gateway: auth: CLI session identity disabled", "user_id", cliSession.UserID)
				s.responder.Error(w, http.StatusForbidden, constants.ErrIdentityDisabled.Error())
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
			s.logger.Warn("gateway: auth: mTLS URI SAN mismatch for CLI session", "path", r.URL.Path, "cli_session_id", cliSessionID)
			s.responder.Error(w, http.StatusForbidden, constants.ErrMTLSIdentityMismatch.Error())
			return true
		}
		// Stamp context with user_id and optional operator session info (for MCP proxying)
		ctx := context.WithValue(r.Context(), constants.ContextKeyUserID, cliSession.UserID)
		if opID := r.Header.Get(constants.HeaderOperatorID); opID != "" {
			ctx = context.WithValue(ctx, constants.ContextKeyOperatorID, opID)
		}
		if opSessionID := r.Header.Get(constants.HeaderOperatorSessionID); opSessionID != "" {
			ctx = context.WithValue(ctx, constants.ContextKeyOperatorSessionID, opSessionID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
		return true
	}

	return false
}

// handleAppAuth handles authentication for system and external apps via URI SAN.
func (s *AuthService) handleAppAuth(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	if len(r.TLS.PeerCertificates) > 0 {
		cert := r.TLS.PeerCertificates[0]
		for _, uri := range cert.URIs {
			uriStr := uri.String()
			if strings.HasPrefix(uriStr, "spiffe://"+protocol.TrustDomain+"/app/") {
				appID := uriStr
				doc, err := s.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
				if err != nil || doc == nil {
					s.logger.Warn("gateway: auth: app policy not found", "app_id", appID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: load app policy %s: %w", appID, constants.ErrNotFound))
					s.responder.Error(w, http.StatusForbidden, constants.ErrAppPolicyNotFound.Error())
					return true
				}

				var policy models.AppPolicy
				data, err := json.Marshal(doc.Data)
				if err != nil {
					s.logger.Error("gateway: auth: marshal app policy", "app_id", appID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: marshal app policy %s: %w", appID, constants.ErrRequestMarshalFailed))
					s.responder.Error(w, http.StatusInternalServerError, constants.ErrInvalidAppPolicy.Error())
					return true
				}
				if err := json.Unmarshal(data, &policy); err != nil {
					s.logger.Error("gateway: auth: unmarshal app policy", "app_id", appID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: unmarshal app policy %s: %w", appID, constants.ErrResponseParseFailed))
					s.responder.Error(w, http.StatusInternalServerError, constants.ErrInvalidAppPolicy.Error())
					return true
				}

				if s.privilegedRoutes.IsPrivileged(r.URL.Path) {
					s.responder.Error(w, http.StatusForbidden, constants.ErrPrivilegedEndpointAccess.Error())
					return true
				}

				if err := s.enforceAppPolicy(r, &policy, appID); err != nil {
					s.logger.Warn("App policy enforcement failed", "app_id", appID, "error", err)
					if ae, ok := err.(*AuthError); ok {
						s.responder.Error(w, ae.Status, ae.Message)
					} else {
						s.responder.Error(w, http.StatusForbidden, err.Error())
					}
					return true
				}

				ctx := context.WithValue(r.Context(), constants.ContextKeyAppID, appID)
				// Delegated certs carry a second URI SAN for the requestor user identity.
				// Extract it so processGatewayTransaction can bind both identities to
				// the signed governance envelope (RequestorUserId + ActingAppId).
				wid2 := protocol.NewWorkloadIdentity()
				for _, u2 := range cert.URIs {
					u2Str := u2.String()
					if strings.HasPrefix(u2Str, "spiffe://"+protocol.TrustDomain+"/user/") {
						if userID, ok := wid2.ExtractUserIDFromUserSAN(u2Str); ok {
							ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
						}
						break
					}
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return true
			}
		}
	}
	return false
}

// getLimiter returns a rate limiter for the given app ID, creating one if needed.
func (s *AuthService) getLimiter(appID string, rps int) *tokenBucket {
	s.muLimiters.Lock()
	defer s.muLimiters.Unlock()

	limiter, exists := s.limiters[appID]
	if !exists {
		// Create a new limiter with the configured RPS and a burst of 2x RPS
		limiter = newTokenBucket(float64(rps), rps*2)
		s.limiters[appID] = limiter
	}
	return limiter
}

// enforceAppPolicy validates that the request complies with the app's policy.
// It checks collection access, event types, intents, and rate limits.
func (s *AuthService) enforceAppPolicy(r *http.Request, policy *models.AppPolicy, appID string) error {
	// Check rate limit (if configured)
	if policy.RateLimitRPS > 0 {
		limiter := s.getLimiter(appID, policy.RateLimitRPS)
		if !limiter.Allow() {
			s.logger.Warn("gateway: auth: app rate limit exceeded", "app_id", appID, "rate_limit_rps", policy.RateLimitRPS, "path", r.URL.Path)
			return &AuthError{Message: fmt.Sprintf("rate limit exceeded (%d RPS)", policy.RateLimitRPS), Reason: constants.AuthErrorReasonRateLimitExceeded, Status: http.StatusTooManyRequests}
		}
	}

	// Check payload size (if configured)
	if policy.MaxPayloadBytes > 0 {
		if r.ContentLength > policy.MaxPayloadBytes {
			return &AuthError{Message: fmt.Sprintf("payload exceeds maximum allowed size of %d bytes", policy.MaxPayloadBytes), Reason: constants.AuthErrorReasonPayloadTooLarge, Status: http.StatusRequestEntityTooLarge}
		}
	}

	// Check collection access for /_query paths (already blocked, but for future-proofing)
	if strings.HasPrefix(r.URL.Path, constants.APIPaths.QueryPrefix) {
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
				return &AuthError{Message: fmt.Sprintf("collection '%s' not in allowed collections", collection), Reason: constants.AuthErrorReasonCollectionNotAllowed, Status: http.StatusForbidden}
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
func (s *AuthService) cliCertBoundToOperator(certURIs []*url.URL, cliSessionID, userID, operatorSessionID string) (bool, error) {
	if cliSessionID == "" || operatorSessionID == "" {
		return false, nil
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
		return false, nil
	}
	doc, err := s.db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
	if err != nil {
		return false, fmt.Errorf("gateway: auth: load CLI session %s for cert binding: %w", cliSessionID, constants.ErrNotFound)
	}
	if doc == nil {
		return false, nil
	}
	var cliSession models.CLISession
	b, err := json.Marshal(doc.Data)
	if err != nil {
		return false, fmt.Errorf("gateway: auth: marshal CLI session %s for cert binding: %w", cliSessionID, constants.ErrRequestMarshalFailed)
	}
	if err := json.Unmarshal(b, &cliSession); err != nil {
		return false, fmt.Errorf("gateway: auth: unmarshal CLI session %s for cert binding: %w", cliSessionID, constants.ErrResponseParseFailed)
	}
	if !cliSession.ExpiresAt.IsZero() && cliSession.ExpiresAt.Before(time.Now()) {
		return false, nil
	}
	return cliSession.OperatorSessionID == operatorSessionID, nil
}

// WebSocketAuth returns an http.Handler that authenticates WebSocket connections.
func (s *AuthService) WebSocketAuth(next http.Handler) http.Handler {
	// Re-use the main Middleware logic for WebSockets to ensure consistency.
	// Middleware already handles /ws/ prefix specifically and bootstrap bypass.
	return s.Middleware(next)
}

// ValidateWebSessionCookie extracts and validates the web session cookie from
// the request, returning the web session ID and user ID if valid. This is used
// by both WebSessionAuth middleware and the unified SSE stream handler.
func (s *AuthService) ValidateWebSessionCookie(r *http.Request, db *CanonicalDBService) (webSessionID, userID string, err error) {
	cookie, err := r.Cookie(constants.WebSessionCookieName)
	if err != nil || cookie == nil {
		return "", "", constants.ErrWebSessionCookieRequired
	}

	webSessionID = cookie.Value
	if webSessionID == "" {
		return "", "", constants.ErrWebSessionCookieInvalid
	}

	doc, err := db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionWebSessions), webSessionID)
	if err != nil {
		s.logger.Error("gateway: auth: load web session", "web_session_id", webSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: load web session %s: %w", webSessionID, constants.ErrNotFound))
		return "", "", constants.ErrWebSessionValidationFailed
	}
	if doc == nil {
		return "", "", constants.ErrWebSessionNotFound
	}

	var webSession models.WebSession
	data, err := json.Marshal(doc.Data)
	if err != nil {
		s.logger.Error("gateway: auth: marshal web session", "web_session_id", webSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: marshal web session %s: %w", webSessionID, constants.ErrRequestMarshalFailed))
		return "", "", constants.ErrSessionParseFailed
	}
	if err := json.Unmarshal(data, &webSession); err != nil {
		s.logger.Error("gateway: auth: unmarshal web session", "web_session_id", webSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: unmarshal web session %s: %w", webSessionID, constants.ErrResponseParseFailed))
		return "", "", constants.ErrSessionParseFailed
	}

	if time.Now().UnixMilli() > webSession.ExpiresAtUnixMs {
		return "", "", constants.ErrWebSessionExpired
	}

	if s.userSvc != nil {
		user := s.getCachedUser(webSession.UserID)
		if user == nil {
			user, err = s.userSvc.GetByID(webSession.UserID)
			if err != nil {
				s.logger.Error("gateway: auth: load user for web session", "user_id", webSession.UserID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: load user %s for web session: %w", webSession.UserID, constants.ErrUserNotFound))
				return "", "", constants.ErrIdentityValidationFailed
			}
			if user != nil {
				s.cacheUser(webSession.UserID, user)
			}
		}
		if user != nil && !user.IsActive() {
			return "", "", constants.ErrIdentityDisabled
		}
	}

	return webSessionID, webSession.UserID, nil
}

// WebSessionAuth validates web session cookies and stamps context with user_id.
// This is for browser-based authentication on the public gateway.
func (s *AuthService) WebSessionAuth(next http.Handler, db *CanonicalDBService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webSessionID, userID, err := s.ValidateWebSessionCookie(r, db)
		if err != nil {
			s.responder.Error(w, http.StatusUnauthorized, err.Error())
			return
		}
		_ = webSessionID

		ctx := context.WithValue(r.Context(), constants.ContextKeyUserID, userID)
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
			s.responder.Error(w, http.StatusServiceUnavailable, constants.ErrJWTAuthNotConfigured.Error())
			return
		}

		authHeader := r.Header.Get(constants.HeaderAuthorization)
		if !strings.HasPrefix(authHeader, "Bearer ") {
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrJWTSignatureMissing.Error())
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == "" {
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrJWTTokenMissing.Error())
			return
		}

		jwt, err := ParseAndVerifyJWT(r.Context(), tokenString, s.jwks, s.jwtRole, s.jwtIssuer, s.jwtAudience)
		if err != nil {
			s.logger.Warn("gateway: auth: JWT validation failed", string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: verify JWT: %w", constants.ErrFailedToLoadCredentials))
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrJWTInvalidToken.Error())
			return
		}

		if jwt.Claims.Sub == "" {
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrJWTSessionSubjectMissing.Error())
			return
		}

		// JIT User Provisioning: get or create user by subject
		// Try cache first
		user := s.getCachedUser(jwt.Claims.Sub)
		if user == nil {
			var err error
			user, err = s.userSvc.GetBySub(jwt.Claims.Sub)
			if err != nil {
				s.logger.Error("gateway: auth: JIT user lookup failed", "sub", jwt.Claims.Sub, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: lookup user by sub %s: %w", jwt.Claims.Sub, constants.ErrUserNotFound))
				s.responder.Error(w, http.StatusInternalServerError, constants.ErrIdentityValidationFailed.Error())
				return
			}
			if user != nil {
				s.cacheUser(jwt.Claims.Sub, user)
			}
		}
		if user == nil {
			var jitErr error
			user, jitErr = s.userSvc.CreateUserWithSub(jwt.Claims.Sub)
			if jitErr != nil {
				s.logger.Error("gateway: auth: JIT user provisioning failed", "sub", jwt.Claims.Sub, "error", jitErr)
				s.responder.Error(w, http.StatusInternalServerError, constants.ErrUserCreationFailed.Error())
				return
			}
			s.cacheUser(jwt.Claims.Sub, user)
		}

		if !user.IsActive() {
			s.responder.Error(w, http.StatusForbidden, constants.ErrIdentityDisabled.Error())
			return
		}

		// Persona Mapping: map JWT roles to binding persona
		bindingPersona, err := s.personaSvc.MapRolesToPersona(jwt.Roles)
		if err != nil {
			s.logger.Warn("gateway: auth: map roles to persona failed, using default", "state", string(constants.ConnectionStateError), "error", err)
			bindingPersona = "default"
		}

		// Extract tenant_id from JWT claims (if present)
		tenantID := jwt.Claims.TenantID
		if tenantID == "" {
			tenantID = "default"
		}

		// Stamp context with identity and persona
		ctx := context.WithValue(r.Context(), constants.ContextKeyUserID, user.ID)
		ctx = context.WithValue(ctx, constants.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, constants.ContextKeyBindingPersona, bindingPersona)

		s.logger.Debug("gateway: auth: JWT authentication successful", "user_id", user.ID, "tenant_id", tenantID, "persona", bindingPersona)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// safeTruncateID returns the first n characters of id followed by "...", or the
// full id if it is shorter than n. This prevents panics on short or empty IDs.
func safeTruncateID(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n] + "..."
}
