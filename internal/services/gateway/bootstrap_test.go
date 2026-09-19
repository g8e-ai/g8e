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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// loadEmbeddedOperatorDoc fetches and unmarshals the embedded-operator
// document for assertions.
func loadEmbeddedOperatorDoc(t *testing.T, docStore *DocumentStoreService) *models.OperatorDocumentGo {
	t.Helper()
	doc, err := docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
	require.NoError(t, err)
	require.NotNil(t, doc, "embedded operator document must exist")
	b, err := json.Marshal(doc.Data)
	require.NoError(t, err)
	var op models.OperatorDocumentGo
	require.NoError(t, json.Unmarshal(b, &op))
	op.ID = doc.ID
	return &op
}

func TestBootstrapFlow(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)

	cliPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cliCsrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "g8e-cli-test",
			Organization: []string{"g8e"},
		},
	}
	cliCsrBytes, err := x509.CreateCertificateRequest(rand.Reader, &cliCsrTemplate, cliPriv)
	require.NoError(t, err)
	cliCsrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: cliCsrBytes})

	// 1. Initial status - not bootstrapped (no users yet)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
	rr := httptest.NewRecorder()
	h.bootstrapController.handleBootstrapStatus(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var statusResp models.BootstrapStatusResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &statusResp))
	assert.False(t, statusResp.Bootstrapped, "bootstrapped is false on a fresh gateway with no users")

	// 2. Perform bootstrap (creates the first real user, the gateway admin).
	// The embedded operator is certless: the request carries only the CLI CSR.
	bootstrapBody := map[string]string{
		"cli_csr_pem":        string(cliCsrPEM),
		"system_fingerprint": "test-fingerprint",
	}
	body, err := json.Marshal(bootstrapBody)
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rr = httptest.NewRecorder()
	h.bootstrapController.handleLocalBootstrapWithURL(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code, "Bootstrap failed: %s", rr.Body.String())

	var resp models.BootstrapResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, resp.Success, "Bootstrap response success: %v", resp)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), resp.OperatorID, "bootstrap binds the embedded operator: %v", resp)
	require.NotEmpty(t, resp.HubTrustBundle, "hub_trust_bundle is missing: %v", resp)
	require.NotEmpty(t, resp.OperatorSessionID, "operator_session_id is missing: %v", resp)
	require.NotEmpty(t, resp.CLISessionID, "cli_session_id is missing: %v", resp)

	require.NotNil(t, resp.User, "user is missing: %v", resp)
	bootstrapUserID := resp.User.ID
	bootstrapSessionID := resp.OperatorSessionID
	cliSessionID := resp.CLISessionID
	require.NotEmpty(t, cliSessionID, "cli_session_id must be non-empty")
	require.NotEqual(t, bootstrapSessionID, cliSessionID,
		"cli_session_id MUST be a distinct identifier from operator_session_id - session types are strictly disjoint")

	// The bootstrap claim bound the embedded operator to the first user and
	// minted the operator session the CLI session is bound to.
	op := loadEmbeddedOperatorDoc(t, infra.DocStore)
	assert.True(t, op.Claimed, "embedded operator is claimed by bootstrap")
	assert.Equal(t, bootstrapUserID, op.UserID)
	assert.Equal(t, bootstrapSessionID, op.OperatorSessionID)
	assert.Equal(t, constants.OperatorTypeEmbedded, op.OperatorType)
	assert.Equal(t, "test-fingerprint", op.SystemFingerprint)

	opSession, err := infra.OperatorSessionSvc.GetActiveSessionForUser(bootstrapUserID)
	require.NoError(t, err)
	require.NotNil(t, opSession, "operator_sessions document must exist for the bootstrap session")
	assert.Equal(t, bootstrapSessionID, opSession.ID)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), opSession.OperatorID)

	cliSession, err := infra.CLISessionSvc.loadCLISession(cliSessionID)
	require.NoError(t, err)
	assert.Equal(t, bootstrapSessionID, cliSession.OperatorSessionID,
		"cli_sessions document binds the bootstrap operator session")

	// 3. Status - now bootstrapped (the first user exists)
	req = httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
	rr = httptest.NewRecorder()
	h.bootstrapController.handleBootstrapStatus(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &statusResp))
	assert.True(t, statusResp.Bootstrapped, "bootstrapped flips to true once the first user exists")

	// 4. Verify the first user is active and is the admin (first user). There
	// is no ephemeral bootstrap-user concept and no retirement flow: the user
	// created by bootstrap IS the first human enrollee and stays active.
	user, err := h.adminController.userSvc.GetByID(bootstrapUserID)
	require.NoError(t, err)
	assert.True(t, user.IsActive())
	isFirst, err := h.adminController.userSvc.IsFirstUser(bootstrapUserID)
	require.NoError(t, err)
	assert.True(t, isFirst, "the bootstrap-created user is the first user (admin)")

	// 5. Verify the user can authenticate via the operator session
	validatedOp, err := h.authMiddleware.ValidateOperatorSession(bootstrapSessionID)
	require.NoError(t, err)
	assert.Equal(t, bootstrapUserID, validatedOp.UserID)

	// 6. Verify the user remains active (no retirement). The old
	// create-then-retire dance is gone; the first user is a real user that
	// is never disabled by a later login.
	user, err = h.adminController.userSvc.GetByID(bootstrapUserID)
	require.NoError(t, err)
	assert.True(t, user.IsActive(), "the first user is never retired by a later enrollment")
}
