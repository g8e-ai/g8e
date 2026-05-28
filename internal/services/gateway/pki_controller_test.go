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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestPKIController(t *testing.T) (*PKIController, *config.Config, *GatewayDBService) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	os.RemoveAll(secretsDir)
	os.MkdirAll(secretsDir, 0755)

	backend, err := keystore.NewTestBackend()
	require.NoError(t, err)
	ks, err := keystore.NewWithBackend(t.TempDir(), logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnsurePermissions())
	sm := &SecretManager{
		db:         db.db,
		secretsDir: t.TempDir(),
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	appEnrollment := NewAppEnrollmentService(db, pki, logger)
	resp := responder.New(logger)

	controller := newPKIController(cfg, logger, db, pki, appEnrollment, resp)
	return controller, cfg, db
}

func TestPKIController_HandlePKIHubBundle(t *testing.T) {
	t.Run("Success - GET returns PEM bundle", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodGet, "/api/pki/hub-bundle", nil)
		rr := httptest.NewRecorder()

		c.handlePKIHubBundle(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/x-pem-file", rr.Header().Get("Content-Type"))
		assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", rr.Header().Get("X-Frame-Options"))
		assert.NotEmpty(t, rr.Body.Bytes())
		assert.Contains(t, string(rr.Body.Bytes()), "BEGIN CERTIFICATE")
	})

	t.Run("Failure - POST method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/hub-bundle", nil)
		rr := httptest.NewRecorder()

		c.handlePKIHubBundle(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Failure - PKI error returns 500", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		c.pki = &PKIAuthority{}

		req := httptest.NewRequest(http.MethodGet, "/api/pki/hub-bundle", nil)
		rr := httptest.NewRecorder()

		c.handlePKIHubBundle(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.JSONEq(t, `{"error":"failed to read hub bundle"}`, rr.Body.String())
	})
}

func TestPKIController_HandlePKIFingerprint(t *testing.T) {
	t.Run("Success - GET returns SHA256 fingerprint", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodGet, "/api/pki/fingerprint", nil)
		rr := httptest.NewRecorder()

		c.handlePKIFingerprint(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]string
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp["root_ca"])
		assert.Contains(t, resp["root_ca"], "sha256:")
	})

	t.Run("Failure - POST method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/fingerprint", nil)
		rr := httptest.NewRecorder()

		c.handlePKIFingerprint(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Failure - Root CA file not found", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		c.pki = &PKIAuthority{}

		req := httptest.NewRequest(http.MethodGet, "/api/pki/fingerprint", nil)
		rr := httptest.NewRecorder()

		c.handlePKIFingerprint(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.JSONEq(t, `{"error":"failed to read root CA"}`, rr.Body.String())
	})

	t.Run("Failure - Invalid PEM format", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		pkiDir := c.pki.PKIDir()
		rootPath := filepath.Join(pkiDir, "root", "root_ca.crt")
		err := os.WriteFile(rootPath, []byte("invalid pem data"), 0644)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/pki/fingerprint", nil)
		rr := httptest.NewRecorder()

		c.handlePKIFingerprint(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.JSONEq(t, `{"error":"invalid root CA PEM"}`, rr.Body.String())
	})
}

