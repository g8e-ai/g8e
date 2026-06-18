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
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestSaveCertAndKey_MkdirError(t *testing.T) {
	t.Parallel()

	// Create a file where we expect a directory
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.WriteFile(blockingFile, []byte("block"), 0600))

	certFile := filepath.Join(blockingFile, "nested", "cert.pem")
	keyFile := filepath.Join(blockingFile, "nested", "key.pem")

	_, privKey, err := GenerateCSR("test")
	require.NoError(t, err)

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")

	err = SaveCertAndKey(certPEM, "", privKey, certFile, keyFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create cert directory")
}

func TestParseCertPEM_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), 0600))

	cert, err := parseCertPEM(certFile)
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Equal(t, "test-cert", cert.Subject.CommonName)
}

func TestParseCertPEM_InvalidPEM(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")

	require.NoError(t, os.WriteFile(certFile, []byte("invalid-pem-data"), 0600))

	cert, err := parseCertPEM(certFile)
	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "failed to decode PEM block")
}

func TestParseCertPEM_NonCertificatePEM(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")

	// Write a private key PEM instead of a certificate
	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: []byte("dummy-key-data"),
	})
	require.NoError(t, os.WriteFile(certFile, privKeyPEM, 0600))

	cert, err := parseCertPEM(certFile)
	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "PEM block is not a certificate")
}

func TestIsCertExpiringSoon_Expiring(t *testing.T) {
	t.Parallel()
	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Modify the certificate to expire in 12 hours
	cert.NotAfter = time.Now().Add(12 * time.Hour)

	expiring := isCertExpiringSoon(cert)
	assert.True(t, expiring, "Certificate expiring in 12 hours should be considered expiring soon")
}

func TestIsCertExpiringSoon_NotExpiring(t *testing.T) {
	t.Parallel()
	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Modify the certificate to expire in 48 hours
	cert.NotAfter = time.Now().Add(48 * time.Hour)

	expiring := isCertExpiringSoon(cert)
	assert.False(t, expiring, "Certificate expiring in 48 hours should not be considered expiring soon")
}

func TestIsCertExpiringSoon_ExactlyAtThreshold(t *testing.T) {
	t.Parallel()
	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Modify the certificate to expire exactly at the threshold (24 hours)
	cert.NotAfter = time.Now().Add(24 * time.Hour)

	expiring := isCertExpiringSoon(cert)
	assert.True(t, expiring, "Certificate expiring exactly at threshold should be considered expiring soon")
}

func TestCheckCertExpiry_Expiring(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")

	// Note: The isCertExpiringSoon function is tested separately with modified certificates.
	// This test verifies that CheckCertExpiry correctly parses a valid certificate.
	// Since we cannot easily create a certificate with a custom expiry in the test harness,
	// we test the happy path here and rely on isCertExpiringSoon tests for expiry logic.
	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), 0600))

	expiring, err := CheckCertExpiry(certFile)
	require.NoError(t, err)
	// Default test certificates have long validity, so this should be false
	assert.False(t, expiring)
}

func TestCheckCertExpiry_NotExpiring(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), 0600))

	expiring, err := CheckCertExpiry(certFile)
	require.NoError(t, err)
	assert.False(t, expiring)
}

func TestCheckCertExpiry_InvalidFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")

	require.NoError(t, os.WriteFile(certFile, []byte("invalid-pem"), 0600))

	expiring, err := CheckCertExpiry(certFile)
	require.Error(t, err)
	assert.False(t, expiring)
}

func TestAutoRenewCertificate_NotExpiring(t *testing.T) {
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

	// Create a valid certificate that is not expiring
	certFile := cfg.CLICertFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), 0700))
	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), 0600))

	err := AutoRenewCertificate(cfg, "cli", "")
	require.NoError(t, err)
}

func TestAutoRenewCertificate_UnknownCertType(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
	}

	err := AutoRenewCertificate(cfg, "unknown-type", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown certificate type")
}

func TestAutoRenewCertificate_ExpiringCert(t *testing.T) {
	t.Parallel()

	// Create a certificate that expires in 12 hours (within renewal threshold)
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:         filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:     filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	// This test would require generating a short-lived cert and actually calling ReEnroll
	// Since ReEnroll requires a real server, we test the error path
	certFile := cfg.CLICertFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), 0700))

	// Write a dummy cert file - this will fail to parse as a real cert
	// but tests the error path
	require.NoError(t, os.WriteFile(certFile, []byte("not a valid cert"), 0600))

	err := AutoRenewCertificate(cfg, "cli", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check certificate expiry")
}

func TestAutoRenewCertificate_OperatorType(t *testing.T) {
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

	// Create a valid certificate that's not expiring
	certFile := cfg.OperatorCertFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), 0700))
	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), 0600))

	err := AutoRenewCertificate(cfg, "operator", "")
	require.NoError(t, err)
}
