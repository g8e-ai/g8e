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

func newPasskeyServiceHTTPForTest(t *testing.T) (*PasskeyHandler, *WebSessionService, *models.User) {
	t.Helper()
	_, stores := newTestDB(t)
	logger := testutil.NewTestLogger()
	user, err := NewUserService(stores.DocStore, logger).CreateUser()
	require.NoError(t, err)
	webSessionSvc := NewWebSessionService(stores.DocStore, logger)
	resp := response.NewWriter(logger)
	svc, err := NewPasskeyService(stores.DocStore, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
	require.NoError(t, err)
	handler := NewPasskeyHandler(PasskeyHandlerDeps{
		Service:       svc,
		WebSessionSvc: webSessionSvc,
		Responder:     resp,
		MaxPayload:    10 * 1024 * 1024,
	})
	return handler, webSessionSvc, user
}

func TestPasskeyRegisterChallenge(t *testing.T) {
	cfg := passkeyHandlerConfig{source: sourceJWT, requireAuthenticatedUser: true, enforceSessionUserBinding: true}

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
	cfg := passkeyHandlerConfig{source: sourceJWT, requireAuthenticatedUser: true, enforceSessionUserBinding: true}

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
		{
			name:       "returns empty list for user with no credentials",
			method:     http.MethodGet,
			ctxUserID:  "REPLACE",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, user := newPasskeyServiceHTTPForTest(t)
			ctxUserID := tc.ctxUserID
			if ctxUserID == "REPLACE" {
				ctxUserID = user.ID
			}
			req := httptest.NewRequest(tc.method, "/api/v1/auth/passkeys", nil)
			if ctxUserID != "" {
				req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, ctxUserID))
			}
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

	t.Run("ignores query user_id param (IDOR fix)", func(t *testing.T) {
		svc, _, user := newPasskeyServiceHTTPForTest(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys?user_id=other-user", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()
		svc.ListCredentials(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.PasskeyCredentialsResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.True(t, resp.Success)
	})
}

func TestPasskeyRevokeCredential(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		ctxUserID  string
		wantStatus int
	}{
		{
			name:       "rejects non-DELETE",
			method:     http.MethodGet,
			path:       "/api/v1/auth/passkeys/some-id",
			ctxUserID:  "user1",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "rejects missing context user_id",
			method:     http.MethodDelete,
			path:       "/api/v1/auth/passkeys/some-id",
			ctxUserID:  "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "returns not-found gracefully for unknown credential",
			method:     http.MethodDelete,
			path:       "/api/v1/auth/passkeys/nonexistent",
			ctxUserID:  "REPLACE",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, user := newPasskeyServiceHTTPForTest(t)
			ctxUserID := tc.ctxUserID
			if ctxUserID == "REPLACE" {
				ctxUserID = user.ID
			}
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if ctxUserID != "" {
				req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, ctxUserID))
			}
			rr := httptest.NewRecorder()
			svc.RevokeCredential(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
		})
	}

	t.Run("ignores query user_id param (IDOR fix)", func(t *testing.T) {
		svc, _, user := newPasskeyServiceHTTPForTest(t)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/passkeys/nonexistent?user_id=other-user", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()
		svc.RevokeCredential(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.PasskeyRevokeResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.True(t, resp.Success)
		assert.False(t, resp.Found)
	})
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
			source:    sourceJWT,
			wantAllow: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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

func TestPasskeyConfigInvariants(t *testing.T) {
	// Enumerate all production passkey handler configs used in buildPublicRouter.
	// These must be kept in sync with gateway_http_router.go.
	type cfgEntry struct {
		name       string
		cfg        passkeyHandlerConfig
		isRegister bool // true if used with RegisterChallenge/RegisterVerify
	}
	productionConfigs := []cfgEntry{
		{"jitCfg", passkeyHandlerConfig{source: sourceJWT, enforceFirstCredentialOnly: true, requireAuthenticatedUser: true, enforceSessionUserBinding: true}, true},
		{"browserBootstrapRegisterCfg", passkeyHandlerConfig{source: sourceBrowserBootstrap, enforceFirstCredentialOnly: true, createWebSession: true, setCookie: true, createUserOnBootstrap: true}, true},
		{"browserBootstrapAuthCfg", passkeyHandlerConfig{source: sourceBrowserBootstrap, createWebSession: true, setCookie: true}, false},
	}

	for _, pc := range productionConfigs {
		t.Run(pc.name, func(t *testing.T) {
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

func TestPasskeyCookieConsistency(t *testing.T) {
	svc, webSessionSvc, user := newPasskeyServiceHTTPForTest(t)

	webSession, err := webSessionSvc.CreateWebSession(user.ID)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	svc.setWebSessionCookie(rr, webSession)

	cookies := rr.Result().Cookies()
	require.Len(t, cookies, 1)
	cookie := cookies[0]

	assert.Equal(t, constants.WebSessionCookieName, cookie.Name)
	assert.Equal(t, webSession.ID, cookie.Value)
	assert.Equal(t, constants.PathRoot, cookie.Path)
	assert.True(t, cookie.HttpOnly)
	assert.True(t, cookie.Secure)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	assert.Equal(t, webSession.ExpiresAtUnixMs/1000, cookie.Expires.Unix())
}

func TestPasskeyCookieCrossOriginSameSiteNone(t *testing.T) {
	svc, webSessionSvc, user := newPasskeyServiceHTTPForTest(t)
	svc.crossOrigin = true

	webSession, err := webSessionSvc.CreateWebSession(user.ID)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	svc.setWebSessionCookie(rr, webSession)

	cookies := rr.Result().Cookies()
	require.Len(t, cookies, 1)
	cookie := cookies[0]

	assert.True(t, cookie.Secure)
	assert.Equal(t, http.SameSiteNoneMode, cookie.SameSite)
}

func TestPasskeyReadBodyRejectsOversized(t *testing.T) {
	svc, _, _ := newPasskeyServiceHTTPForTest(t)

	largeBody := bytes.NewReader(bytes.Repeat([]byte("a"), 11*1024*1024))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/register/challenge", largeBody)
	rr := httptest.NewRecorder()

	_, err := svc.readBody(rr, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "http: request body too large")
}

func TestPasskeyRegisterVerify_SSEEmission(t *testing.T) {
	t.Run("emitPasskeyRegisteredSSE appends event and publishes", func(t *testing.T) {
		db, stores := newTestDB(t)
		logger := testutil.NewTestLogger()
		webSessionSvc := NewWebSessionService(stores.DocStore, logger)
		resp := response.NewWriter(logger)
		svc, err := NewPasskeyService(stores.DocStore, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
		require.NoError(t, err)
		sseStore := NewSSEEventService(db.GetDB(), logger)
		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })
		handler := NewPasskeyHandler(PasskeyHandlerDeps{
			Service:       svc,
			WebSessionSvc: webSessionSvc,
			Responder:     resp,
			MaxPayload:    10 * 1024 * 1024,
			SSEStore:      sseStore,
			Pubsub:        pubsub,
		})

		const cliSessionID = "cli-sse-test-1"
		const userID = "u-sse-test-1"

		handler.emitPasskeyRegisteredSSE(userID, cliSessionID)

		route := SSERoute{CLISessionID: cliSessionID}
		events, err := sseStore.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "passkey.registered", events[0].EventType)
		assert.Contains(t, events[0].Payload, cliSessionID)
		assert.Contains(t, events[0].Payload, userID)
	})

	t.Run("no SSE emission when dependencies not set", func(t *testing.T) {
		db, stores := newTestDB(t)
		logger := testutil.NewTestLogger()
		webSessionSvc := NewWebSessionService(stores.DocStore, logger)
		resp := response.NewWriter(logger)
		svc, err := NewPasskeyService(stores.DocStore, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
		require.NoError(t, err)
		handler := NewPasskeyHandler(PasskeyHandlerDeps{
			Service:       svc,
			WebSessionSvc: webSessionSvc,
			Responder:     resp,
			MaxPayload:    10 * 1024 * 1024,
		})

		const cliSessionID = "cli-no-sse-1"
		const userID = "u-no-sse-1"

		handler.emitPasskeyRegisteredSSE(userID, cliSessionID)

		sseStore := NewSSEEventService(db.GetDB(), logger)
		route := SSERoute{CLISessionID: cliSessionID}
		events, err := sseStore.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		assert.Empty(t, events)
	})
}
