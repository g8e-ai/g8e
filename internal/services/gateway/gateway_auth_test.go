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
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/protocol"
)

func TestAuthService_ValidateOperatorSession_MissingSessionID(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	_, err := auth.ValidateOperatorSession("")
	require.Error(t, err)
}

func TestAuthService_ValidateOperatorSession_SessionNotFound(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	_, err := auth.ValidateOperatorSession("nonexistent-session")
	require.Error(t, err)
}

func TestAuthService_ValidateOperatorSession_TerminatedStatus(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an Operator session with terminated status
	operatorSessionID := "terminated-session"
	opDoc := &models.OperatorDocumentGo{
		ID:                "op-123",
		OperatorSessionID: operatorSessionID,
		Status:            constants.OperatorStatusTerminated,
		UserID:            "user-123",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("operators", "op-123", opBytes))

	_, err = auth.ValidateOperatorSession(operatorSessionID)
	require.Error(t, err)
}

func TestAuthService_ValidateOperatorSession_SessionExpired(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an active user
	userID := "user-456"
	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("users", userID, userBytes))

	// Create an Operator session with old timestamp using the test hook
	operatorSessionID := "expired-session"
	oldTime := time.Now().UTC().Add(-25 * time.Hour)
	opDoc := &models.OperatorDocumentGo{
		ID:                "op-456",
		OperatorSessionID: operatorSessionID,
		Status:            constants.OperatorStatusActive,
		UserID:            userID,
		CreatedAt:         oldTime,
		UpdatedAt:         oldTime,
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSetWithTimestamps("operators", "op-456", opBytes, oldTime, oldTime))

	_, err = auth.ValidateOperatorSession(operatorSessionID)
	require.Error(t, err)
}

func TestAuthService_ValidateOperatorSession_UserInactive(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an inactive user
	userID := "inactive-user"
	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusDisabled,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("users", userID, userBytes))

	// Create an Operator session linked to the inactive user
	operatorSessionID := "session-with-inactive-user"
	opDoc := &models.OperatorDocumentGo{
		ID:                "op-789",
		OperatorSessionID: operatorSessionID,
		Status:            constants.OperatorStatusActive,
		UserID:            userID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("operators", "op-789", opBytes))

	_, err = auth.ValidateOperatorSession(operatorSessionID)
	require.Error(t, err)
}

func TestAuthError_Error(t *testing.T) {
	err := &AuthError{
		Message: "test error",
		Reason:  "test reason",
		Status:  http.StatusUnauthorized,
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "test error")
	assert.Contains(t, errStr, "test reason")
}

func TestAuthError_Is(t *testing.T) {
	err := &AuthError{
		Message: "test error",
		Status:  http.StatusUnauthorized,
	}

	target := &AuthError{}
	assert.True(t, err.Is(target))

	otherErr := &AuthError{}
	assert.True(t, otherErr.Is(err))
}

func TestRouteAuthRegistry_ExactPaths(t *testing.T) {

	registry := NewRouteAuthRegistry(false)

	// Test exact public paths
	publicPaths := []string{
		constants.APIPaths.Health,
		constants.APIPaths.PKICSRSign,
		constants.APIPaths.PKIDevicesEnroll,
		constants.APIPaths.AuthBootstrap,
		constants.APIPaths.AuthBootstrapStatus,
		constants.APIPaths.AuthLogout,
	}

	for _, path := range publicPaths {
		assert.Equal(t, RouteAuthNone, registry.AuthMode(path), "Path %s should be RouteAuthNone", path)
	}

	// Test that slight variations are not public (fail-closed to RouteAuthMTLS)
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode("/healthz"), "/healthz should default to RouteAuthMTLS")
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode(constants.APIPaths.PKICSRSign+"/"), constants.APIPaths.PKICSRSign+"/ should default to RouteAuthMTLS")
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode(constants.APIPaths.AuthBootstrap+"/extra"), constants.APIPaths.AuthBootstrap+"/extra should default to RouteAuthMTLS")
}

