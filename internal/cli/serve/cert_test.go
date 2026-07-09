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
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
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

	path := filepath.Join(t.TempDir(), constants.TestCertCrtFilename)
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

func TestParseCertPEM(t *testing.T) {
	validCertPath := generateTestCert(t, time.Now(), time.Now().Add(365*24*time.Hour))

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	keyPath := filepath.Join(t.TempDir(), constants.TestECPrivateKeyFilename)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0600))

	corruptPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("corrupted cert data")})
	corruptPath := filepath.Join(t.TempDir(), constants.TestCorruptCrtFilename)
	require.NoError(t, os.WriteFile(corruptPath, corruptPEM, 0600))

	invalidPath := filepath.Join(t.TempDir(), constants.TestInvalidPEMFilename)
	require.NoError(t, os.WriteFile(invalidPath, []byte("not a PEM file"), 0600))

	nonExistentPath := filepath.Join(t.TempDir(), constants.TestNonExistentCrtFilename)

	tests := []struct {
		name     string
		certPath string
		wantErr  error
		check    func(*testing.T, *x509.Certificate)
	}{
		{
			name:     "Success",
			certPath: validCertPath,
			wantErr:  nil,
			check: func(t *testing.T, cert *x509.Certificate) {
				assert.Equal(t, "test-cert", cert.Subject.CommonName)
				assert.True(t, cert.IsCA)
			},
		},
		{
			name:     "NonExistentFile",
			certPath: nonExistentPath,
			wantErr:  constants.ErrCertReadFailed,
		},
		{
			name:     "InvalidPEMData",
			certPath: invalidPath,
			wantErr:  constants.ErrPEMDecodeFailed,
		},
		{
			name:     "WrongPEMType",
			certPath: keyPath,
			wantErr:  constants.ErrInvalidPEMType,
		},
		{
			name:     "CorruptedCertBytes",
			certPath: corruptPath,
			wantErr:  constants.ErrCertParseFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := ParseCertPEM(tt.certPath)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cert)
			if tt.check != nil {
				tt.check(t, cert)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsCertExpiringSoon
// ---------------------------------------------------------------------------

func TestIsCertExpiringSoon(t *testing.T) {
	tests := []struct {
		name     string
		notAfter time.Time
		want     bool
		msg      string
	}{
		{
			name:     "ExpiringSoon",
			notAfter: time.Now().Add(12 * time.Hour),
			want:     true,
		},
		{
			name:     "NotExpiringSoon",
			notAfter: time.Now().Add(30 * 24 * time.Hour),
			want:     false,
		},
		{
			name:     "AlreadyExpired",
			notAfter: time.Now().Add(-1 * time.Hour),
			want:     true,
		},
		{
			name:     "ExactlyAtThreshold",
			notAfter: time.Now().Add(24 * time.Hour),
			want:     true,
			msg:      "cert expiring in exactly 24h should be considered expiring soon",
		},
		{
			name:     "JustBeyondThreshold",
			notAfter: time.Now().Add(24*time.Hour + time.Minute),
			want:     false,
			msg:      "cert expiring just beyond 24h should not be considered expiring soon",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := &x509.Certificate{NotAfter: tt.notAfter}
			if tt.msg != "" {
				assert.Equal(t, tt.want, IsCertExpiringSoon(cert), tt.msg)
			} else {
				assert.Equal(t, tt.want, IsCertExpiringSoon(cert))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GenerateCSR
// ---------------------------------------------------------------------------

func TestGenerateCSR(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
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
	})

	t.Run("EmptyCommonName", func(t *testing.T) {
		csrPEM, privKey, err := GenerateCSR("")
		require.NoError(t, err)
		assert.NotEmpty(t, csrPEM)
		assert.NotNil(t, privKey)

		block, _ := pem.Decode([]byte(csrPEM))
		require.NotNil(t, block)
		csr, err := x509.ParseCertificateRequest(block.Bytes)
		require.NoError(t, err)
		assert.Empty(t, csr.Subject.CommonName)
	})

	t.Run("Uniqueness", func(t *testing.T) {
		csr1, key1, err := GenerateCSR("test")
		require.NoError(t, err)

		csr2, key2, err := GenerateCSR("test")
		require.NoError(t, err)

		assert.NotEqual(t, csr1, csr2, "two CSRs with same CN should differ due to random keys")
		assert.False(t, key1.Equal(key2), "private keys should be distinct")
	})
}

// ---------------------------------------------------------------------------
// LoadTrustBundle
// ---------------------------------------------------------------------------

func TestLoadTrustBundle(t *testing.T) {
	caPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))

	tests := []struct {
		name         string
		setup        func(t *testing.T) (explicitPath string, initPaths bool)
		wantLoaded   bool
		wantRawCA    []byte
		wantRawCANil bool
	}{
		{
			name: "ExplicitPath",
			setup: func(t *testing.T) (string, bool) {
				require.NoError(t, paths.InitWithBase(t.TempDir()))
				explicitPath := filepath.Join(t.TempDir(), constants.TestCABundleFilename)
				require.NoError(t, os.WriteFile(explicitPath, caPEM, 0644))
				return explicitPath, true
			},
			wantLoaded: true,
			wantRawCA:  caPEM,
		},
		{
			name: "DefaultPath",
			setup: func(t *testing.T) (string, bool) {
				require.NoError(t, paths.InitWithBase(t.TempDir()))
				require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.CaCertPath), 0700))
				require.NoError(t, os.WriteFile(paths.Infra.CaCertPath, caPEM, 0644))
				return "", true
			},
			wantLoaded: true,
			wantRawCA:  caPEM,
		},
		{
			name: "NoFilesFound",
			setup: func(t *testing.T) (string, bool) {
				require.NoError(t, paths.InitWithBase(t.TempDir()))
				return "", true
			},
			wantLoaded:   false,
			wantRawCANil: true,
		},
		{
			name: "ExplicitPathInvalidPEM",
			setup: func(t *testing.T) (string, bool) {
				require.NoError(t, paths.InitWithBase(t.TempDir()))
				invalidPath := filepath.Join(t.TempDir(), constants.TestInvalidPEMFilename)
				require.NoError(t, os.WriteFile(invalidPath, []byte("not a PEM"), 0644))
				return invalidPath, true
			},
			wantLoaded:   false,
			wantRawCANil: true,
		},
		{
			name: "ExplicitPathFallsBackToDefault",
			setup: func(t *testing.T) (string, bool) {
				require.NoError(t, paths.InitWithBase(t.TempDir()))
				require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.CaCertPath), 0700))
				require.NoError(t, os.WriteFile(paths.Infra.CaCertPath, caPEM, 0644))
				return filepath.Join(t.TempDir(), constants.TestDoesNotExistPEMFilename), true
			},
			wantLoaded: true,
			wantRawCA:  caPEM,
		},
		{
			name: "ExplicitPathPriority",
			setup: func(t *testing.T) (string, bool) {
				tmpDir := t.TempDir()
				require.NoError(t, paths.InitWithBase(tmpDir))
				explicitPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
				explicitPath := filepath.Join(tmpDir, constants.TestExplicitPEMFilename)
				require.NoError(t, os.WriteFile(explicitPath, explicitPEM, 0644))
				defaultPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
				require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.CaCertPath), 0700))
				require.NoError(t, os.WriteFile(paths.Infra.CaCertPath, defaultPEM, 0644))
				return explicitPath, true
			},
			wantLoaded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			explicitPath, _ := tt.setup(t)

			ts := certs.NewTrustStore(nil)
			logger := testLogger()

			loaded := LoadTrustBundle(logger, explicitPath, ts)
			assert.Equal(t, tt.wantLoaded, loaded)

			if tt.wantRawCANil {
				assert.Nil(t, ts.GetRawCA())
			} else if tt.wantRawCA != nil {
				assert.Equal(t, tt.wantRawCA, ts.GetRawCA())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LogCertBundle
// ---------------------------------------------------------------------------

func TestLogCertBundle(t *testing.T) {
	certPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tests := []struct {
		name    string
		label   string
		data    []byte
		wantErr bool
	}{
		{
			name:  "SingleCert",
			label: "test-bundle",
			data:  certPEM,
		},
		{
			name:  "MultipleCerts",
			label: "multi-bundle",
			data:  append(certPEM, generateTestCertPEM(t, time.Now(), time.Now().Add(200*24*time.Hour))...),
		},
		{
			name:  "EmptyData",
			label: "empty",
			data:  []byte{},
		},
		{
			name:  "NonCertPEMBlock",
			label: "mixed",
			data:  keyPEM,
		},
		{
			name:  "NilData",
			label: "nil",
			data:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testLogger()
			assert.NotPanics(t, func() {
				LogCertBundle(logger, tt.label, tt.data)
			})
		})
	}
}

