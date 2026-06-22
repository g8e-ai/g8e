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

package auth

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBootstrap_Success tests the successful bootstrap flow with a mock server.
func TestBootstrap_Success(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := testutil.GenerateTestCertificate(t, "test-ca")
	_ = keyPEM
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/auth/bootstrap", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req models.BootstrapRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.NotEmpty(t, req.CSR)
		assert.NotEmpty(t, req.CLICSR)
		assert.NotEmpty(t, req.SystemFingerprint)

		resp := RegistrationResponse{
			Success:           true,
			OperatorSessionID: "op-sess-123",
			CLISessionID:      "cli-sess-456",
			OperatorID:        "op-789",
			OperatorCert:      certPEM,
			CLICert:           certPEM,
			HubTrustBundle:    certPEM,
			UserID:            "user-abc",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	// Test the success case - the function should successfully bootstrap
	resp, err := BootstrapWithURL(cfg, operatorCSR, cliCSR, "", server.URL)
	require.NoError(t, err)
	assert.Equal(t, "op-sess-123", resp.OperatorSessionID)
	assert.Equal(t, "cli-sess-456", resp.CLISessionID)
	assert.Equal(t, "op-789", resp.OperatorID)
	assert.Equal(t, "user-abc", resp.UserID)
}

func TestBootstrap_HTTPError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	// Use httptest.Server to simulate connection error by closing immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection immediately to simulate network error
		hijacker, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hijacker.Hijack()
			conn.Close()
		}
	}))
	defer server.Close()

	resp, err := BootstrapWithURL(cfg, operatorCSR, cliCSR, "", server.URL+"/api/v1/auth/bootstrap")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrEnrollmentFailed))
}

func TestBootstrap_ErrorResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RegistrationResponse{
			Success: false,
			Error:   "invalid CSR format",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	resp, err := BootstrapWithURL(cfg, operatorCSR, cliCSR, "", server.URL)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrEnrollmentFailed))
}

func TestBootstrap_InvalidJSONResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid-json{{{"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	resp, err := BootstrapWithURL(cfg, operatorCSR, cliCSR, "", server.URL)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrInvalidJSONResponse))
}

func TestBootstrap_FingerprintVerification(t *testing.T) {
	t.Parallel()

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-ca")
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)
	hash := sha256.Sum256(block.Bytes)
	expectedFP := hex.EncodeToString(hash[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RegistrationResponse{
			Success:        true,
			OperatorCert:   certPEM,
			CLICert:        certPEM,
			HubTrustBundle: certPEM,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	// Test with correct fingerprint
	resp, err := BootstrapWithURL(cfg, operatorCSR, cliCSR, expectedFP, server.URL)
	require.NoError(t, err)
	assert.NotNil(t, resp)

	// Test with wrong fingerprint
	resp, err = BootstrapWithURL(cfg, operatorCSR, cliCSR, "deadbeef", server.URL)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrValidationFailed))
}

func TestEnrollWithGateway_Success(t *testing.T) {
	t.Parallel()

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-ca")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/auth/device/enroll", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var req map[string]string
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.NotEmpty(t, req["csr_pem"])
		assert.NotEmpty(t, req["cli_csr_pem"])
		assert.NotEmpty(t, req["system_fingerprint"])
		assert.NotEmpty(t, req["hostname"])

		resp := RegistrationResponse{
			Success:        true,
			OperatorCert:   certPEM,
			CLICert:        certPEM,
			HubTrustBundle: certPEM,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	// Extract host:port from server URL
	serverURL := strings.TrimPrefix(server.URL, "http://")

	resp, err := EnrollWithGateway(cfg, serverURL, operatorCSR, cliCSR, "")
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.OperatorCert)
	assert.NotEmpty(t, resp.CLICert)
}

func TestEnrollWithGateway_NonSuccessResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RegistrationResponse{
			Success: false,
			Error:   "enrollment failed: invalid credentials",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	serverURL := strings.TrimPrefix(server.URL, "http://")

	resp, err := EnrollWithGateway(cfg, serverURL, operatorCSR, cliCSR, "")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrEnrollmentFailed))
}

func TestCLIEnroll_Success(t *testing.T) {
	t.Parallel()

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-ca")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/auth/cli/enroll", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req models.CLIEnrollRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.NotEmpty(t, req.CLICSR)
		assert.NotEmpty(t, req.SystemFingerprint)

		resp := RegistrationResponse{
			Success:      true,
			CLISessionID: "cli-sess-456",
			CLICert:      certPEM,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	resp, err := CLIEnroll(cfg, cliCSR, server.URL)
	require.NoError(t, err)
	assert.Equal(t, "cli-sess-456", resp.CLISessionID)
	assert.NotEmpty(t, resp.CLICert)
}

func TestCLIEnroll_HTTPError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	// Use httptest.Server to simulate connection error by closing immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection immediately to simulate network error
		hijacker, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hijacker.Hijack()
			conn.Close()
		}
	}))
	defer server.Close()

	resp, err := CLIEnroll(cfg, cliCSR, server.URL+"/api/v1/auth/cli/enroll")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrEnrollmentFailed))
}

func TestCLIEnroll_ErrorResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RegistrationResponse{
			Success: false,
			Error:   "CLI enrollment failed: invalid CSR",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	resp, err := CLIEnroll(cfg, cliCSR, server.URL)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrEnrollmentFailed))
}

