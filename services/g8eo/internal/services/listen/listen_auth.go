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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
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

// AuthService handles authentication for the Listen service.
type AuthService struct {
	db         *ListenDBService
	pki        *PKIAuthority
	logger     *slog.Logger
	userSvc    *UserService
	secretsDir string
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *ListenDBService, pki *PKIAuthority, logger *slog.Logger, userSvc *UserService, secretsDir string) *AuthService {
	return &AuthService{
		db:         db,
		pki:        pki,
		logger:     logger,
		userSvc:    userSvc,
		secretsDir: secretsDir,
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

	docs, err := s.db.DocQuery(string(constants.CollectionOperators), filters, "", 1)
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
	if op.Status == constants.Status.OperatorStatus.Terminated {
		return nil, &AuthError{
			Message: "operator identity disabled",
			Reason:  string(constants.Status.OperatorStatus.Terminated),
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
	// This is the single chokepoint that makes retirement real — without it,
	// a stale CLI cert can still talk to the substrate.
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

	docs, err := s.db.DocQuery(string(constants.CollectionOperators), filters, "", 1)
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

		// These routes are new protocol entry points.
		// Native Registration Path (Phase 4)
		// This endpoint is the new sovereign entry point for enrolling binaries.
		// It MUST be accessible without an internal token as it is the first step
		// of the trust bootstrap.
		if r.URL.Path == "/api/pki/sign-csr" ||
			r.URL.Path == "/api/auth/device-link/register" {
			next.ServeHTTP(w, r)
			return
		}

		// [PIVOT] Enforce mTLS for all other routes (Phase 6)
		// Client certificates must be present and verified by the hub/root CA.
		// tls.VerifyClientCertIfGiven ensures the chain is already verified if present.
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			s.logger.Warn("mTLS required but no client certificate provided", "path", r.URL.Path)
			s.jsonError(w, http.StatusUnauthorized, "mTLS client certificate required")
			return
		}

		// [PIVOT] Verify certificate revocation status (Phase 6)
		if s.pki != nil {
			if err := s.pki.VerifyCertificate(r.TLS.PeerCertificates[0]); err != nil {
				s.logger.Warn("mTLS client certificate revoked or invalid", "path", r.URL.Path, "error", err)
				s.jsonError(w, http.StatusUnauthorized, "mTLS client certificate revoked or invalid")
				return
			}
		}

		// We prioritize session auth for operators.
		// [PIVOT] Prefer standard Authorization: Bearer <token> header (Plan §4.6)
		operatorSessionID := s.ExtractOperatorSessionID(r)

		cliSessionID := r.Header.Get(constants.HeaderCLISessionID)

		if operatorSessionID != "" {
			op, err := s.ValidateOperatorSession(operatorSessionID)
			if err == nil {
				// [PIVOT] Verify URI SAN identity (Phase 6)
				// The client cert must bind to the same operator session.
				if len(r.TLS.PeerCertificates) > 0 {
					cert := r.TLS.PeerCertificates[0]
					match := false
					for _, uri := range cert.URIs {
						// spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>
						if strings.Contains(uri.String(), "/"+op.ID+"/"+operatorSessionID) {
							match = true
							break
						}
					}
					if !match {
						s.logger.Warn("mTLS URI SAN mismatch for operator session", "path", r.URL.Path, "operator_id", op.ID, "operator_session_id", operatorSessionID)
						s.jsonError(w, http.StatusForbidden, "mTLS identity mismatch")
						return
					}
				}

				next.ServeHTTP(w, r)
				return
			}
			s.logger.Warn("Invalid operator session attempt", "operator_session_id", operatorSessionID[:8]+"...", "error", err)

			// If it's a structured AuthError, return it properly
			if ae, ok := err.(*AuthError); ok {
				s.jsonError(w, ae.Status, ae.Message) // Note: jsonError wraps it in {"error": ...}
				return
			}
		} else if cliSessionID != "" {
			// CLI authentication via CLI session ID and CLI certificate
			// Verify the CLI certificate matches the CLI session ID
			if len(r.TLS.PeerCertificates) > 0 {
				cert := r.TLS.PeerCertificates[0]
				match := false
				for _, uri := range cert.URIs {
					// spiffe://g8e.local/cli/<user_id>/<cli_session_id>
					if strings.Contains(uri.String(), "/cli/"+cliSessionID) {
						match = true
						break
					}
				}
				if !match {
					s.logger.Warn("mTLS URI SAN mismatch for CLI session", "path", r.URL.Path, "cli_session_id", cliSessionID)
					s.jsonError(w, http.StatusForbidden, "mTLS identity mismatch")
					return
				}
			}

			next.ServeHTTP(w, r)
			return
		} else {
			// [PIVOT] System/App Authentication via URI SAN (Phase 6)
			// If no session ID is provided, we check if the certificate belongs to a trusted system app.
			// Note: /_query requires operator session authentication - no app bypass allowed.
			if len(r.TLS.PeerCertificates) > 0 {
				cert := r.TLS.PeerCertificates[0]
				for _, uri := range cert.URIs {
					// Reference apps use spiffe://g8e.local/app/<app_id>
					if strings.Contains(uri.String(), "/app/"+constants.Status.ComponentName.G8EE) {
						next.ServeHTTP(w, r)
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

		s.jsonError(w, http.StatusUnauthorized, "protocol authentication required")
	})
}

// WebSocketAuth returns an http.Handler that authenticates WebSocket connections.
func (s *AuthService) WebSocketAuth(next http.Handler) http.Handler {
	// Re-use the main Middleware logic for WebSockets to ensure consistency.
	// Middleware already handles /ws/ prefix specifically and bootstrap bypass.
	return s.Middleware(next)
}

func (s *AuthService) jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set(constants.HeaderContentType, "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: msg})
}

// WebSessionAuth validates web session cookies and stamps context with user_id.
// This is for browser-based authentication on the public listener.
func (s *AuthService) WebSessionAuth(next http.Handler, db *ListenDBService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("g8e_session")
		if err != nil || cookie == nil {
			s.jsonError(w, http.StatusUnauthorized, "session cookie required")
			return
		}

		sessionID := cookie.Value
		if sessionID == "" {
			s.jsonError(w, http.StatusUnauthorized, "invalid session cookie")
			return
		}

		// Validate web session
		doc, err := db.DocGet(string(constants.CollectionWebSessions), sessionID)
		if err != nil {
			s.jsonError(w, http.StatusUnauthorized, "session validation failed")
			return
		}
		if doc == nil {
			s.jsonError(w, http.StatusUnauthorized, "session not found")
			return
		}

		// Check expiry
		var session models.WebSession
		data, err := json.Marshal(doc.Data)
		if err != nil {
			s.jsonError(w, http.StatusUnauthorized, "session parse failed")
			return
		}
		if err := json.Unmarshal(data, &session); err != nil {
			s.jsonError(w, http.StatusUnauthorized, "session parse failed")
			return
		}

		if time.Now().UnixMilli() > session.ExpiresAtUnixMs {
			s.jsonError(w, http.StatusUnauthorized, "session expired")
			return
		}

		// Check if the user is active (plan §4.6)
		if s.userSvc != nil {
			user, err := s.userSvc.GetByID(session.UserID)
			if err != nil {
				s.jsonError(w, http.StatusUnauthorized, "user validation failed")
				return
			}
			if user != nil && !user.IsActive() {
				s.jsonError(w, http.StatusUnauthorized, "identity disabled")
				return
			}
		}

		// Stamp context with user_id
		ctx := context.WithValue(r.Context(), "user_id", session.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
