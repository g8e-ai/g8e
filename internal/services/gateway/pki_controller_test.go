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

const (
	testDeviceLinkTokenPrefix = "dlk_"
	testMinTokenLength        = 10
	testOrganizationID        = "org-123"
	testOperatorID            = "op-456"
	testUserID                = "user-789"
	testWorkloadSessionID     = "ws-012"
	testAppName               = "test-app"
	testAppType               = "mcp-client"
	testSerial                = "test-serial-123"
	testRevocationReason      = "key-compromise"
)

var (
	testValidToken = testDeviceLinkTokenPrefix + "test_token_12345"
)

type httpTestCase struct {
	name           string
	method         string
	body           []byte
	headers        map[string]string
	setup          func(*testing.T, *PKIController, *GatewayDBService)
	expectedStatus int
	expectedBody   string
	validateResp   func(*testing.T, *httptest.ResponseRecorder)
}

func setupTestPKIController(t *testing.T) (*PKIController, *config.Config, *GatewayDBService) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()

	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err, "failed to open gateway DB service")
	t.Cleanup(func() { db.Close() })

	require.NoError(t, os.RemoveAll(secretsDir), "failed to clean secrets dir")
	require.NoError(t, os.MkdirAll(secretsDir, 0755), "failed to create secrets dir")

	backend, err := keystore.NewTestBackend()
	require.NoError(t, err, "failed to create test keystore backend")

	ks, err := keystore.NewWithBackend(t.TempDir(), logger, backend)
	require.NoError(t, err, "failed to create keystore")
	require.NoError(t, ks.Initialize(), "failed to initialize keystore")
	require.NoError(t, ks.EnsurePermissions(), "failed to ensure keystore permissions")

	sm := &SecretManager{
		db:         db.db,
		secretsDir: t.TempDir(),
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	require.NoError(t, pki.EnsurePKI(nil), "failed to ensure PKI")

	appEnrollment := NewAppEnrollmentService(db, pki, logger)
	resp := responder.New(logger)

	controller := newPKIController(cfg, logger, db, pki, appEnrollment, resp)
	return controller, cfg, db
}

func createValidDeviceLink(t *testing.T, db *GatewayDBService, token string, expiresAt time.Time) {
	t.Helper()

	linkData := map[string]interface{}{
		"expires_at": expiresAt.Format(time.RFC3339),
		"user_id":    testUserID,
	}
	linkJSON, err := json.Marshal(linkData)
	require.NoError(t, err, "failed to marshal device-link data")

	db.KVSet("g8e:device-link:"+token, string(linkJSON), 0)
}

func runHTTPTest(t *testing.T, tc httpTestCase, handler func(*httptest.ResponseRecorder, *http.Request)) {
	t.Helper()

	c, _, db := setupTestPKIController(t)

	if tc.setup != nil {
		tc.setup(t, c, db)
	}

	req := httptest.NewRequest(tc.method, "/api/pki/test", bytes.NewReader(tc.body))
	for k, v := range tc.headers {
		req.Header.Set(k, v)
	}

	rr := httptest.NewRecorder()
	handler(rr, req)

	assert.Equal(t, tc.expectedStatus, rr.Code, "expected status code mismatch")

	if tc.expectedBody != "" {
		assert.JSONEq(t, tc.expectedBody, rr.Body.String(), "expected body mismatch")
	}

	if tc.validateResp != nil {
		tc.validateResp(t, rr)
	}
}

func TestPKIController_HandlePKIHubBundle(t *testing.T) {
	tests := []httpTestCase{
		{
			name:           "Success - GET returns PEM bundle",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				assert.Equal(t, "application/x-pem-file", rr.Header().Get("Content-Type"))
				assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
				assert.Equal(t, "DENY", rr.Header().Get("X-Frame-Options"))
				assert.NotEmpty(t, rr.Body.Bytes(), "response body should not be empty")
				assert.Contains(t, rr.Body.String(), "BEGIN CERTIFICATE", "body should contain PEM certificate")
			},
		},
		{
			name:           "Failure - POST method not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   `{"error":"method not allowed"}`,
		},
		{
			name:   "Failure - PKI error returns 500",
			method: http.MethodGet,
			setup: func(t *testing.T, c *PKIController, _ *GatewayDBService) {
				c.pki = &PKIAuthority{}
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"failed to read hub bundle"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runHTTPTest(t, tc, func(rr *httptest.ResponseRecorder, req *http.Request) {
				c, _, _ := setupTestPKIController(t)
				if tc.setup != nil {
					tc.setup(t, c, nil)
				}
				c.handlePKIHubBundle(rr, req)
			})
		})
	}
}

