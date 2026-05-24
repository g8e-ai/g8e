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
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
// RequestDeviceLink
// ---------------------------------------------------------------------------
// Note: RequestDeviceLink and RegisterDeviceLink make HTTP calls to the configured
// OperatorBootstrapURL. These functions require either:
// 1. Dependency injection of an HTTP client for unit testing, or
// 2. Integration testing with a real test server
// For now, these are tested via integration/e2e tests rather than unit tests.

// ---------------------------------------------------------------------------
// SaveCredentials / LoadCredentials
// ---------------------------------------------------------------------------

func TestSaveAndLoadCredentials(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, ".g8e"),
		PKIDir:         filepath.Join(tmpDir, ".g8e", "pki"),
		SecretsDir:     filepath.Join(tmpDir, ".g8e", "secrets"),
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
		RuntimeDir:     filepath.Join(tmpDir, ".g8e"),
		PKIDir:         filepath.Join(tmpDir, ".g8e", "pki"),
		SecretsDir:     filepath.Join(tmpDir, ".g8e", "secrets"),
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
		RuntimeDir:     filepath.Join(tmpDir, ".g8e"),
		PKIDir:         filepath.Join(tmpDir, ".g8e", "pki"),
		SecretsDir:     filepath.Join(tmpDir, ".g8e", "secrets"),
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
		RuntimeDir:     filepath.Join(tmpDir, ".g8e"),
		PKIDir:         filepath.Join(tmpDir, ".g8e", "pki"),
		SecretsDir:     filepath.Join(tmpDir, ".g8e", "secrets"),
		CredentialsDir: tmpDir,
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
	hubBundle := filepath.Join(cfg.CredentialsDir, "hub-bundle.pem")
	require.NoError(t, os.WriteFile(hubBundle, []byte("hub-bundle"), 0600))

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
		RuntimeDir:     filepath.Join(tmpDir, ".g8e"),
		PKIDir:         filepath.Join(tmpDir, ".g8e", "pki"),
		SecretsDir:     filepath.Join(tmpDir, ".g8e", "secrets"),
		CredentialsDir: tmpDir,
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
		RuntimeDir:     filepath.Join(tmpDir, ".g8e"),
		PKIDir:         filepath.Join(tmpDir, ".g8e", "pki"),
		SecretsDir:     filepath.Join(tmpDir, ".g8e", "secrets"),
		CredentialsDir: tmpDir,
	}

	err := CheckOperatorRunning(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator is not running")
}

func TestCheckOperatorRunning_InvalidPID(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, ".g8e"),
		PKIDir:         filepath.Join(tmpDir, ".g8e", "pki"),
		SecretsDir:     filepath.Join(tmpDir, ".g8e", "secrets"),
		CredentialsDir: tmpDir,
	}

	pidDir := filepath.Join(cfg.RuntimeDir, "pids")
	require.NoError(t, os.MkdirAll(pidDir, 0700))
	pidFile := filepath.Join(pidDir, "operator.pid")
	require.NoError(t, os.WriteFile(pidFile, []byte("invalid-pid"), 0600))

	err := CheckOperatorRunning(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse pid")
}

func TestCheckOperatorRunning_ProcessNotRunning(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, ".g8e"),
		PKIDir:         filepath.Join(tmpDir, ".g8e", "pki"),
		SecretsDir:     filepath.Join(tmpDir, ".g8e", "secrets"),
		CredentialsDir: tmpDir,
	}

	pidDir := filepath.Join(cfg.RuntimeDir, "pids")
	require.NoError(t, os.MkdirAll(pidDir, 0700))
	pidFile := filepath.Join(pidDir, "operator.pid")
	require.NoError(t, os.WriteFile(pidFile, []byte("99999"), 0600))

	err := CheckOperatorRunning(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator process not running")
}
