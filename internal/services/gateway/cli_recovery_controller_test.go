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
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// setupTestCLIRecoveryController creates a CLIRecoveryController backed by
// the full test infrastructure (real DB, PKI, session services). A real
// active user is created so the recovery flow has someone to approve.
// Returns the controller and the created approving user.
func setupTestCLIRecoveryController(t *testing.T) (*CLIRecoveryController, *models.User) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	recoverySvc := NewCLIRecoveryService(infra.Stores.DocStore, infra.Logger)

	c := newCLIRecoveryController(CLIRecoveryControllerDeps{
		Cfg:                infra.Cfg,
		Logger:             infra.Logger,
		RecoverySvc:        recoverySvc,
		UserSvc:            infra.UserSvc,
		PKI:                infra.PKI,
		CLISessionSvc:      infra.CLISessionSvc,
		OperatorSessionSvc: infra.OperatorSessionSvc,
		DocStore:           infra.Stores.DocStore,
		Responder:          infra.Responder,
	})

	// Create a real active user to act as the approver.
	user, err := infra.UserSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, user)

	return c, user
}

// recoveryRequestJSON builds the JSON body for a recovery request.
func recoveryRequestJSON(t *testing.T, csrPEM string) []byte {
	t.Helper()
	b, err := json.Marshal(models.CLIRecoveryRequestRequest{
		CLICSRPEM:         csrPEM,
		SystemFingerprint: "test-sys-fp",
		LocalOSUser:       &models.LocalOSUser{Username: "bob"},
	})
	require.NoError(t, err)
	return b
}

// newRecoveryRequest calls handleRecoveryRequest and returns the parsed response.
func newRecoveryRequest(t *testing.T, c *CLIRecoveryController, csrPEM string) models.CLIRecoveryRequestResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryRequest, bytes.NewReader(recoveryRequestJSON(t, csrPEM)))
	rr := httptest.NewRecorder()
	c.handleRecoveryRequest(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code, "recovery request should succeed, body: %s", rr.Body.String())
	var resp models.CLIRecoveryRequestResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.NotEmpty(t, resp.Token)
	require.NotEmpty(t, resp.RequestID)
	require.NotEmpty(t, resp.ApprovalURL)
	require.True(t, resp.ExpiresAt.After(time.Now().UTC()))
	return resp
}

// approveRecovery calls handleRecoveryApprove with the given user context.
func approveRecovery(t *testing.T, c *CLIRecoveryController, token string, approve bool, userID string) models.CLIRecoveryApproveResponse {
	t.Helper()
	body, err := json.Marshal(models.CLIRecoveryApproveRequest{Token: token, Approve: approve})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryApprove, bytes.NewReader(body))
	if userID != "" {
		ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, userID)
		req = req.WithContext(ctx)
	}
	rr := httptest.NewRecorder()
	c.handleRecoveryApprove(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "approve should succeed, body: %s", rr.Body.String())
	var resp models.CLIRecoveryApproveResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp
}

// completeRecovery calls handleRecoveryComplete and returns the recorder + parsed response (if successful).
func completeRecovery(t *testing.T, c *CLIRecoveryController, token string, signature []byte) (*httptest.ResponseRecorder, models.CLIRecoveryCompleteResponse) {
	t.Helper()
	sigB64 := ""
	if signature != nil {
		sigB64 = base64.StdEncoding.EncodeToString(signature)
	}
	body, err := json.Marshal(models.CLIRecoveryCompleteRequest{Token: token, Signature: sigB64})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryComplete, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	c.handleRecoveryComplete(rr, req)
	var resp models.CLIRecoveryCompleteResponse
	if rr.Code == http.StatusCreated {
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	}
	return rr, resp
}

// ---------------------------------------------------------------------------
// handleRecoveryRequest
// ---------------------------------------------------------------------------

func TestCLIRecoveryController_Request_Success(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	csrPEM, _, _ := generateTestCSR(t, "recovery-test-cli")

	resp := newRecoveryRequest(t, c, csrPEM)

	assert.NotEmpty(t, resp.Token)
	assert.Contains(t, resp.ApprovalURL, "#recovery=")
	assert.Contains(t, resp.ApprovalURL, resp.Token)
	// Token must be in the fragment, not in the query/path.
	assert.False(t, strings.Contains(strings.Split(resp.ApprovalURL, "#")[0], resp.Token))
}

func TestCLIRecoveryController_Request_MethodNotAllowed(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthCLIRecoveryRequest, nil)
	rr := httptest.NewRecorder()
	c.handleRecoveryRequest(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestCLIRecoveryController_Request_InvalidJSON(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryRequest, strings.NewReader("{invalid"))
	rr := httptest.NewRecorder()
	c.handleRecoveryRequest(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrInvalidJSONBody.Error())
}

