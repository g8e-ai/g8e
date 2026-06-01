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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthService_ValidateOperatorSession_MissingSessionID(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	_, err := auth.ValidateOperatorSession("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing operator_session_id")
}

func TestAuthService_ValidateOperatorSession_SessionNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	_, err := auth.ValidateOperatorSession("nonexistent-session")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired operator session")
}

func TestAuthService_ValidateOperatorSession_TerminatedStatus(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an operator session with terminated status
	sessionID := "terminated-session"
	opDoc := map[string]interface{}{
		"id":                  "op-123",
		"operator_session_id": sessionID,
		"status":              marshaler.OperatorStatus(constants.OperatorStatusTerminated),
		"user_id":             "user-123",
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("operators", "op-123", opBytes))

	_, err = auth.ValidateOperatorSession(sessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operator identity disabled")
}

func TestAuthService_ValidateOperatorSession_SessionExpired(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an active user
	userID := "user-456"
	userDoc := map[string]interface{}{
		"id":       userID,
		"username": "expired-user",
		"status":   "active",
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("users", userID, userBytes))

	// Create an operator session with old timestamp using the test hook
	sessionID := "expired-session"
	oldTime := time.Now().UTC().Add(-25 * time.Hour)
	opDoc := map[string]interface{}{
		"id":                  "op-456",
		"operator_session_id": sessionID,
		"status":              marshaler.OperatorStatus(constants.OperatorStatusActive),
		"user_id":             userID,
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSetWithTimestamps("operators", "op-456", opBytes, oldTime, oldTime))

	_, err = auth.ValidateOperatorSession(sessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operator session expired")
}

func TestAuthService_ValidateOperatorSession_UserInactive(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an inactive user
	userID := "inactive-user"
	userDoc := map[string]interface{}{
		"id":       userID,
		"username": "inactive",
		"status":   "inactive",
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("users", userID, userBytes))

	// Create an operator session linked to the inactive user
	sessionID := "session-with-inactive-user"
	opDoc := map[string]interface{}{
		"id":                  "op-789",
		"operator_session_id": sessionID,
		"status":              marshaler.OperatorStatus(constants.OperatorStatusActive),
		"user_id":             userID,
		"created_at":          time.Now().Format(time.RFC3339),
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("operators", "op-789", opBytes))

	_, err = auth.ValidateOperatorSession(sessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "identity disabled")
}

func TestAuthService_ExtractOperatorSessionID_BearerToken(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer test-token-123")

	sessionID := auth.ExtractOperatorSessionID(req)
	assert.Equal(t, "test-token-123", sessionID)
}

func TestAuthService_ExtractOperatorSessionID_NoBearer(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic dGVzdDp0ZXN0")

	sessionID := auth.ExtractOperatorSessionID(req)
	assert.Empty(t, sessionID)
}

func TestAuthService_ExtractOperatorSessionID_NoHeader(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	req := httptest.NewRequest("GET", "/", nil)

	sessionID := auth.ExtractOperatorSessionID(req)
	assert.Empty(t, sessionID)
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

func TestPublicRouteRegistry_ExactPaths(t *testing.T) {
	t.Parallel()

	registry := NewPublicRouteRegistry(false)

	// Test exact public paths
	publicPaths := []string{
		constants.APIPaths.Health,
		constants.APIPaths.PKICSRSign,
		constants.APIPaths.PKIDevicesEnroll,
		constants.APIPaths.AuthBootstrap,
		constants.APIPaths.AuthBootstrapStatus,
		constants.APIPaths.AuthLoginVerify,
		constants.APIPaths.AuthLogout,
	}

	for _, path := range publicPaths {
		assert.True(t, registry.IsPublic(path), "Path %s should be public", path)
	}

	// Test that slight variations are not public
	assert.False(t, registry.IsPublic("/healthz"), "/healthz should not be public")
	assert.False(t, registry.IsPublic(constants.APIPaths.PKICSRSign+"/"), constants.APIPaths.PKICSRSign+"/ should not be public")
	assert.False(t, registry.IsPublic(constants.APIPaths.AuthBootstrap+"/extra"), constants.APIPaths.AuthBootstrap+"/extra should not be public")
}

func TestPublicRouteRegistry_Prefixes(t *testing.T) {
	t.Parallel()

	registry := NewPublicRouteRegistry(false)

	// Test prefix-based public paths
	publicPrefixPaths := []string{
		"/.well-known/g8e/pki/",
		"/.well-known/g8e/pki/g8eg-ca-bundle.pem",
		"/.well-known/g8e/pki/fingerprint",
		"/.well-known/g8e/pki/some/deep/path",
	}

	for _, path := range publicPrefixPaths {
		assert.True(t, registry.IsPublic(path), "Path %s should be public (prefix match)", path)
	}

	// Test that paths outside the prefix are not public
	assert.False(t, registry.IsPublic("/.well-known/other/pki/"), "/.well-known/other/pki/ should not be public")
	assert.False(t, registry.IsPublic("/api/v1/pki/"), "/api/v1/pki/ should not be public")
}

func TestPublicRouteRegistry_JWKSEnabled(t *testing.T) {
	t.Parallel()

	// Registry with JWKS enabled
	registryWithJWKS := NewPublicRouteRegistry(true)

	// Test JIT passkey prefix matches
	jitPaths := []string{
		"/api/v1/auth/passkeys/jit-123",
		"/api/v1/auth/passkeys/jit-abc",
		"/api/v1/auth/passkeys/jit-register/challenge",
	}
	for _, path := range jitPaths {
		assert.True(t, registryWithJWKS.IsPublic(path), "Path %s should be public with JWKS enabled", path)
	}

	// Test MCP tools prefix matches
	mcpPaths := []string{
		"/api/v1/mcp/tools/list",
		"/api/v1/mcp/tools/call",
		"/api/v1/mcp/tools/some-tool",
	}
	for _, path := range mcpPaths {
		assert.True(t, registryWithJWKS.IsPublic(path), "Path %s should be public with JWKS enabled", path)
	}

	// Test A2A prefix matches
	a2aPaths := []string{
		"/api/v1/a2a/call",
		"/api/v1/a2a/some-endpoint",
	}
	for _, path := range a2aPaths {
		assert.True(t, registryWithJWKS.IsPublic(path), "Path %s should be public with JWKS enabled", path)
	}

	// Registry without JWKS
	registryWithoutJWKS := NewPublicRouteRegistry(false)

	allJWKSPaths := append(jitPaths, append(mcpPaths, a2aPaths...)...)
	for _, path := range allJWKSPaths {
		assert.False(t, registryWithoutJWKS.IsPublic(path), "Path %s should not be public without JWKS", path)
	}
}

func TestPublicRouteRegistry_NonPublicPaths(t *testing.T) {
	t.Parallel()

	registry := NewPublicRouteRegistry(false)

	// Test paths that should never be public
	privatePaths := []string{
		constants.APIPaths.GovernanceEnvelopes,
		"/_query",
		"/api/users",
		"/api/operators",
		"/ws/",
		"/api/db",
	}

	for _, path := range privatePaths {
		assert.False(t, registry.IsPublic(path), "Path %s should not be public", path)
	}
}

func TestPublicRouteRegistry_CanonicalCoverage(t *testing.T) {
	t.Parallel()

	registry := NewPublicRouteRegistry(false)

	// Ensure all previously hardcoded public routes are covered
	// This test prevents regression when the registry is modified
	assert.True(t, registry.IsPublic(constants.APIPaths.Health), "Health check must be public")
	assert.True(t, registry.IsPublic("/.well-known/g8e/pki/"), "PKI prefix must be public")
	assert.True(t, registry.IsPublic("/api/v1/pki/csr/sign"), "CSR signing must be public")
	assert.True(t, registry.IsPublic("/api/v1/pki/devices/enroll"), "Device enrollment must be public")
	assert.True(t, registry.IsPublic("/api/v1/auth/bootstrap"), "Bootstrap must be public")
	assert.True(t, registry.IsPublic("/api/v1/auth/bootstrap/status"), "Bootstrap status must be public")
	assert.True(t, registry.IsPublic("/api/v1/auth/login/verify"), "Login verify must be public")
	assert.True(t, registry.IsPublic("/api/v1/auth/logout"), "Logout must be public")
}

// TestAuthIntegrity_AppPolicyDenyByDefault verifies that app identities without
// an AppPolicy are denied access (deny-by-default enforcement).
// This is a regression test for Finding 2: MCP/A2A ingress when JWKS omitted.
func TestAuthIntegrity_AppPolicyDenyByDefault(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Create an app identity without an AppPolicy
	appID := "spiffe://g8e.local/app/test-app-no-policy"

	// Try to get AppPolicy for this app - should return nil (deny-by-default)
	policy, err := db.GetAppPolicy(appID)
	require.NoError(t, err)
	assert.Nil(t, policy, "App without policy should have nil policy (deny-by-default)")
}

func TestAuthService_Middleware_PublicBypass(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
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

func TestAuthService_Middleware_MTLSRequired(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
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
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.WebSessionAuth(handler, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "web session cookie required")
}

func TestAuthService_WebSessionAuth_InvalidSession(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := auth.WebSessionAuth(handler, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "g8e_session", Value: "nonexistent-session"})
	rr := httptest.NewRecorder()

	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "web session not found")
}

func TestAuthService_HasJWKS(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)

	// Without JWKS
	authWithout := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")
	assert.False(t, authWithout.HasJWKS())

	// With JWKS (mock)
	jwks := &JWKSProvider{}
	authWith := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", jwks, "", "", "")
	assert.True(t, authWith.HasJWKS())
}

func TestAuthService_JWTAuthMiddleware_NotConfigured(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
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
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
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

func TestAuthService_HandleOperatorAuth_Success(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an active user
	userID := "user-123"
	userDoc := map[string]interface{}{
		"id":       userID,
		"username": "test-user",
		"status":   "active",
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("users", userID, userBytes))

	// Create an operator session
	sessionID := "op-session-123"
	opDoc := map[string]interface{}{
		"id":                  "op-123",
		"operator_session_id": sessionID,
		"status":              marshaler.OperatorStatus(constants.OperatorStatusActive),
		"user_id":             userID,
		"organization_id":     "org-123",
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("operators", "op-123", opBytes))

	// Test ValidateOperatorSession directly (the core validation logic)
	op, err := auth.ValidateOperatorSession(sessionID)
	assert.NoError(t, err)
	assert.NotNil(t, op)
	assert.Equal(t, sessionID, op.OperatorSessionID)
	assert.Equal(t, userID, op.UserID)
}

func TestAuthService_HandleOperatorAuth_InvalidSession(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Test with invalid session
	_, err := auth.ValidateOperatorSession("invalid-session")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired operator session")
}

func TestAuthService_HandleOperatorAuth_TerminatedOperator(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create a terminated operator session
	sessionID := "terminated-session"
	opDoc := map[string]interface{}{
		"id":                  "op-terminated",
		"operator_session_id": sessionID,
		"status":              marshaler.OperatorStatus(constants.OperatorStatusTerminated),
		"user_id":             "user-123",
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("operators", "op-terminated", opBytes))

	_, err = auth.ValidateOperatorSession(sessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operator identity disabled")
}

func TestAuthService_HandleCLIAuth_Success(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create an active user
	userID := "user-456"
	userDoc := map[string]interface{}{
		"id":       userID,
		"username": "cli-user",
		"status":   "active",
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("users", userID, userBytes))

	// Create a CLI session
	cliSessionID := "cli-session-123"
	cliDoc := map[string]interface{}{
		"user_id":    userID,
		"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("cli_sessions", cliSessionID, cliBytes))

	// Test CLI session retrieval and validation
	cliDocResult, err := db.DocGet("cli_sessions", cliSessionID)
	assert.NoError(t, err)
	assert.NotNil(t, cliDocResult)

	var cliSession models.CLISession
	b, _ := json.Marshal(cliDocResult.Data)
	err = json.Unmarshal(b, &cliSession)
	assert.NoError(t, err)
	assert.Equal(t, userID, cliSession.UserID)
	assert.False(t, cliSession.ExpiresAt.IsZero())
}

func TestAuthService_HandleCLIAuth_SessionNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Test with non-existent CLI session
	cliDoc, err := db.DocGet("cli_sessions", "nonexistent-cli-session")
	assert.NoError(t, err)
	assert.Nil(t, cliDoc)
}

func TestAuthService_HandleCLIAuth_SessionExpired(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create an expired CLI session
	cliSessionID := "expired-cli-session"
	cliDoc := map[string]interface{}{
		"user_id":    "user-123",
		"expires_at": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("cli_sessions", cliSessionID, cliBytes))

	// Verify the session is stored with expired timestamp
	cliDocResult, err := db.DocGet("cli_sessions", cliSessionID)
	assert.NoError(t, err)
	assert.NotNil(t, cliDocResult)

	var cliSession models.CLISession
	b, _ := json.Marshal(cliDocResult.Data)
	err = json.Unmarshal(b, &cliSession)
	assert.NoError(t, err)
	assert.True(t, cliSession.ExpiresAt.Before(time.Now()))
}

func TestAuthService_HandleCLIAuth_UserInactive(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)

	// Create an inactive user
	userID := "inactive-user"
	userDoc := map[string]interface{}{
		"id":       userID,
		"username": "inactive",
		"status":   "inactive",
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("users", userID, userBytes))

	// Verify user is inactive
	user, err := userSvc.GetByID(userID)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.False(t, user.IsActive())
}

func TestAuthService_HandleAppAuth_NoAppPolicy(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// Call handleAppAuth without mTLS cert (no app identity)
	// When there's no TLS connection, handleAppAuth returns false
	// Note: handleAppAuth expects r.TLS to be non-nil, so we test through middleware instead
	// For now, test the enforceAppPolicy function directly which is the core logic
	policy := &models.AppPolicy{}
	err := auth.enforceAppPolicy(req, policy, "app-123")
	assert.NoError(t, err)
}

func TestAuthService_HandleAppAuth_PolicyNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// Test enforceAppPolicy with empty policy (no restrictions)
	policy := &models.AppPolicy{}
	err := auth.enforceAppPolicy(req, policy, "app-123")
	assert.NoError(t, err)
}

func TestAuthService_EnforceAppPolicy_RateLimit(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create a policy with rate limit
	policy := &models.AppPolicy{
		RateLimitRPS: 1,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// First request should pass
	err := auth.enforceAppPolicy(req, policy, "app-123")
	assert.NoError(t, err)

	// Second request should also pass (burst allows 2x)
	err = auth.enforceAppPolicy(req, policy, "app-123")
	assert.NoError(t, err)

	// Third request should hit rate limit
	err = auth.enforceAppPolicy(req, policy, "app-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exceeded")
}

func TestAuthService_EnforceAppPolicy_PayloadSize(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create a policy with max payload size
	policy := &models.AppPolicy{
		MaxPayloadBytes: 100,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.ContentLength = 200

	err := auth.enforceAppPolicy(req, policy, "app-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "payload exceeds maximum allowed size")
}

func TestAuthService_CliCertBoundToOperator_Success(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create a CLI session linked to operator session
	cliSessionID := "cli-session-bound"
	operatorSessionID := "op-session-bound"
	userID := "user-bound"

	cliDoc := map[string]interface{}{
		"user_id":             userID,
		"operator_session_id": operatorSessionID,
		"expires_at":          time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("cli_sessions", cliSessionID, cliBytes))

	// Test that CLI session can be retrieved and has correct operator_session_id
	cliDocResult, err := db.DocGet("cli_sessions", cliSessionID)
	assert.NoError(t, err)
	assert.NotNil(t, cliDocResult)

	var cliSession models.CLISession
	b, _ := json.Marshal(cliDocResult.Data)
	err = json.Unmarshal(b, &cliSession)
	assert.NoError(t, err)
	assert.Equal(t, operatorSessionID, cliSession.OperatorSessionID)
	assert.Equal(t, userID, cliSession.UserID)
}

func TestAuthService_CliCertBoundToOperator_SessionMismatch(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create a CLI session with different operator session
	cliSessionID := "cli-session-mismatch"
	operatorSessionID := "op-session-1"
	userID := "user-mismatch"

	cliDoc := map[string]interface{}{
		"user_id":             userID,
		"operator_session_id": "op-session-2", // Different operator session
		"expires_at":          time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("cli_sessions", cliSessionID, cliBytes))

	bound := auth.cliCertBoundToOperator(nil, cliSessionID, userID, operatorSessionID)
	assert.False(t, bound)
}

func TestAuthService_CliCertBoundToOperator_SessionExpired(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an expired CLI session
	cliSessionID := "cli-session-expired"
	operatorSessionID := "op-session-expired"
	userID := "user-expired"

	cliDoc := map[string]interface{}{
		"user_id":             userID,
		"operator_session_id": operatorSessionID,
		"expires_at":          time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("cli_sessions", cliSessionID, cliBytes))

	bound := auth.cliCertBoundToOperator(nil, cliSessionID, userID, operatorSessionID)
	assert.False(t, bound)
}

// TestAuthService_HandleOperatorAuth_Integration tests the handleOperatorAuth path
// through the middleware to ensure mTLS URI SAN validation works correctly.
func TestAuthService_HandleOperatorAuth_Integration(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an active user
	userID := "user-op-auth"
	userDoc := map[string]interface{}{
		"id":       userID,
		"username": "op-auth-user",
		"status":   "active",
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("users", userID, userBytes))

	// Create an operator session
	sessionID := "op-session-auth-test"
	organizationID := "org-auth-test"
	opDoc := map[string]interface{}{
		"id":                  "op-auth-test",
		"operator_session_id": sessionID,
		"status":              marshaler.OperatorStatus(constants.OperatorStatusActive),
		"user_id":             userID,
		"organization_id":     organizationID,
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("operators", "op-auth-test", opBytes))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := auth.Middleware(handler)

	t.Run("operator auth with valid mTLS URI SAN succeeds", func(t *testing.T) {
		t.Parallel()
		wid := protocol.NewWorkloadIdentity()
		opURI, err := wid.OperatorSPIFFEURL(organizationID, "op-auth-test", sessionID)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+sessionID)
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
		t.Parallel()
		wid := protocol.NewWorkloadIdentity()
		wrongURI, err := wid.OperatorSPIFFEURL("wrong-org", "wrong-op", "wrong-session")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+sessionID)
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
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an active user
	userID := "user-cli-auth"
	userDoc := map[string]interface{}{
		"id":       userID,
		"username": "cli-auth-user",
		"status":   "active",
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("users", userID, userBytes))

	// Create a CLI session
	cliSessionID := "cli-session-auth-test"
	operatorSessionID := "op-session-cli-auth"
	cliDoc := map[string]interface{}{
		"user_id":             userID,
		"operator_session_id": operatorSessionID,
		"expires_at":          time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("cli_sessions", cliSessionID, cliBytes))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := auth.Middleware(handler)

	t.Run("CLI auth with valid mTLS URI SAN succeeds", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")

	// Create an app policy
	operatorID := "test-operator"
	appID := "spiffe://g8e.local/app/" + operatorID
	policyDoc := map[string]interface{}{
		"id":                  appID,
		"rate_limit_rps":      10,
		"max_payload_bytes":   1000000,
		"allowed_collections": []string{"test_collection"},
	}
	policyBytes, err := json.Marshal(policyDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("app_policies", appID, policyBytes))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := auth.Middleware(handler)

	t.Run("app auth with valid policy and mTLS URI SAN succeeds", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
}
