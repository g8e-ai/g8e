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

// RouteAuthMode classifies how a route should be authenticated.
type RouteAuthMode int

const (
	// RouteAuthNone: truly public routes (health, PKI bootstrap, console SPA, passkey console).
	// No authentication required.
	RouteAuthNone RouteAuthMode = iota
	// RouteAuthMTLS: mTLS only (SSE push, governance, PKI management, operators, audit).
	// Requires a verified client certificate with valid operator/CLI/app identity.
	RouteAuthMTLS
	// RouteAuthWebSession: web session cookie only (users/me, auth/sessions/me, passkeys management, approvals browser).
	// Validates the web_session cookie and stamps context with user_id + web_session_id.
	RouteAuthWebSession
	// RouteAuthDual: mTLS OR web session cookie (SSE stream, SSE events).
	// Tries mTLS first (stronger auth); falls back to cookie if no cert present.
	RouteAuthDual
)

// RouteAuthRegistry classifies every route by its auth requirement.
// Exact paths are matched precisely (highest priority). Prefixes allow any path under the prefix.
// No excluded prefixes — each route is explicitly classified by mode.
type RouteAuthRegistry struct {
	exactPaths  map[string]RouteAuthMode
	prefixes    map[string]RouteAuthMode
	jwksEnabled bool
}