func TestPKIController_HandlePKIFingerprint(t *testing.T) {
	tests := []httpTestCase{
		{
			name:           "Success - GET returns SHA256 fingerprint",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp map[string]string
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err, "failed to unmarshal response")
				assert.NotEmpty(t, resp["root_ca"], "root_ca fingerprint should not be empty")
				assert.Contains(t, resp["root_ca"], "sha256:", "fingerprint should contain sha256 prefix")
			},
		},
		{
			name:           "Failure - POST method not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   `{"error":"method not allowed"}`,
		},
		{
			name:   "Failure - Root CA file not found",
			method: http.MethodGet,
			setup: func(t *testing.T, c *PKIController, _ *GatewayDBService) {
				c.pki = &PKIAuthority{}
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"failed to read root CA"}`,
		},
		{
			name:   "Failure - Invalid PEM format",
			method: http.MethodGet,
			setup: func(t *testing.T, c *PKIController, _ *GatewayDBService) {
				pkiDir := c.pki.PKIDir()
				rootPath := filepath.Join(pkiDir, "root", "root_ca.crt")
				err := os.WriteFile(rootPath, []byte("invalid pem data"), 0644)
				require.NoError(t, err, "failed to write invalid PEM data")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"invalid root CA PEM"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runHTTPTest(t, tc, func(rr *httptest.ResponseRecorder, req *http.Request) {
				c, _, _ := setupTestPKIController(t)
				if tc.setup != nil {
					tc.setup(t, c, nil)
				}
				c.handlePKIFingerprint(rr, req)
			})
		})
	}
}

func TestPKIController_HandlePKISignCSR(t *testing.T) {
	validCSRPayload := map[string]string{
		"csr_pem":             generateTestCSR(t),
		"leaf_type":           "operator",
		"organization_id":     testOrganizationID,
		"operator_id":         testOperatorID,
		"user_id":             testUserID,
		"workload_session_id": testWorkloadSessionID,
	}

	tests := []httpTestCase{
		{
			name:           "Success - POST signs CSR and returns cert",
			method:         http.MethodPost,
			body:           mustMarshalJSON(t, validCSRPayload),
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp map[string]string
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err, "failed to unmarshal response")
				assert.NotEmpty(t, resp["certificate_pem"], "certificate_pem should not be empty")
				assert.NotEmpty(t, resp["certificate_chain_pem"], "certificate_chain_pem should not be empty")
				assert.Contains(t, resp["certificate_pem"], "BEGIN CERTIFICATE", "certificate should contain PEM header")
			},
		},
		{
			name:           "Failure - GET method not allowed",
			method:         http.MethodGet,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   `{"error":"method not allowed"}`,
		},
		{
			name:           "Failure - Invalid JSON",
			method:         http.MethodPost,
			body:           []byte("invalid json"),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"invalid JSON"}`,
		},
		{
			name:   "Failure - PKI signing error",
			method: http.MethodPost,
			body: mustMarshalJSON(t, map[string]string{
				"csr_pem":   "invalid csr",
				"leaf_type": "operator",
			}),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runHTTPTest(t, tc, func(rr *httptest.ResponseRecorder, req *http.Request) {
				c, _, _ := setupTestPKIController(t)
				if tc.setup != nil {
					tc.setup(t, c, nil)
				}
				c.handlePKISignCSR(rr, req)
			})
		})
	}
}