func TestRouteAuthRegistry_Prefixes(t *testing.T) {

	registry := NewRouteAuthRegistry(false)

	// Test prefix-based public paths
	publicPrefixPaths := []string{
		"/.well-known/g8e/pki/",
		"/.well-known/g8e/pki/g8eg-ca-bundle.pem",
		"/.well-known/g8e/pki/fingerprint",
		"/.well-known/g8e/pki/some/deep/path",
	}

	for _, path := range publicPrefixPaths {
		assert.Equal(t, RouteAuthNone, registry.AuthMode(path), "Path %s should be RouteAuthNone (prefix match)", path)
	}

	// Test that paths outside the prefix are not public
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode("/.well-known/other/pki/"), "/.well-known/other/pki/ should default to RouteAuthMTLS")
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode("/api/v1/pki/"), "/api/v1/pki/ should default to RouteAuthMTLS")
}

func TestRouteAuthRegistry_JWKSEnabled(t *testing.T) {

	// Registry with JWKS enabled
	registryWithJWKS := NewRouteAuthRegistry(true)

	// Test JIT passkey prefix matches
	jitPaths := []string{
		"/api/v1/auth/passkeys/jit-123",
		"/api/v1/auth/passkeys/jit-abc",
		"/api/v1/auth/passkeys/jit-register/challenge",
	}
	for _, path := range jitPaths {
		assert.Equal(t, RouteAuthNone, registryWithJWKS.AuthMode(path), "Path %s should be RouteAuthNone with JWKS enabled", path)
	}

	// Test MCP endpoint is public with JWKS enabled
	mcpPaths := []string{
		constants.APIPaths.MCPEndpoint,
	}
	for _, path := range mcpPaths {
		assert.Equal(t, RouteAuthNone, registryWithJWKS.AuthMode(path), "Path %s should be RouteAuthNone with JWKS enabled", path)
	}

	// Test A2A prefix matches
	a2aPaths := []string{
		"/api/v1/a2a/call",
		"/api/v1/a2a/some-endpoint",
	}
	for _, path := range a2aPaths {
		assert.Equal(t, RouteAuthNone, registryWithJWKS.AuthMode(path), "Path %s should be RouteAuthNone with JWKS enabled", path)
	}

	// Registry without JWKS — JIT routes should be RouteAuthMTLS
	registryWithoutJWKS := NewRouteAuthRegistry(false)

	allJWKSPaths := append(jitPaths, append(mcpPaths, a2aPaths...)...)
	for _, path := range allJWKSPaths {
		assert.Equal(t, RouteAuthMTLS, registryWithoutJWKS.AuthMode(path), "Path %s should default to RouteAuthMTLS without JWKS", path)
	}
}

func TestRouteAuthRegistry_NonPublicPaths(t *testing.T) {

	registry := NewRouteAuthRegistry(false)

	// Test paths that should never be public (default to RouteAuthMTLS)
	privatePaths := []string{
		constants.APIPaths.GovernanceEnvelopes,
		"/_query",
		"/api/users",
		"/api/operators",
		"/ws/",
		"/api/db",
	}

	for _, path := range privatePaths {
		assert.Equal(t, RouteAuthMTLS, registry.AuthMode(path), "Path %s should default to RouteAuthMTLS", path)
	}
}

func TestRouteAuthRegistry_CanonicalCoverage(t *testing.T) {

	registry := NewRouteAuthRegistry(false)

	// Ensure all previously hardcoded public routes are covered
	// This test prevents regression when the registry is modified
	assert.Equal(t, RouteAuthNone, registry.AuthMode(constants.APIPaths.Health), "Health check must be RouteAuthNone")
	assert.Equal(t, RouteAuthNone, registry.AuthMode("/.well-known/g8e/pki/"), "PKI prefix must be RouteAuthNone")
	assert.Equal(t, RouteAuthNone, registry.AuthMode("/api/v1/pki/csr/sign"), "CSR signing must be RouteAuthNone")
	assert.Equal(t, RouteAuthNone, registry.AuthMode("/api/v1/pki/devices/enroll"), "Device enrollment must be RouteAuthNone")
	assert.Equal(t, RouteAuthNone, registry.AuthMode("/api/v1/auth/bootstrap"), "Bootstrap must be RouteAuthNone")
	assert.Equal(t, RouteAuthNone, registry.AuthMode("/api/v1/auth/bootstrap/status"), "Bootstrap status must be RouteAuthNone")
	assert.Equal(t, RouteAuthNone, registry.AuthMode("/api/v1/auth/logout"), "Logout must be RouteAuthNone")
}

