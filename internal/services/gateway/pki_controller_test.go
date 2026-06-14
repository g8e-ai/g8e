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
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/gateway/scripts"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testOrganizationID    = "org-123"
	testOperatorID        = "op-456"
	testUserID            = "user-789"
	testWorkloadSessionID = "ws-012"
	testAppName           = "test-app"
	testAppType           = "mcp-client"
	testSerial            = "test-serial-123"
	testRevocationReason  = "key-compromise"
)

type httpTestCase struct {
	name           string
	method         string
	body           []byte
	headers        map[string]string
	setup          func(*testing.T, *PKIController, *CanonicalDBService)
	expectedStatus int
	expectedBody   string
	validateResp   func(*testing.T, *httptest.ResponseRecorder)
}

func setupTestPKIController(t *testing.T) (*PKIController, *config.Config, *CanonicalDBService) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := tempDir(t)
	pkiDir := tempDir(t)
	secretsDir := tempDir(t)

	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false, nil)
	require.NoError(t, err, "failed to open gateway DB service")
	t.Cleanup(func() { db.Close() })

	require.NoError(t, os.RemoveAll(secretsDir), "failed to clean secrets dir")
	require.NoError(t, os.MkdirAll(secretsDir, 0755), "failed to create secrets dir")

	backend, err := keystore.NewTestBackend()
	require.NoError(t, err, "failed to create test keystore backend")

	ks, err := keystore.NewWithBackend(tempDir(t), logger, backend)
	require.NoError(t, err, "failed to create keystore")
	require.NoError(t, ks.Initialize(), "failed to initialize keystore")
	require.NoError(t, ks.EnforcePermissions(), "failed to enforce keystore permissions")

	sm := &SecretManager{
		db:         db.db,
		secretsDir: tempDir(t),
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	require.NoError(t, pki.InitializePKI(nil), "failed to ensure PKI")

	appEnrollment := NewAppEnrollmentService(db, pki, logger)
	resp := response.NewWriter(logger)

	// Initialize script templates
	if err := scripts.Init(logger); err != nil {
		t.Fatalf("Failed to initialize script templates: %v", err)
	}

	controller := newPKIController(cfg, logger, db, pki, appEnrollment, nil, resp)
	return controller, cfg, db
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
			setup: func(t *testing.T, c *PKIController, _ *CanonicalDBService) {
				c.pki = &PKIAuthority{}
			},
			expectedStatus: http.StatusInternalServerError,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp map[string]string
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Contains(t, resp["error"], "pki: read trust bundle")
			},
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
				c.handlePKICABundle(rr, req)
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
				assert.Len(t, resp["root_ca"], 64, "fingerprint should be 64 hex characters (SHA256)")
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
			setup: func(t *testing.T, c *PKIController, _ *CanonicalDBService) {
				c.pki = &PKIAuthority{}
			},
			expectedStatus: http.StatusInternalServerError,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp map[string]string
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Contains(t, resp["error"], "pki: read root CA")
			},
		},
		{
			name:   "Failure - Invalid PEM format",
			method: http.MethodGet,
			setup: func(t *testing.T, c *PKIController, _ *CanonicalDBService) {
				pkiDir := c.pki.PKIDir()
				rootPath := filepath.Join(pkiDir, "root", "root_ca.crt")
				err := os.WriteFile(rootPath, []byte("invalid pem data"), 0644)
				require.NoError(t, err, "failed to write invalid PEM data")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"pki: invalid root CA PEM"}`,
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
		"csr_pem":             testutil.GenerateTestCSRP256(t, "test-operator"),
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
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp map[string]string
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Contains(t, resp["error"], "pki: unmarshal CSR sign request")
			},
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
				c.handlePKICSRSign(rr, req)
			})
		})
	}
}

func TestPKIController_HandlePKICertificatesRevoke(t *testing.T) {
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
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp map[string]string
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Contains(t, resp["error"], "pki: unmarshal revoke request")
			},
		},
		{
			name:           "Failure - Missing serial",
			method:         http.MethodPost,
			body:           mustMarshalJSON(t, map[string]string{"reason": testRevocationReason}),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"pki: serial required"}`,
		},
		{
			name:   "Failure - PKI revocation error",
			method: http.MethodPost,
			body:   mustMarshalJSON(t, validRevokePayload),
			setup: func(t *testing.T, c *PKIController, _ *CanonicalDBService) {
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
				c.handlePKICertificatesRevoke(rr, req)
			})
		})
	}
}