func TestPKIController_HandlePKIRevoke(t *testing.T) {
	validRevokePayload := map[string]string{
		"serial": testSerial,
		"reason": testRevocationReason,
	}

	tests := []httpTestCase{
		{
			name:           "Success - POST revokes certificate",
			method:         http.MethodPost,
			body:           mustMarshalJSON(t, validRevokePayload),
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err, "failed to unmarshal response")
				assert.Equal(t, "ok", resp["status"], "status should be ok")
			},
		},
		{
			name:           "Failure - GET method not allowed",
			method:         http.MethodGet,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   `{"error":"method not allowed"}`,
		},
		{
			name:           "Failure - Invalid JSON",
			method:         http.MethodPost,
			body:           []byte("invalid json"),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"invalid JSON"}`,
		},
		{
			name:           "Failure - Missing serial",
			method:         http.MethodPost,
			body:           mustMarshalJSON(t, map[string]string{"reason": testRevocationReason}),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"serial required"}`,
		},
		{
			name:   "Failure - PKI revocation error",
			method: http.MethodPost,
			body:   mustMarshalJSON(t, validRevokePayload),
			setup: func(t *testing.T, c *PKIController, _ *GatewayDBService) {
				c.pki = &PKIAuthority{}
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runHTTPTest(t, tc, func(rr *httptest.ResponseRecorder, req *http.Request) {
				c, _, _ := setupTestPKIController(t)
				if tc.setup != nil {
					tc.setup(t, c, nil)
				}
				c.handlePKIRevoke(rr, req)
			})
		})
	}
}

func TestPKIController_HandlePKIRevocationBundle(t *testing.T) {
	tests := []httpTestCase{
		{
			name:           "Success - GET returns revocation bundle",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp map[string]string
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err, "failed to unmarshal response")
				assert.NotEmpty(t, resp["bundle_json"], "bundle_json should not be empty")
				assert.NotEmpty(t, resp["signature"], "signature should not be empty")
			},
		},
		{
			name:           "Failure - POST method not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   `{"error":"method not allowed"}`,
		},
		{
			name:   "Failure - PKI bundle generation error",
			method: http.MethodGet,
			setup: func(t *testing.T, c *PKIController, _ *GatewayDBService) {
				c.pki = &PKIAuthority{}
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runHTTPTest(t, tc, func(rr *httptest.ResponseRecorder, req *http.Request) {
				c, _, _ := setupTestPKIController(t)
				if tc.setup != nil {
					tc.setup(t, c, nil)
				}
				c.handlePKIRevocationBundle(rr, req)
			})
		})
	}
}

