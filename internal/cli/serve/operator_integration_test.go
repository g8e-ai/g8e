// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package serve

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// ---------------------------------------------------------------------------
// resolveKeyPath — filesystem-dependent tests
// ---------------------------------------------------------------------------

func TestResolveKeyPath_DefaultOperatorKey(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	opKeyRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorKey)
	require.NoError(t, fileSvc.WriteFile(context.Background(), opKeyRel, []byte("fake key"), constants.PermFilePrivate))

	result := resolveKeyPath("", fileSvc, testLogger())
	assert.Equal(t, fileSvc.Resolve(opKeyRel), result)
}

func TestResolveKeyPath_FallsBackToClientKey(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	cliKeyRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient, constants.PkiFileOperatorKey)
	require.NoError(t, fileSvc.WriteFile(context.Background(), cliKeyRel, []byte("fake key"), constants.PermFilePrivate))

	result := resolveKeyPath("", fileSvc, testLogger())
	assert.Equal(t, fileSvc.Resolve(cliKeyRel), result)
}

func TestResolveKeyPath_NoFilesFound(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	result := resolveKeyPath("", fileSvc, testLogger())
	assert.Equal(t, "", result)
}

func TestResolveKeyPath_OperatorKeyTakesPrecedenceOverClientKey(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	opKeyRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorKey)
	require.NoError(t, fileSvc.WriteFile(context.Background(), opKeyRel, []byte("op key"), constants.PermFilePrivate))
	cliKeyRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient, constants.PkiFileOperatorKey)
	require.NoError(t, fileSvc.WriteFile(context.Background(), cliKeyRel, []byte("client key"), constants.PermFilePrivate))

	result := resolveKeyPath("", fileSvc, testLogger())
	assert.Equal(t, fileSvc.Resolve(opKeyRel), result,
		"operator key should take precedence over client key when both exist")
}

func TestResolveKeyPath_ExplicitOverridesDefaults(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	opKeyRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorKey)
	require.NoError(t, fileSvc.WriteFile(context.Background(), opKeyRel, []byte("op key"), constants.PermFilePrivate))

	result := resolveKeyPath("/explicit/key.pem", fileSvc, testLogger())
	assert.Equal(t, "/explicit/key.pem", result,
		"explicit path should override default file lookup")
}

// ---------------------------------------------------------------------------
// resolveCertPath — filesystem-dependent tests
// ---------------------------------------------------------------------------

func TestResolveCertPath_DefaultOperatorCert(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	opCertRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorCert)
	require.NoError(t, fileSvc.WriteFile(context.Background(), opCertRel, []byte("fake cert"), constants.PermFilePrivate))

	result := resolveCertPath("", fileSvc, testLogger())
	assert.Equal(t, fileSvc.Resolve(opCertRel), result)
}

func TestResolveCertPath_FallsBackToClientCert(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	cliCertRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient, constants.PkiFileOperatorCert)
	require.NoError(t, fileSvc.WriteFile(context.Background(), cliCertRel, []byte("fake cert"), constants.PermFilePrivate))

	result := resolveCertPath("", fileSvc, testLogger())
	assert.Equal(t, fileSvc.Resolve(cliCertRel), result)
}

func TestResolveCertPath_NoFilesFound(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	result := resolveCertPath("", fileSvc, testLogger())
	assert.Equal(t, "", result)
}

func TestResolveCertPath_OperatorCertTakesPrecedenceOverClientCert(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	opCertRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorCert)
	require.NoError(t, fileSvc.WriteFile(context.Background(), opCertRel, []byte("op cert"), constants.PermFilePrivate))
	cliCertRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient, constants.PkiFileOperatorCert)
	require.NoError(t, fileSvc.WriteFile(context.Background(), cliCertRel, []byte("client cert"), constants.PermFilePrivate))

	result := resolveCertPath("", fileSvc, testLogger())
	assert.Equal(t, fileSvc.Resolve(opCertRel), result,
		"operator cert should take precedence over client cert when both exist")
}

func TestResolveCertPath_ExplicitOverridesDefaults(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	opCertRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorCert)
	require.NoError(t, fileSvc.WriteFile(context.Background(), opCertRel, []byte("op cert"), constants.PermFilePrivate))

	result := resolveCertPath("/explicit/cert.pem", fileSvc, testLogger())
	assert.Equal(t, "/explicit/cert.pem", result,
		"explicit path should override default file lookup")
}

// ---------------------------------------------------------------------------
// loadClientCertPair
// ---------------------------------------------------------------------------

// generateTestKeyCertPair creates a self-signed cert and matching ECDSA private key
// in PEM format, writing them to temp files. Returns (certPath, keyPath).
func generateTestKeyCertPair(t *testing.T) (string, string) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-client"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	dir := testutil.TempDir(t)
	certPath := filepath.Join(dir, constants.TestClientCrtFilename)
	keyPath := filepath.Join(dir, constants.TestClientKeyFilename)
	require.NoError(t, os.WriteFile(certPath, certPEM, constants.PermFilePrivate))
	require.NoError(t, os.WriteFile(keyPath, keyPEM, constants.PermFilePrivate))
	return certPath, keyPath
}

func TestLoadClientCertPair_Success(t *testing.T) {
	certPath, keyPath := generateTestKeyCertPair(t)

	cert, certPEM, err := loadClientCertPair(certPath, keyPath)
	require.NoError(t, err)
	assert.NotNil(t, cert.Certificate)
	assert.NotEmpty(t, certPEM)
	assert.Contains(t, string(certPEM), "BEGIN CERTIFICATE")
}