// NewRouteAuthRegistry creates a registry with the canonical route auth classifications.
func NewRouteAuthRegistry(jwksEnabled bool) *RouteAuthRegistry {
	r := &RouteAuthRegistry{
		exactPaths:  make(map[string]RouteAuthMode),
		prefixes:    make(map[string]RouteAuthMode),
		jwksEnabled: jwksEnabled,
	}

	// --- RouteAuthNone: truly public routes ---

	// Health check (always public)
	r.addExact(constants.APIPaths.Health, RouteAuthNone)

	// Landing page (always public to allow redirecting to /console)
	r.addExact(constants.APIPaths.Landing, RouteAuthNone)

	// State endpoint (always public for envelope binding)
	r.addExact(constants.APIPaths.State, RouteAuthNone)

	// PKI bootstrap routes (public material only)
	r.addPrefix(constants.APIPaths.WellKnownPKIPrefix, RouteAuthNone)
	r.addPrefix(constants.APIPaths.WellKnownBinPrefix, RouteAuthNone)

	// Deploy script endpoints (public for initial deployment)
	r.addExact(constants.APIPaths.DeployScriptLinux, RouteAuthNone)
	r.addExact(constants.APIPaths.DeployScriptWindows, RouteAuthNone)

	// Protocol entry points (CSR enrollment, bootstrap)
	r.addExact(constants.APIPaths.PKICSRSign, RouteAuthNone)
	r.addExact(constants.APIPaths.PKIDevicesEnroll, RouteAuthNone)
	r.addExact(constants.APIPaths.AuthBootstrap, RouteAuthNone)
	r.addExact(constants.APIPaths.AuthBootstrapStatus, RouteAuthNone)
	r.addExact(constants.APIPaths.AuthLogout, RouteAuthNone)
	r.addExact(constants.APIPaths.AuthEnrollmentTokenValidate, RouteAuthNone)
	r.addPrefix(constants.APIPaths.ApprovePage, RouteAuthNone)

	// CLI recovery discovery surface — public, token-scoped.
	// The opaque token (looked up via its hash) and proof-of-possession
	// signature provide the authorization context; no mTLS or cookie required.
	r.addExact(constants.APIPaths.AuthCLIRecoveryRequest, RouteAuthNone)
	r.addExact(constants.APIPaths.AuthCLIRecoveryStatus, RouteAuthNone)
	r.addExact(constants.APIPaths.AuthCLIRecoveryComplete, RouteAuthNone)

	// Console SPA (public, no auth required)
	r.addPrefix(constants.APIPaths.ConsolePrefix, RouteAuthNone)

	// Passkey console routes (public, no mTLS for browser access)
	r.addPrefix(constants.APIPaths.AuthPasskeysConsolePrefix, RouteAuthNone)

	// CLI-initiated enrollment passkey routes (public — the enrollment
	// token is the credential, same model as AuthEnrollmentTokenValidate).
	r.addPrefix(constants.APIPaths.AuthPasskeysEnrollmentPrefix, RouteAuthNone)

	// --- RouteAuthDual: SSE consumer endpoints (mTLS for CLI/operator, web session cookie for browser) ---
	r.addExact(constants.APIPaths.SSEStream, RouteAuthDual)
	r.addExact(constants.APIPaths.SSEEvents, RouteAuthDual)

	// --- RouteAuthWebSession: browser-facing routes (cookie auth) ---
	r.addPrefix(constants.APIPaths.UsersPrefix, RouteAuthWebSession)
	r.addPrefix(constants.APIPaths.AuthSessionsPrefix, RouteAuthWebSession)
	r.addPrefix(constants.APIPaths.Approvals, RouteAuthWebSession)
	r.addPrefix(constants.APIPaths.AuthPasskeys, RouteAuthWebSession)

	// CLI recovery approval — browser console, authenticated existing user.
	r.addExact(constants.APIPaths.AuthCLIRecoveryApprove, RouteAuthWebSession)

	// --- RouteAuthMTLS: mTLS-protected sub-paths under WebSession prefixes ---
	// These exact paths must be checked before the WebSession prefix matches.
	r.addExact(constants.APIPaths.AuthPasskeysCLIStatus, RouteAuthMTLS)
	r.addExact(constants.APIPaths.ApprovalsCLIStatus, RouteAuthMTLS)
	r.addExact(constants.APIPaths.ApprovalsCLIList, RouteAuthMTLS)
	r.addExact(constants.APIPaths.AuthEnrollmentTokenGenerate, RouteAuthMTLS)

	// CLI rotation — mTLS only. The caller's identity (user ID + active CLI
	// session ID) is derived from the verified CLI certificate URI SAN.
	// The fail-closed default already covers this path, but the explicit
	// classification documents the requirement and is asserted by tests.
	r.addExact(constants.APIPaths.AuthCLIRotate, RouteAuthMTLS)

	// JIT passkey routes: RouteAuthNone when JWKS is enabled (JWT middleware handles auth),
	// RouteAuthMTLS when JWKS is disabled (not accessible without mTLS).
	if jwksEnabled {
		r.addPrefix(constants.APIPaths.AuthPasskeysJITPrefix, RouteAuthNone)
		// MCP endpoint is public when JWKS is enabled for BYO clients (JWT middleware handles auth)
		r.addExact(constants.APIPaths.MCPEndpoint, RouteAuthNone)
		// A2A endpoints are public when JWKS is enabled
		r.addPrefix(constants.APIPaths.A2APrefix, RouteAuthNone)
	} else {
		// When JWKS is disabled, JIT passkey routes require mTLS
		r.addPrefix(constants.APIPaths.AuthPasskeysJITPrefix, RouteAuthMTLS)
	}

	return r
}

func (r *RouteAuthRegistry) addExact(path string, mode RouteAuthMode) {
	r.exactPaths[path] = mode
}

func (r *RouteAuthRegistry) addPrefix(prefix string, mode RouteAuthMode) {
	r.prefixes[prefix] = mode
}

// AuthMode returns the authentication mode for the given path.
// Exact paths are checked first (highest priority), then prefix matches.
// Unknown routes default to RouteAuthMTLS (fail-closed).
func (r *RouteAuthRegistry) AuthMode(path string) RouteAuthMode {
	// Check exact matches first (highest priority)
	if mode, ok := r.exactPaths[path]; ok {
		return mode
	}

	// Check prefix matches — longest prefix wins
	var bestPrefix string
	var bestMode RouteAuthMode
	found := false
	for prefix, mode := range r.prefixes {
		if strings.HasPrefix(path, prefix) {
			if !found || len(prefix) > len(bestPrefix) {
				bestPrefix = prefix
				bestMode = mode
				found = true
			}
		}
	}
	if found {
		return bestMode
	}

	// Fail-closed default: unknown routes require mTLS
	return RouteAuthMTLS
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
	t, ok := target.(*AuthError)
	if !ok {
		return false
	}
	if t.Reason == "" {
		return true
	}
	return e.Reason == t.Reason
}

