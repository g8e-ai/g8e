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
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/gateway/scripts"
	"github.com/g8e-ai/g8e/internal/testutil"
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

	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()

	ks := newTestKeystore(t, secretsDir, logger)
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, constants.VaultDirname), logger, true, "", false, ks)
	require.NoError(t, err, "failed to open gateway DB service")
	t.Cleanup(func() { db.Close() })

	sm := &SecretManager{
		db:         db.db,
		secretsDir: secretsDir,
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

	// Create minimal registration service for tests that need it
	userSvc := NewUserService(db, logger)
	cliSessionSvc := NewCLISessionService(db, logger)
	operatorSessionSvc := NewOperatorSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)

	controller := newPKIController(cfg, logger, db, pki, appEnrollment, reg, resp)
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

// makeTestSpiffeCert returns a self-signed cert with a SPIFFE URI SAN so that
// ExtractUserIDFromCert succeeds with the given userID.
func makeTestSpiffeCert(t *testing.T, userID string) *x509.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	spiffeURI, err := url.Parse("spiffe://g8e.local/cli/" + userID + "/cli-session-test")
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-cert"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{spiffeURI},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cert
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
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
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
			},
		},
		{
			name:   "Failure - Invalid PEM format",
			method: http.MethodGet,
			setup: func(t *testing.T, c *PKIController, _ *CanonicalDBService) {
				pkiDir := c.pki.PKIDir()
				rootPath := filepath.Join(pkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCA)
				err := os.WriteFile(rootPath, []byte("invalid pem data"), 0644)
				require.NoError(t, err, "failed to write invalid PEM data")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"failed to decode PEM block"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
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
			},
		},
		{
			name:           "Failure - Missing serial",
			method:         http.MethodPost,
			body:           mustMarshalJSON(t, map[string]string{"reason": testRevocationReason}),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"missing required field"}`,
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
		c, _, _ := setupTestPKIController(t)

		body := []byte("test body content")
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))

		read, err := c.readBody(req)
		require.NoError(t, err, "failed to read body")
		assert.Equal(t, body, read, "read body should match input")
	})

	t.Run("Failure - Body exceeds max payload", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)

		largeBody := make([]byte, c.cfg.Gateway.MaxPayloadBytes+1)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(largeBody))

		_, err := c.readBody(req)
		require.Error(t, err, "should return error for oversized body")
	})
}

func TestNewPKIController(t *testing.T) {
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
	c, _, _ := setupTestPKIController(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/pki/ca-bundle", nil)
	rr := httptest.NewRecorder()

	c.handlePKICABundle(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-pem-file", rr.Header().Get("Content-Type"))
	assert.NotEmpty(t, rr.Body.Bytes())
}

func TestPKIController_HandleTrustScriptWindows(t *testing.T) {

	t.Run("Success - GET returns Windows script", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/pki/trust-windows", nil)
		rr := httptest.NewRecorder()

		c.handleTrustScriptWindows(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/x-powershell", rr.Header().Get("Content-Type"))
		assert.NotEmpty(t, rr.Body.Bytes())
		script := rr.Body.String()
		assert.Contains(t, script, "CA bundle installed")
		assert.NotContains(t, script, "Downloading g8e Node")
	})

	t.Run("Success - GET with X-Forwarded-Host uses external host", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/pki/trust-windows", nil)
		req.Header.Set("X-Forwarded-Host", "192.168.1.62")
		rr := httptest.NewRecorder()

		c.handleTrustScriptWindows(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		script := rr.Body.String()
		assert.Contains(t, script, "192.168.1.62")
		assert.NotContains(t, script, "localhost")
	})

	t.Run("Success - GET with Host header uses that host", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/pki/trust-windows", nil)
		req.Host = "192.168.1.62:8080"
		rr := httptest.NewRecorder()

		c.handleTrustScriptWindows(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		script := rr.Body.String()
		assert.Contains(t, script, "192.168.1.62")
		assert.NotContains(t, script, "localhost")
	})

	t.Run("Success - GET with localhost uses LocalAddrContextKey IP", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/pki/trust-windows", nil)
		req.Host = "localhost:8080"
		// Set LocalAddrContextKey to simulate a non-loopback server address
		localAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.62"), Port: 8080}
		ctx := context.WithValue(req.Context(), http.LocalAddrContextKey, localAddr)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		c.handleTrustScriptWindows(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		script := rr.Body.String()
		assert.Contains(t, script, "192.168.1.62")
		assert.NotContains(t, script, "localhost")
	})
}

func TestPKIController_HandleTrustScriptLinux(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/pki/trust-linux", nil)
	rr := httptest.NewRecorder()

	c.handleTrustScriptLinux(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-sh", rr.Header().Get("Content-Type"))
	assert.NotEmpty(t, rr.Body.Bytes())
	script := rr.Body.String()
	assert.Contains(t, script, "CA bundle installed")
	assert.Contains(t, script, "CA bundle trusted system-wide")
	assert.Contains(t, script, "update-ca-certificates")
	assert.NotContains(t, script, "Downloading g8e Node")
}

func TestPKIController_HandleTrustScriptWindowsAlias(t *testing.T) {
	tests := []httpTestCase{
		{
			name:           "Failure - POST method not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   `{"error":"method not allowed"}`,
		},
		{
			name:           "Success - GET returns Windows script",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				assert.Equal(t, "application/x-powershell", rr.Header().Get("Content-Type"))
				assert.NotEmpty(t, rr.Body.Bytes())
				script := rr.Body.String()
				assert.Contains(t, script, "CA bundle installed")
				assert.NotContains(t, script, "Downloading g8e Node")
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runHTTPTest(t, tc, func(rr *httptest.ResponseRecorder, req *http.Request) {
				c, _, _ := setupTestPKIController(t)
				if tc.setup != nil {
					tc.setup(t, c, nil)
				}
				c.handleTrustScriptWindowsAlias(rr, req)
			})
		})
	}
}

func TestPKIController_HandleNodeBinaryDownload(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

	// Create binaries directory and a test binary
	binaryDir := filepath.Join(c.pki.PKIDir(), constants.PkiSubdirBinaries)
	require.NoError(t, os.MkdirAll(binaryDir, 0755))
	testNodeBinaryPath := filepath.Join(binaryDir, "g8e-windows-amd64.exe") // Test-specific binary name, acceptable as test data
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
	c, _, _ := setupTestPKIController(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/bin/g8e-linux-amd64", nil)
	rr := httptest.NewRecorder()

	c.handleNodeBinaryDownload(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPKIController_HandleNodeBinaryDownload_InvalidName(t *testing.T) {
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

func TestPKIController_HandleDeployScriptLinux(t *testing.T) {

	t.Run("Failure - POST method not allowed", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/g8e-deploy.sh", nil)
		rr := httptest.NewRecorder()
		c.handleDeployScriptLinux(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Success - GET returns Linux deploy script with GATEWAY_HOST", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, "/g8e-deploy.sh", nil)
		req.Host = "test.example.com"
		rr := httptest.NewRecorder()
		c.handleDeployScriptLinux(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/x-sh", rr.Header().Get("Content-Type"))
		assert.NotEmpty(t, rr.Body.Bytes())
		script := rr.Body.String()
		assert.Contains(t, script, "GATEWAY_HOST")
		assert.Contains(t, script, "test.example.com")
	})
}

func TestPKIController_HandleDeployScriptWindows(t *testing.T) {

	t.Run("Failure - POST method not allowed", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/g8e-deploy.ps1", nil)
		rr := httptest.NewRecorder()
		c.handleDeployScriptWindows(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Success - GET with X-Forwarded-Host uses external host", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, "/g8e-deploy.ps1", nil)
		req.Header.Set("X-Forwarded-Host", "external.host")
		rr := httptest.NewRecorder()
		c.handleDeployScriptWindows(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/x-powershell", rr.Header().Get("Content-Type"))
		script := rr.Body.String()
		assert.Contains(t, script, "external.host")
	})

	t.Run("Success - GET with localhost uses LocalAddrContextKey IP", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, "/g8e-deploy.ps1", nil)
		req.Host = "localhost:8080"
		// Set LocalAddrContextKey to simulate a non-loopback server address
		localAddr := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 8080}
		ctx := context.WithValue(req.Context(), http.LocalAddrContextKey, localAddr)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		c.handleDeployScriptWindows(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/x-powershell", rr.Header().Get("Content-Type"))
		script := rr.Body.String()
		assert.Contains(t, script, "10.0.0.1")
	})
}

func TestPKIController_HandlePKIAppsEnroll(t *testing.T) {

	t.Run("Failure - GET method not allowed", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pki/apps/enroll", nil)
		rr := httptest.NewRecorder()
		c.handlePKIAppsEnroll(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Failure - app enrollment service not available", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		db := &CanonicalDBService{}
		pki := &PKIAuthority{}
		resp := response.NewWriter(logger)

		// Initialize script templates
		if err := scripts.Init(logger); err != nil {
			t.Fatalf("Failed to initialize script templates: %v", err)
		}

		controller := newPKIController(cfg, logger, db, pki, nil, nil, resp)

		validPayload := map[string]string{
			"csr_pem":  testutil.GenerateTestCSRP256(t, "test-app"),
			"app_name": "test-app",
			"app_type": "mcp-client",
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/apps/enroll", bytes.NewReader(mustMarshalJSON(t, validPayload)))
		rr := httptest.NewRecorder()
		controller.handlePKIAppsEnroll(rr, req)
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.JSONEq(t, `{"error":"service unavailable"}`, rr.Body.String())
	})

	t.Run("Failure - malformed JSON", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/apps/enroll", bytes.NewReader([]byte("invalid json")))
		rr := httptest.NewRecorder()
		c.handlePKIAppsEnroll(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Success - valid CSR request", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		validPayload := map[string]string{
			"csr_pem":  testutil.GenerateTestCSRP256(t, "test-app"),
			"app_name": "test-app",
			"app_type": "mcp-client",
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/apps/enroll", bytes.NewReader(mustMarshalJSON(t, validPayload)))
		rr := httptest.NewRecorder()
		c.handlePKIAppsEnroll(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp AppEnrollResponse
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.AppCert)
		assert.NotEmpty(t, resp.CertChain)
		assert.NotEmpty(t, resp.AppID)
	})
}

func TestPKIController_HandlePKIDevicesEnroll(t *testing.T) {

	t.Run("Failure - GET method not allowed", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pki/devices/enroll", nil)
		rr := httptest.NewRecorder()
		c.handlePKIDevicesEnroll(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - no TLS", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/devices/enroll", nil)
		rr := httptest.NewRecorder()
		c.handlePKIDevicesEnroll(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Failure - empty peer certificates", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/devices/enroll", nil)
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{}}
		rr := httptest.NewRecorder()
		c.handlePKIDevicesEnroll(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Failure - invalid JSON body", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/devices/enroll", bytes.NewReader([]byte("{invalid")))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, "user-123")}}
		rr := httptest.NewRecorder()
		c.handlePKIDevicesEnroll(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - missing CSR", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		payload := map[string]string{}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/devices/enroll", bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, "user-123")}}
		rr := httptest.NewRecorder()
		c.handlePKIDevicesEnroll(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestPKIController_HandlePKIAppsDelegated(t *testing.T) {

	t.Run("Failure - GET method not allowed", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pki/apps/delegated", nil)
		rr := httptest.NewRecorder()
		c.handlePKIAppsDelegated(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Failure - no TLS connection", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/apps/delegated", nil)
		rr := httptest.NewRecorder()
		c.handlePKIAppsDelegated(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"missing certificate"}`, rr.Body.String())
	})

	t.Run("Failure - empty peer certificates", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/apps/delegated", nil)
		req.TLS = &tls.ConnectionState{}
		rr := httptest.NewRecorder()
		c.handlePKIAppsDelegated(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"missing certificate"}`, rr.Body.String())
	})

	t.Run("Failure - invalid JSON body", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/apps/delegated", strings.NewReader("{invalid}"))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, "user-123")}}
		rr := httptest.NewRecorder()
		c.handlePKIAppsDelegated(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - missing CSR", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		body := map[string]string{"app_name": "test-app"}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/apps/delegated", bytes.NewReader(b))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, "user-123")}}
		rr := httptest.NewRecorder()
		c.handlePKIAppsDelegated(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"missing required field"}`, rr.Body.String())
	})

	t.Run("Failure - missing app_name", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		body := map[string]string{"csr_pem": testutil.GenerateTestCSRP256(t, "test-app")}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/apps/delegated", bytes.NewReader(b))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, "user-123")}}
		rr := httptest.NewRecorder()
		c.handlePKIAppsDelegated(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"missing required field"}`, rr.Body.String())
	})

	t.Run("Failure - invalid app name (special characters)", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		body := map[string]string{
			"csr_pem":  testutil.GenerateTestCSRP256(t, "test-app"),
			"app_name": "invalid@name",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/apps/delegated", bytes.NewReader(b))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, "user-123")}}
		rr := httptest.NewRecorder()
		c.handlePKIAppsDelegated(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - invalid CSR PEM format", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		body := map[string]string{
			"csr_pem":  "not-a-valid-csr",
			"app_name": "test-app",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pki/apps/delegated", bytes.NewReader(b))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, "user-123")}}
		rr := httptest.NewRecorder()
		c.handlePKIAppsDelegated(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"pki: invalid CSR PEM"}`, rr.Body.String())
	})
}

func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err, "failed to marshal JSON")
	return b
}
