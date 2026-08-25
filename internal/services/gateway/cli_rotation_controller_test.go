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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// setupTestCLIRotationController creates a CLIRotationController backed by
// the full test infrastructure (real DB, PKI, session services). A real
// active user is created so rotation has a valid identity. Returns the
// controller and the created user.
func setupTestCLIRotationController(t *testing.T) (*CLIRotationController, *models.User) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	c := newCLIRotationController(CLIRotationControllerDeps{
		Cfg:           infra.Cfg,
		Logger:        infra.Logger,
		PKI:           infra.PKI,
		CLISessionSvc: infra.CLISessionSvc,
		UserSvc:       infra.UserSvc,
		Responder:     infra.Responder,
	})

	user, err := infra.UserSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, user)

	return c, user
}

// persistActiveCLISession creates an active CLI session for the given user
// and returns the session ID. The session is signed with a real CSR so the
// cert serial/fingerprint are populated (rotation revokes the old cert by
// serial).
func persistActiveCLISession(t *testing.T, c *CLIRotationController, userID string) (cliSessionID, operatorSessionID, certSerial string) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	// Re-use the controller's underlying services by signing through the
	// PKI authority that the test controller was built with. We can't
	// reach the controller's private pki field from here, so we sign via
	// a fresh infra's PKI — but that would issue a cert under a different
	// CA. Instead, we sign via the controller's pki by calling the
	// exported SignCSR through a helper. Since CLIRotationController
	// keeps pki private, the test creates the session document directly
	// with a known serial so the revocation path can be exercised.
	_ = infra
	operatorSessionID = "op-session-" + userID
	cliSessionID = "cli-session-" + userID
	certSerial = "test-serial-" + userID

	// Persist a CLI session document directly. We don't need a real cert
	// for the rotation tests — the controller revokes by serial, and a
	// missing revocation document is a no-op (RevokeCertificate just
	// DocSets the serial).
	doc := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
		SystemFingerprint: "test-sys-fp",
		CertFingerprint:   "test-cert-fp-" + userID,
		CertSerial:        certSerial,
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(1 * time.Hour),
		AbsoluteExpiresAt: time.Now().UTC().Add(1 * time.Hour),
		IdleExpiresAt:     time.Now().UTC().Add(1 * time.Hour),
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       string(constants.HeartbeatTypeBootstrap),
	}
	b, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, c.cliSessionSvc.db.DocSet(
		marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, b,
	))
	return cliSessionID, operatorSessionID, certSerial
}

// rotationRequestWithContext builds a POST /rotate request with the mTLS
// context (user ID + CLI session ID) stamped, matching what the unified
// auth middleware does for RouteAuthMTLS routes.
func rotationRequestWithContext(t *testing.T, userID, cliSessionID, csrPEM string) *http.Request {
	t.Helper()
	body, err := json.Marshal(models.CLIRotationRequest{CLICSRPEM: csrPEM})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRotate, bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, cliSessionID)
	return req.WithContext(ctx)
}

// parseRotationResponse parses a successful rotation response.
func parseRotationResponse(t *testing.T, rr *httptest.ResponseRecorder) models.CLIRotationResponse {
	t.Helper()
	require.Equalf(t, http.StatusCreated, rr.Code, "rotation should succeed, body: %s", rr.Body.String())
	var resp models.CLIRotationResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	return resp
}

// ---------------------------------------------------------------------------
// handleRotate — success path
// ---------------------------------------------------------------------------

func TestCLIRotationController_Rotate_Success(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	oldSessionID, _, _ := persistActiveCLISession(t, c, user.ID)

	csrPEM, newPrivKey, _ := generateTestCSR(t, "rotation-new-cli")
	req := rotationRequestWithContext(t, user.ID, oldSessionID, csrPEM)
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)

	resp := parseRotationResponse(t, rr)
	assert.NotEmpty(t, resp.CLISessionID)
	assert.NotEqual(t, oldSessionID, resp.CLISessionID, "rotation must issue a new session ID")
	assert.NotEmpty(t, resp.CLICert)
	assert.NotEmpty(t, resp.CLICertChain)
	assert.NotEmpty(t, resp.HubTrustBundle)
	assert.Equal(t, user.ID, resp.UserID)

	// The issued cert's public key must match the new CSR's private key.
	block, _ := pem.Decode([]byte(resp.CLICert))
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	certPubKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok)
	assert.True(t, newPrivKey.PublicKey.Equal(certPubKey), "issued cert must match the new CSR key pair")
}

func TestCLIRotationController_Rotate_OldSessionDeactivated(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	oldSessionID, _, _ := persistActiveCLISession(t, c, user.ID)

	csrPEM, _, _ := generateTestCSR(t, "rotation-deactivate-old")
	req := rotationRequestWithContext(t, user.ID, oldSessionID, csrPEM)
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	// The old session must now be inactive.
	oldDoc, err := c.cliSessionSvc.db.DocGet(
		marshaler.CollectionName(constants.CollectionCLISessions), oldSessionID)
	require.NoError(t, err)
	require.NotNil(t, oldDoc)
	var oldSession models.CLISession
	dataBytes, err := json.Marshal(oldDoc.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(dataBytes, &oldSession))
	assert.False(t, oldSession.IsActive, "old session must be deactivated after rotation")
}