func TestCLIRecoveryController_Request_MissingCSR(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	body, _ := json.Marshal(models.CLIRecoveryRequestRequest{SystemFingerprint: "fp"})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryRequest, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	c.handleRecoveryRequest(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLICSRRequired.Error())
}

func TestCLIRecoveryController_Request_Unbootstrapped(t *testing.T) {
	// Use fresh infrastructure with no users created.
	infra := setupTestInfrastructure(t, false)
	recoverySvc := NewCLIRecoveryService(infra.Stores.DocStore, infra.Logger)
	c := newCLIRecoveryController(CLIRecoveryControllerDeps{
		Cfg:         infra.Cfg,
		Logger:      infra.Logger,
		RecoverySvc: recoverySvc,
		UserSvc:     infra.UserSvc,
		PKI:         infra.PKI,
		Responder:   infra.Responder,
	})

	csrPEM, _, _ := generateTestCSR(t, "recovery-test-cli")
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryRequest, bytes.NewReader(recoveryRequestJSON(t, csrPEM)))
	rr := httptest.NewRecorder()
	c.handleRecoveryRequest(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "bootstrap")
}

// ---------------------------------------------------------------------------
// handleRecoveryStatus
// ---------------------------------------------------------------------------

func TestCLIRecoveryController_Status_Pending(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	csrPEM, _, _ := generateTestCSR(t, "recovery-status-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthCLIRecoveryStatus+"?token="+resp.Token, nil)
	rr := httptest.NewRecorder()
	c.handleRecoveryStatus(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var statusResp models.CLIRecoveryStatusResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &statusResp))
	assert.True(t, statusResp.Success)
	assert.Equal(t, models.CLIRecoveryStatePending, statusResp.State)
}

func TestCLIRecoveryController_Status_Approved(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, _, _ := generateTestCSR(t, "recovery-status-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	approveRecovery(t, c, resp.Token, true, user.ID)

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthCLIRecoveryStatus+"?token="+resp.Token, nil)
	rr := httptest.NewRecorder()
	c.handleRecoveryStatus(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var statusResp models.CLIRecoveryStatusResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &statusResp))
	assert.Equal(t, models.CLIRecoveryStateApproved, statusResp.State)
}