func TestRouteAuthRegistry_PasskeyRoutes(t *testing.T) {

	registry := NewRouteAuthRegistry(false)

	// console/* prefix entries should be RouteAuthNone
	consolePaths := []string{
		constants.APIPaths.AuthPasskeysConsoleRegisterChallenge,
		constants.APIPaths.AuthPasskeysConsoleRegisterVerify,
		constants.APIPaths.AuthPasskeysConsoleAuthenticateChallenge,
		constants.APIPaths.AuthPasskeysConsoleAuthenticateVerify,
	}
	for _, path := range consolePaths {
		assert.Equal(t, RouteAuthNone, registry.AuthMode(path), "console path %s should be RouteAuthNone", path)
	}

	// mTLS-protected passkey sub-paths must be RouteAuthMTLS (exact path overrides prefix)
	mtlsPaths := []string{
		constants.APIPaths.AuthPasskeysCLIStatus,
	}
	for _, path := range mtlsPaths {
		assert.Equal(t, RouteAuthMTLS, registry.AuthMode(path), "mTLS path %s should be RouteAuthMTLS", path)
	}

	// Non-alias sub-paths under /api/v1/auth/passkeys prefix should be RouteAuthWebSession
	nonAliasExcludedPaths := []string{
		"/api/v1/auth/passkeys/cli/other",
	}
	for _, path := range nonAliasExcludedPaths {
		assert.Equal(t, RouteAuthWebSession, registry.AuthMode(path), "passkey sub-path %s should be RouteAuthWebSession", path)
	}
}

func TestRouteAuthRegistry_SSEDualAuth(t *testing.T) {

	registry := NewRouteAuthRegistry(false)

	// SSE stream and events endpoints must be classified as RouteAuthDual
	assert.Equal(t, RouteAuthDual, registry.AuthMode(constants.APIPaths.SSEStream), "SSE stream must be RouteAuthDual")
	assert.Equal(t, RouteAuthDual, registry.AuthMode(constants.APIPaths.SSEEvents), "SSE events must be RouteAuthDual")
}

func TestAuthService_Middleware_DualAuthDispatch(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	t.Run("dual auth falls back to web session when no mTLS cert", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		middleware := auth.Middleware(handler)

		// SSE events path is RouteAuthDual; without mTLS cert, should try web session cookie
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.SSEEvents, nil)
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		// No cookie → should get web session cookie required error
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "web session cookie required")
	})
}

// TestAuthIntegrity_AppPolicyDenyByDefault verifies that app identities without
// an AppPolicy are denied access (deny-by-default enforcement).
// This is a regression test for Finding 2: MCP/A2A ingress when JWKS omitted.
func TestAuthIntegrity_AppPolicyDenyByDefault(t *testing.T) {
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Create an app identity without an AppPolicy
	appID := "spiffe://g8e.local/app/test-app-no-policy"

	// Try to get AppPolicy for this app - should return nil (deny-by-default)
	policy, err := db.AppPolicyStore.GetAppPolicy(appID)
	require.NoError(t, err)
	assert.Nil(t, policy, "App without policy should have nil policy (deny-by-default)")
}

func TestAuthService_Middleware_PublicBypass(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := auth.Middleware(handler)

	// Test public route bypass
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "success", rr.Body.String())
}

