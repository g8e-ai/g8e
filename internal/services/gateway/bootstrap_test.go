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

	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestBootstrapFlow(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	// Generate real CSRs for the test
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "g8e-operator-test",
			Organization: []string{"g8e"},
		},
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, priv)
	require.NoError(t, err)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

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

	// 2. Perform bootstrap (creates the first real user, the gateway admin)
	bootstrapBody := map[string]string{
		"csr_pem":            string(csrPEM),
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
	require.NotEmpty(t, resp.OperatorCert, "operator_cert is missing: %v", resp)
	require.NotEmpty(t, resp.OperatorCertChain, "operator_cert_chain is missing: %v", resp)
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
	op, err := h.authMiddleware.ValidateOperatorSession(bootstrapSessionID)
	require.NoError(t, err)
	assert.Equal(t, bootstrapUserID, op.UserID)

	// 6. Verify the user remains active (no retirement). The old
	// create-then-retire dance is gone; the first user is a real user that
	// is never disabled by a later login.
	user, err = h.adminController.userSvc.GetByID(bootstrapUserID)
	require.NoError(t, err)
	assert.True(t, user.IsActive(), "the first user is never retired by a later enrollment")
}
