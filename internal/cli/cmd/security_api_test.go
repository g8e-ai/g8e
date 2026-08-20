// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestPEMCert creates a self-signed PEM certificate for testing.
func generateTestPEMCert(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-root"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageCertSign,
		IsCA:         true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	return pemBytes
}

// --- Security Validate with valid PEM ---

func TestSecurityValidateCmd_API_ValidPEMSuccess(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	pkiDir := filepath.Join(tmpDir, "pki")
	secretsDir := filepath.Join(tmpDir, "secrets")

	validCert := generateTestPEMCert(t)

	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "root"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "trust"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.crt"), validCert, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem"), validCert, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "warden_pub.pem"), []byte("dummy warden"), 0644))

	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "bootstrap_digest.json"), []byte("{}"), 0644))

	cmd := securityValidateCmdWithConfig(newFileSvc)
	cmd.Flags().Set("pki-dir", pkiDir)
	cmd.Flags().Set("secrets-dir", secretsDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Security validation passed")
	assert.Contains(t, buf.String(), "root_ca.crt is valid PEM")
	assert.Contains(t, buf.String(), "Trust bundle is valid PEM")
}

func TestSecurityValidateCmd_API_InvalidPEM(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	pkiDir := filepath.Join(tmpDir, "pki")
	secretsDir := filepath.Join(tmpDir, "secrets")

	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "root"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "trust"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.crt"), []byte("not a cert"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem"), []byte("not a bundle"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "warden_pub.pem"), []byte("dummy warden"), 0644))

	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "bootstrap_digest.json"), []byte("{}"), 0644))

	cmd := securityValidateCmdWithConfig(newFileSvc)
	cmd.Flags().Set("pki-dir", pkiDir)
	cmd.Flags().Set("secrets-dir", secretsDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Contains(t, buf.String(), "root_ca.crt is not a valid PEM")
	assert.Contains(t, buf.String(), "Trust bundle is not valid PEM")
}

func TestSecurityValidateCmd_API_PortCheckOutput(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	pkiDir := filepath.Join(tmpDir, "pki")
	secretsDir := filepath.Join(tmpDir, "secrets")

	validCert := generateTestPEMCert(t)

	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "root"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "trust"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.crt"), validCert, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem"), validCert, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "warden_pub.pem"), []byte("dummy warden"), 0644))

	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "bootstrap_digest.json"), []byte("{}"), 0644))

	cmd := securityValidateCmdWithConfig(newFileSvc)
	cmd.Flags().Set("pki-dir", pkiDir)
	cmd.Flags().Set("secrets-dir", secretsDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Port Availability")
	assert.Contains(t, output, fmt.Sprintf("Port %d", constants.Ports.OperatorHttp))
	assert.Contains(t, output, fmt.Sprintf("Port %d", constants.Ports.OperatorHttps))
}