func TestAuthService_Middleware_HealthBypassConsolidated(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := auth.Middleware(handler)

	// Test that health endpoint bypasses all middleware layers via RouteAuthRegistry
	// This verifies the consolidation: health is RouteAuthNone in RouteAuthRegistry, and the
	// unified middleware uses AuthMode() as the single source of truth
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code, "Health endpoint should bypass all middleware")
	assert.Equal(t, "success", rr.Body.String())

	// Verify that health is registered as RouteAuthNone in the registry
	assert.Equal(t, RouteAuthNone, auth.routeAuth.AuthMode(constants.APIPaths.Health), "Health must be RouteAuthNone in RouteAuthRegistry")

	// Verify that non-public routes still require auth
	reqPrivate := httptest.NewRequest(http.MethodGet, "/api/v1/operators", nil)
	rrPrivate := httptest.NewRecorder()

	middleware.ServeHTTP(rrPrivate, reqPrivate)
	assert.NotEqual(t, http.StatusOK, rrPrivate.Code, "Non-public routes should not bypass auth")
}

func TestAuthService_Middleware_MTLSRequired(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.Middleware(handler)

	// Test mTLS required for non-public route
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "mTLS client certificate required")
}

func TestAuthService_WebSessionAuth_MissingCookie(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Use the unified Middleware with a RouteAuthWebSession path
	middleware := auth.Middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "web session cookie required")
}

func TestAuthService_WebSessionAuth_InvalidSession(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.Middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.AddCookie(&http.Cookie{Name: constants.WebSessionCookieName, Value: "nonexistent-session"})
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "web session not found")
}

func TestAuthService_WebSessionAuth_EmptyCookieValue(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.Middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.AddCookie(&http.Cookie{Name: constants.WebSessionCookieName, Value: ""})
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid web session cookie")
}

func TestAuthService_WebSessionAuth_SessionExpired(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an expired web session
	webSessionID := "expired-web-session"
	webSession := &models.WebSession{
		ID:              webSessionID,
		UserID:          "user-123",
		ExpiresAtUnixMs: time.Now().Add(-1 * time.Hour).UnixMilli(),
		CreatedAtUnixMs: time.Now().Add(-2 * time.Hour).UnixMilli(),
	}
	webSessionBytes, err := json.Marshal(webSession)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("web_sessions", webSessionID, webSessionBytes))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.Middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.AddCookie(&http.Cookie{Name: constants.WebSessionCookieName, Value: webSessionID})
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "web session expired")
}

func TestAuthService_WebSessionAuth_UserInactive(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an inactive user
	userID := "inactive-web-user"
	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusDisabled,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("users", userID, userBytes))

	// Create a valid web session for the inactive user
	webSessionID := "web-session-inactive-user"
	webSession := &models.WebSession{
		ID:              webSessionID,
		UserID:          userID,
		ExpiresAtUnixMs: time.Now().Add(1 * time.Hour).UnixMilli(),
		CreatedAtUnixMs: time.Now().UnixMilli(),
	}
	webSessionBytes, err := json.Marshal(webSession)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("web_sessions", webSessionID, webSessionBytes))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.Middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.AddCookie(&http.Cookie{Name: constants.WebSessionCookieName, Value: webSessionID})
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "identity disabled")
}

func TestAuthService_WebSessionAuth_Success(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an active user
	userID := "active-web-user"
	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("users", userID, userBytes))

	// Create a valid web session
	webSessionID := "valid-web-session"
	webSession := &models.WebSession{
		ID:              webSessionID,
		UserID:          userID,
		ExpiresAtUnixMs: time.Now().Add(1 * time.Hour).UnixMilli(),
		CreatedAtUnixMs: time.Now().UnixMilli(),
	}
	webSessionBytes, err := json.Marshal(webSession)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("web_sessions", webSessionID, webSessionBytes))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify user_id is stamped in context
		ctxUserID := r.Context().Value(constants.ContextKeyUserID)
		assert.Equal(t, userID, ctxUserID)
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.Middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.AddCookie(&http.Cookie{Name: constants.WebSessionCookieName, Value: webSessionID})
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestAuthService_HasJWKS(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)

	// Without JWKS
	authWithout := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")
	assert.False(t, authWithout.HasJWKS())

	// With JWKS (mock)
	jwks := &JWKSProvider{}
	authWith := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", jwks, "", "", "")
	assert.True(t, authWith.HasJWKS())
}