func TestPKIController_HandleAppEnroll(t *testing.T) {
	validEnrollPayload := map[string]string{
		"csr_pem":  generateTestCSR(t),
		"app_name": testAppName,
		"app_type": testAppType,
	}

	tests := []httpTestCase{
		{
			name:    "Success - POST enrolls app with valid device-link token",
			method:  http.MethodPost,
			body:    mustMarshalJSON(t, validEnrollPayload),
			headers: map[string]string{"Authorization": "Bearer " + testValidToken},
			setup: func(t *testing.T, _ *PKIController, db *GatewayDBService) {
				createValidDeviceLink(t, db, testValidToken, time.Now().Add(1*time.Hour))
			},
			expectedStatus: http.StatusCreated,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err, "failed to unmarshal response")
				assert.True(t, resp["success"].(bool), "success should be true")
				assert.NotEmpty(t, resp["app_cert"], "app_cert should not be empty")
				assert.NotEmpty(t, resp["cert_chain"], "cert_chain should not be empty")
				assert.NotEmpty(t, resp["app_id"], "app_id should not be empty")
			},
		},
		{
			name:           "Failure - GET method not allowed",
			method:         http.MethodGet,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   `{"error":"method not allowed"}`,
		},
		{
			name:   "Failure - App enrollment service not available",
			method: http.MethodPost,
			setup: func(t *testing.T, c *PKIController, _ *GatewayDBService) {
				c.appEnrollment = nil
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   `{"error":"app enrollment service not available"}`,
		},
		{
			name:           "Failure - Missing bearer token",
			method:         http.MethodPost,
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"missing bearer token"}`,
		},
		{
			name:           "Failure - Invalid token format (no dlk_ prefix)",
			method:         http.MethodPost,
			headers:        map[string]string{"Authorization": "Bearer invalid_token"},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"invalid device-link token format"}`,
		},
		{
			name:           "Failure - Token too short",
			method:         http.MethodPost,
			headers:        map[string]string{"Authorization": "Bearer dlk_short"},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"invalid device-link token format"}`,
		},
		{
			name:           "Failure - Device-link token not found",
			method:         http.MethodPost,
			headers:        map[string]string{"Authorization": "Bearer dlk_nonexistent_token_12345"},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"device-link token not found"}`,
		},
		{
			name:    "Failure - Invalid device-link token data",
			method:  http.MethodPost,
			headers: map[string]string{"Authorization": "Bearer " + testValidToken},
			setup: func(t *testing.T, _ *PKIController, db *GatewayDBService) {
				db.KVSet("g8e:device-link:"+testValidToken, "invalid json", 0)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"invalid device-link token data"}`,
		},
		{
			name:    "Failure - Device-link token missing expiry",
			method:  http.MethodPost,
			headers: map[string]string{"Authorization": "Bearer " + testValidToken},
			setup: func(t *testing.T, _ *PKIController, db *GatewayDBService) {
				linkData := map[string]interface{}{"user_id": testUserID}
				linkJSON, err := json.Marshal(linkData)
				require.NoError(t, err, "failed to marshal device-link data")
				db.KVSet("g8e:device-link:"+testValidToken, string(linkJSON), 0)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"device-link token missing expiry"}`,
		},
		{
			name:    "Failure - Invalid device-link token expiry format",
			method:  http.MethodPost,
			headers: map[string]string{"Authorization": "Bearer " + testValidToken},
			setup: func(t *testing.T, _ *PKIController, db *GatewayDBService) {
				linkData := map[string]interface{}{"expires_at": "invalid-date"}
				linkJSON, err := json.Marshal(linkData)
				require.NoError(t, err, "failed to marshal device-link data")
				db.KVSet("g8e:device-link:"+testValidToken, string(linkJSON), 0)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"invalid device-link token expiry"}`,
		},
		{
			name:    "Failure - Device-link token expired",
			method:  http.MethodPost,
			headers: map[string]string{"Authorization": "Bearer " + testValidToken},
			setup: func(t *testing.T, _ *PKIController, db *GatewayDBService) {
				createValidDeviceLink(t, db, testValidToken, time.Now().Add(-1*time.Hour))
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"device-link token expired"}`,
		},
		{
			name:    "Failure - Invalid request JSON",
			method:  http.MethodPost,
			body:    []byte("invalid json"),
			headers: map[string]string{"Authorization": "Bearer " + testValidToken},
			setup: func(t *testing.T, _ *PKIController, db *GatewayDBService) {
				createValidDeviceLink(t, db, testValidToken, time.Now().Add(1*time.Hour))
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"invalid JSON"}`,
		},
		{
			name:   "Failure - App enrollment validation error",
			method: http.MethodPost,
			body: mustMarshalJSON(t, map[string]string{
				"csr_pem":  "",
				"app_name": "",
				"app_type": "",
			}),
			headers: map[string]string{"Authorization": "Bearer " + testValidToken},
			setup: func(t *testing.T, _ *PKIController, db *GatewayDBService) {
				createValidDeviceLink(t, db, testValidToken, time.Now().Add(1*time.Hour))
			},
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err, "failed to unmarshal response")
				if success, ok := resp["success"].(bool); ok {
					assert.False(t, success, "success should be false")
				}
				assert.NotEmpty(t, resp["error"], "error should not be empty")
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runHTTPTest(t, tc, func(rr *httptest.ResponseRecorder, req *http.Request) {
				c, _, db := setupTestPKIController(t)
				if tc.setup != nil {
					tc.setup(t, c, db)
				}
				c.handleAppEnroll(rr, req)
			})
		})
	}
}

func TestPKIController_ReadBody(t *testing.T) {
	t.Run("Success - Reads body within limit", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		body := []byte("test body content")
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))

		read, err := c.readBody(req)
		require.NoError(t, err, "failed to read body")
		assert.Equal(t, body, read, "read body should match input")
	})

	t.Run("Failure - Body exceeds max payload", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestPKIController(t)

		largeBody := make([]byte, c.cfg.Gateway.MaxPayloadBytes+1)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(largeBody))

		_, err := c.readBody(req)
		assert.Error(t, err, "should return error for oversized body")
	})
}

func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err, "failed to marshal JSON")
	return b
}
