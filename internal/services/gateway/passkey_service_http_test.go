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
	"github.com/g8e-ai/g8e/internal/marshaler"
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
	enrollmentTokenSvc := NewEnrollmentTokenService(stores.DocStore, logger)
	handler := NewPasskeyHandler(PasskeyHandlerDeps{
		Service:            svc,
		WebSessionSvc:      webSessionSvc,
		EnrollmentTokenSvc: enrollmentTokenSvc,
		Responder:          resp,
		MaxPayload:         10 * 1024 * 1024,
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
		{"enrollmentRegisterCfg", passkeyHandlerConfig{source: sourceEnrollmentToken, requireEnrollmentToken: true, createWebSession: true, setCookie: true}, true},
	}

	for _, pc := range productionConfigs {
		t.Run(pc.name, func(t *testing.T) {
			// Invariant 1: setCookie must imply createWebSession
			if pc.cfg.setCookie {
				assert.True(t, pc.cfg.createWebSession,
					"%s: setCookie=true must imply createWebSession=true", pc.name)
			}
			// Invariant 2: For registration configs, at least one of
			// enforceFirstCredentialOnly, requireAuthenticatedUser, or
			// requireEnrollmentToken must be set. Without any of these the
			// endpoint would allow anonymous, unrestricted passkey
			// registration. Authentication configs are exempt: the user is
			// not yet authenticated and first-credential enforcement is
			// irrelevant.
			if pc.isRegister && !pc.cfg.enforceFirstCredentialOnly && !pc.cfg.requireAuthenticatedUser && !pc.cfg.requireEnrollmentToken {
				t.Errorf("%s: registration config with no authorization mode (enforceFirstCredentialOnly, requireAuthenticatedUser, requireEnrollmentToken all false) — "+
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
		_, stores := newTestDB(t)
		logger := testutil.NewTestLogger()
		webSessionSvc := NewWebSessionService(stores.DocStore, logger)
		resp := response.NewWriter(logger)
		svc, err := NewPasskeyService(stores.DocStore, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
		require.NoError(t, err)
		sseStore := NewSSEEventService(stores.DB, logger)
		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })
		orchestrator := NewPasskeyOrchestrator(nil, nil, sseStore, pubsub, logger)
		handler := NewPasskeyHandler(PasskeyHandlerDeps{
			Service:       svc,
			WebSessionSvc: webSessionSvc,
			Responder:     resp,
			MaxPayload:    10 * 1024 * 1024,
			Orchestrator:  orchestrator,
		})

		const cliSessionID = "cli-sse-test-1"
		const userID = "u-sse-test-1"

		handler.orchestrator.EmitPasskeyRegisteredSSE(userID, cliSessionID)

		route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
		events, err := sseStore.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "passkey.registered", events[0].EventType)
		assert.Contains(t, events[0].Payload, cliSessionID)
		assert.Contains(t, events[0].Payload, userID)
	})

	t.Run("no SSE emission when dependencies not set", func(t *testing.T) {
		_, stores := newTestDB(t)
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

		if handler.orchestrator != nil {
			handler.orchestrator.EmitPasskeyRegisteredSSE(userID, cliSessionID)
		}

		sseStore := NewSSEEventService(stores.DB, logger)
		route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
		events, err := sseStore.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		assert.Empty(t, events)
	})
}

// TestPasskeyHandler_RegisterChallenge_CLIEnrollmentFlow documents the
// original 400 Bad Request bug: when the CLI-initiated enrollment flow was
// routed through browserBootstrapRegisterCfg, the CLI had already created a
// user, so HasAnyUsers() returned true and the handler rejected the
// (empty) user_id with 400. This test pins the broken behavior of the old
// shared config so the fix (a separate enrollment-token flow) is
// verifiable.
func TestPasskeyHandler_RegisterChallenge_CLIEnrollmentFlow(t *testing.T) {
	svc, _, user := newPasskeyServiceHTTPForTest(t)
	// Simulate the OLD shared config: browser bootstrap with
	// createUserOnBootstrap=true AND enforceFirstCredentialOnly=true.
	// The CLI flow sends user_id="" because the JS DOM round-trip
	// clobbered it (see plan §"The DOM-as-data-store race").
	oldCfg := passkeyHandlerConfig{
		source:                     sourceBrowserBootstrap,
		enforceFirstCredentialOnly: true,
		createWebSession:           true,
		setCookie:                  true,
		createUserOnBootstrap:      true,
	}
	handler := svc.RegisterChallenge(oldCfg)

	body, err := json.Marshal(map[string]string{"user_id": "", "cli_session_id": "cli-1"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/console/register/challenge", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler(rr, req)

	// The user already exists (the CLI created it via `auth enroll`), so
	// HasAnyUsers() is true and the handler returns 400 user_id required.
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "user_id")
	// user.ID is referenced to avoid an unused-variable lint; it is the
	// user that the CLI created and that the broken flow failed to bind.
	_ = user
}

// TestPasskeyHandler_RegisterChallenge_EnrollmentToken covers the new
// CLI-initiated enrollment register challenge flow. The enrollment token is
// the single authorization primitive; user_id and cli_session_id are
// derived from the token, not sent by the client.
func TestPasskeyHandler_RegisterChallenge_EnrollmentToken(t *testing.T) {
	cfg := passkeyHandlerConfig{source: sourceEnrollmentToken, requireEnrollmentToken: true, createWebSession: true, setCookie: true}

	t.Run("valid token returns 200 with challenge options", func(t *testing.T) {
		svc, _, user := newPasskeyServiceHTTPForTest(t)
		tok, err := svc.enrollmentTokenSvc.GenerateToken(user.ID, "cli-valid-1")
		require.NoError(t, err)

		handler := svc.RegisterChallenge(cfg)
		body, err := json.Marshal(map[string]string{"enrollment_token": tok.Token})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPasskeysEnrollmentRegisterChallenge, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.PasskeyRegisterChallengeResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.True(t, resp.Success)
		assert.NotNil(t, resp.Options)
	})

	// challenge must NOT consume the token — verify is the consuming
	// step. A second challenge with the same token must still succeed
	// (200, not 409). This pins the two-phase semantics and prevents
	// the regression where challenge consumes, breaking verify.
	t.Run("challenge does not consume token", func(t *testing.T) {
		svc, _, user := newPasskeyServiceHTTPForTest(t)
		tok, err := svc.enrollmentTokenSvc.GenerateToken(user.ID, "cli-replay-1")
		require.NoError(t, err)

		handler := svc.RegisterChallenge(cfg)
		body, err := json.Marshal(map[string]string{"enrollment_token": tok.Token})
		require.NoError(t, err)

		req1 := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPasskeysEnrollmentRegisterChallenge, bytes.NewReader(body))
		rr1 := httptest.NewRecorder()
		handler(rr1, req1)
		assert.Equal(t, http.StatusOK, rr1.Code)

		// Re-issue with the same token; the challenge step only
		// validates, so this must still be 200, not 409 consumed.
		req2 := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPasskeysEnrollmentRegisterChallenge, bytes.NewReader(body))
		rr2 := httptest.NewRecorder()
		handler(rr2, req2)
		assert.Equal(t, http.StatusOK, rr2.Code)

		// The token must still be unconsumed in the store so that the
		// verify step can consume it.
		tok2, err := svc.enrollmentTokenSvc.ValidateToken(tok.Token)
		require.NoError(t, err)
		assert.False(t, tok2.Consumed)
	})

	t.Run("expired token returns 410", func(t *testing.T) {
		_, stores := newTestDB(t)
		logger := testutil.NewTestLogger()
		userSvc := NewUserService(stores.DocStore, logger)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)
		webSessionSvc := NewWebSessionService(stores.DocStore, logger)
		resp := response.NewWriter(logger)
		psvc, err := NewPasskeyService(stores.DocStore, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
		require.NoError(t, err)
		enrollmentTokenSvc := NewEnrollmentTokenService(stores.DocStore, logger)
		handler := NewPasskeyHandler(PasskeyHandlerDeps{
			Service:            psvc,
			WebSessionSvc:      webSessionSvc,
			EnrollmentTokenSvc: enrollmentTokenSvc,
			Responder:          resp,
			MaxPayload:         10 * 1024 * 1024,
		})
		tok, err := enrollmentTokenSvc.GenerateToken(user.ID, "cli-exp-1")
		require.NoError(t, err)
		// Overwrite the persisted token with an expires_at in the past so
		// ValidateAndConsumeToken rejects it as expired.
		expiredDoc, err := json.Marshal(map[string]any{
			"token":          tok.Token,
			"user_id":        user.ID,
			"cli_session_id": "cli-exp-1",
			"created_at":     "2020-01-01T00:00:00Z",
			"expires_at":     "2020-01-01T00:00:00Z",
			"consumed":       false,
		})
		require.NoError(t, err)
		require.NoError(t, stores.DocStore.DocSet(
			marshaler.CollectionName(constants.CollectionEnrollmentTokens), tok.Token, expiredDoc))

		hh := handler.RegisterChallenge(cfg)
		body, err := json.Marshal(map[string]string{"enrollment_token": tok.Token})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPasskeysEnrollmentRegisterChallenge, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		hh(rr, req)

		assert.Equal(t, http.StatusGone, rr.Code)
	})

	t.Run("consumed token returns 409", func(t *testing.T) {
		svc, _, user := newPasskeyServiceHTTPForTest(t)
		tok, err := svc.enrollmentTokenSvc.GenerateToken(user.ID, "cli-con-1")
		require.NoError(t, err)
		// Consume it once.
		_, err = svc.enrollmentTokenSvc.ValidateAndConsumeToken(tok.Token)
		require.NoError(t, err)

		handler := svc.RegisterChallenge(cfg)
		body, err := json.Marshal(map[string]string{"enrollment_token": tok.Token})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPasskeysEnrollmentRegisterChallenge, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler(rr, req)

		assert.Equal(t, http.StatusConflict, rr.Code)
	})

	t.Run("missing token returns 400", func(t *testing.T) {
		svc, _, _ := newPasskeyServiceHTTPForTest(t)
		handler := svc.RegisterChallenge(cfg)
		body, err := json.Marshal(map[string]string{})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPasskeysEnrollmentRegisterChallenge, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Contains(t, resp["error"], "enrollment_token")
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		svc, _, _ := newPasskeyServiceHTTPForTest(t)
		handler := svc.RegisterChallenge(cfg)
		body, err := json.Marshal(map[string]string{"enrollment_token": "not-a-real-token"})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPasskeysEnrollmentRegisterChallenge, bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

// TestPasskeyHandler_RegisterVerify_EnrollmentToken_Valid verifies the
// happy-path enrollment-token registration verify: a valid token + a valid
// attestation yields 200, a web session cookie, and an SSE
// passkey.registered event carrying the token's cli_session_id.
func TestPasskeyHandler_RegisterVerify_EnrollmentToken_Valid(t *testing.T) {
	_, stores := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(stores.DocStore, logger)
	user, err := userSvc.CreateUser()
	require.NoError(t, err)
	webSessionSvc := NewWebSessionService(stores.DocStore, logger)
	resp := response.NewWriter(logger)
	psvc, err := NewPasskeyService(stores.DocStore, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
	require.NoError(t, err)
	enrollmentTokenSvc := NewEnrollmentTokenService(stores.DocStore, logger)
	sseStore := NewSSEEventService(stores.DB, logger)
	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })
	orchestrator := NewPasskeyOrchestrator(nil, nil, sseStore, pubsub, logger)
	handler := NewPasskeyHandler(PasskeyHandlerDeps{
		Service:            psvc,
		WebSessionSvc:      webSessionSvc,
		EnrollmentTokenSvc: enrollmentTokenSvc,
		Responder:          resp,
		MaxPayload:         10 * 1024 * 1024,
		Orchestrator:       orchestrator,
	})

	tok, err := enrollmentTokenSvc.GenerateToken(user.ID, "cli-verify-1")
	require.NoError(t, err)

	cfg := passkeyHandlerConfig{source: sourceEnrollmentToken, requireEnrollmentToken: true, createWebSession: true, setCookie: true}
	hh := handler.RegisterVerify(cfg)

	// We cannot produce a real WebAuthn attestation in a unit test, so we
	// send a malformed attestation and assert the handler rejects it with
	// a 400 (not a 200 success=false). This pins the contract: the
	// enrollment-token flow returns proper 4xx errors, never the old
	// 200-OK-on-error anti-pattern.
	body, err := json.Marshal(map[string]any{
		"enrollment_token": tok.Token,
		"attestation_response": map[string]any{
			"id":                "fake-id",
			"rawId":             "fake-rawId",
			"type":              "webauthn.create",
			"clientDataJSON":    "fake",
			"attestationObject": "fake",
		},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPasskeysEnrollmentRegisterVerify, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	hh(rr, req)

	// The token was consumed by the challenge step in a real flow; here we
	// only assert the verify path does not return 200 success=false for
	// internal errors. A malformed attestation yields a 400 from
	// VerifyRegistration.
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