// ---------------------------------------------------------------------------
// ExportActuatorPublicKey
// ---------------------------------------------------------------------------

func TestExportActuatorPublicKey(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		pkiDir := filepath.Join(t.TempDir(), constants.TestPkiDirname)
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

		var parsed models.ActuatorPublicKeyExport
		require.NoError(t, json.Unmarshal(jsonData, &parsed))
		assert.Equal(t, "test-key-id", parsed.KeyID)
		assert.Equal(t, hex.EncodeToString(pubKey), parsed.PublicKey)
		assert.Equal(t, "ed25519", parsed.Algorithm)
	})

	t.Run("EmptyPKIDir", func(t *testing.T) {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		err = ExportActuatorPublicKey("", pubKey, "key-id", testLogger())
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrPKIDirRequired))
	})

	t.Run("NilLogger", func(t *testing.T) {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		pkiDir := filepath.Join(t.TempDir(), constants.TestPkiDirname)

		err = ExportActuatorPublicKey(pkiDir, pubKey, "key-id", nil)
		require.NoError(t, err)

		pemPath := filepath.Join(pkiDir, constants.ActuatorPubPEMFilename)
		_, err = os.ReadFile(pemPath)
		require.NoError(t, err, "PEM file should be created even with nil logger")
	})

	t.Run("OverwriteExisting", func(t *testing.T) {
		pubKey1, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		pubKey2, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		pkiDir := filepath.Join(t.TempDir(), constants.TestPkiDirname)
		logger := testLogger()

		require.NoError(t, ExportActuatorPublicKey(pkiDir, pubKey1, "key-1", logger))
		require.NoError(t, ExportActuatorPublicKey(pkiDir, pubKey2, "key-2", logger))

		jsonPath := filepath.Join(pkiDir, constants.ActuatorPubJSONFilename)
		jsonData, err := os.ReadFile(jsonPath)
		require.NoError(t, err)

		var parsed models.ActuatorPublicKeyExport
		require.NoError(t, json.Unmarshal(jsonData, &parsed))
		assert.Equal(t, "key-2", parsed.KeyID)
		assert.Equal(t, hex.EncodeToString(pubKey2), parsed.PublicKey)
	})

	t.Run("CreatesNestedDir", func(t *testing.T) {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		pkiDir := filepath.Join(t.TempDir(), constants.TestNestedDirname, constants.TestDeepDirname, constants.TestPkiDirname)
		logger := testLogger()

		err = ExportActuatorPublicKey(pkiDir, pubKey, "key-id", logger)
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(pkiDir, constants.ActuatorPubPEMFilename))
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			assert.True(t, info.Mode().Perm() == 0600)
		}
	})
}