func TestPKIController_HandlePKISignCSR(t *testing.T) {
	t.Run("Success - POST signs CSR and returns cert", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		csr := generateTestCSR(t)
		body := map[string]string{
			"csr_pem":             csr,
			"leaf_type":           "operator",
			"organization_id":     "org-123",
			"operator_id":         "op-456",
			"user_id":             "user-789",
			"workload_session_id": "ws-012",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/pki/sign-csr", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handlePKISignCSR(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]string
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp["certificate_pem"])
		assert.NotEmpty(t, resp["certificate_chain_pem"])
		assert.Contains(t, resp["certificate_pem"], "BEGIN CERTIFICATE")
	})

	t.Run("Failure - GET method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodGet, "/api/pki/sign-csr", nil)
		rr := httptest.NewRecorder()

		c.handlePKISignCSR(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Failure - Invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/sign-csr", bytes.NewReader([]byte("invalid json")))
		rr := httptest.NewRecorder()

		c.handlePKISignCSR(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"invalid JSON"}`, rr.Body.String())
	})

	t.Run("Failure - PKI signing error", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		body := map[string]string{
			"csr_pem":   "invalid csr",
			"leaf_type": "operator",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/pki/sign-csr", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handlePKISignCSR(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestPKIController_HandlePKIRevoke(t *testing.T) {
	t.Run("Success - POST revokes certificate", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		body := map[string]string{
			"serial": "test-serial-123",
			"reason": "key-compromise",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/pki/revoke", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handlePKIRevoke(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "ok", resp["status"])
	})

	t.Run("Failure - GET method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodGet, "/api/pki/revoke", nil)
		rr := httptest.NewRecorder()

		c.handlePKIRevoke(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Failure - Invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/revoke", bytes.NewReader([]byte("invalid json")))
		rr := httptest.NewRecorder()

		c.handlePKIRevoke(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"invalid JSON"}`, rr.Body.String())
	})

	t.Run("Failure - Missing serial", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		body := map[string]string{
			"reason": "key-compromise",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/pki/revoke", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handlePKIRevoke(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"serial required"}`, rr.Body.String())
	})

	t.Run("Failure - PKI revocation error", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		c.pki = &PKIAuthority{}

		body := map[string]string{
			"serial": "test-serial",
			"reason": "key-compromise",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/pki/revoke", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handlePKIRevoke(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestPKIController_HandlePKIRevocationBundle(t *testing.T) {
	t.Run("Success - GET returns revocation bundle", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodGet, "/api/pki/revocation-bundle", nil)
		rr := httptest.NewRecorder()

		c.handlePKIRevocationBundle(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]string
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp["bundle_json"])
		assert.NotEmpty(t, resp["signature"])
	})

	t.Run("Failure - POST method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/revocation-bundle", nil)
		rr := httptest.NewRecorder()

		c.handlePKIRevocationBundle(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Failure - PKI bundle generation error", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		c.pki = &PKIAuthority{}

		req := httptest.NewRequest(http.MethodGet, "/api/pki/revocation-bundle", nil)
		rr := httptest.NewRecorder()

		c.handlePKIRevocationBundle(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestPKIController_HandleAppEnroll(t *testing.T) {
	t.Run("Success - POST enrolls app with valid device-link token", func(t *testing.T) {
		t.Parallel()
		c, _, db := setupTestPKIController(t)

		token := "dlk_test_token_12345"
		linkData := map[string]interface{}{
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			"user_id":    "user-123",
		}
		linkJSON, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token, string(linkJSON), 0)

		csr := generateTestCSR(t)
		body := map[string]string{
			"csr_pem":  csr,
			"app_name": "test-app",
			"app_type": "mcp-client",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["app_cert"])
		assert.NotEmpty(t, resp["cert_chain"])
		assert.NotEmpty(t, resp["app_id"])
	})

	t.Run("Failure - GET method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodGet, "/api/pki/app-enroll", nil)
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Failure - App enrollment service not available", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		c.appEnrollment = nil

		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", nil)
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.JSONEq(t, `{"error":"app enrollment service not available"}`, rr.Body.String())
	})

	t.Run("Failure - Missing bearer token", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", nil)
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"missing bearer token"}`, rr.Body.String())
	})

	t.Run("Failure - Invalid token format (no dlk_ prefix)", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", nil)
		req.Header.Set("Authorization", "Bearer invalid_token")
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"invalid device-link token format"}`, rr.Body.String())
	})

	t.Run("Failure - Token too short", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", nil)
		req.Header.Set("Authorization", "Bearer dlk_short")
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"invalid device-link token format"}`, rr.Body.String())
	})

	t.Run("Failure - Device-link token not found", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", nil)
		req.Header.Set("Authorization", "Bearer dlk_nonexistent_token_12345")
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"device-link token not found"}`, rr.Body.String())
	})

	t.Run("Failure - Invalid device-link token data", func(t *testing.T) {
		t.Parallel()
		c, _, db := setupTestPKIController(t)

		token := "dlk_test_token_12345"
		db.KVSet("g8e:device-link:"+token, "invalid json", 0)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"invalid device-link token data"}`, rr.Body.String())
	})

	t.Run("Failure - Device-link token missing expiry", func(t *testing.T) {
		t.Parallel()
		c, _, db := setupTestPKIController(t)

		token := "dlk_test_token_12345"
		linkData := map[string]interface{}{
			"user_id": "user-123",
		}
		linkJSON, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token, string(linkJSON), 0)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"device-link token missing expiry"}`, rr.Body.String())
	})

	t.Run("Failure - Invalid device-link token expiry format", func(t *testing.T) {
		t.Parallel()
		c, _, db := setupTestPKIController(t)

		token := "dlk_test_token_12345"
		linkData := map[string]interface{}{
			"expires_at": "invalid-date",
		}
		linkJSON, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token, string(linkJSON), 0)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"invalid device-link token expiry"}`, rr.Body.String())
	})

	t.Run("Failure - Device-link token expired", func(t *testing.T) {
		t.Parallel()
		c, _, db := setupTestPKIController(t)

		token := "dlk_test_token_12345"
		linkData := map[string]interface{}{
			"expires_at": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		}
		linkJSON, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token, string(linkJSON), 0)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"device-link token expired"}`, rr.Body.String())
	})

	t.Run("Failure - Invalid request JSON", func(t *testing.T) {
		t.Parallel()
		c, _, db := setupTestPKIController(t)

		token := "dlk_test_token_12345"
		linkData := map[string]interface{}{
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		}
		linkJSON, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token, string(linkJSON), 0)

		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"invalid JSON"}`, rr.Body.String())
	})

	t.Run("Failure - App enrollment validation error", func(t *testing.T) {
		t.Parallel()
		c, _, db := setupTestPKIController(t)

		token := "dlk_test_token_12345"
		linkData := map[string]interface{}{
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		}
		linkJSON, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token, string(linkJSON), 0)

		body := map[string]string{
			"csr_pem":  "",
			"app_name": "",
			"app_type": "",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/pki/app-enroll", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		c.handleAppEnroll(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		if success, ok := resp["success"].(bool); ok {
			assert.False(t, success)
		}
		assert.NotEmpty(t, resp["error"])
	})
}

func TestPKIController_ReadBody(t *testing.T) {
	t.Run("Success - Reads body within limit", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		body := []byte("test body content")
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))

		read, err := c.readBody(req)
		require.NoError(t, err)
		assert.Equal(t, body, read)
	})

	t.Run("Failure - Body exceeds max payload", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		largeBody := make([]byte, c.cfg.Gateway.MaxPayloadBytes+1)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(largeBody))

		_, err := c.readBody(req)
		assert.Error(t, err)
	})
}
