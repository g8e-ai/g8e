// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// setupTestCLIRefreshController creates a CLIRefreshController backed by
// the full test infrastructure (real DB, PKI, session services). A real
// active user is created so refresh has a valid identity. Returns the
// controller and the created user.
func setupTestCLIRefreshController(t *testing.T) (*CLIRefreshController, *models.User) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	c := newCLIRefreshController(CLIRefreshControllerDeps{
		Cfg:                infra.Cfg,
		Logger:             infra.Logger,
		CLISessionSvc:      infra.CLISessionSvc,
		OperatorSessionSvc: infra.OperatorSessionSvc,
		UserSvc:            infra.UserSvc,
		Responder:          infra.Responder,
	})

	user, err := infra.UserSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, user)

	return c, user
}

// persistCLISessionForController creates an active CLI session document
// directly in the doc store for the given user, returning the session ID.
// Used by controller tests to set up the "old session" state that refresh
// deactivates.
func persistCLISessionForController(t *testing.T, c *CLIRefreshController, userID, sessionID, operatorSessionID string) {
	t.Helper()
	doc := models.CLISession{
		ID:                sessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
		SystemFingerprint: "test-sys-fp",
		CertFingerprint:   "test-cert-fp",
		CertSerial:        "test-serial",
		IsActive:          true,
		SessionType:       string(constants.SessionTypeCLI),
		LoginMethod:       string(constants.HeartbeatTypeBootstrap),
	}
	b, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, c.cliSessionSvc.db.DocSet(
		marshaler.CollectionName(constants.CollectionCLISessions), sessionID, b,
	))
}

// refreshRequestWithContext builds a POST /refresh request with the mTLS
// context (user ID + old CLI session ID) stamped, matching what the unified
// auth middleware does for RouteAuthMTLS routes. The body is empty — the
// cert is the proof of identity.
func refreshRequestWithContext(t *testing.T, userID, oldCLISessionID string) *http.Request {
	t.Helper()
	body, err := json.Marshal(models.CLIRefreshRequest{})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRefresh, bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, oldCLISessionID)
	return req.WithContext(ctx)
}

// parseRefreshResponse parses a successful refresh response.
func parseRefreshResponse(t *testing.T, rr *httptest.ResponseRecorder) models.CLIRefreshResponse {
	t.Helper()
	require.Equalf(t, http.StatusCreated, rr.Code, "refresh should succeed, body: %s", rr.Body.String())
	var resp models.CLIRefreshResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	return resp
}

// ---------------------------------------------------------------------------
// handleRefresh — success path
// ---------------------------------------------------------------------------

// TestCLIRefreshController_Refresh_Success verifies the primary recovery
// path: the cert is still valid, the old CLI session exists and is active,
// and the controller issues a new session bound to the same user. The old
// session is deactivated; the new session is active and persisted.
func TestCLIRefreshController_Refresh_Success(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "refresh-ctrl-old-1"
	persistCLISessionForController(t, c, user.ID, oldSessionID, "op-ctrl-1")

	req := refreshRequestWithContext(t, user.ID, oldSessionID)
	rr := httptest.NewRecorder()
	c.handleRefresh(rr, req)

	resp := parseRefreshResponse(t, rr)
	assert.NotEmpty(t, resp.CLISessionID)
	assert.NotEqual(t, oldSessionID, resp.CLISessionID, "refresh must issue a new session ID")
	assert.Equal(t, user.ID, resp.UserID)

	// Old session must be deactivated.
	oldDoc, err := c.cliSessionSvc.db.DocGet(
		marshaler.CollectionName(constants.CollectionCLISessions), oldSessionID)
	require.NoError(t, err)
	require.NotNil(t, oldDoc)
	var oldSession models.CLISession
	dataBytes, err := json.Marshal(oldDoc.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(dataBytes, &oldSession))
	assert.False(t, oldSession.IsActive, "old session must be deactivated after refresh")

	// New session must be active and bound to the same user.
	newDoc, err := c.cliSessionSvc.db.DocGet(
		marshaler.CollectionName(constants.CollectionCLISessions), resp.CLISessionID)
	require.NoError(t, err)
	require.NotNil(t, newDoc)
	var newSession models.CLISession
	dataBytes, err = json.Marshal(newDoc.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(dataBytes, &newSession))
	assert.True(t, newSession.IsActive, "new session must be active")
	assert.Equal(t, user.ID, newSession.UserID)
}

