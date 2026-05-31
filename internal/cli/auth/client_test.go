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
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// extractPortFromURL extracts the port number from a httptest server URL
func extractPortFromURL(url string) int {
	// httptest URLs are like "http://127.0.0.1:12345"
	// Split by "://" first to get the host:port part
	parts := strings.Split(url, "://")
	if len(parts) < 2 {
		return 0
	}
	// Then split by ":" to get the port
	hostPort := parts[1]
	portParts := strings.Split(hostPort, ":")
	if len(portParts) < 2 {
		return 0
	}
	var port int
	fmt.Sscanf(portParts[1], "%d", &port)
	return port
}

// ---------------------------------------------------------------------------
// GenerateCSR
// ---------------------------------------------------------------------------

func TestGenerateCSR_Success(t *testing.T) {
	t.Parallel()
	csrPEM, privKey, err := GenerateCSR("test-common-name")

	require.NoError(t, err)
	require.NotNil(t, privKey)
	assert.NotEmpty(t, csrPEM)

	// Verify it's a valid PEM block
	block, _ := pem.Decode([]byte(csrPEM))
	require.NotNil(t, block)
	assert.Equal(t, "CERTIFICATE REQUEST", block.Type)

	// Verify it's a valid CSR
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, "test-common-name", csr.Subject.CommonName)
	assert.Equal(t, []string{"g8e"}, csr.Subject.Organization)
}

func TestGenerateCSR_DifferentCommonNames(t *testing.T) {
	t.Parallel()
	testCases := []string{
		"operator-1",
		"cli-device",
		"test-node.example.com",
	}

	for _, cn := range testCases {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			csrPEM, privKey, err := GenerateCSR(cn)

			require.NoError(t, err)
			require.NotNil(t, privKey)
			assert.NotEmpty(t, csrPEM)

			block, _ := pem.Decode([]byte(csrPEM))
			require.NotNil(t, block)
			csr, err := x509.ParseCertificateRequest(block.Bytes)
			require.NoError(t, err)
			assert.Equal(t, cn, csr.Subject.CommonName)
		})
	}
}

// ---------------------------------------------------------------------------
// NewSecureHTTPClient
// ---------------------------------------------------------------------------

func TestNewSecureHTTPClient_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	trustBundlePath := filepath.Join(tmpDir, "trust-bundle.pem")

	// Generate a test CA certificate
	caPEM, _ := testutil.GenerateTestCertificate(t, "test-ca")
	require.NoError(t, os.WriteFile(trustBundlePath, []byte(caPEM), 0600))

	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = trustBundlePath

	client, err := NewSecureHTTPClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify TLS config is set correctly
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.Equal(t, uint16(tls.VersionTLS13), transport.TLSClientConfig.MinVersion)
}

func TestNewSecureHTTPClient_MissingTrustBundlePath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	client, err := NewSecureHTTPClient(cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "trust bundle path not configured")
}

func TestNewSecureHTTPClient_InvalidPEM(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	trustBundlePath := filepath.Join(tmpDir, "trust-bundle.pem")

	require.NoError(t, os.WriteFile(trustBundlePath, []byte("invalid-pem-data"), 0600))

	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = trustBundlePath

	client, err := NewSecureHTTPClient(cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "failed to parse CA certificates")
}

// ---------------------------------------------------------------------------
// Bootstrap
// ---------------------------------------------------------------------------
// Note: Bootstrap has complex CA download logic that is difficult to test with httptest.
// This function is tested via integration/e2e tests with a real Operator.

// ---------------------------------------------------------------------------
// ReEnroll
// ---------------------------------------------------------------------------
// Note: ReEnroll requires mTLS with existing certificates and is tested via integration tests.

// ---------------------------------------------------------------------------
// CheckBootstrapStatus
// ---------------------------------------------------------------------------
// Note: CheckBootstrapStatus requires TLS and is tested via integration tests.

// ---------------------------------------------------------------------------
// SaveCredentials / LoadCredentials
// ---------------------------------------------------------------------------

func TestSaveAndLoadCredentials(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
	}

	creds := &Credentials{
		OperatorSessionID: "op-sess-123",
		UserID:            "user-456",
		OperatorID:        "op-789",
		CLISessionID:      "cli-sess-abc",
	}

	err := SaveCredentials(cfg, creds)
	require.NoError(t, err)

	loaded, err := LoadCredentials(cfg)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, creds.OperatorSessionID, loaded.OperatorSessionID)
	assert.Equal(t, creds.UserID, loaded.UserID)
	assert.Equal(t, creds.OperatorID, loaded.OperatorID)
	assert.Equal(t, creds.CLISessionID, loaded.CLISessionID)
}

func TestLoadCredentials_NotFound(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
	}

	loaded, err := LoadCredentials(cfg)
	require.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestLoadCredentials_InvalidJSON(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
	}

	credsFile := cfg.CredentialsFile()
	require.NoError(t, os.MkdirAll(cfg.CredentialsDir, 0700))
	require.NoError(t, os.WriteFile(credsFile, []byte("invalid-json{{{"), 0600))

	loaded, err := LoadCredentials(cfg)
	require.Error(t, err)
	assert.Nil(t, loaded)
	assert.Contains(t, err.Error(), "failed to parse credentials")
}

// ---------------------------------------------------------------------------
// DeleteCredentials
// ---------------------------------------------------------------------------

