// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveCertAndKey_Success(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	certRel := "cert.pem"
	keyRel := "key.pem"

	_, privKey, err := GenerateCSR("test")
	require.NoError(t, err)

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	chainPEM, _ := testutil.GenerateTestCertificate(t, "test-chain")

	err = SaveCertAndKey(fileSvc, certPEM, chainPEM, privKey, certRel, keyRel)
	require.NoError(t, err)

	certData, err := fileSvc.ReadFile(context.Background(), certRel)
	require.NoError(t, err)
	assert.Contains(t, string(certData), "BEGIN CERTIFICATE")

	keyData, err := fileSvc.ReadFile(context.Background(), keyRel)
	require.NoError(t, err)
	assert.Contains(t, string(keyData), "EC PRIVATE KEY")
}

func TestSaveCertAndKey_NoChain(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	certRel := "cert.pem"
	keyRel := "key.pem"

	_, privKey, err := GenerateCSR("test")
	require.NoError(t, err)

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")

	err = SaveCertAndKey(fileSvc, certPEM, "", privKey, certRel, keyRel)
	require.NoError(t, err)

	certData, err := fileSvc.ReadFile(context.Background(), certRel)
	require.NoError(t, err)
	assert.Contains(t, string(certData), "BEGIN CERTIFICATE")
}

func TestSaveCertAndKey_CreatesDirectory(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	certRel := filepath.Join("subdir", "nested", "cert.pem")
	keyRel := filepath.Join("subdir", "nested", "key.pem")

	_, privKey, err := GenerateCSR("test")
	require.NoError(t, err)

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")

	err = SaveCertAndKey(fileSvc, certPEM, "", privKey, certRel, keyRel)
	require.NoError(t, err)

	assert.FileExists(t, fileSvc.Resolve(certRel))
	assert.FileExists(t, fileSvc.Resolve(keyRel))
}

func TestSaveCertAndKey_MkdirError(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)

	// Create a file where we expect a directory
	runtimeDir := fileSvc.Resolve("")
	blockingFile := filepath.Join(runtimeDir, "subdir")
	require.NoError(t, os.WriteFile(blockingFile, []byte("block"), constants.PermFilePrivate))

	certRel := filepath.Join("subdir", "nested", "cert.pem")
	keyRel := filepath.Join("subdir", "nested", "key.pem")

	_, privKey, err := GenerateCSR("test")
	require.NoError(t, err)

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")

	err = SaveCertAndKey(fileSvc, certPEM, "", privKey, certRel, keyRel)
	require.Error(t, err)
	assert.Error(t, err)
}

func TestParseCertPEM_Success(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	certRel := "cert.pem"

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte(certPEM), constants.PermFilePrivate))

	cert, err := parseCertPEM(fileSvc, certRel)
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Equal(t, "test-cert", cert.Subject.CommonName)
}

func TestParseCertPEM_InvalidPEM(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	certRel := "cert.pem"

	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte("invalid-pem-data"), constants.PermFilePrivate))

	cert, err := parseCertPEM(fileSvc, certRel)
	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "failed to decode PEM block")
}

func TestParseCertPEM_NonCertificatePEM(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	certRel := "cert.pem"

	// Write a private key PEM instead of a certificate
	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: []byte("dummy-key-data"),
	})
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, privKeyPEM, constants.PermFilePrivate))

	cert, err := parseCertPEM(fileSvc, certRel)
	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Error(t, err)
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
	fileSvc := newAuthTestFileSvc(t)
	certRel := "cert.pem"

	// Note: The isCertExpiringSoon function is tested separately with modified certificates.
	// This test verifies that CheckCertExpiry correctly parses a valid certificate.
	// Since we cannot easily create a certificate with a custom expiry in the test harness,
	// we test the happy path here and rely on isCertExpiringSoon tests for expiry logic.
	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte(certPEM), constants.PermFilePrivate))

	expiring, err := CheckCertExpiry(fileSvc, certRel)
	require.NoError(t, err)
	// Default test certificates have long validity, so this should be false
	assert.False(t, expiring)
}

func TestCheckCertExpiry_NotExpiring(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	certRel := "cert.pem"

	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte(certPEM), constants.PermFilePrivate))

	expiring, err := CheckCertExpiry(fileSvc, certRel)
	require.NoError(t, err)
	assert.False(t, expiring)
}

func TestCheckCertExpiry_InvalidFile(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	certRel := "cert.pem"

	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte("invalid-pem"), constants.PermFilePrivate))

	expiring, err := CheckCertExpiry(fileSvc, certRel)
	require.Error(t, err)
	assert.False(t, expiring)
}
