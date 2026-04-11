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
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

// AuthService handles internal authentication for the Listen service.
type AuthService struct {
	db            *ListenDBService
	logger        *slog.Logger
	tokenProvider func() string
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *ListenDBService, logger *slog.Logger) *AuthService {
	s := &AuthService{
		db:     db,
		logger: logger,
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

	// 2. Try SSL volume file (authoritative source for bootstrap secrets)
	if tokenBytes, err := os.ReadFile("/ssl/internal_auth_token"); err == nil {
		if token := strings.TrimSpace(string(tokenBytes)); token != "" {
			return token
		}
	}

	// 3. Try to load from DB (fallback)
	doc, err := s.db.DocGet("settings", "platform_settings")
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

// Middleware returns an http.Handler that authenticates requests.
func (s *AuthService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always allow health check without a token
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get(constants.HeaderInternalAuth)
		if token == "" && strings.HasPrefix(r.URL.Path, "/ws/") {
			token = r.URL.Query().Get("token")
		}

		expected := s.GetInternalAuthToken()

		// During bootstrap (no token yet), we MUST allow certain paths to proceed
		// so the token can be seeded and services can coordinate.
		if expected == "" {
			// Allow access to platform_settings so it can be discovered/seeded.
			if (r.Method == http.MethodGet || r.Method == http.MethodPut) && r.URL.Path == "/db/settings/platform_settings" {
				next.ServeHTTP(w, r)
				return
			}
			// Allow KV operations during bootstrap for caching and coordination
			if strings.HasPrefix(r.URL.Path, "/kv/") {
				next.ServeHTTP(w, r)
				return
			}
			// Allow VSOD to connect via WebSocket during bootstrap
			if strings.HasPrefix(r.URL.Path, "/ws/") {
				next.ServeHTTP(w, r)
				return
			}
			// Allow CA certificate fetching for bootstrap (Operators need this to establish TLS)
			if r.Method == http.MethodGet && r.URL.Path == "/ssl/ca.crt" {
				next.ServeHTTP(w, r)
				return
			}

			s.logger.Warn("Unauthorized internal API access attempt (token not initialized)", "path", r.URL.Path, "method", r.Method)
			s.jsonError(w, http.StatusUnauthorized, "internal auth token not initialized")
			return
		}

		if token != expected {
			s.logger.Warn("Unauthorized internal API access attempt",
				"path", r.URL.Path,
				"method", r.Method,
				"ip", r.RemoteAddr,
				"token_len", len(token),
				"expected_len", len(expected))

			// For WebSockets, return a plain text error for 401.
			// Handshake fails if a JSON body is returned instead of just the 401 status.
			if strings.HasPrefix(r.URL.Path, "/ws/") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			s.jsonError(w, http.StatusUnauthorized, "invalid internal auth token")
			return
		}

		next.ServeHTTP(w, r)
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
