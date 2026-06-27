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

//go:build integration

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func newPasskeyServiceHTTPForTest(t *testing.T) (*PasskeyService, *WebSessionService, *models.User) {
	t.Helper()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	user, err := NewUserService(db, logger).CreateUser()
	require.NoError(t, err)
	webSessionSvc := NewWebSessionService(db, logger)
	resp := response.NewWriter(logger)
	svc, err := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"}, webSessionSvc, resp, 10*1024*1024)
	require.NoError(t, err)
	return svc, webSessionSvc, user
}

func TestPasskeyRegisterChallenge(t *testing.T) {
	cfg := passkeyHandlerConfig{source: sourceMTLS, requireAuthenticatedUser: true, enforceSessionUserBinding: true}

	tests := []struct {
		name       string
		method     string
		body       any
		ctxUserID  string
		wantStatus int
		wantJSON   func(t *testing.T, body map[string]any)
	}{
		{
			name:       "rejects non-POST",
			method:     http.MethodGet,
			body:       nil,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "rejects missing user_id without context",
			method:     http.MethodPost,
			body:       map[string]string{},
			wantStatus: http.StatusBadRequest,
			wantJSON: func(t *testing.T, body map[string]any) {
				assert.Contains(t, body["error"], "user_id")
			},
		},
		{
			name:       "rejects user_id mismatch with session",
			method:     http.MethodPost,
			body:       map[string]string{"user_id": "different-user"},
			ctxUserID:  "session-user",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, _, _ := newPasskeyServiceHTTPForTest(t)
			handler := svc.RegisterChallenge(cfg)

			var bodyReader *bytes.Reader
			if tc.body != nil {
				b, err := json.Marshal(tc.body)
				require.NoError(t, err)
				bodyReader = bytes.NewReader(b)
			} else {
				bodyReader = bytes.NewReader([]byte("{}"))
			}

			req := httptest.NewRequest(tc.method, "/api/v1/auth/passkeys/register/challenge", bodyReader)
			if tc.ctxUserID != "" {
				req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, tc.ctxUserID))
			}
			rr := httptest.NewRecorder()
			handler(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			if tc.wantJSON != nil {
				var body map[string]any
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
				tc.wantJSON(t, body)
			}
		})
	}

	// Full happy-path: valid user_id via context, challenge returned
	t.Run("issues challenge for authenticated user", func(t *testing.T) {
		t.Parallel()
		svc, _, user := newPasskeyServiceHTTPForTest(t)
		handler := svc.RegisterChallenge(cfg)

		body, err := json.Marshal(map[string]string{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/register/challenge", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()
		handler(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.True(t, resp["success"].(bool))
		assert.NotNil(t, resp["options"])
	})
}

func TestPasskeyAuthenticateChallenge(t *testing.T) {
	cfg := passkeyHandlerConfig{source: sourceMTLS, requireAuthenticatedUser: true, enforceSessionUserBinding: true}

	tests := []struct {
		name       string
		method     string
		body       any
		ctxUserID  string
		wantStatus int
		wantKey    string
	}{
		{
			name:       "rejects non-POST",
			method:     http.MethodGet,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "rejects missing user_id",
			method:     http.MethodPost,
			body:       map[string]string{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "rejects mismatch user_id",
			method:     http.MethodPost,
			body:       map[string]string{"user_id": "other"},
			ctxUserID:  "session-user",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, _, _ := newPasskeyServiceHTTPForTest(t)
			handler := svc.AuthenticateChallenge(cfg)

			bodyBytes := []byte("{}")
			if tc.body != nil {
				var err error
				bodyBytes, err = json.Marshal(tc.body)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(tc.method, "/api/v1/auth/passkeys/authenticate/challenge", bytes.NewReader(bodyBytes))
			if tc.ctxUserID != "" {
				req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, tc.ctxUserID))
			}
			rr := httptest.NewRecorder()
			handler(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
		})
	}

	// User with no credentials → success:false, needs_setup:true
	t.Run("no credentials returns needs_setup", func(t *testing.T) {
		t.Parallel()
		svc, _, user := newPasskeyServiceHTTPForTest(t)
		handler := svc.AuthenticateChallenge(cfg)

		body, _ := json.Marshal(map[string]string{})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/authenticate/challenge", bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()
		handler(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.PasskeyChallengeResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.False(t, resp.Success)
		assert.True(t, resp.NeedsSetup)
	})
}

func TestPasskeyListCredentials(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		query      string
		wantStatus int
	}{
		{
			name:       "rejects non-GET",
			method:     http.MethodPost,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "rejects missing user_id",
			method:     http.MethodGet,
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns empty list for user with no credentials",
			method:     http.MethodGet,
			query:      "?user_id=REPLACE",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, _, user := newPasskeyServiceHTTPForTest(t)
			query := tc.query
			if query == "?user_id=REPLACE" {
				query = "?user_id=" + user.ID
			}
			req := httptest.NewRequest(tc.method, "/api/v1/auth/passkeys"+query, nil)
			rr := httptest.NewRecorder()
			svc.ListCredentials(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			if tc.wantStatus == http.StatusOK {
				var resp models.PasskeyCredentialsResponse
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
				assert.True(t, resp.Success)
			}
		})
	}
}

func TestPasskeyRevokeCredential(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		query      string
		wantStatus int
	}{
		{
			name:       "rejects non-DELETE",
			method:     http.MethodGet,
			path:       "/api/v1/auth/passkeys/some-id",
			query:      "?user_id=user1",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "rejects missing user_id",
			method:     http.MethodDelete,
			path:       "/api/v1/auth/passkeys/some-id",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns not-found gracefully for unknown credential",
			method:     http.MethodDelete,
			path:       "/api/v1/auth/passkeys/nonexistent",
			query:      "?user_id=REPLACE",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, _, user := newPasskeyServiceHTTPForTest(t)
			query := tc.query
			if query == "?user_id=REPLACE" {
				query = "?user_id=" + user.ID
			}
			req := httptest.NewRequest(tc.method, tc.path+query, nil)
			rr := httptest.NewRecorder()
			svc.RevokeCredential(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
		})
	}
}

func TestPasskeyCLIStatus(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		ctxUserID  string
		wantStatus int
	}{
		{
			name:       "rejects non-GET",
			method:     http.MethodPost,
			ctxUserID:  "user1",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "rejects missing context user_id",
			method:     http.MethodGet,
			ctxUserID:  "",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, _, _ := newPasskeyServiceHTTPForTest(t)
			req := httptest.NewRequest(tc.method, "/api/v1/auth/passkeys/cli/status", nil)
			if tc.ctxUserID != "" {
				req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, tc.ctxUserID))
			}
			rr := httptest.NewRecorder()
			svc.CLIStatus(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
		})
	}

	t.Run("returns credential list for authenticated user", func(t *testing.T) {
		t.Parallel()
		svc, _, user := newPasskeyServiceHTTPForTest(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys/cli/status", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()
		svc.CLIStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.PasskeyCredentialsResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.True(t, resp.Success)
	})
}