func TestCLIRecoveryController_Status_NotFound(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthCLIRecoveryStatus+"?token=nonexistent-token", nil)
	rr := httptest.NewRecorder()
	c.handleRecoveryStatus(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestCLIRecoveryController_Status_MissingToken(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthCLIRecoveryStatus, nil)
	rr := httptest.NewRecorder()
	c.handleRecoveryStatus(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCLIRecoveryController_Status_MethodNotAllowed(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryStatus, nil)
	rr := httptest.NewRecorder()
	c.handleRecoveryStatus(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

// ---------------------------------------------------------------------------
// handleRecoveryApprove
// ---------------------------------------------------------------------------

func TestCLIRecoveryController_Approve_Success(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, _, _ := generateTestCSR(t, "recovery-approve-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	approveResp := approveRecovery(t, c, resp.Token, true, user.ID)
	assert.True(t, approveResp.Success)
	assert.Equal(t, models.CLIRecoveryStateApproved, approveResp.State)
}

func TestCLIRecoveryController_Deny_Success(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, _, _ := generateTestCSR(t, "recovery-deny-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	body, _ := json.Marshal(models.CLIRecoveryApproveRequest{Token: resp.Token, Approve: false})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryApprove, bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	c.handleRecoveryApprove(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var approveResp models.CLIRecoveryApproveResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &approveResp))
	assert.Equal(t, models.CLIRecoveryStateDenied, approveResp.State)
}

func TestCLIRecoveryController_Approve_AlreadyApproved(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, _, _ := generateTestCSR(t, "recovery-approve-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	approveRecovery(t, c, resp.Token, true, user.ID)

	// Second approval should fail with consumed.
	body, _ := json.Marshal(models.CLIRecoveryApproveRequest{Token: resp.Token, Approve: true})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryApprove, bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	c.handleRecoveryApprove(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLIRecoveryRequestConsumed.Error())
}

func TestCLIRecoveryController_Approve_NotFound(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	body, _ := json.Marshal(models.CLIRecoveryApproveRequest{Token: "nonexistent", Approve: true})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryApprove, bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	c.handleRecoveryApprove(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestCLIRecoveryController_Approve_MissingUserContext(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	csrPEM, _, _ := generateTestCSR(t, "recovery-approve-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	body, _ := json.Marshal(models.CLIRecoveryApproveRequest{Token: resp.Token, Approve: true})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryApprove, bytes.NewReader(body))
	// No user context — simulate unauthenticated request.
	rr := httptest.NewRecorder()
	c.handleRecoveryApprove(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestCLIRecoveryController_Approve_InactiveUser(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, _, _ := generateTestCSR(t, "recovery-approve-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	// Disable the approving user.
	require.NoError(t, c.userSvc.Disable(user.ID, "test", "actor", "op"))

	body, _ := json.Marshal(models.CLIRecoveryApproveRequest{Token: resp.Token, Approve: true})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryApprove, bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	c.handleRecoveryApprove(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "not active")
}

func TestCLIRecoveryController_Approve_MethodNotAllowed(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthCLIRecoveryApprove, nil)
	rr := httptest.NewRecorder()
	c.handleRecoveryApprove(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestCLIRecoveryController_Approve_MissingToken(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	body, _ := json.Marshal(models.CLIRecoveryApproveRequest{Approve: true})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryApprove, bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	c.handleRecoveryApprove(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ---------------------------------------------------------------------------
// handleRecoveryComplete
// ---------------------------------------------------------------------------

func TestCLIRecoveryController_Complete_Success(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, privKey, _ := generateTestCSR(t, "recovery-complete-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	approveRecovery(t, c, resp.Token, true, user.ID)

	// Sign the request ID with the CSR private key for proof-of-possession.
	sig := signProofOfPossession(t, privKey, resp.RequestID)

	rr, completeResp := completeRecovery(t, c, resp.Token, sig)
	require.Equal(t, http.StatusCreated, rr.Code, "body: %s", rr.Body.String())

	assert.True(t, completeResp.Success)
	assert.NotEmpty(t, completeResp.CLICert)
	assert.NotEmpty(t, completeResp.CLICertChain)
	assert.NotEmpty(t, completeResp.HubTrustBundle)
	assert.NotEmpty(t, completeResp.CLISessionID)
	assert.Equal(t, user.ID, completeResp.UserID)
	assert.NotEmpty(t, completeResp.OperatorSessionID)
	assert.NotEmpty(t, completeResp.OperatorID)
}

func TestCLIRecoveryController_Complete_NotApproved(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	csrPEM, privKey, _ := generateTestCSR(t, "recovery-complete-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	// Don't approve — try to complete directly.
	sig := signProofOfPossession(t, privKey, resp.RequestID)
	rr, _ := completeRecovery(t, c, resp.Token, sig)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLIRecoveryNotApproved.Error())
}

func TestCLIRecoveryController_Complete_Denied(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, privKey, _ := generateTestCSR(t, "recovery-complete-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	// Deny the request.
	approveRecovery(t, c, resp.Token, false, user.ID)

	sig := signProofOfPossession(t, privKey, resp.RequestID)
	rr, _ := completeRecovery(t, c, resp.Token, sig)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLIRecoveryRequestDenied.Error())
}

func TestCLIRecoveryController_Complete_AlreadyCompleted(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, privKey, _ := generateTestCSR(t, "recovery-complete-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	approveRecovery(t, c, resp.Token, true, user.ID)
	sig := signProofOfPossession(t, privKey, resp.RequestID)

	// First completion succeeds.
	rr1, _ := completeRecovery(t, c, resp.Token, sig)
	require.Equal(t, http.StatusCreated, rr1.Code)

	// Second completion should fail with consumed.
	rr2, _ := completeRecovery(t, c, resp.Token, sig)
	assert.Equal(t, http.StatusConflict, rr2.Code)
	assert.Contains(t, rr2.Body.String(), constants.ErrCLIRecoveryRequestConsumed.Error())
}

func TestCLIRecoveryController_Complete_InvalidProofOfPossession(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, _, _ := generateTestCSR(t, "recovery-complete-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	approveRecovery(t, c, resp.Token, true, user.ID)

	// Generate a different key and sign with it — should fail PoP verification.
	_, wrongKey, _ := generateTestCSR(t, "wrong-key-cli")
	wrongSig := signProofOfPossession(t, wrongKey, resp.RequestID)

	rr, _ := completeRecovery(t, c, resp.Token, wrongSig)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLIRecoveryProofInvalid.Error())
}

func TestCLIRecoveryController_Complete_WrongMessage(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, privKey, _ := generateTestCSR(t, "recovery-complete-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	approveRecovery(t, c, resp.Token, true, user.ID)

	// Sign the wrong message (not the request ID).
	sig := signProofOfPossession(t, privKey, "wrong-message")

	rr, _ := completeRecovery(t, c, resp.Token, sig)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLIRecoveryProofInvalid.Error())
}

func TestCLIRecoveryController_Complete_InvalidBase64Signature(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, _, _ := generateTestCSR(t, "recovery-complete-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	approveRecovery(t, c, resp.Token, true, user.ID)

	body, _ := json.Marshal(models.CLIRecoveryCompleteRequest{Token: resp.Token, Signature: "!!!not-base64!!!"})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryComplete, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	c.handleRecoveryComplete(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLIRecoveryProofInvalid.Error())
}

func TestCLIRecoveryController_Complete_NotFound(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	_, privKey, _ := generateTestCSR(t, "recovery-complete-cli")
	sig := signProofOfPossession(t, privKey, "fake-request-id")

	rr, _ := completeRecovery(t, c, "nonexistent-token", sig)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestCLIRecoveryController_Complete_MissingToken(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	body, _ := json.Marshal(models.CLIRecoveryCompleteRequest{Signature: "sig"})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryComplete, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	c.handleRecoveryComplete(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCLIRecoveryController_Complete_MissingSignature(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	body, _ := json.Marshal(models.CLIRecoveryCompleteRequest{Token: "some-token"})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRecoveryComplete, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	c.handleRecoveryComplete(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCLIRecoveryController_Complete_MethodNotAllowed(t *testing.T) {
	c, _ := setupTestCLIRecoveryController(t)
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthCLIRecoveryComplete, nil)
	rr := httptest.NewRecorder()
	c.handleRecoveryComplete(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

// ---------------------------------------------------------------------------
// Concurrent completion — only one caller can complete
// ---------------------------------------------------------------------------

func TestCLIRecoveryController_Complete_ConcurrentOnlyOneSucceeds(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, privKey, _ := generateTestCSR(t, "recovery-concurrent-cli")
	resp := newRecoveryRequest(t, c, csrPEM)

	approveRecovery(t, c, resp.Token, true, user.ID)
	sig := signProofOfPossession(t, privKey, resp.RequestID)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var (
		mu          sync.Mutex
		successes   int
		consumedErr int
		otherErr    int
	)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			rr, _ := completeRecovery(t, c, resp.Token, sig)
			mu.Lock()
			switch rr.Code {
			case http.StatusCreated:
				successes++
			case http.StatusConflict:
				consumedErr++
			default:
				otherErr++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, successes, "exactly one concurrent completion should succeed")
	assert.Equal(t, goroutines-1, consumedErr, "all other completions should get consumed error")
	assert.Equal(t, 0, otherErr, "no other errors expected")
}

// ---------------------------------------------------------------------------
// Full lifecycle: request → approve → complete → verify cert is usable
// ---------------------------------------------------------------------------

func TestCLIRecoveryController_FullLifecycle(t *testing.T) {
	c, user := setupTestCLIRecoveryController(t)
	csrPEM, privKey, _ := generateTestCSR(t, "recovery-lifecycle-cli")

	// 1. Create recovery request
	resp := newRecoveryRequest(t, c, csrPEM)
	assert.Equal(t, models.CLIRecoveryStatePending, recoveryStatus(t, c, resp.Token))

	// 2. Approve via browser console
	approveResp := approveRecovery(t, c, resp.Token, true, user.ID)
	assert.Equal(t, models.CLIRecoveryStateApproved, approveResp.State)
	assert.Equal(t, models.CLIRecoveryStateApproved, recoveryStatus(t, c, resp.Token))

	// 3. Complete with proof-of-possession
	sig := signProofOfPossession(t, privKey, resp.RequestID)
	rr, completeResp := completeRecovery(t, c, resp.Token, sig)
	require.Equal(t, http.StatusCreated, rr.Code)

	// 4. Verify the issued certificate parses and matches the CSR public key.
	block, _ := pem.Decode([]byte(completeResp.CLICert))
	require.NotNil(t, block, "CLI cert must be valid PEM")
	require.Equal(t, "CERTIFICATE", block.Type)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// The cert's public key must match the CSR's private key.
	certPubKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok, "issued cert must use an ECDSA key")
	assert.True(t, privKey.PublicKey.Equal(certPubKey), "issued cert public key must match CSR key pair")

	// 5. Status should now be completed.
	assert.Equal(t, models.CLIRecoveryStateCompleted, recoveryStatus(t, c, resp.Token))

	// 6. Token replay should fail.
	rr2, _ := completeRecovery(t, c, resp.Token, sig)
	assert.Equal(t, http.StatusConflict, rr2.Code)
}

// recoveryStatus is a helper that returns the current state of a recovery request.
func recoveryStatus(t *testing.T, c *CLIRecoveryController, token string) models.CLIRecoveryState {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthCLIRecoveryStatus+"?token="+token, nil)
	rr := httptest.NewRecorder()
	c.handleRecoveryStatus(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "status check failed, body: %s", rr.Body.String())
	var resp models.CLIRecoveryStatusResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp.State
}