func TestPKIController_HandlePKIRevocationBundle(t *testing.T) {
	tests := []httpTestCase{
		{
			name:           "Success - GET returns CRL",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				// After Phase 2, endpoint returns standard X.509 CRL (DER-encoded binary)
				crlDER := rr.Body.Bytes()
				assert.NotEmpty(t, crlDER, "CRL DER should not be empty")

				// Verify it's a valid CRL
				crl, err := x509.ParseRevocationList(crlDER)
				require.NoError(t, err, "response should be a valid X.509 CRL")
				assert.NotNil(t, crl, "CRL should parse successfully")
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
			setup: func(t *testing.T, c *PKIController, _ *CanonicalDBService) {
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
		require.Error(t, err, "should return error for oversized body")
	})
}

func TestNewPKIController(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Initialize script templates
	if err := scripts.Init(logger); err != nil {
		t.Fatalf("Failed to initialize script templates: %v", err)
	}

	db := &CanonicalDBService{}
	pki := &PKIAuthority{}
	appEnrollment := &AppEnrollmentService{}
	registration := &RegistrationService{}
	responder := &response.Writer{}

	controller := newPKIController(cfg, logger, db, pki, appEnrollment, registration, responder)

	assert.NotNil(t, controller)
	assert.Equal(t, cfg, controller.cfg)
	assert.Equal(t, logger, controller.logger)
	assert.Equal(t, db, controller.db)
	assert.Equal(t, pki, controller.pki)
	assert.Equal(t, appEnrollment, controller.appEnrollment)
	assert.Equal(t, registration, controller.registration)
	assert.Equal(t, responder, controller.responder)
}

func TestPKIController_HandlePKICABundle(t *testing.T) {
	t.Parallel()
	c, _, _ := setupTestPKIController(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/pki/ca-bundle", nil)
	rr := httptest.NewRecorder()

	c.handlePKICABundle(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-pem-file", rr.Header().Get("Content-Type"))
	assert.NotEmpty(t, rr.Body.Bytes())
}

func TestPKIController_HandleTrustScriptWindows(t *testing.T) {
	t.Parallel()
	c, _, _ := setupTestPKIController(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/pki/trust-windows", nil)
	rr := httptest.NewRecorder()

	c.handleTrustScriptWindows(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-powershell", rr.Header().Get("Content-Type"))
	assert.NotEmpty(t, rr.Body.Bytes())
	script := rr.Body.String()
	assert.Contains(t, script, "CA bundle installed")
	assert.Contains(t, script, "Download g8e Node")
	assert.Contains(t, script, "security pki enroll")
	assert.Contains(t, script, "Enrollment complete")
}

func TestPKIController_HandleTrustScriptLinux(t *testing.T) {
	t.Parallel()
	c, _, _ := setupTestPKIController(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/pki/trust-linux", nil)
	rr := httptest.NewRecorder()

	c.handleTrustScriptLinux(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-sh", rr.Header().Get("Content-Type"))
	assert.NotEmpty(t, rr.Body.Bytes())
	script := rr.Body.String()
	assert.Contains(t, script, "CA bundle installed")
	assert.Contains(t, script, "g8e auth login")
}

func TestPKIController_HandleNodeBinaryDownload(t *testing.T) {
	t.Parallel()
	c, _, _ := setupTestPKIController(t)

	// Create binaries directory and a test binary
	binaryDir := filepath.Join(c.pki.PKIDir(), constants.PkiSubdirBinaries)
	require.NoError(t, os.MkdirAll(binaryDir, 0755))
	testNodeBinaryPath := filepath.Join(binaryDir, "g8e-windows-amd64.exe")
	testNodeBinaryContent := []byte("test binary content")
	require.NoError(t, os.WriteFile(testNodeBinaryPath, testNodeBinaryContent, 0644))

	req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/bin/g8e-windows-amd64.exe", nil)
	rr := httptest.NewRecorder()

	c.handleNodeBinaryDownload(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/octet-stream", rr.Header().Get("Content-Type"))
	assert.Equal(t, "attachment; filename=g8e-windows-amd64.exe", rr.Header().Get("Content-Disposition"))
	assert.Equal(t, testNodeBinaryContent, rr.Body.Bytes())
}

func TestPKIController_HandleNodeBinaryDownload_NotFound(t *testing.T) {
	t.Parallel()
	c, _, _ := setupTestPKIController(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/bin/g8e-linux-amd64", nil)
	rr := httptest.NewRecorder()

	c.handleNodeBinaryDownload(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPKIController_HandleNodeBinaryDownload_InvalidName(t *testing.T) {
	t.Parallel()
	c, _, _ := setupTestPKIController(t)

	testCases := []string{
		"../../../etc/passwd",
		"malicious.exe",
		"g8e-unknown-os-amd64",
		"g8e-linux-unknown-arch",
		"random-file.txt",
		".hidden",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/bin/"+tc, nil)
			rr := httptest.NewRecorder()

			c.handleNodeBinaryDownload(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err, "failed to marshal JSON")
	return b
}