func TestCLIRotationController_Rotate_NewCertUsableForMTLS(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	oldSessionID, _, _ := persistActiveCLISession(t, c, user.ID)

	csrPEM, newPrivKey, _ := generateTestCSR(t, "rotation-usable-cli")
	req := rotationRequestWithContext(t, user.ID, oldSessionID, csrPEM)
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)
	resp := parseRotationResponse(t, rr)

	// The new session must be active and bound to the same user.
	newDoc, err := c.cliSessionSvc.db.DocGet(
		marshaler.CollectionName(constants.CollectionCLISessions), resp.CLISessionID)
	require.NoError(t, err)
	require.NotNil(t, newDoc)
	var newSession models.CLISession
	dataBytes, err := json.Marshal(newDoc.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(dataBytes, &newSession))
	assert.True(t, newSession.IsActive, "new session must be active")
	assert.Equal(t, user.ID, newSession.UserID)
	assert.NotEmpty(t, newSession.CertFingerprint)
	assert.NotEmpty(t, newSession.CertSerial)

	// The cert/key pair must be a valid ECDSA key pair (already asserted
	// in the success test; here we re-confirm the key is on P-256).
	block, _ := pem.Decode([]byte(resp.CLICert))
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	certPubKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok)
	assert.Equal(t, elliptic.P256(), certPubKey.Curve)
	assert.True(t, newPrivKey.PublicKey.Equal(certPubKey))
}

// ---------------------------------------------------------------------------
// handleRotate — error paths
// ---------------------------------------------------------------------------

func TestCLIRotationController_Rotate_MissingUserContext(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	persistActiveCLISession(t, c, user.ID)

	csrPEM, _, _ := generateTestCSR(t, "rotation-no-user-ctx")
	body, _ := json.Marshal(models.CLIRotationRequest{CLICSRPEM: csrPEM})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRotate, bytes.NewReader(body))
	// No user context — simulate unauthenticated request.
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestCLIRotationController_Rotate_MissingCLISessionContext(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	persistActiveCLISession(t, c, user.ID)

	csrPEM, _, _ := generateTestCSR(t, "rotation-no-session-ctx")
	body, _ := json.Marshal(models.CLIRotationRequest{CLICSRPEM: csrPEM})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRotate, bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID)
	// No CLI session ID in context.
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestCLIRotationController_Rotate_MissingCSR(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	oldSessionID, _, _ := persistActiveCLISession(t, c, user.ID)

	req := rotationRequestWithContext(t, user.ID, oldSessionID, "")
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLIRotationCSRRequired.Error())
}

func TestCLIRotationController_Rotate_InvalidJSON(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	oldSessionID, _, _ := persistActiveCLISession(t, c, user.ID)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRotate, bytes.NewReader([]byte("{invalid")))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID)
	ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, oldSessionID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrInvalidJSONBody.Error())
}

func TestCLIRotationController_Rotate_MethodNotAllowed(t *testing.T) {
	c, _ := setupTestCLIRotationController(t)
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthCLIRotate, nil)
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestCLIRotationController_Rotate_SessionNotFound(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	// Don't persist a session — use a random ID that doesn't exist.
	csrPEM, _, _ := generateTestCSR(t, "rotation-missing-session")
	req := rotationRequestWithContext(t, user.ID, "nonexistent-session-id", csrPEM)
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestCLIRotationController_Rotate_SessionUserMismatch(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	// Persist a session for user A, then try to rotate it as user B.
	oldSessionID, _, _ := persistActiveCLISession(t, c, user.ID)

	otherUser, err := c.userSvc.CreateUser()
	require.NoError(t, err)

	csrPEM, _, _ := generateTestCSR(t, "rotation-user-mismatch")
	req := rotationRequestWithContext(t, otherUser.ID, oldSessionID, csrPEM)
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrMTLSIdentityMismatch.Error())
}

func TestCLIRotationController_Rotate_UserNotActive(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	oldSessionID, _, _ := persistActiveCLISession(t, c, user.ID)

	// Disable the user after the session was created.
	require.NoError(t, c.userSvc.Disable(user.ID, "test", "actor", "op"))

	csrPEM, _, _ := generateTestCSR(t, "rotation-disabled-user")
	req := rotationRequestWithContext(t, user.ID, oldSessionID, csrPEM)
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "not active")
}

func TestCLIRotationController_Rotate_AlreadyDeactivatedSession(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	oldSessionID, _, _ := persistActiveCLISession(t, c, user.ID)

	// Deactivate the session first.
	require.NoError(t, c.cliSessionSvc.DeactivateCLISession(oldSessionID))

	csrPEM, _, _ := generateTestCSR(t, "rotation-already-deactivated")
	req := rotationRequestWithContext(t, user.ID, oldSessionID, csrPEM)
	rr := httptest.NewRecorder()
	c.handleRotate(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLISessionAlreadyDeactivated.Error())
}

// ---------------------------------------------------------------------------
// Concurrent rotation — exactly one caller succeeds
// ---------------------------------------------------------------------------

func TestCLIRotationController_Rotate_ConcurrentOnlyOneSucceeds(t *testing.T) {
	c, user := setupTestCLIRotationController(t)
	oldSessionID, _, _ := persistActiveCLISession(t, c, user.ID)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var (
		mu          sync.Mutex
		successes   int
		conflictErr int
		otherErr    int
	)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			csrPEM, _, _ := generateTestCSR(t, "rotation-concurrent")
			req := rotationRequestWithContext(t, user.ID, oldSessionID, csrPEM)
			rr := httptest.NewRecorder()
			c.handleRotate(rr, req)
			mu.Lock()
			switch rr.Code {
			case http.StatusCreated:
				successes++
			case http.StatusConflict:
				conflictErr++
			default:
				otherErr++
				t.Logf("unexpected status: %d body: %s", rr.Code, rr.Body.String())
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, successes, "exactly one concurrent rotation should succeed")
	assert.Equal(t, goroutines-1, conflictErr, "all other rotations should get conflict (already deactivated)")
	assert.Equal(t, 0, otherErr, "no other errors expected")
}
