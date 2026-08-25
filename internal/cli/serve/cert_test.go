// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	govsvc "github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
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

	path := filepath.Join(testutil.TempDir(t), constants.TestCertCrtFilename)
	require.NoError(t, os.WriteFile(path, certPEM, constants.PermFilePrivate))
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

// restorePorts snapshots constants.Ports and restores it after the test.
func restorePorts(t *testing.T) {
	t.Helper()
	snapshot := constants.Ports
	t.Cleanup(func() {
		constants.Ports = snapshot
	})
}

// getServerPort extracts the TCP port from an httptest.Server listener.
func getServerPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	addr, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "expected *net.TCPAddr from httptest listener")
	return addr.Port
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
	keyPath := filepath.Join(testutil.TempDir(t), constants.TestECPrivateKeyFilename)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, constants.PermFilePrivate))

	corruptPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("corrupted cert data")})
	corruptPath := filepath.Join(testutil.TempDir(t), constants.TestCorruptCrtFilename)
	require.NoError(t, os.WriteFile(corruptPath, corruptPEM, constants.PermFilePrivate))

	invalidPath := filepath.Join(testutil.TempDir(t), constants.TestInvalidPEMFilename)
	require.NoError(t, os.WriteFile(invalidPath, []byte("not a PEM file"), constants.PermFilePrivate))

	nonExistentPath := filepath.Join(testutil.TempDir(t), constants.TestNonExistentCrtFilename)

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
		assert.NotEqual(t, x509.Ed25519, csr.PublicKeyAlgorithm,
			"CSR must not use Ed25519 (excluded from Go FIPS TLS mode)")
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
	caBundleRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)

	tests := []struct {
		name         string
		setup        func(t *testing.T) (explicitPath string, fileSvc fs.RuntimeFileService)
		wantLoaded   bool
		wantRawCA    []byte
		wantRawCANil bool
	}{
		{
			name: "ExplicitPath",
			setup: func(t *testing.T) (string, fs.RuntimeFileService) {
				fileSvc := newTestFileSvc(t)
				explicitPath := filepath.Join(testutil.TempDir(t), constants.TestCABundleFilename)
				require.NoError(t, os.WriteFile(explicitPath, caPEM, constants.PermFilePublic))
				return explicitPath, fileSvc
			},
			wantLoaded: true,
			wantRawCA:  caPEM,
		},
		{
			name: "DefaultPath",
			setup: func(t *testing.T) (string, fs.RuntimeFileService) {
				fileSvc := newTestFileSvc(t)
				require.NoError(t, fileSvc.WriteFile(context.Background(), caBundleRel, caPEM, constants.PermFilePublic))
				return "", fileSvc
			},
			wantLoaded: true,
			wantRawCA:  caPEM,
		},
		{
			name: "NoFilesFound",
			setup: func(t *testing.T) (string, fs.RuntimeFileService) {
				fileSvc := newTestFileSvc(t)
				return "", fileSvc
			},
			wantLoaded:   false,
			wantRawCANil: true,
		},
		{
			name: "ExplicitPathInvalidPEM",
			setup: func(t *testing.T) (string, fs.RuntimeFileService) {
				fileSvc := newTestFileSvc(t)
				invalidPath := filepath.Join(testutil.TempDir(t), constants.TestInvalidPEMFilename)
				require.NoError(t, os.WriteFile(invalidPath, []byte("not a PEM"), constants.PermFilePublic))
				return invalidPath, fileSvc
			},
			wantLoaded:   false,
			wantRawCANil: true,
		},
		{
			name: "ExplicitPathFallsBackToDefault",
			setup: func(t *testing.T) (string, fs.RuntimeFileService) {
				fileSvc := newTestFileSvc(t)
				require.NoError(t, fileSvc.WriteFile(context.Background(), caBundleRel, caPEM, constants.PermFilePublic))
				return filepath.Join(testutil.TempDir(t), constants.TestDoesNotExistPEMFilename), fileSvc
			},
			wantLoaded: true,
			wantRawCA:  caPEM,
		},
		{
			name: "ExplicitPathPriority",
			setup: func(t *testing.T) (string, fs.RuntimeFileService) {
				fileSvc := newTestFileSvc(t)
				explicitPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
				explicitPath := filepath.Join(testutil.TempDir(t), constants.TestExplicitPEMFilename)
				require.NoError(t, os.WriteFile(explicitPath, explicitPEM, constants.PermFilePublic))
				defaultPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
				require.NoError(t, fileSvc.WriteFile(context.Background(), caBundleRel, defaultPEM, constants.PermFilePublic))
				return explicitPath, fileSvc
			},
			wantLoaded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			explicitPath, fileSvc := tt.setup(t)

			ts := certs.NewTrustStore(nil)
			logger := testLogger()

			loaded := LoadTrustBundle(context.Background(), logger, explicitPath, fileSvc, ts)
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
		fileSvc := newTestFileSvc(t)
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		logger := testLogger()

		err = govsvc.ExportActuatorPublicKey(fileSvc, pubKey, "test-key-id", logger)
		require.NoError(t, err)

		pemRel := filepath.Join(constants.PkiDirname, constants.ActuatorPubPEMFilename)
		pemData, err := fileSvc.ReadFile(context.Background(), pemRel)
		require.NoError(t, err)

		block, _ := pem.Decode(pemData)
		require.NotNil(t, block)
		assert.Equal(t, "PUBLIC KEY", block.Type)
		assert.Equal(t, []byte(pubKey), block.Bytes)

		jsonRel := filepath.Join(constants.PkiDirname, constants.ActuatorPubJSONFilename)
		jsonData, err := fileSvc.ReadFile(context.Background(), jsonRel)
		require.NoError(t, err)

		var parsed models.ActuatorPublicKeyExport
		require.NoError(t, json.Unmarshal(jsonData, &parsed))
		assert.Equal(t, "test-key-id", parsed.KeyID)
		assert.Equal(t, hex.EncodeToString(pubKey), parsed.PublicKey)
		assert.Equal(t, "ed25519", parsed.Algorithm)
	})

	t.Run("NilLogger", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		err = govsvc.ExportActuatorPublicKey(fileSvc, pubKey, "key-id", nil)
		require.NoError(t, err)

		pemRel := filepath.Join(constants.PkiDirname, constants.ActuatorPubPEMFilename)
		_, err = fileSvc.ReadFile(context.Background(), pemRel)
		require.NoError(t, err, "PEM file should be created even with nil logger")
	})

	t.Run("OverwriteExisting", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)
		pubKey1, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		pubKey2, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		logger := testLogger()

		require.NoError(t, govsvc.ExportActuatorPublicKey(fileSvc, pubKey1, "key-1", logger))
		require.NoError(t, govsvc.ExportActuatorPublicKey(fileSvc, pubKey2, "key-2", logger))

		jsonRel := filepath.Join(constants.PkiDirname, constants.ActuatorPubJSONFilename)
		jsonData, err := fileSvc.ReadFile(context.Background(), jsonRel)
		require.NoError(t, err)

		var parsed models.ActuatorPublicKeyExport
		require.NoError(t, json.Unmarshal(jsonData, &parsed))
		assert.Equal(t, "key-2", parsed.KeyID)
		assert.Equal(t, hex.EncodeToString(pubKey2), parsed.PublicKey)
	})

	t.Run("FilePermissions", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		logger := testLogger()

		err = govsvc.ExportActuatorPublicKey(fileSvc, pubKey, "key-id", logger)
		require.NoError(t, err)

		pemRel := filepath.Join(constants.PkiDirname, constants.ActuatorPubPEMFilename)
		info, err := fileSvc.Stat(context.Background(), pemRel)
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			assert.True(t, info.Mode().Perm() == constants.PermFilePrivate)
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
		expiring, err := checkCertExpiry(filepath.Join(testutil.TempDir(t), constants.TestNonExistentCrtFilename))
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
		fileSvc := newTestFileSvc(t)
		certFile := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.TestClientCrtFilename))
		keyFile := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.TestClientKeyFilename))

		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		certContent := "fake-cert-pem-content"
		keyPEM, err := saveRenewedCerts(context.Background(), fileSvc, certFile, keyFile, certContent, privKey)
		require.NoError(t, err)
		assert.NotEmpty(t, keyPEM)
		assert.Contains(t, string(keyPEM), "EC PRIVATE KEY")

		certRel, err := fileSvc.Rel(certFile)
		require.NoError(t, err)
		savedCert, err := fileSvc.ReadFile(context.Background(), certRel)
		require.NoError(t, err)
		assert.Equal(t, certContent, string(savedCert))

		keyRel, err := fileSvc.Rel(keyFile)
		require.NoError(t, err)
		savedKey, err := fileSvc.ReadFile(context.Background(), keyRel)
		require.NoError(t, err)
		assert.Contains(t, string(savedKey), "EC PRIVATE KEY")
	})

	t.Run("CertChainAppendedToCertContent", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)
		certFile := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.TestClientCrtFilename))
		keyFile := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.TestClientKeyFilename))

		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		certContent := "-----BEGIN CERTIFICATE-----\nleaf\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nintermediate\n-----END CERTIFICATE-----"
		_, err = saveRenewedCerts(context.Background(), fileSvc, certFile, keyFile, certContent, privKey)
		require.NoError(t, err)

		certRel, err := fileSvc.Rel(certFile)
		require.NoError(t, err)
		savedCert, err := fileSvc.ReadFile(context.Background(), certRel)
		require.NoError(t, err)
		assert.Contains(t, string(savedCert), "leaf")
		assert.Contains(t, string(savedCert), "intermediate")
	})

	t.Run("KeyPathOutsideRuntime_ReturnsKeyWriteFailed", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)
		certFile := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.TestClientCrtFilename))
		keyFile := filepath.Join(testutil.TempDir(t), constants.TestClientKeyFilename)

		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		_, err = saveRenewedCerts(context.Background(), fileSvc, certFile, keyFile, "cert-content", privKey)
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrKeyWriteFailed))
	})

	t.Run("CertPathOutsideRuntime_ReturnsCertSaveFailed", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)
		keyFile := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.TestClientKeyFilename))
		certFile := filepath.Join(testutil.TempDir(t), constants.TestClientCrtFilename)

		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		_, err = saveRenewedCerts(context.Background(), fileSvc, certFile, keyFile, "cert-content", privKey)
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrCertSaveFailed))
	})
}