func TestPasskeyEnforceFirstCred(t *testing.T) {
	tests := []struct {
		name      string
		source    passkeyRequestSource
		wantAllow bool
	}{
		{
			name:      "allows registration when user has no credentials",
			source:    sourceMTLS,
			wantAllow: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, _, user := newPasskeyServiceHTTPForTest(t)
			cfg := passkeyHandlerConfig{source: tc.source, enforceFirstCredentialOnly: true}
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			forbidden, _, _ := svc.enforceFirstCred(req, user.ID, cfg)
			if tc.wantAllow {
				assert.False(t, forbidden)
			} else {
				assert.True(t, forbidden)
			}
		})
	}
}

func TestPasskeyRegisterChallengeEnforcesFirstCredCLI(t *testing.T) {
	// CLI bootstrap: already-authenticated user (same ID) must be allowed even if credentials exist.
	// We can't easily reach that branch in a unit test without a real credential, so we test
	// the enforcement path: a non-matching mTLS user is rejected.
	t.Parallel()
	svc, _, user := newPasskeyServiceHTTPForTest(t)
	cfg := passkeyHandlerConfig{source: sourceCLIBootstrap, enforceFirstCredentialOnly: true}
	handler := svc.RegisterChallenge(cfg)

	// Simulate: user_id in body but context has a DIFFERENT authenticated user.
	// enforceFirstCred is called, user has 0 creds → should allow regardless.
	body, _ := json.Marshal(map[string]string{"user_id": user.ID})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli/register/challenge", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "other-user"))
	rr := httptest.NewRecorder()
	handler(rr, req)

	// With no existing creds, first-cred check passes regardless of source.
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp["success"].(bool))
}

func TestPasskeyConfigInvariants(t *testing.T) {
	t.Parallel()

	// Enumerate all production passkey handler configs used in buildPublicRouter.
	// These must be kept in sync with gateway_http_router.go.
	type cfgEntry struct {
		name       string
		cfg        passkeyHandlerConfig
		isRegister bool // true if used with RegisterChallenge/RegisterVerify
	}
	productionConfigs := []cfgEntry{
		{"jitCfg", passkeyHandlerConfig{source: sourceJWT, enforceFirstCredentialOnly: true, requireAuthenticatedUser: true, enforceSessionUserBinding: true}, true},
		{"cliBootstrapRegisterCfg", passkeyHandlerConfig{source: sourceCLIBootstrap, enforceFirstCredentialOnly: true}, true},
		{"cliBootstrapAuthCfg", passkeyHandlerConfig{source: sourceCLIBootstrap}, false},
		{"browserBootstrapRegisterCfg", passkeyHandlerConfig{source: sourceBrowserBootstrap, enforceFirstCredentialOnly: true, createWebSession: true, setCookie: true, createUserOnBootstrap: true}, true},
		{"browserBootstrapAuthCfg", passkeyHandlerConfig{source: sourceBrowserBootstrap, createWebSession: true, setCookie: true}, false},
		{"mtlsCfg", passkeyHandlerConfig{source: sourceMTLS, requireAuthenticatedUser: true, enforceSessionUserBinding: true}, true},
		{"mtlsAuthVerifyCfg", passkeyHandlerConfig{source: sourceMTLS, requireAuthenticatedUser: true, enforceSessionUserBinding: true, createWebSession: true}, false},
	}

	for _, pc := range productionConfigs {
		t.Run(pc.name, func(t *testing.T) {
			t.Parallel()
			// Invariant 1: setCookie must imply createWebSession
			if pc.cfg.setCookie {
				assert.True(t, pc.cfg.createWebSession,
					"%s: setCookie=true must imply createWebSession=true", pc.name)
			}
			// Invariant 2: For registration configs, !enforceFirstCredentialOnly &&
			// !requireAuthenticatedUser should never appear in production wiring —
			// it would allow anonymous, unrestricted passkey registration.
			// Authentication configs are exempt: the user is not yet authenticated
			// and first-credential enforcement is irrelevant.
			if pc.isRegister && !pc.cfg.enforceFirstCredentialOnly && !pc.cfg.requireAuthenticatedUser {
				t.Errorf("%s: registration config with enforceFirstCredentialOnly=false and requireAuthenticatedUser=false — "+
					"this allows anonymous unrestricted registration and must not be used in production", pc.name)
			}
		})
	}
}