func TestDeleteCredentials_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths: &config.PathsConfig{
			Infra: struct {
				AppCertDir           string `json:"app_cert_dir"`
				CACertPath           string `json:"ca_cert_path"`
				DBPath               string `json:"db_path"`
				DocsDir              string `json:"docs_dir"`
				PKIDir               string `json:"pki_dir"`
				ProtocolConstantsDir string `json:"protocol_constants_dir"`
				ProtocolDir          string `json:"protocol_dir"`
				ProtocolModelsDir    string `json:"protocol_models_dir"`
				SecretsDir           string `json:"secrets_dir"`
				SSHConfigPath        string `json:"ssh_config_path"`
			}{
				CACertPath: filepath.Join(tmpDir, ".g8e/pki/trust/g8e-gw-ca-bundle.pem"),
			},
		},
	}

	creds := &Credentials{
		OperatorSessionID: "op-sess-123",
		UserID:            "user-456",
		OperatorID:        "op-789",
		CLISessionID:      "cli-sess-abc",
	}

	require.NoError(t, SaveCredentials(cfg, creds))

	certDir := cfg.CredentialsDir
	require.NoError(t, os.MkdirAll(certDir, 0700))
	require.NoError(t, os.WriteFile(cfg.CLICertFile(), []byte("cli-cert"), 0600))
	require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), []byte("cli-key"), 0600))
	require.NoError(t, os.WriteFile(cfg.OperatorCertFile(), []byte("op-cert"), 0600))
	require.NoError(t, os.WriteFile(cfg.OperatorKeyFile(), []byte("op-key"), 0600))
	hubBundle := cfg.TrustBundlePath()
	require.NoError(t, os.MkdirAll(filepath.Dir(hubBundle), 0700))
	require.NoError(t, os.WriteFile(hubBundle, []byte("g8e-gw-ca-bundle"), 0600))

	err := DeleteCredentials(cfg)
	require.NoError(t, err)

	assert.NoFileExists(t, cfg.CredentialsFile())
	assert.NoFileExists(t, cfg.CLICertFile())
	assert.NoFileExists(t, cfg.CLIKeyFile())
	assert.NoFileExists(t, cfg.OperatorCertFile())
	assert.NoFileExists(t, cfg.OperatorKeyFile())
	assert.NoFileExists(t, hubBundle)
}

func TestDeleteCredentials_NonExistentFiles(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths: &config.PathsConfig{
			Infra: struct {
				AppCertDir           string `json:"app_cert_dir"`
				CACertPath           string `json:"ca_cert_path"`
				DBPath               string `json:"db_path"`
				DocsDir              string `json:"docs_dir"`
				PKIDir               string `json:"pki_dir"`
				ProtocolConstantsDir string `json:"protocol_constants_dir"`
				ProtocolDir          string `json:"protocol_dir"`
				ProtocolModelsDir    string `json:"protocol_models_dir"`
				SecretsDir           string `json:"secrets_dir"`
				SSHConfigPath        string `json:"ssh_config_path"`
			}{
				CACertPath: filepath.Join(tmpDir, ".g8e/pki/trust/g8e-gw-ca-bundle.pem"),
			},
		},
	}

	err := DeleteCredentials(cfg)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// SaveCertAndKey
// ---------------------------------------------------------------------------

func TestSaveCertAndKey_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	_, privKey, err := GenerateCSR("test")
	require.NoError(t, err)

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	chainPEM, _ := testutil.GenerateTestCertificate(t, "test-chain")

	err = SaveCertAndKey(certPEM, chainPEM, privKey, certFile, keyFile)
	require.NoError(t, err)

	certData, err := os.ReadFile(certFile)
	require.NoError(t, err)
	assert.Contains(t, string(certData), "BEGIN CERTIFICATE")

	keyData, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	assert.Contains(t, string(keyData), "EC PRIVATE KEY")
}

func TestSaveCertAndKey_NoChain(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	_, privKey, err := GenerateCSR("test")
	require.NoError(t, err)

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")

	err = SaveCertAndKey(certPEM, "", privKey, certFile, keyFile)
	require.NoError(t, err)

	certData, err := os.ReadFile(certFile)
	require.NoError(t, err)
	assert.Contains(t, string(certData), "BEGIN CERTIFICATE")
}

func TestSaveCertAndKey_CreatesDirectory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir", "nested")
	certFile := filepath.Join(subDir, "cert.pem")
	keyFile := filepath.Join(subDir, "key.pem")

	_, privKey, err := GenerateCSR("test")
	require.NoError(t, err)

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")

	err = SaveCertAndKey(certPEM, "", privKey, certFile, keyFile)
	require.NoError(t, err)

	assert.FileExists(t, certFile)
	assert.FileExists(t, keyFile)
}

// ---------------------------------------------------------------------------
// CheckOperatorRunning
// ---------------------------------------------------------------------------

func TestCheckOperatorRunning_NotRunning(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	err := CheckOperatorRunning(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator is not running or not responding")
}

func TestCheckOperatorRunning_HealthCheckFailed(t *testing.T) {
	t.Parallel()

	// Test with a non-existent port
	err := CheckOperatorRunningAtURL("https://localhost:99999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running or not responding")
}

func TestCheckOperatorRunning_Success(t *testing.T) {
	t.Parallel()

	// Start a test server on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	url := fmt.Sprintf("http://127.0.0.1:%s", port)
	err = CheckOperatorRunningAtURL(url)
	require.NoError(t, err)
}

func TestCheckOperatorRunning_InvalidURL(t *testing.T) {
	t.Parallel()

	err := CheckOperatorRunningAtURL("invalid-url")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid operator URL")
}

func TestCheckOperatorRunning_URLWithoutProtocol(t *testing.T) {
	t.Parallel()

	err := CheckOperatorRunningAtURL("localhost:8440")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid operator URL")
}