// cacheEntry represents a cached value with expiration time.
type cacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

// AuthService handles authentication for the Gateway service.
type AuthService struct {
	db         *DocumentStoreService
	pki        *PKIAuthority
	logger     *slog.Logger
	userSvc    *UserService
	personaSvc *PersonaService
	responder  *response.Writer

	jwks        *JWKSProvider
	jwtRole     string
	jwtIssuer   string
	jwtAudience string

	routeAuth        *RouteAuthRegistry
	privilegedRoutes *PrivilegedRouteRegistry

	// Rate limiting state for app policies
	muLimiters sync.Mutex
	limiters   map[string]*tokenBucket

	// Auth caching (5-minute TTL)
	userCache sync.Map // userID -> *cacheEntry[*models.User]
}

// NewAuthService creates a new AuthService.
func NewAuthService(docStore *DocumentStoreService, pki *PKIAuthority, logger *slog.Logger, userSvc *UserService, personaSvc *PersonaService, responder *response.Writer, jwks *JWKSProvider, jwtRole, jwtIssuer, jwtAudience string) *AuthService {
	jwksEnabled := jwks != nil
	return &AuthService{
		db:               docStore,
		pki:              pki,
		logger:           logger,
		userSvc:          userSvc,
		personaSvc:       personaSvc,
		responder:        responder,
		jwks:             jwks,
		jwtRole:          jwtRole,
		jwtIssuer:        jwtIssuer,
		jwtAudience:      jwtAudience,
		routeAuth:        NewRouteAuthRegistry(jwksEnabled),
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
	ce := entry.(*cacheEntry[*models.User])
	if time.Now().After(ce.expiresAt) {
		s.userCache.Delete(userID)
		return nil
	}
	return ce.value
}

// cacheUser stores a user in cache with 5-minute TTL.
func (s *AuthService) cacheUser(userID string, user *models.User) {
	if userID == "" || user == nil {
		return
	}
	s.userCache.Store(userID, &cacheEntry[*models.User]{
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

// getAndValidateUser fetches a user by ID (cache-first), caches the result,
// and verifies the user is active. Returns the user if active, nil if not found,
// or an error if the lookup fails or the user is disabled.
func (s *AuthService) getAndValidateUser(userID string) (*models.User, error) {
	if s.userSvc == nil || userID == "" {
		return nil, nil
	}
	user := s.getCachedUser(userID)
	if user == nil {
		var err error
		user, err = s.userSvc.GetByID(userID)
		if err != nil {
			return nil, fmt.Errorf("gateway: auth: load user %s: %w: %w", userID, err, constants.ErrUserNotFound)
		}
		if user != nil {
			s.cacheUser(userID, user)
		}
	}
	if user != nil && !user.IsActive() {
		return nil, &AuthError{Message: constants.ErrIdentityDisabled.Error(), Reason: constants.AuthErrorReasonIdentityDisabled, Status: http.StatusForbidden}
	}
	return user, nil
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

	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 1)
	if err != nil {
		return nil, fmt.Errorf("gateway: auth: query operator session: %w", err)
	}

	if len(docs) == 0 {
		return nil, &AuthError{Message: constants.ErrGatewayOperatorSessionInvalid.Error(), Status: http.StatusUnauthorized}
	}

	// Convert Document to OperatorDocumentGo
	b, err := json.Marshal(docs[0].ForWire())
	if err != nil {
		return nil, fmt.Errorf("gateway: auth: marshal operator document: %w: %w", err, constants.ErrRequestMarshalFailed)
	}

	var op models.OperatorDocumentGo
	if err := json.Unmarshal(b, &op); err != nil {
		return nil, fmt.Errorf("gateway: auth: unmarshal operator document: %w: %w", err, constants.ErrResponseParseFailed)
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
	if _, err := s.getAndValidateUser(op.UserID); err != nil {
		return nil, err
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

// Middleware returns an http.Handler that authenticates requests using the unified auth middleware.
// The route auth mode is determined by RouteAuthRegistry, and auth is enforced accordingly.
// No bypasses — every request goes through the middleware. Fail-closed for unknown routes.
func (s *AuthService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := s.routeAuth.AuthMode(r.URL.Path)

		switch mode {
		case RouteAuthNone:
			// Truly public routes: no auth required
			next.ServeHTTP(w, r)
			return

		case RouteAuthMTLS:
			// mTLS only: enforce cert presence + revocation + identity extraction
			s.handleMTLSAuth(w, r, next)
			return

		case RouteAuthWebSession:
			// Web session cookie only: validate cookie, stamp context
			s.handleWebSessionAuth(w, r, next)
			return

		case RouteAuthDual:
			// Dual auth: try mTLS first, fall back to web session cookie
			if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
				s.handleMTLSAuth(w, r, next)
			} else {
				s.handleWebSessionAuth(w, r, next)
			}
			return

		default:
			// Fail-closed: unknown routes default to RouteAuthMTLS
			s.handleMTLSAuth(w, r, next)
			return
		}
	})
}

// handleMTLSAuth enforces mTLS cert presence, revocation check, and operator/CLI/app identity extraction.
// It stamps the context with identity info and calls next on success.
func (s *AuthService) handleMTLSAuth(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		s.logger.Warn("gateway: auth: mTLS required but no client certificate provided", "path", r.URL.Path)
		s.responder.Error(w, http.StatusUnauthorized, constants.ErrMTLSCertRequired.Error())
		return
	}

	// Verify certificate revocation status
	if s.pki != nil {
		if err := s.pki.VerifyCertificate(r.TLS.PeerCertificates[0]); err != nil {
			s.logger.Warn("gateway: auth: mTLS certificate revoked", "path", r.URL.Path, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: verify certificate: %w: %w", err, constants.ErrCertParseFailed))
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrMTLSCertRevoked.Error())
			return
		}
	}

	// Extract operator session ID from mTLS certificate SPIFFE URI SAN
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
	if strings.HasPrefix(r.URL.Path, constants.APIPaths.WSPrefix) {
		http.Error(w, constants.ErrGatewayUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	s.responder.Error(w, http.StatusUnauthorized, constants.ErrProtocolAuthRequired.Error())
}

// handleWebSessionAuth validates the web session cookie and stamps context with user_id + web_session_id.
func (s *AuthService) handleWebSessionAuth(w http.ResponseWriter, r *http.Request, next http.Handler) {
	webSessionID, userID, err := s.ValidateWebSessionCookie(r)
	if err != nil {
		s.responder.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	ctx := context.WithValue(r.Context(), constants.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, constants.ContextKeyWebSessionID, webSessionID)
	next.ServeHTTP(w, r.WithContext(ctx))
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
		if cliSessionID != "" {
			ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, cliSessionID)
		}
		if webSessionID := r.Header.Get(constants.HeaderWebSessionID); webSessionID != "" {
			ctx = context.WithValue(ctx, constants.ContextKeyWebSessionID, webSessionID)
		}
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

		cliDoc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		if err != nil {
			s.logger.Error("gateway: auth: load CLI session", "cli_session_id", cliSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: load CLI session %s: %w: %w", cliSessionID, err, constants.ErrNotFound))
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
			s.logger.Error("gateway: auth: marshal CLI session", "cli_session_id", cliSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: marshal CLI session %s: %w: %w", cliSessionID, err, constants.ErrRequestMarshalFailed))
			s.responder.Error(w, http.StatusInternalServerError, constants.ErrSessionParseFailed.Error())
			return true
		}
		if err := json.Unmarshal(b, &cliSession); err != nil {
			s.logger.Error("gateway: auth: unmarshal CLI session", "cli_session_id", cliSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: unmarshal CLI session %s: %w: %w", cliSessionID, err, constants.ErrResponseParseFailed))
			s.responder.Error(w, http.StatusInternalServerError, constants.ErrSessionParseFailed.Error())
			return true
		}

		if !cliSession.ExpiresAt.IsZero() && cliSession.ExpiresAt.Before(time.Now()) {
			s.logger.Warn("gateway: auth: CLI session expired", "cli_session_id", cliSessionID)
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrCLISessionExpired.Error())
			return true
		}

		if _, err := s.getAndValidateUser(cliSession.UserID); err != nil {
			if ae, ok := err.(*AuthError); ok {
				s.logger.Warn("gateway: auth: CLI session identity disabled", "user_id", cliSession.UserID)
				s.responder.Error(w, ae.Status, ae.Message)
			} else {
				s.logger.Error("gateway: auth: load user for CLI session", "user_id", cliSession.UserID, string(constants.ConnectionStateError), err)
				s.responder.Error(w, http.StatusInternalServerError, constants.ErrIdentityValidationFailed.Error())
			}
			return true
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
		// Stamp context with user_id, cli_session_id, and optional operator session info (for MCP proxying)
		ctx := context.WithValue(r.Context(), constants.ContextKeyUserID, cliSession.UserID)
		ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, cliSessionID)
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
		wid := protocol.NewWorkloadIdentity()
		for _, uri := range cert.URIs {
			uriStr := uri.String()
			if wid.IsAppSAN(uriStr) {
				appID := uriStr
				doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
				if err != nil {
					s.logger.Warn("gateway: auth: app policy not found", "app_id", appID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: load app policy %s: %w", appID, err))
					s.responder.Error(w, http.StatusForbidden, constants.ErrAppPolicyNotFound.Error())
					return true
				}
				if doc == nil {
					s.logger.Warn("gateway: auth: app policy not found", "app_id", appID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: load app policy %s: %w", appID, constants.ErrNotFound))
					s.responder.Error(w, http.StatusForbidden, constants.ErrAppPolicyNotFound.Error())
					return true
				}

				var policy models.AppPolicy
				data, err := json.Marshal(doc.Data)
				if err != nil {
					s.logger.Error("gateway: auth: marshal app policy", "app_id", appID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: marshal app policy %s: %w: %w", appID, err, constants.ErrRequestMarshalFailed))
					s.responder.Error(w, http.StatusInternalServerError, constants.ErrInvalidAppPolicy.Error())
					return true
				}
				if err := json.Unmarshal(data, &policy); err != nil {
					s.logger.Error("gateway: auth: unmarshal app policy", "app_id", appID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: unmarshal app policy %s: %w: %w", appID, err, constants.ErrResponseParseFailed))
					s.responder.Error(w, http.StatusInternalServerError, constants.ErrInvalidAppPolicy.Error())
					return true
				}

				if s.privilegedRoutes.IsPrivileged(r.URL.Path) {
					s.responder.Error(w, http.StatusForbidden, constants.ErrPrivilegedEndpointAccess.Error())
					return true
				}

				if err := s.enforceAppPolicy(r, &policy, appID); err != nil {
					s.logger.Warn("gateway: auth: app policy enforcement failed", "app_id", appID, string(constants.ConnectionStateError), err)
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
					if wid2.IsUserSAN(u2Str) {
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
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
	if err != nil {
		return false, fmt.Errorf("gateway: auth: load CLI session %s for cert binding: %w: %w", cliSessionID, err, constants.ErrNotFound)
	}
	if doc == nil {
		return false, nil
	}
	var cliSession models.CLISession
	b, err := json.Marshal(doc.Data)
	if err != nil {
		return false, fmt.Errorf("gateway: auth: marshal CLI session %s for cert binding: %w: %w", cliSessionID, err, constants.ErrRequestMarshalFailed)
	}
	if err := json.Unmarshal(b, &cliSession); err != nil {
		return false, fmt.Errorf("gateway: auth: unmarshal CLI session %s for cert binding: %w: %w", cliSessionID, err, constants.ErrResponseParseFailed)
	}
	if !cliSession.ExpiresAt.IsZero() && cliSession.ExpiresAt.Before(time.Now()) {
		return false, nil
	}
	return cliSession.OperatorSessionID == operatorSessionID, nil
}

// ValidateWebSessionCookie extracts and validates the web session cookie from
// the request, returning the web session ID and user ID if valid. This is used
// by the unified auth middleware for RouteAuthWebSession and RouteAuthDual modes.
func (s *AuthService) ValidateWebSessionCookie(r *http.Request) (webSessionID, userID string, err error) {
	cookie, err := r.Cookie(constants.WebSessionCookieName)
	if err != nil || cookie == nil {
		return "", "", constants.ErrWebSessionCookieRequired
	}

	webSessionID = cookie.Value
	if webSessionID == "" {
		return "", "", constants.ErrWebSessionCookieInvalid
	}

	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionWebSessions), webSessionID)
	if err != nil {
		s.logger.Error("gateway: auth: load web session", "web_session_id", webSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: load web session %s: %w: %w", webSessionID, err, constants.ErrNotFound))
		return "", "", constants.ErrWebSessionValidationFailed
	}
	if doc == nil {
		return "", "", constants.ErrWebSessionNotFound
	}

	var webSession models.WebSession
	data, err := json.Marshal(doc.Data)
	if err != nil {
		s.logger.Error("gateway: auth: marshal web session", "web_session_id", webSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: marshal web session %s: %w: %w", webSessionID, err, constants.ErrRequestMarshalFailed))
		return "", "", constants.ErrSessionParseFailed
	}
	if err := json.Unmarshal(data, &webSession); err != nil {
		s.logger.Error("gateway: auth: unmarshal web session", "web_session_id", webSessionID, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: unmarshal web session %s: %w: %w", webSessionID, err, constants.ErrResponseParseFailed))
		return "", "", constants.ErrSessionParseFailed
	}

	if time.Now().UnixMilli() > webSession.ExpiresAtUnixMs {
		return "", "", constants.ErrWebSessionExpired
	}

	if user, err := s.getAndValidateUser(webSession.UserID); err != nil {
		if ae, ok := err.(*AuthError); ok {
			return "", "", ae
		}
		s.logger.Error("gateway: auth: load user for web session", "user_id", webSession.UserID, string(constants.ConnectionStateError), err)
		return "", "", constants.ErrIdentityValidationFailed
	} else if user != nil {
		return webSession.ID, webSession.UserID, nil
	}

	return webSessionID, webSession.UserID, nil
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
		if !strings.HasPrefix(authHeader, constants.BearerScheme) {
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrJWTSignatureMissing.Error())
			return
		}

		tokenString := strings.TrimPrefix(authHeader, constants.BearerScheme)
		if tokenString == "" {
			s.responder.Error(w, http.StatusUnauthorized, constants.ErrJWTTokenMissing.Error())
			return
		}

		jwt, err := ParseAndVerifyJWT(r.Context(), tokenString, s.jwks, s.jwtRole, s.jwtIssuer, s.jwtAudience)
		if err != nil {
			s.logger.Warn("gateway: auth: JWT validation failed", string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: verify JWT: %w: %w", err, constants.ErrFailedToLoadCredentials))
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
				s.logger.Error("gateway: auth: JIT user lookup failed", "sub", jwt.Claims.Sub, string(constants.ConnectionStateError), fmt.Errorf("gateway: auth: lookup user by sub %s: %w: %w", jwt.Claims.Sub, err, constants.ErrUserNotFound))
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
			s.logger.Warn("gateway: auth: map roles to persona failed, using default", string(constants.ConnectionStateError), err)
			bindingPersona = constants.DefaultBindingPersona
		}

		// Extract tenant_id from JWT claims (if present)
		tenantID := jwt.Claims.TenantID
		if tenantID == "" {
			tenantID = constants.DefaultTenantID
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