// TestCLIRefreshController_Refresh_OldSessionMissing_StillSucceeds verifies
// the gateway-volume-reset case: the old session ID from the cert URI SAN
// does not match any persisted session, but the user has an active operator
// session. The controller inherits the operator binding from the active
// operator session and issues a new CLI session — the cert is the proof of
// identity, not the old session's state.
func TestCLIRefreshController_Refresh_OldSessionMissing_StillSucceeds(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)

	// Persist an active operator session for the user so the controller can
	// inherit the operator binding when the old CLI session is missing.
	require.NoError(t, c.operatorSessionSvc.PersistOperatorSession(
		"op-refresh-missing", user.ID, "org-1", "op-id-1", "mTLS",
	))

	// No old CLI session persisted — simulate a volume reset.
	req := refreshRequestWithContext(t, user.ID, "refresh-ctrl-missing-old")
	rr := httptest.NewRecorder()
	c.handleRefresh(rr, req)

	resp := parseRefreshResponse(t, rr)
	assert.NotEmpty(t, resp.CLISessionID)
	assert.Equal(t, user.ID, resp.UserID)

	// New session is active and bound to the inherited operator session.
	newDoc, err := c.cliSessionSvc.db.DocGet(
		marshaler.CollectionName(constants.CollectionCLISessions), resp.CLISessionID)
	require.NoError(t, err)
	require.NotNil(t, newDoc)
	var newSession models.CLISession
	dataBytes, err := json.Marshal(newDoc.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(dataBytes, &newSession))
	assert.True(t, newSession.IsActive)
	assert.Equal(t, user.ID, newSession.UserID)
	assert.Equal(t, "op-refresh-missing", newSession.OperatorSessionID, "new session must inherit the active operator session binding")
}

// TestCLIRefreshController_Refresh_OldSessionMissing_NoOperatorSession_ReturnsClearError
// verifies the fail-closed case: the old CLI session is missing AND the user
// has no active operator session. The controller returns a clear, actionable
// error pointing to re-enrollment rather than a generic 500.
func TestCLIRefreshController_Refresh_OldSessionMissing_NoOperatorSession_ReturnsClearError(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)

	// No old CLI session and no operator session persisted.
	req := refreshRequestWithContext(t, user.ID, "refresh-ctrl-no-op-sess")
	rr := httptest.NewRecorder()
	c.handleRefresh(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), "re-enroll")
}

// ---------------------------------------------------------------------------
// handleRefresh — error paths
// ---------------------------------------------------------------------------

// TestCLIRefreshController_Refresh_MissingUserContext verifies that a
// request with no authenticated user context (the auth middleware did not
// stamp ContextKeyUserID) is rejected with 401. This is the fail-closed
// guard against a misconfigured route that bypasses the auth middleware.
func TestCLIRefreshController_Refresh_MissingUserContext(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	persistCLISessionForController(t, c, user.ID, "refresh-ctrl-no-ctx", "op-ctrl-no-ctx")

	body, _ := json.Marshal(models.CLIRefreshRequest{})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRefresh, bytes.NewReader(body))
	// No user context — simulate unauthenticated request.
	rr := httptest.NewRecorder()
	c.handleRefresh(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestCLIRefreshController_Refresh_UserNotActive verifies that a disabled
// user is rejected with 403 even though the cert is still valid. An expired
// session does not bypass user-disabled checks.
func TestCLIRefreshController_Refresh_UserNotActive(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "refresh-ctrl-disabled-user"
	persistCLISessionForController(t, c, user.ID, oldSessionID, "op-ctrl-disabled")

	// Disable the user after the session was created.
	require.NoError(t, c.userSvc.Disable(user.ID, "test", "actor", "op"))

	req := refreshRequestWithContext(t, user.ID, oldSessionID)
	rr := httptest.NewRecorder()
	c.handleRefresh(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "not active")
}

// TestCLIRefreshController_Refresh_MethodNotAllowed verifies that a non-POST
// request is rejected with 405.
func TestCLIRefreshController_Refresh_MethodNotAllowed(t *testing.T) {
	c, _ := setupTestCLIRefreshController(t)
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthCLIRefresh, nil)
	rr := httptest.NewRecorder()
	c.handleRefresh(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}
