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

	dbDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)

	ks := newTestKeystore(t, fileSvc, logger)
	db, stores, err := OpenCanonicalDBService(dbDir, fileSvc.Resolve(constants.VaultDirname), logger, "", ks, fileSvc)
	require.NoError(t, err, "failed to open gateway DB service")
	t.Cleanup(func() { db.Close() })

	sm := db.GetSecretManager()

	pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
	require.NoError(t, pki.InitializePKI(nil), "failed to ensure PKI")

	appEnrollment := NewAppEnrollmentService(stores.DocStore, pki, logger)
	resp := response.NewWriter(logger)

	// Initialize script templates
	if err := scripts.Init(logger); err != nil {
		t.Fatalf("Failed to initialize script templates: %v", err)
	}

	// Create minimal registration service for tests that need it
	userSvc := NewUserService(stores.DocStore, logger)
	cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
	operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
	reg := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)

	controller := newPKIController(PKIControllerDeps{Cfg: cfg, Logger: logger, PKI: pki, AppEnrollment: appEnrollment, Registration: reg, Responder: resp})
	return controller, cfg, db
}

// setupMinimalPKIController creates a PKIController with only config, logger,
// and responder — no database, no PKI authority, no enrollment or registration
// services. Use this for handler tests that do not touch the database (e.g.
// deploy script rendering). Avoiding OpenCanonicalDBService eliminates the
// two-SQLite-DB teardown whose WAL-checkpoint fsync can block indefinitely
// under CI I/O contention.
func setupMinimalPKIController(t *testing.T) *PKIController {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	resp := response.NewWriter(logger)
	if err := scripts.Init(logger); err != nil {
		t.Fatalf("Failed to initialize script templates: %v", err)
	}
	return newPKIController(PKIControllerDeps{Cfg: cfg, Logger: logger, Responder: resp})
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
				c.pki = &PKIAuthority{fileSvc: newTestFileSvc(t)}
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
				c.pki = &PKIAuthority{fileSvc: newTestFileSvc(t)}
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
				relPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA)
				err := c.pki.fileSvc.WriteFile(context.Background(), relPath, []byte("invalid pem data"), constants.PermFilePublic)
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

	t.Run("Failure - no mTLS returns 401", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PKICSRSign, bytes.NewReader(mustMarshalJSON(t, validCSRPayload)))
		req.TLS = nil
		rr := httptest.NewRecorder()
		c.handlePKICSRSign(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"missing certificate"}`, rr.Body.String())
	})

	t.Run("Success - POST with mTLS signs CSR and returns cert", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PKICSRSign, bytes.NewReader(mustMarshalJSON(t, validCSRPayload)))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, testUserID)}}
		rr := httptest.NewRecorder()
		c.handlePKICSRSign(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["certificate_pem"], "certificate_pem should not be empty")
		assert.NotEmpty(t, resp["certificate_chain_pem"], "certificate_chain_pem should not be empty")
		assert.Contains(t, resp["certificate_pem"], "BEGIN CERTIFICATE", "certificate should contain PEM header")
	})

	t.Run("Failure - GET method not allowed", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.PKICSRSign, nil)
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, testUserID)}}
		rr := httptest.NewRecorder()
		c.handlePKICSRSign(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Failure - Invalid JSON", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PKICSRSign, bytes.NewReader([]byte("invalid json")))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, testUserID)}}
		rr := httptest.NewRecorder()
		c.handlePKICSRSign(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - PKI signing error", func(t *testing.T) {
		c, _, _ := setupTestPKIController(t)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PKICSRSign, bytes.NewReader(mustMarshalJSON(t, map[string]string{
			"csr_pem":   "invalid csr",
			"leaf_type": "operator",
		})))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestSpiffeCert(t, testUserID)}}
		rr := httptest.NewRecorder()
		c.handlePKICSRSign(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
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

	pki := &PKIAuthority{}
	appEnrollment := &AppEnrollmentService{}
	registration := &RegistrationService{}
	responder := &response.Writer{}

	controller := newPKIController(PKIControllerDeps{Cfg: cfg, Logger: logger, PKI: pki, AppEnrollment: appEnrollment, Registration: registration, Responder: responder})

	assert.NotNil(t, controller)
	assert.Equal(t, cfg, controller.cfg)
	assert.Equal(t, logger, controller.logger)
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

func TestPKIController_HandleNodeBinaryDownload(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

	// Create binaries directory and a test binary
	binaryRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirBinaries, "g8e-windows-amd64.exe")
	testNodeBinaryContent := []byte("test binary content")
	require.NoError(t, c.pki.fileSvc.WriteFile(context.Background(), binaryRelPath, testNodeBinaryContent, constants.PermFilePublic))

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

func TestPKIController_HandleNodeBinaryDownload_ImageBakedBinDir(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

	// Use a temp directory to simulate the image-baked /opt/g8e/bin directory
	// where the Dockerfile copies all platform binaries. The controller's
	// nodeBinariesDir field is overridden so the test does not require root
	// to write to /opt/g8e.
	binDir := t.TempDir()
	c.nodeBinariesDir = binDir

	testContent := []byte("image-baked binary content")
	binaryPath := filepath.Join(binDir, "g8e-darwin-arm64")
	require.NoError(t, os.WriteFile(binaryPath, testContent, constants.PermFilePublic))

	req := httptest.NewRequest(http.MethodGet, "/.well-known/g8e/bin/g8e-darwin-arm64", nil)
	rr := httptest.NewRecorder()

	c.handleNodeBinaryDownload(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/octet-stream", rr.Header().Get("Content-Type"))
	assert.Equal(t, "attachment; filename=g8e-darwin-arm64", rr.Header().Get("Content-Disposition"))
	assert.Equal(t, testContent, rr.Body.Bytes())
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
		c := setupMinimalPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/g8e-deploy.sh", nil)
		rr := httptest.NewRecorder()
		c.handleDeployScriptLinux(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Success - GET returns Linux deploy script with GATEWAY_HOST", func(t *testing.T) {
		c := setupMinimalPKIController(t)
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
		c := setupMinimalPKIController(t)
		req := httptest.NewRequest(http.MethodPost, "/g8e-deploy.ps1", nil)
		rr := httptest.NewRecorder()
		c.handleDeployScriptWindows(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Success - GET with X-Forwarded-Host uses external host", func(t *testing.T) {
		c := setupMinimalPKIController(t)
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
		c := setupMinimalPKIController(t)
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
		assert.JSONEq(t, `{"error":"csr_pem is required"}`, rr.Body.String())
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
		assert.JSONEq(t, `{"error":"app_name is required"}`, rr.Body.String())
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
		assert.JSONEq(t, `{"error":"invalid CSR PEM format"}`, rr.Body.String())
	})
}

func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err, "failed to marshal JSON")
	return b
}