func TestCLIEnroll_InvalidJSONResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid-json{{{"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	resp, err := CLIEnroll(cfg, cliCSR, server.URL)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrInvalidJSONResponse))
}

func TestEnrollWithGateway_HTTPError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	// Use a port that's not listening
	resp, err := EnrollWithGateway(cfg, "localhost:59999", operatorCSR, cliCSR, "")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrHTTPRequestExecuteFailed))
}

func TestEnrollWithGateway_BadStatusCode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	serverURL := strings.TrimPrefix(server.URL, "http://")

	resp, err := EnrollWithGateway(cfg, serverURL, operatorCSR, cliCSR, "")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrHTTPStatusError))
}

func TestEnrollWithGateway_FingerprintVerification(t *testing.T) {
	t.Parallel()

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-ca")
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)
	hash := sha256.Sum256(block.Bytes)
	expectedFP := hex.EncodeToString(hash[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RegistrationResponse{
			Success:        true,
			OperatorCert:   certPEM,
			CLICert:        certPEM,
			HubTrustBundle: certPEM,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	serverURL := strings.TrimPrefix(server.URL, "http://")

	// Test with correct fingerprint
	resp, err := EnrollWithGateway(cfg, serverURL, operatorCSR, cliCSR, expectedFP)
	require.NoError(t, err)
	assert.NotNil(t, resp)

	// Test with wrong fingerprint
	resp, err = EnrollWithGateway(cfg, serverURL, operatorCSR, cliCSR, "deadbeef")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, constants.ErrValidationFailed))
}

func TestCheckBootstrapStatus_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/auth/bootstrap/status", r.URL.Path)
		resp := map[string]bool{"bootstrapped": true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	bootstrapped, err := CheckBootstrapStatus(cfg, server.URL)
	require.NoError(t, err)
	assert.True(t, bootstrapped)
}

func TestCheckBootstrapStatus_NotBootstrapped(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/auth/bootstrap/status", r.URL.Path)
		resp := map[string]bool{"bootstrapped": false}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	bootstrapped, err := CheckBootstrapStatus(cfg, server.URL)
	require.NoError(t, err)
	assert.False(t, bootstrapped)
}

func TestCheckBootstrapStatus_HTTPError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	// When gateway is not reachable, CheckBootstrapStatus returns false without error
	bootstrapped, err := CheckBootstrapStatus(cfg, "")
	require.NoError(t, err)
	assert.False(t, bootstrapped)
}

func TestCheckBootstrapStatus_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/auth/bootstrap/status", r.URL.Path)
		w.Write([]byte("invalid-json{{{"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	bootstrapped, err := CheckBootstrapStatus(cfg, server.URL)
	require.Error(t, err)
	assert.False(t, bootstrapped)
	assert.True(t, errors.Is(err, constants.ErrInvalidJSONResponse))
}

func TestReEnroll_TrustBundleFetchError(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	trustBundlePath := filepath.Join(tmpDir, "trust-bundle.pem")
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = trustBundlePath

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	// Use httptest.Server to simulate connection error by closing immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection immediately to simulate network error
		hijacker, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hijacker.Hijack()
			conn.Close()
		}
	}))
	defer server.Close()

	_, err = ReEnroll(cfg, operatorCSR, cliCSR, "", server.URL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrHTTPRequestExecuteFailed))
}

func TestReEnroll_TrustBundleEmpty(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/.well-known/g8e/pki/ca-bundle", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("")) // Empty response
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	_, err = ReEnroll(cfg, operatorCSR, cliCSR, "", server.URL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEmptyTrustBundle))
}

func TestReEnroll_TrustBundleBadStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/.well-known/g8e/pki/ca-bundle", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	_, err = ReEnroll(cfg, operatorCSR, cliCSR, "", server.URL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrHTTPStatusError))
}

func TestReEnroll_CLICertLoadError(t *testing.T) {
	t.Parallel()

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-ca")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/.well-known/g8e/pki/ca-bundle", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(certPEM))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = filepath.Join(tmpDir, "trust-bundle.pem")

	// Don't create CLI cert/key files - they should be missing
	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	_, err = ReEnroll(cfg, operatorCSR, cliCSR, "", server.URL)
	require.Error(t, err)
	// The error should be related to missing CLI certificate
	assert.True(t, errors.Is(err, constants.ErrFailedToLoadClientCertificate))
}

func TestReEnroll_InvalidCAPEM(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/.well-known/g8e/pki/ca-bundle", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid-pem-data"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = filepath.Join(tmpDir, "trust-bundle.pem")

	// Create valid matching CLI cert/key files
	certPEM, keyPEM := testutil.GenerateTestCertificate(t, "test-cli")

	// Re-parse the keyPEM to *ecdsa.PrivateKey as required by SaveCertAndKey
	block, _ := pem.Decode([]byte(keyPEM))
	require.NotNil(t, block)
	privKey, err := x509.ParseECPrivateKey(block.Bytes)
	require.NoError(t, err)

	err = SaveCertAndKey(certPEM, "", privKey, cfg.CLICertFile(), cfg.CLIKeyFile())
	require.NoError(t, err)

	operatorCSR, _, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	cliCSR, _, err := GenerateCSR("test-cli")
	require.NoError(t, err)

	_, err = ReEnroll(cfg, operatorCSR, cliCSR, "", server.URL)
	require.Error(t, err)
	// The error should be related to the invalid CA bundle
	assert.True(t, errors.Is(err, constants.ErrCAParseFailed))
}
