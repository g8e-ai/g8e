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

package serve

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger returns a silent logger suitable for unit tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// generateTestCert creates a self-signed x509 certificate for testing,
// writes it to a temp file, and returns the file path.
func generateTestCert(t *testing.T, notBefore, notAfter time.Time) string {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-cert"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	path := filepath.Join(t.TempDir(), "test.crt")
	require.NoError(t, os.WriteFile(path, certPEM, 0600))
	return path
}

// generateTestCertPEM returns PEM-encoded certificate bytes for testing.
func generateTestCertPEM(t *testing.T, notBefore, notAfter time.Time) []byte {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})
}

// ---------------------------------------------------------------------------
// ParseCertPEM
// ---------------------------------------------------------------------------

func TestParseCertPEM_Success(t *testing.T) {
	certPath := generateTestCert(t, time.Now(), time.Now().Add(365*24*time.Hour))

	cert, err := ParseCertPEM(certPath)
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Equal(t, "test-cert", cert.Subject.CommonName)
	assert.True(t, cert.IsCA)
}

func TestParseCertPEM_NonExistentFile(t *testing.T) {
	_, err := ParseCertPEM(filepath.Join(t.TempDir(), "nonexistent.crt"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrCertReadFailed))
}

func TestParseCertPEM_InvalidPEMData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.pem")
	require.NoError(t, os.WriteFile(path, []byte("not a PEM file"), 0600))

	_, err := ParseCertPEM(path)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrPEMDecodeFailed))
}

func TestParseCertPEM_WrongPEMType(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	path := filepath.Join(t.TempDir(), "key.pem")
	require.NoError(t, os.WriteFile(path, keyPEM, 0600))

	_, err = ParseCertPEM(path)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidPEMType))
}

func TestParseCertPEM_CorruptedCertBytes(t *testing.T) {
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("corrupted cert data"),
	})

	path := filepath.Join(t.TempDir(), "corrupt.crt")
	require.NoError(t, os.WriteFile(path, certPEM, 0600))

	_, err := ParseCertPEM(path)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrCertParseFailed))
}

// ---------------------------------------------------------------------------
// IsCertExpiringSoon
// ---------------------------------------------------------------------------

func TestIsCertExpiringSoon_ExpiringSoon(t *testing.T) {
	cert := &x509.Certificate{
		NotAfter: time.Now().Add(12 * time.Hour),
	}
	assert.True(t, IsCertExpiringSoon(cert))
}

func TestIsCertExpiringSoon_NotExpiringSoon(t *testing.T) {
	cert := &x509.Certificate{
		NotAfter: time.Now().Add(30 * 24 * time.Hour),
	}
	assert.False(t, IsCertExpiringSoon(cert))
}

func TestIsCertExpiringSoon_AlreadyExpired(t *testing.T) {
	cert := &x509.Certificate{
		NotAfter: time.Now().Add(-1 * time.Hour),
	}
	assert.True(t, IsCertExpiringSoon(cert))
}

func TestIsCertExpiringSoon_ExactlyAtThreshold(t *testing.T) {
	cert := &x509.Certificate{
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	assert.True(t, IsCertExpiringSoon(cert), "cert expiring in exactly 24h should be considered expiring soon")
}

func TestIsCertExpiringSoon_JustBeyondThreshold(t *testing.T) {
	cert := &x509.Certificate{
		NotAfter: time.Now().Add(24*time.Hour + time.Minute),
	}
	assert.False(t, IsCertExpiringSoon(cert), "cert expiring just beyond 24h should not be considered expiring soon")
}

// ---------------------------------------------------------------------------
// GenerateCSR
// ---------------------------------------------------------------------------

func TestGenerateCSR_Success(t *testing.T) {
	csrPEM, privKey, err := GenerateCSR("test-operator")
	require.NoError(t, err)
	assert.NotEmpty(t, csrPEM)
	assert.NotNil(t, privKey)

	block, _ := pem.Decode([]byte(csrPEM))
	require.NotNil(t, block)
	assert.Equal(t, "CERTIFICATE REQUEST", block.Type)

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, "test-operator", csr.Subject.CommonName)
	assert.Equal(t, []string{"g8e"}, csr.Subject.Organization)

	ecPub, ok := privKey.Public().(*ecdsa.PublicKey)
	require.True(t, ok)
	assert.Equal(t, elliptic.P256(), ecPub.Curve)
}

func TestGenerateCSR_EmptyCommonName(t *testing.T) {
	csrPEM, privKey, err := GenerateCSR("")
	require.NoError(t, err)
	assert.NotEmpty(t, csrPEM)
	assert.NotNil(t, privKey)

	block, _ := pem.Decode([]byte(csrPEM))
	require.NotNil(t, block)
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	assert.Empty(t, csr.Subject.CommonName)
}