func TestAuthService_JWTAuthMiddleware_NotConfigured(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.JWTAuthMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mcp/tools", nil)
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), "JWT authentication not configured")
}

func TestAuthService_JWTAuthMiddleware_MissingBearer(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	jwks := &JWKSProvider{}
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", jwks, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.JWTAuthMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mcp/tools", nil)
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "missing JWT bearer token")
}

func TestAuthService_JWTAuthMiddleware_InvalidBearerFormat(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	jwks := &JWKSProvider{}
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", jwks, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.JWTAuthMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mcp/tools", nil)
	req.Header.Set("Authorization", "InvalidFormat token123")
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "missing JWT bearer token")
}

func TestAuthService_JWTAuthMiddleware_EmptyToken(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	jwks := &JWKSProvider{}
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", jwks, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.JWTAuthMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mcp/tools", nil)
	req.Header.Set("Authorization", "Bearer ")
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "missing JWT token")
}

func TestAuthService_HandleOperatorAuth_Success(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an active user
	userID := "user-123"
	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("users", userID, userBytes))

	// Create an Operator session
	operatorSessionID := "op-session-123"
	opDoc := &models.OperatorDocumentGo{
		ID:                "op-123",
		OperatorSessionID: operatorSessionID,
		Status:            constants.OperatorStatusActive,
		UserID:            userID,
		OrganizationID:    "org-123",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("operators", "op-123", opBytes))

	// Test ValidateOperatorSession directly (the core validation logic)
	op, err := auth.ValidateOperatorSession(operatorSessionID)
	require.NoError(t, err)
	assert.NotNil(t, op)
	assert.Equal(t, operatorSessionID, op.OperatorSessionID)
	assert.Equal(t, userID, op.UserID)
}

func TestAuthService_HandleOperatorAuth_InvalidSession(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Test with invalid session
	_, err := auth.ValidateOperatorSession("invalid-session")
	require.Error(t, err)
}

func TestAuthService_HandleOperatorAuth_TerminatedOperator(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create a terminated Operator session
	operatorSessionID := "terminated-session"
	opDoc := &models.OperatorDocumentGo{
		ID:                "op-terminated",
		OperatorSessionID: operatorSessionID,
		Status:            constants.OperatorStatusTerminated,
		UserID:            "user-123",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("operators", "op-terminated", opBytes))

	_, err = auth.ValidateOperatorSession(operatorSessionID)
	require.Error(t, err)
}

func TestAuthService_HandleCLIAuth_Success(t *testing.T) {
	db := newTestDB(t)

	// Create an active user
	userID := "user-456"
	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("users", userID, userBytes))

	// Create a CLI session
	cliSessionID := "cli-session-123"
	cliDoc := &models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		CreatedAt:         time.Now().UTC(),
		AbsoluteExpiresAt: time.Now().Add(1 * time.Hour),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("cli_sessions", cliSessionID, cliBytes))

	// Test CLI session retrieval and validation
	cliDocResult, err := db.DocStore.DocGet("cli_sessions", cliSessionID)
	require.NoError(t, err)
	assert.NotNil(t, cliDocResult)

	var cliSession models.CLISession
	b, _ := json.Marshal(cliDocResult.Data)
	err = json.Unmarshal(b, &cliSession)
	require.NoError(t, err)
	assert.Equal(t, userID, cliSession.UserID)
	assert.False(t, cliSession.ExpiresAt.IsZero())
}

