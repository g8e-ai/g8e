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
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
)

// AuthService handles internal authentication for the Listen service.
type AuthService struct {
	db            *ListenDBService
	logger        *slog.Logger
	secretsDir    string
	tokenProvider func() string
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *ListenDBService, logger *slog.Logger, secretsDir string) *AuthService {
	s := &AuthService{
		db:         db,
		logger:     logger,
		secretsDir: secretsDir,
	}
	s.tokenProvider = s.defaultTokenProvider
	return s
}

// defaultTokenProvider is the default implementation that checks env, SSL volume, and DB.
func (s *AuthService) defaultTokenProvider() string {
	// 1. Try environment variable first (deployer override)
	if token := os.Getenv(string(constants.EnvVar.InternalAuthToken)); token != "" {
		return token
	}

	// 2. Try secrets volume file (authoritative source for bootstrap secrets)
	tokenPath := filepath.Join(s.secretsDir, "internal_auth_token")
	if tokenBytes, err := os.ReadFile(tokenPath); err == nil {
		if token := strings.TrimSpace(string(tokenBytes)); token != "" {
			return token
		}
	}

	// 3. Try to load from DB (fallback)
	doc, err := s.db.DocGet(string(constants.CollectionSettings), string(constants.DocIDPlatformSettings))
	if err != nil || doc == nil {
		return ""
	}

	if tokenVal, ok := doc.Data["internal_auth_token"]; ok {
		var val string
		if err := json.Unmarshal(tokenVal, &val); err == nil {
			return val
		}
	}

	return ""
}

// GetInternalAuthToken retrieves the current internal auth token via the configured provider.
func (s *AuthService) GetInternalAuthToken() string {
	return s.tokenProvider()
}

// ValidateOperatorSession checks if a session ID is valid and returns the operator document.
func (s *AuthService) ValidateOperatorSession(sessionID string) (*models.OperatorDocumentGo, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("missing session id")
	}

	filters := []models.DocFilter{
		{Field: "operator_session_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", sessionID))},
		{Field: "status", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.Status.OperatorStatus.Active))},
	}

	docs, err := s.db.DocQuery(string(constants.CollectionOperators), filters, "", 1)
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return nil, fmt.Errorf("invalid or expired operator session")
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

	return &op, nil
}

// ValidateAPIKey checks if an API key is valid and returns the operator document.
func (s *AuthService) ValidateAPIKey(apiKey string) (*models.OperatorDocumentGo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	filters := []models.DocFilter{
		{Field: "operator_api_key", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", apiKey))},
	}

	docs, err := s.db.DocQuery(string(constants.CollectionOperators), filters, "", 1)
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return nil, fmt.Errorf("invalid api key")
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

	return &op, nil
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
		if r.URL.Path == "/api/pki/sign-csr" || r.URL.Path == "/api/auth/device-link/register" {
			next.ServeHTTP(w, r)
			return
		}

		// We prioritize session/key auth.
		sessionID := r.Header.Get(constants.HeaderOperatorSessionID)
		apiKey := r.Header.Get(constants.HeaderOperatorAPIKey)

		if sessionID != "" {
			if _, err := s.ValidateOperatorSession(sessionID); err == nil {
				next.ServeHTTP(w, r)
				return
			}
			s.logger.Warn("Invalid operator session attempt", "session_id", sessionID[:8]+"...")
		}

		if apiKey != "" {
			if _, err := s.ValidateAPIKey(apiKey); err == nil {
				next.ServeHTTP(w, r)
				return
			}
			s.logger.Warn("Invalid API key attempt", "api_key", apiKey[:8]+"...")
		}

		// [PIVOT] Native Registration Path (Phase 4)
		// This endpoint is the new sovereign entry point for enrolling binaries.
		// It MUST be accessible without an internal token as it is the first step
		// of the trust bootstrap.
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/auth/link/") && strings.HasSuffix(r.URL.Path, "/register") {
			next.ServeHTTP(w, r)
			return
		}

		s.logger.Warn("Unauthorized internal API access attempt (protocol auth required)", "path", r.URL.Path, "method", r.Method)

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