// ---------------------------------------------------------------------------
// RenewOperatorCertificate (error paths only — Tier 1)
// ---------------------------------------------------------------------------

func TestRenewOperatorCertificate_NonExistentCertFile(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	cfg := &config.Config{Endpoint: "https://fake:8443"}
	ci := certs.NewClientIdentity(tls.Certificate{})

	err := RenewOperatorCertificate(context.Background(), cfg, fileSvc, filepath.Join(testutil.TempDir(t), constants.TestNonExistentCrtFilename), constants.TestNonExistentCrtFilename, ci)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrCertParseFailed))
}

func TestRenewOperatorCertificate_CertNotExpiring(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	certPath := generateTestCert(t, time.Now(), time.Now().Add(365*24*time.Hour))
	keyPath := filepath.Join(filepath.Dir(certPath), constants.TestECPrivateKeyFilename)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, os.WriteFile(keyPath, keyPEM, constants.PermFilePrivate))

	cfg := &config.Config{Endpoint: "https://fake:8443"}
	ci := certs.NewClientIdentity(tls.Certificate{})

	err = RenewOperatorCertificate(context.Background(), cfg, fileSvc, certPath, keyPath, ci)
	require.NoError(t, err, "cert not expiring soon should return nil without making network calls")
}

// ---------------------------------------------------------------------------
// RunClientCertRenewalLoop (context cancellation only — Tier 1)
// ---------------------------------------------------------------------------

func TestRunClientCertRenewalLoop_ContextCancellation(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	cfg := &config.Config{Endpoint: "https://fake:8443"}
	ci := certs.NewClientIdentity(tls.Certificate{})
	logger := testLogger()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		RunClientCertRenewalLoop(ctx, cfg, fileSvc, constants.TestNonExistentCrtFilename, constants.TestNonExistentCrtFilename, logger, ci)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunClientCertRenewalLoop did not stop after context cancellation")
	}
}