// ---------------------------------------------------------------------------
// checkCertExpiry
// ---------------------------------------------------------------------------

func TestCheckCertExpiry(t *testing.T) {
	t.Run("CertExpiringSoon_ReturnsTrue", func(t *testing.T) {
		certPath := generateTestCert(t, time.Now(), time.Now().Add(1*time.Hour))
		expiring, err := checkCertExpiry(certPath)
		require.NoError(t, err)
		assert.True(t, expiring)
	})

	t.Run("CertNotExpiring_ReturnsFalse", func(t *testing.T) {
		certPath := generateTestCert(t, time.Now(), time.Now().Add(365*24*time.Hour))
		expiring, err := checkCertExpiry(certPath)
		require.NoError(t, err)
		assert.False(t, expiring)
	})

	t.Run("NonExistentFile_ReturnsError", func(t *testing.T) {
		expiring, err := checkCertExpiry(filepath.Join(t.TempDir(), constants.TestNonExistentCrtFilename))
		require.Error(t, err)
		assert.False(t, expiring)
		assert.True(t, errors.Is(err, constants.ErrCertReadFailed))
	})
}

// ---------------------------------------------------------------------------
// buildMTLSClient
// ---------------------------------------------------------------------------

func TestBuildMTLSClient(t *testing.T) {
	t.Run("ValidCAPEM_ReturnsClientWithTLS13", func(t *testing.T) {
		caPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))

		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		clientTemplate := x509.Certificate{
			SerialNumber: big.NewInt(10),
			Subject:      pkix.Name{CommonName: "test-client"},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(365 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
		}
		clientDER, err := x509.CreateCertificate(rand.Reader, &clientTemplate, &clientTemplate, &privKey.PublicKey, privKey)
		require.NoError(t, err)
		clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})

		keyDER, err := x509.MarshalECPrivateKey(privKey)
		require.NoError(t, err)
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

		cliCert, err := tls.X509KeyPair(clientCertPEM, keyPEM)
		require.NoError(t, err)

		client, err := buildMTLSClient(caPEM, cliCert)
		require.NoError(t, err)
		require.NotNil(t, client)

		transport, ok := client.Transport.(*http.Transport)
		require.True(t, ok)
		assert.EqualValues(t, tls.VersionTLS13, transport.TLSClientConfig.MinVersion)
		assert.Len(t, transport.TLSClientConfig.Certificates, 1)
		assert.NotNil(t, transport.TLSClientConfig.RootCAs)
	})

	t.Run("InvalidCAPEM_ReturnsCAParseFailed", func(t *testing.T) {
		cliCert := tls.Certificate{}
		client, err := buildMTLSClient([]byte("not a valid PEM"), cliCert)
		require.Error(t, err)
		assert.Nil(t, client)
		assert.True(t, errors.Is(err, constants.ErrCAParseFailed))
	})

	t.Run("EmptyCAPEM_ReturnsCAParseFailed", func(t *testing.T) {
		cliCert := tls.Certificate{}
		client, err := buildMTLSClient([]byte{}, cliCert)
		require.Error(t, err)
		assert.Nil(t, client)
		assert.True(t, errors.Is(err, constants.ErrCAParseFailed))
	})
}

