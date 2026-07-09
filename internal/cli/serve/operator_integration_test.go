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

package serve

import (
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
	"github.com/g8e-ai/g8e/internal/paths"
)

// ---------------------------------------------------------------------------
// resolveKeyPath — filesystem-dependent tests
// ---------------------------------------------------------------------------

func TestResolveKeyPath_DefaultOperatorKey(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorKeyPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorKeyPath, []byte("fake key"), 0600))

	result := resolveKeyPath("", testLogger())
	assert.Equal(t, paths.Infra.OperatorKeyPath, result)
}

func TestResolveKeyPath_FallsBackToClientKey(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.ClientOperatorKeyPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.ClientOperatorKeyPath, []byte("fake key"), 0600))

	result := resolveKeyPath("", testLogger())
	assert.Equal(t, paths.Infra.ClientOperatorKeyPath, result)
}

func TestResolveKeyPath_NoFilesFound(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	result := resolveKeyPath("", testLogger())
	assert.Equal(t, "", result)
}

func TestResolveKeyPath_OperatorKeyTakesPrecedenceOverClientKey(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorKeyPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorKeyPath, []byte("op key"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.ClientOperatorKeyPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.ClientOperatorKeyPath, []byte("client key"), 0600))

	result := resolveKeyPath("", testLogger())
	assert.Equal(t, paths.Infra.OperatorKeyPath, result,
		"operator key should take precedence over client key when both exist")
}

func TestResolveKeyPath_ExplicitOverridesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorKeyPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorKeyPath, []byte("op key"), 0600))

	result := resolveKeyPath("/explicit/key.pem", testLogger())
	assert.Equal(t, "/explicit/key.pem", result,
		"explicit path should override default file lookup")
}

// ---------------------------------------------------------------------------
// resolveCertPath — filesystem-dependent tests
// ---------------------------------------------------------------------------

func TestResolveCertPath_DefaultOperatorCert(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorCertPath, []byte("fake cert"), 0600))

	result := resolveCertPath("", testLogger())
	assert.Equal(t, paths.Infra.OperatorCertPath, result)
}

func TestResolveCertPath_FallsBackToClientCert(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.ClientOperatorCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.ClientOperatorCertPath, []byte("fake cert"), 0600))

	result := resolveCertPath("", testLogger())
	assert.Equal(t, paths.Infra.ClientOperatorCertPath, result)
}

func TestResolveCertPath_NoFilesFound(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	result := resolveCertPath("", testLogger())
	assert.Equal(t, "", result)
}

func TestResolveCertPath_OperatorCertTakesPrecedenceOverClientCert(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorCertPath, []byte("op cert"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.ClientOperatorCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.ClientOperatorCertPath, []byte("client cert"), 0600))

	result := resolveCertPath("", testLogger())
	assert.Equal(t, paths.Infra.OperatorCertPath, result,
		"operator cert should take precedence over client cert when both exist")
}

func TestResolveCertPath_ExplicitOverridesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorCertPath, []byte("op cert"), 0600))

	result := resolveCertPath("/explicit/cert.pem", testLogger())
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

	dir := t.TempDir()
	certPath := filepath.Join(dir, constants.TestClientCrtFilename)
	keyPath := filepath.Join(dir, constants.TestClientKeyFilename)
	require.NoError(t, os.WriteFile(certPath, certPEM, 0600))
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0600))
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

	_, _, err := loadClientCertPair(filepath.Join(t.TempDir(), constants.TestNonExistentCrtFilename), keyPath)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrReadClientCert))
}

func TestLoadClientCertPair_NonExistentKeyFile(t *testing.T) {
	certPath, _ := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(certPath, filepath.Join(t.TempDir(), constants.TestNonExistentKeyFilename))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrReadPrivateKey))
}

func TestLoadClientCertPair_InvalidCertPEM(t *testing.T) {
	dir := t.TempDir()
	invalidCert := filepath.Join(dir, constants.TestInvalidCrtFilename)
	require.NoError(t, os.WriteFile(invalidCert, []byte("not a PEM file"), 0600))

	_, keyPath := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(invalidCert, keyPath)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrLoadCertKeyPair))
}

func TestLoadClientCertPair_InvalidKeyPEM(t *testing.T) {
	certPath, _ := generateTestKeyCertPair(t)

	dir := t.TempDir()
	invalidKey := filepath.Join(dir, constants.TestInvalidKeyFilename)
	require.NoError(t, os.WriteFile(invalidKey, []byte("not a PEM file"), 0600))

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