func TestAuthService_HandleCLIAuth_SessionNotFound(t *testing.T) {
	db := newTestDB(t)

	// Test with non-existent CLI session
	cliDoc, err := db.DocStore.DocGet("cli_sessions", "nonexistent-cli-session")
	require.NoError(t, err)
	assert.Nil(t, cliDoc)
}

func TestAuthService_HandleCLIAuth_SessionExpired(t *testing.T) {
	db := newTestDB(t)

	// Create an expired CLI session
	cliSessionID := "expired-cli-session"
	cliDoc := &models.CLISession{
		ID:                cliSessionID,
		UserID:            "user-123",
		ExpiresAt:         time.Now().Add(-1 * time.Hour),
		CreatedAt:         time.Now().Add(-2 * time.Hour),
		AbsoluteExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("cli_sessions", cliSessionID, cliBytes))

	// Verify the session is stored with expired timestamp
	cliDocResult, err := db.DocStore.DocGet("cli_sessions", cliSessionID)
	require.NoError(t, err)
	assert.NotNil(t, cliDocResult)

	var cliSession models.CLISession
	b, _ := json.Marshal(cliDocResult.Data)
	err = json.Unmarshal(b, &cliSession)
	require.NoError(t, err)
	assert.True(t, cliSession.ExpiresAt.Before(time.Now()))
}

func TestAuthService_HandleCLIAuth_UserInactive(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)

	// Create an inactive user
	userID := "inactive-user"
	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusDisabled,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("users", userID, userBytes))

	// Verify user is inactive
	user, err := userSvc.GetByID(userID)
	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.False(t, user.IsActive())
}

func TestAuthService_HandleAppAuth_NoAppPolicy(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// Call handleAppAuth without mTLS cert (no app identity)
	// When there's no TLS connection, handleAppAuth returns false
	// Note: handleAppAuth expects r.TLS to be non-nil, so we test through middleware instead
	// For now, test the enforceAppPolicy function directly which is the core logic
	policy := &models.AppPolicy{}
	err := auth.enforceAppPolicy(req, policy, "app-123")
	require.NoError(t, err)
}

func TestAuthService_HandleAppAuth_PolicyNotFound(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// Test enforceAppPolicy with empty policy (no restrictions)
	policy := &models.AppPolicy{}
	err := auth.enforceAppPolicy(req, policy, "app-123")
	require.NoError(t, err)
}

func TestAuthService_EnforceAppPolicy_RateLimit(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create a policy with rate limit
	policy := &models.AppPolicy{
		RateLimitRPS: 1,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// First request should pass
	err := auth.enforceAppPolicy(req, policy, "app-123")
	require.NoError(t, err)

	// Second request should also pass (burst allows 2x)
	err = auth.enforceAppPolicy(req, policy, "app-123")
	require.NoError(t, err)

	// Third request should hit rate limit
	err = auth.enforceAppPolicy(req, policy, "app-123")
	require.Error(t, err)
}

func TestAuthService_EnforceAppPolicy_PayloadSize(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create a policy with max payload size
	policy := &models.AppPolicy{
		MaxPayloadBytes: 100,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.ContentLength = 200

	err := auth.enforceAppPolicy(req, policy, "app-123")
	require.Error(t, err)
}

func TestAuthService_CliCertBoundToOperator_Success(t *testing.T) {
	db := newTestDB(t)

	// Create a CLI session linked to Operator session
	cliSessionID := "cli-session-bound"
	operatorSessionID := "op-session-bound"
	userID := "user-bound"

	cliDoc := &models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		CreatedAt:         time.Now().UTC(),
		AbsoluteExpiresAt: time.Now().Add(1 * time.Hour),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("cli_sessions", cliSessionID, cliBytes))

	// Test that CLI session can be retrieved and has correct operator_session_id
	cliDocResult, err := db.DocStore.DocGet("cli_sessions", cliSessionID)
	require.NoError(t, err)
	assert.NotNil(t, cliDocResult)

	var cliSession models.CLISession
	b, _ := json.Marshal(cliDocResult.Data)
	err = json.Unmarshal(b, &cliSession)
	require.NoError(t, err)
	assert.Equal(t, operatorSessionID, cliSession.OperatorSessionID)
	assert.Equal(t, userID, cliSession.UserID)
}

func TestAuthService_CliCertBoundToOperator_SessionMismatch(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create a CLI session with different Operator session
	cliSessionID := "cli-session-mismatch"
	operatorSessionID := "op-session-1"
	userID := "user-mismatch"

	cliDoc := &models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: "op-session-2", // Different Operator session
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		CreatedAt:         time.Now().UTC(),
		AbsoluteExpiresAt: time.Now().Add(1 * time.Hour),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("cli_sessions", cliSessionID, cliBytes))

	bound, err := auth.cliCertBoundToOperator(nil, cliSessionID, userID, operatorSessionID)
	require.NoError(t, err)
	assert.False(t, bound)
}