// ---------------------------------------------------------------------------
// saveRenewedCerts
// ---------------------------------------------------------------------------

func TestSaveRenewedCerts(t *testing.T) {
	t.Run("Success_WritesKeyAndCertFiles", func(t *testing.T) {
		dir := t.TempDir()
		certFile := filepath.Join(dir, constants.TestClientCrtFilename)
		keyFile := filepath.Join(dir, constants.TestClientKeyFilename)

		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		certContent := "fake-cert-pem-content"
		keyPEM, err := saveRenewedCerts(certFile, keyFile, certContent, privKey)
		require.NoError(t, err)
		assert.NotEmpty(t, keyPEM)
		assert.Contains(t, string(keyPEM), "EC PRIVATE KEY")

		savedCert, err := os.ReadFile(certFile)
		require.NoError(t, err)
		assert.Equal(t, certContent, string(savedCert))

		savedKey, err := os.ReadFile(keyFile)
		require.NoError(t, err)
		assert.Contains(t, string(savedKey), "EC PRIVATE KEY")
	})

	t.Run("CertChainAppendedToCertContent", func(t *testing.T) {
		dir := t.TempDir()
		certFile := filepath.Join(dir, constants.TestClientCrtFilename)
		keyFile := filepath.Join(dir, constants.TestClientKeyFilename)

		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		certContent := "-----BEGIN CERTIFICATE-----\nleaf\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nintermediate\n-----END CERTIFICATE-----"
		_, err = saveRenewedCerts(certFile, keyFile, certContent, privKey)
		require.NoError(t, err)

		savedCert, err := os.ReadFile(certFile)
		require.NoError(t, err)
		assert.Contains(t, string(savedCert), "leaf")
		assert.Contains(t, string(savedCert), "intermediate")
	})

	t.Run("NonExistentKeyDir_ReturnsKeyWriteFailed", func(t *testing.T) {
		certFile := filepath.Join(t.TempDir(), constants.TestClientCrtFilename)
		keyFile := filepath.Join(t.TempDir(), constants.TestNestedDirname, constants.TestClientKeyFilename)

		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		_, err = saveRenewedCerts(certFile, keyFile, "cert-content", privKey)
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrKeyWriteFailed))
	})

	t.Run("NonExistentCertDir_ReturnsCertSaveFailed", func(t *testing.T) {
		keyFile := filepath.Join(t.TempDir(), constants.TestClientKeyFilename)
		certFile := filepath.Join(t.TempDir(), constants.TestNestedDirname, constants.TestClientCrtFilename)

		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		_, err = saveRenewedCerts(certFile, keyFile, "cert-content", privKey)
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrCertSaveFailed))
	})
}

// ---------------------------------------------------------------------------
// RenewOperatorCertificate (error paths only — Tier 1)
// ---------------------------------------------------------------------------

func TestRenewOperatorCertificate_NonExistentCertFile(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	cfg := &config.Config{Endpoint: "https://fake:8443"}
	ci := certs.NewClientIdentity(tls.Certificate{})

	err := RenewOperatorCertificate(context.Background(), cfg, filepath.Join(t.TempDir(), constants.TestNonExistentCrtFilename), constants.TestNonExistentCrtFilename, ci)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrCertParseFailed))
}

func TestRenewOperatorCertificate_CertNotExpiring(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	certPath := generateTestCert(t, time.Now(), time.Now().Add(365*24*time.Hour))
	keyPath := filepath.Join(filepath.Dir(certPath), constants.TestECPrivateKeyFilename)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0600))

	cfg := &config.Config{Endpoint: "https://fake:8443"}
	ci := certs.NewClientIdentity(tls.Certificate{})

	err = RenewOperatorCertificate(context.Background(), cfg, certPath, keyPath, ci)
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
		RunClientCertRenewalLoop(ctx, cfg, constants.TestNonExistentCrtFilename, constants.TestNonExistentCrtFilename, logger, ci)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunClientCertRenewalLoop did not stop after context cancellation")
	}
}