func TestLoadClientCertPair_NonExistentCertFile(t *testing.T) {
	_, keyPath := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(filepath.Join(testutil.TempDir(t), constants.TestNonExistentCrtFilename), keyPath)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrReadClientCert))
}

func TestLoadClientCertPair_NonExistentKeyFile(t *testing.T) {
	certPath, _ := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(certPath, filepath.Join(testutil.TempDir(t), constants.TestNonExistentKeyFilename))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrReadPrivateKey))
}

func TestLoadClientCertPair_InvalidCertPEM(t *testing.T) {
	dir := testutil.TempDir(t)
	invalidCert := filepath.Join(dir, constants.TestInvalidCrtFilename)
	require.NoError(t, os.WriteFile(invalidCert, []byte("not a PEM file"), constants.PermFilePrivate))

	_, keyPath := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(invalidCert, keyPath)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrLoadCertKeyPair))
}

func TestLoadClientCertPair_InvalidKeyPEM(t *testing.T) {
	certPath, _ := generateTestKeyCertPair(t)

	dir := testutil.TempDir(t)
	invalidKey := filepath.Join(dir, constants.TestInvalidKeyFilename)
	require.NoError(t, os.WriteFile(invalidKey, []byte("not a PEM file"), constants.PermFilePrivate))

	_, _, err := loadClientCertPair(certPath, invalidKey)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrLoadCertKeyPair))
}

func TestLoadClientCertPair_MismatchedKeyCertPair(t *testing.T) {
	certPath1, _ := generateTestKeyCertPair(t)
	_, keyPath2 := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(certPath1, keyPath2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrLoadCertKeyPair))
}

func TestLoadClientCertPair_EmptyCertFile(t *testing.T) {
	dir := testutil.TempDir(t)
	emptyCert := filepath.Join(dir, constants.TestClientCrtFilename)
	require.NoError(t, os.WriteFile(emptyCert, []byte{}, constants.PermFilePrivate))

	_, keyPath := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(emptyCert, keyPath)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrLoadCertKeyPair),
		"empty cert file should fail at X509KeyPair stage with ErrLoadCertKeyPair")
}

func TestLoadClientCertPair_EmptyKeyFile(t *testing.T) {
	certPath, _ := generateTestKeyCertPair(t)

	dir := testutil.TempDir(t)
	emptyKey := filepath.Join(dir, constants.TestClientKeyFilename)
	require.NoError(t, os.WriteFile(emptyKey, []byte{}, constants.PermFilePrivate))

	_, _, err := loadClientCertPair(certPath, emptyKey)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrLoadCertKeyPair),
		"empty key file should fail at X509KeyPair stage with ErrLoadCertKeyPair")
}

func TestLoadClientCertPair_CertPathIsDirectory(t *testing.T) {
	dir := testutil.TempDir(t)
	_, keyPath := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(dir, keyPath)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrReadClientCert),
		"cert path pointing to a directory should fail with ErrReadClientCert")
}

func TestLoadClientCertPair_KeyPathIsDirectory(t *testing.T) {
	certPath, _ := generateTestKeyCertPair(t)
	dir := testutil.TempDir(t)

	_, _, err := loadClientCertPair(certPath, dir)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrReadPrivateKey),
		"key path pointing to a directory should fail with ErrReadPrivateKey")
}

func TestLoadClientCertPair_BothPathsNonExistent(t *testing.T) {
	dir := testutil.TempDir(t)
	nonExistentCert := filepath.Join(dir, constants.TestNonExistentCrtFilename)
	nonExistentKey := filepath.Join(dir, constants.TestNonExistentKeyFilename)

	_, _, err := loadClientCertPair(nonExistentCert, nonExistentKey)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrReadClientCert),
		"cert read error should be returned first when both paths are non-existent")
}

func TestLoadClientCertPair_CertPEMMatchesFileContent(t *testing.T) {
	certPath, keyPath := generateTestKeyCertPair(t)

	expectedPEM, err := os.ReadFile(certPath)
	require.NoError(t, err)

	_, certPEM, err := loadClientCertPair(certPath, keyPath)
	require.NoError(t, err)
	assert.Equal(t, expectedPEM, certPEM,
		"returned certPEM bytes should exactly match the file content on disk")
}

// ---------------------------------------------------------------------------
// resolveKeyPath / resolveCertPath — additional edge cases
// ---------------------------------------------------------------------------

func TestResolveKeyPath_WhitespaceExplicitPathReturnedAsIs(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	result := resolveKeyPath("   ", fileSvc, testLogger())
	assert.Equal(t, "   ", result,
		"whitespace-only explicit path is non-empty so it should be returned as-is")
}

func TestResolveCertPath_WhitespaceExplicitPathReturnedAsIs(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	result := resolveCertPath("   ", fileSvc, testLogger())
	assert.Equal(t, "   ", result,
		"whitespace-only explicit path is non-empty so it should be returned as-is")
}

func TestResolveKeyPath_OnlyClientKeyExists(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	cliKeyRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient, constants.PkiFileOperatorKey)
	require.NoError(t, fileSvc.WriteFile(context.Background(), cliKeyRel, []byte("client key"), constants.PermFilePrivate))

	result := resolveKeyPath("", fileSvc, testLogger())
	assert.Equal(t, fileSvc.Resolve(cliKeyRel), result)
}

func TestResolveCertPath_OnlyClientCertExists(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	cliCertRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient, constants.PkiFileOperatorCert)
	require.NoError(t, fileSvc.WriteFile(context.Background(), cliCertRel, []byte("client cert"), constants.PermFilePrivate))

	result := resolveCertPath("", fileSvc, testLogger())
	assert.Equal(t, fileSvc.Resolve(cliCertRel), result)
}
