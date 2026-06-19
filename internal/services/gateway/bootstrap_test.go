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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

func TestBootstrapFlow(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

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

	// 1. Initial status - not bootstrapped
	req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
	rr := httptest.NewRecorder()
	h.authController.handleBootstrapStatus(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var statusResp models.BootstrapStatusResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &statusResp))
	assert.False(t, statusResp.Bootstrapped)

	// 2. Perform bootstrap
	bootstrapBody := map[string]string{
		"csr_pem":            string(csrPEM),
		"cli_csr_pem":        string(cliCsrPEM),
		"system_fingerprint": "test-fingerprint",
	}
	body, err := json.Marshal(bootstrapBody)
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345" // Simulate loopback
	rr = httptest.NewRecorder()
	h.authController.handleLocalBootstrapWithURL(rr, req)
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

	// 3. Status - now bootstrapped
	req = httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
	rr = httptest.NewRecorder()
	h.authController.handleBootstrapStatus(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &statusResp))
	assert.True(t, statusResp.Bootstrapped)

	// 4. Verify bootstrap user is active
	user, err := h.userSvc.GetByID(bootstrapUserID)
	require.NoError(t, err)
	assert.True(t, user.IsActive())
	assert.True(t, user.IsBootstrap)

	// 5. Verify bootstrap user can authenticate
	op, err := h.auth.ValidateOperatorSession(bootstrapSessionID)
	require.NoError(t, err)
	assert.Equal(t, bootstrapUserID, op.UserID)

	// 6. Simulate real user registration (retirement)
	// We'll call the retirement logic directly as if RegistrationService did it
	realUserID := "user-real-123"
	realOperatorID := "op-real-456"
	err = h.userSvc.Disable(bootstrapUserID, "retired_by_real_login", realUserID, realOperatorID)
	require.NoError(t, err)

	// 7. Verify bootstrap user is now inactive
	user, err = h.userSvc.GetByID(bootstrapUserID)
	require.NoError(t, err)
	assert.False(t, user.IsActive())
	assert.Equal(t, constants.UserStatusDisabled, user.Status)

	// 8. Verify audit entry was created for bootstrap retirement
	filters := []models.DocFilter{
		{Field: "target", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", bootstrapUserID))},
		{Field: "action", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", models.AdminAuditActionRetireLocalOwner))},
	}
	results, err := h.db.DocStore.DocQuery(marshaler.CollectionName(constants.CollectionAuthAdminAudit), filters, "", 0)
	require.NoError(t, err)
	require.Len(t, results, 1, "Expected exactly one audit entry for bootstrap retirement")

	var auditEntry models.AdminAuditEntry
	auditBytes, err := json.Marshal(results[0].ForWire())
	require.NoError(t, err)
	err = json.Unmarshal(auditBytes, &auditEntry)
	require.NoError(t, err)
	assert.Equal(t, models.AdminAuditActionRetireLocalOwner, auditEntry.Action)
	assert.Equal(t, realUserID, auditEntry.Actor)
	assert.Equal(t, realOperatorID, auditEntry.OperatorID)
	require.NotNil(t, auditEntry.Details)
	assert.Equal(t, "retired_by_real_login", auditEntry.Details.Reason)

	// 8. Verify bootstrap user is REJECTED during authentication
	op, err = h.auth.ValidateOperatorSession(bootstrapSessionID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity disabled")
	assert.Nil(t, op)
}