func TestGenerateCSR_Uniqueness(t *testing.T) {
	csr1, key1, err := GenerateCSR("test")
	require.NoError(t, err)

	csr2, key2, err := GenerateCSR("test")
	require.NoError(t, err)

	assert.NotEqual(t, csr1, csr2, "two CSRs with same CN should differ due to random keys")
	assert.NotEqual(t, key1.D, key2.D, "private keys should be distinct")
}

// ---------------------------------------------------------------------------
// LoadTrustBundle
// ---------------------------------------------------------------------------

func TestLoadTrustBundle_ExplicitPath(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	caPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
	explicitPath := filepath.Join(t.TempDir(), "ca-bundle.pem")
	require.NoError(t, os.WriteFile(explicitPath, caPEM, 0644))

	ts := certs.NewTrustStore(nil)
	logger := testLogger()

	loaded := LoadTrustBundle(logger, explicitPath, ts)
	assert.True(t, loaded)
	assert.Equal(t, caPEM, ts.GetRawCA())
}

func TestLoadTrustBundle_DefaultPath(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))

	caPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.CaCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.CaCertPath, caPEM, 0644))

	ts := certs.NewTrustStore(nil)
	logger := testLogger()

	loaded := LoadTrustBundle(logger, "", ts)
	assert.True(t, loaded)
	assert.Equal(t, caPEM, ts.GetRawCA())
}

func TestLoadTrustBundle_NoFilesFound(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	ts := certs.NewTrustStore(nil)
	logger := testLogger()

	loaded := LoadTrustBundle(logger, "", ts)
	assert.False(t, loaded)
	assert.Nil(t, ts.GetRawCA())
}

func TestLoadTrustBundle_ExplicitPathInvalidPEM(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	invalidPath := filepath.Join(t.TempDir(), "invalid.pem")
	require.NoError(t, os.WriteFile(invalidPath, []byte("not a PEM"), 0644))

	ts := certs.NewTrustStore(nil)
	logger := testLogger()

	loaded := LoadTrustBundle(logger, invalidPath, ts)
	assert.False(t, loaded)
	assert.Nil(t, ts.GetRawCA())
}

func TestLoadTrustBundle_ExplicitPathFallsBackToDefault(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))

	caPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.CaCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.CaCertPath, caPEM, 0644))

	nonExistentExplicit := filepath.Join(t.TempDir(), "does-not-exist.pem")

	ts := certs.NewTrustStore(nil)
	logger := testLogger()

	loaded := LoadTrustBundle(logger, nonExistentExplicit, ts)
	assert.True(t, loaded, "should fall back to default path when explicit path does not exist")
	assert.Equal(t, caPEM, ts.GetRawCA())
}

func TestLoadTrustBundle_ExplicitPathPriority(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))

	explicitPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
	explicitPath := filepath.Join(tmpDir, "explicit.pem")
	require.NoError(t, os.WriteFile(explicitPath, explicitPEM, 0644))

	defaultPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.CaCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.CaCertPath, defaultPEM, 0644))

	ts := certs.NewTrustStore(nil)
	logger := testLogger()

	loaded := LoadTrustBundle(logger, explicitPath, ts)
	assert.True(t, loaded)
	assert.Equal(t, explicitPEM, ts.GetRawCA(), "explicit path should take priority over default")
}

// ---------------------------------------------------------------------------
// LogCertBundle
// ---------------------------------------------------------------------------

func TestLogCertBundle_SingleCert(t *testing.T) {
	certPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
	logger := testLogger()

	assert.NotPanics(t, func() {
		LogCertBundle(logger, "test-bundle", certPEM)
	})
}

func TestLogCertBundle_MultipleCerts(t *testing.T) {
	cert1 := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
	cert2 := generateTestCertPEM(t, time.Now(), time.Now().Add(200*24*time.Hour))
	bundle := append(cert1, cert2...)

	logger := testLogger()
	assert.NotPanics(t, func() {
		LogCertBundle(logger, "multi-bundle", bundle)
	})
}

func TestLogCertBundle_EmptyData(t *testing.T) {
	logger := testLogger()
	assert.NotPanics(t, func() {
		LogCertBundle(logger, "empty", []byte{})
	})
}

func TestLogCertBundle_NonCertPEMBlock(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	logger := testLogger()
	assert.NotPanics(t, func() {
		LogCertBundle(logger, "mixed", keyPEM)
	})
}

func TestLogCertBundle_NilData(t *testing.T) {
	logger := testLogger()
	assert.NotPanics(t, func() {
		LogCertBundle(logger, "nil", nil)
	})
}

// ---------------------------------------------------------------------------
// ExportActuatorPublicKey
// ---------------------------------------------------------------------------