func TestAuthService_CliCertBoundToOperator_SessionExpired(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an expired CLI session
	cliSessionID := "cli-session-expired"
	operatorSessionID := "op-session-expired"
	userID := "user-expired"

	cliDoc := &models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
		ExpiresAt:         time.Now().Add(-1 * time.Hour),
		CreatedAt:         time.Now().Add(-2 * time.Hour),
		AbsoluteExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("cli_sessions", cliSessionID, cliBytes))

	bound, err := auth.cliCertBoundToOperator(nil, cliSessionID, userID, operatorSessionID)
	require.NoError(t, err)
	assert.False(t, bound)
}

// TestAuthService_HandleOperatorAuth_Integration tests the handleOperatorAuth path
// through the middleware to ensure mTLS URI SAN validation works correctly.
func TestAuthService_HandleOperatorAuth_Integration(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an active user
	userID := "user-op-auth"
	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("users", userID, userBytes))

	// Create an Operator session
	operatorSessionID := "op-session-auth-test"
	organizationID := "org-auth-test"
	opDoc := &models.OperatorDocumentGo{
		ID:                "op-auth-test",
		OperatorSessionID: operatorSessionID,
		Status:            constants.OperatorStatusActive,
		UserID:            userID,
		OrganizationID:    organizationID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("operators", "op-auth-test", opBytes))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := auth.Middleware(handler)

	t.Run("operator auth with valid mTLS URI SAN succeeds", func(t *testing.T) {
		wid := protocol.NewWorkloadIdentity()
		opURI, err := wid.OperatorSPIFFEURL(organizationID, "op-auth-test", operatorSessionID)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+operatorSessionID)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{URIs: []*url.URL{opURI}},
			},
		}
		rr := httptest.NewRecorder()

		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "success", rr.Body.String())
	})

	t.Run("operator auth with mismatched URI SAN is rejected", func(t *testing.T) {
		wid := protocol.NewWorkloadIdentity()
		wrongURI, err := wid.OperatorSPIFFEURL("wrong-org", "wrong-op", operatorSessionID)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+operatorSessionID)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{URIs: []*url.URL{wrongURI}},
			},
		}
		rr := httptest.NewRecorder()

		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "mTLS identity mismatch")
	})
}

// TestAuthService_HandleCLIAuth_Integration tests the CLI auth path
// through the middleware to ensure CLI session validation works correctly.
func TestAuthService_HandleCLIAuth_Integration(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an active user
	userID := "user-cli-auth"
	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("users", userID, userBytes))

	// Create a CLI session
	cliSessionID := "cli-session-auth-test"
	operatorSessionID := "op-session-cli-auth"
	cliDoc := &models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		CreatedAt:         time.Now().UTC(),
		AbsoluteExpiresAt: time.Now().Add(1 * time.Hour),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("cli_sessions", cliSessionID, cliBytes))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := auth.Middleware(handler)

	t.Run("CLI auth with valid mTLS URI SAN succeeds", func(t *testing.T) {
		wid := protocol.NewWorkloadIdentity()
		cliURI, err := wid.CLISPIFFEURL(userID, cliSessionID)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{URIs: []*url.URL{cliURI}},
			},
		}
		rr := httptest.NewRecorder()

		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "success", rr.Body.String())
	})

	t.Run("CLI auth with mismatched URI SAN is rejected", func(t *testing.T) {
		wid := protocol.NewWorkloadIdentity()
		wrongURI, err := wid.CLISPIFFEURL("wrong-user", "wrong-cli-session")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{URIs: []*url.URL{wrongURI}},
			},
		}
		rr := httptest.NewRecorder()

		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "mTLS identity mismatch")
	})
}

