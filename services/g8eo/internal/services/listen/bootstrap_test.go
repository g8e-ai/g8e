package listen

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

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/marshaler"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootstrapFlow(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	// Generate a real CSR for the test
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "g8e-cli-test",
			Organization: []string{"g8e"},
		},
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, priv)
	require.NoError(t, err)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

	// 1. Initial status - not bootstrapped
	req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
	rr := httptest.NewRecorder()
	h.handleBootstrapStatus(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var statusResp map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &statusResp))
	assert.Equal(t, false, statusResp["bootstrapped"])

	// 2. Perform bootstrap
	bootstrapBody := map[string]string{
		"email":              "superadmin@g8e.local",
		"name":               "Superadmin",
		"csr_pem":            string(csrPEM),
		"system_fingerprint": "test-fingerprint",
	}
	body, _ := json.Marshal(bootstrapBody)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345" // Simulate loopback
	rr = httptest.NewRecorder()
	h.handleBootstrap(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code, "Bootstrap failed: %s", rr.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"], "Bootstrap response success: %v", resp)
	require.NotNil(t, resp["operator_cert"], "operator_cert is missing: %v", resp)
	require.NotNil(t, resp["operator_cert_chain"], "operator_cert_chain is missing: %v", resp)
	require.NotNil(t, resp["hub_trust_bundle"], "hub_trust_bundle is missing: %v", resp)
	require.NotNil(t, resp["operator_session_id"], "operator_session_id is missing: %v", resp)
	require.NotNil(t, resp["cli_session_id"], "cli_session_id is missing: %v", resp)

	userMap := resp["user"].(map[string]interface{})
	bootstrapUserID := userMap["id"].(string)
	bootstrapSessionID := resp["operator_session_id"].(string)
	cliSessionID := resp["cli_session_id"].(string)
	require.NotEmpty(t, cliSessionID, "cli_session_id must be non-empty")
	require.NotEqual(t, bootstrapSessionID, cliSessionID,
		"cli_session_id MUST be a distinct identifier from operator_session_id - session types are strictly disjoint")

	// 3. Status - now bootstrapped
	req = httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
	rr = httptest.NewRecorder()
	h.handleBootstrapStatus(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &statusResp))
	assert.Equal(t, true, statusResp["bootstrapped"])

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
		{Field: "action", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", models.AdminAuditActionRetireLocalSuperadmin))},
	}
	results, err := h.db.DocQuery(marshaler.CollectionName(constants.CollectionAuthAdminAudit), filters, "", 0)
	require.NoError(t, err)
	require.Len(t, results, 1, "Expected exactly one audit entry for bootstrap retirement")

	var auditEntry models.AdminAuditEntry
	auditBytes, err := json.Marshal(results[0].ForWire())
	require.NoError(t, err)
	err = json.Unmarshal(auditBytes, &auditEntry)
	require.NoError(t, err)
	assert.Equal(t, models.AdminAuditActionRetireLocalSuperadmin, auditEntry.Action)
	assert.Equal(t, realUserID, auditEntry.Actor)
	assert.Equal(t, realOperatorID, auditEntry.OperatorID)
	assert.Equal(t, "retired_by_real_login", auditEntry.Details["reason"])

	// 8. Verify bootstrap user is REJECTED during authentication
	op, err = h.auth.ValidateOperatorSession(bootstrapSessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "identity disabled")
	assert.Nil(t, op)
}