func TestExportActuatorPublicKey_Success(t *testing.T) {
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pkiDir := filepath.Join(t.TempDir(), "pki")
	logger := testLogger()

	err = ExportActuatorPublicKey(pkiDir, pubKey, "test-key-id", logger)
	require.NoError(t, err)

	pemPath := filepath.Join(pkiDir, constants.ActuatorPubPEMFilename)
	pemData, err := os.ReadFile(pemPath)
	require.NoError(t, err)

	block, _ := pem.Decode(pemData)
	require.NotNil(t, block)
	assert.Equal(t, "PUBLIC KEY", block.Type)
	assert.Equal(t, []byte(pubKey), block.Bytes)

	jsonPath := filepath.Join(pkiDir, constants.ActuatorPubJSONFilename)
	jsonData, err := os.ReadFile(jsonPath)
	require.NoError(t, err)

	var parsed map[string]string
	require.NoError(t, json.Unmarshal(jsonData, &parsed))
	assert.Equal(t, "test-key-id", parsed["key_id"])
	assert.Equal(t, hex.EncodeToString(pubKey), parsed["public_key"])
	assert.Equal(t, "ed25519", parsed["algorithm"])
}

func TestExportActuatorPublicKey_EmptyPKIDir(t *testing.T) {
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	err = ExportActuatorPublicKey("", pubKey, "key-id", testLogger())
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrPKIDirRequired))
}

func TestExportActuatorPublicKey_NilLogger(t *testing.T) {
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pkiDir := filepath.Join(t.TempDir(), "pki")

	err = ExportActuatorPublicKey(pkiDir, pubKey, "key-id", nil)
	require.NoError(t, err)

	pemPath := filepath.Join(pkiDir, constants.ActuatorPubPEMFilename)
	_, err = os.ReadFile(pemPath)
	require.NoError(t, err, "PEM file should be created even with nil logger")
}

func TestExportActuatorPublicKey_OverwriteExisting(t *testing.T) {
	pubKey1, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pubKey2, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pkiDir := filepath.Join(t.TempDir(), "pki")
	logger := testLogger()

	require.NoError(t, ExportActuatorPublicKey(pkiDir, pubKey1, "key-1", logger))
	require.NoError(t, ExportActuatorPublicKey(pkiDir, pubKey2, "key-2", logger))

	jsonPath := filepath.Join(pkiDir, constants.ActuatorPubJSONFilename)
	jsonData, err := os.ReadFile(jsonPath)
	require.NoError(t, err)

	var parsed map[string]string
	require.NoError(t, json.Unmarshal(jsonData, &parsed))
	assert.Equal(t, "key-2", parsed["key_id"])
	assert.Equal(t, hex.EncodeToString(pubKey2), parsed["public_key"])
}

func TestExportActuatorPublicKey_CreatesNestedDir(t *testing.T) {
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pkiDir := filepath.Join(t.TempDir(), "nested", "deep", "pki")
	logger := testLogger()

	err = ExportActuatorPublicKey(pkiDir, pubKey, "key-id", logger)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(pkiDir, constants.ActuatorPubPEMFilename))
	require.NoError(t, err)
	assert.True(t, info.Mode().Perm() == 0600)
}

// ---------------------------------------------------------------------------
// RenewOperatorCertificate (error paths only — Tier 1)
// ---------------------------------------------------------------------------

func TestRenewOperatorCertificate_NonExistentCertFile(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	cfg := &config.Config{Endpoint: "https://fake:8443"}
	ci := certs.NewClientIdentity(tls.Certificate{})

	err := RenewOperatorCertificate(cfg, filepath.Join(t.TempDir(), "nonexistent.crt"), "nonexistent.key", ci)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrCertParseFailed))
}

func TestRenewOperatorCertificate_CertNotExpiring(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	certPath := generateTestCert(t, time.Now(), time.Now().Add(365*24*time.Hour))
	keyPath := filepath.Join(filepath.Dir(certPath), "key.pem")

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0600))

	cfg := &config.Config{Endpoint: "https://fake:8443"}
	ci := certs.NewClientIdentity(tls.Certificate{})

	err = RenewOperatorCertificate(cfg, certPath, keyPath, ci)
	require.NoError(t, err, "cert not expiring soon should return nil without making network calls")
}

// ---------------------------------------------------------------------------
// RunClientCertRenewalLoop (context cancellation only — Tier 1)
// ---------------------------------------------------------------------------

func TestRunClientCertRenewalLoop_ContextCancellation(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	cfg := &config.Config{Endpoint: "https://fake:8443"}
	ci := certs.NewClientIdentity(tls.Certificate{})
	logger := testLogger()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		RunClientCertRenewalLoop(ctx, cfg, "nonexistent.crt", "nonexistent.key", logger, ci)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunClientCertRenewalLoop did not stop after context cancellation")
	}
}