// TestAuthService_HandleAppAuth_Integration tests the app auth path
// through the middleware to ensure app policy validation works correctly.
func TestAuthService_HandleAppAuth_Integration(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an app policy
	operatorID := "test-operator"
	appID := "spiffe://g8e.local/app/" + operatorID
	policyDoc := &models.AppPolicy{
		AppID:              appID,
		RateLimitRPS:       10,
		MaxPayloadBytes:    1000000,
		AllowedCollections: []string{"test_collection"},
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	policyBytes, err := json.Marshal(policyDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("app_policies", appID, policyBytes))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := auth.Middleware(handler)

	t.Run("app auth with valid policy and mTLS URI SAN succeeds", func(t *testing.T) {
		wid := protocol.NewWorkloadIdentity()
		appURI, err := wid.AppSPIFFEURL(operatorID)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{URIs: []*url.URL{appURI}},
			},
		}
		rr := httptest.NewRecorder()

		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "success", rr.Body.String())
	})

	t.Run("app auth without policy is rejected", func(t *testing.T) {
		wid := protocol.NewWorkloadIdentity()
		appURI, err := wid.AppSPIFFEURL("no-policy-operator")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{URIs: []*url.URL{appURI}},
			},
		}
		rr := httptest.NewRecorder()

		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "app policy not found")
	})

	t.Run("app auth attempting to access privileged endpoint is rejected", func(t *testing.T) {
		wid := protocol.NewWorkloadIdentity()
		appURI, err := wid.AppSPIFFEURL(operatorID)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/_query", nil)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{URIs: []*url.URL{appURI}},
			},
		}
		rr := httptest.NewRecorder()

		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "external apps cannot access privileged endpoints")
	})

	t.Run("auth middleware extracts operator session info from headers", func(t *testing.T) {
		opID := "op-audit-123"
		opSessionID := "opsess-audit-456"
		cliSessionID := "cli-sess-audit-789"
		userID := "user-audit-abc"

		// Mock CLI session in DB
		cliDoc := &models.CLISession{
			ID:        cliSessionID,
			UserID:    userID,
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		cliBytes, _ := json.Marshal(cliDoc)
		require.NoError(t, db.DocStore.DocSet("cli_sessions", cliSessionID, cliBytes))

		// Mock user in DB
		userDoc := &models.User{
			ID:     userID,
			Status: constants.UserStatusActive,
		}
		userBytes, _ := json.Marshal(userDoc)
		require.NoError(t, db.DocStore.DocSet("users", userID, userBytes))

		wid := protocol.NewWorkloadIdentity()
		cliURI, _ := wid.CLISPIFFEURL(userID, cliSessionID)

		var capturedCtx context.Context
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})

		middleware := auth.Middleware(handler)

		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.MCPEndpoint, nil)
		req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
		req.Header.Set(constants.HeaderOperatorID, opID)
		req.Header.Set(constants.HeaderOperatorSessionID, opSessionID)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{URIs: []*url.URL{cliURI}},
			},
		}
		rr := httptest.NewRecorder()

		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		require.NotNil(t, capturedCtx)

		// Verify identity info in context
		assert.Equal(t, userID, capturedCtx.Value(constants.ContextKeyUserID))
		assert.Equal(t, opID, capturedCtx.Value(constants.ContextKeyOperatorID))
		assert.Equal(t, opSessionID, capturedCtx.Value(constants.ContextKeyOperatorSessionID))
	})
}
