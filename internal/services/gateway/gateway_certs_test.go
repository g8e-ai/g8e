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

package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSecretManager is a mock implementation of SecretManager for unit testing.
type mockSecretManager struct {
	getKeyFunc          func(caType string) ([]byte, error)
	storeKeyFunc        func(caType string, keyDER []byte) error
	getServiceKeyFunc   func(serviceName string) ([]byte, error)
	storeServiceKeyFunc func(serviceName string, keyDER []byte) error
}

func (m *mockSecretManager) GetCAPrivateKey(caType string) ([]byte, error) {
	if m.getKeyFunc != nil {
		return m.getKeyFunc(caType)
	}
	return nil, errors.New("mock secret manager: key not found")
}

func (m *mockSecretManager) StoreCAPrivateKey(caType string, keyDER []byte) error {
	if m.storeKeyFunc != nil {
		return m.storeKeyFunc(caType, keyDER)
	}
	return nil
}

func (m *mockSecretManager) GetServicePrivateKey(serviceName string) ([]byte, error) {
	if m.getServiceKeyFunc != nil {
		return m.getServiceKeyFunc(serviceName)
	}
	return nil, errors.New("mock secret manager: service key not found")
}

func (m *mockSecretManager) StoreServicePrivateKey(serviceName string, keyDER []byte) error {
	if m.storeServiceKeyFunc != nil {
		return m.storeServiceKeyFunc(serviceName, keyDER)
	}
	return nil
}

func TestRandomSerial(t *testing.T) {

	t.Run("Generates non-zero serial", func(t *testing.T) {
		serial, err := randomSerial()
		require.NoError(t, err)
		assert.NotNil(t, serial)
		assert.NotZero(t, serial.Int64())
	})

	t.Run("Generates unique serials", func(t *testing.T) {
		serial1, err := randomSerial()
		require.NoError(t, err)

		serial2, err := randomSerial()
		require.NoError(t, err)

		assert.NotEqual(t, serial1, serial2, "serials should be unique")
	})

	t.Run("Serial is within 128-bit range", func(t *testing.T) {
		serial, err := randomSerial()
		require.NoError(t, err)

		limit := new(big.Int).Lsh(big.NewInt(1), 128)
		assert.Less(t, serial.Cmp(limit), 0, "serial should be less than 2^128")
		assert.GreaterOrEqual(t, serial.Cmp(big.NewInt(0)), 0, "serial should be non-negative")
	})
}

func TestWritePEMFile(t *testing.T) {

	t.Run("Writes DER as PEM with type", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		filePath := filepath.Join(tmpDir, "test.crt")

		testData := []byte("test data")
		err := writePEMFile(filePath, "CERTIFICATE", testData, 0644)
		require.NoError(t, err)

		// Verify file exists
		info, err := os.Stat(filePath)
		require.NoError(t, err)
		assert.False(t, info.IsDir())

		// Verify content is PEM-encoded
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)

		block, _ := pem.Decode(content)
		require.NotNil(t, block)
		assert.Equal(t, "CERTIFICATE", block.Type)
		assert.Equal(t, testData, block.Bytes)

		// Verify permissions on Unix
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
		}
	})

	t.Run("Writes raw PEM bytes without type", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		filePath := filepath.Join(tmpDir, "bundle.pem")

		testPEM := `-----BEGIN CERTIFICATE-----
test data
-----END CERTIFICATE-----`
		err := writePEMFile(filePath, "", []byte(testPEM), 0600)
		require.NoError(t, err)

		// Verify file exists
		info, err := os.Stat(filePath)
		require.NoError(t, err)

		// Verify content is written as-is
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, testPEM, string(content))

		// Verify permissions on Unix
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
		}
	})

	t.Run("Requires parent directories to exist", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		filePath := filepath.Join(tmpDir, "subdir", "nested", "test.crt")

		testData := []byte("test data")
		err := writePEMFile(filePath, "CERTIFICATE", testData, 0644)
		assert.Error(t, err, "writePEMFile should fail when parent directories don't exist")
	})

	t.Run("Overwrites existing file", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		filePath := filepath.Join(tmpDir, "test.crt")

		// Write initial content
		err := writePEMFile(filePath, "CERTIFICATE", []byte("initial"), 0644)
		require.NoError(t, err)

		// Overwrite with new content
		err = writePEMFile(filePath, "CERTIFICATE", []byte("updated"), 0644)
		require.NoError(t, err)

		// Verify content was overwritten
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)

		block, _ := pem.Decode(content)
		require.NotNil(t, block)
		assert.Equal(t, []byte("updated"), block.Bytes)
	})

	t.Run("Returns error on invalid path", func(t *testing.T) {
		// Use a path that cannot be created (e.g., inside a file)
		tmpDir := testutil.TempDir(t)
		filePath := filepath.Join(tmpDir, "notadir", "test.crt")

		// Create a file instead of directory
		err := os.WriteFile(filepath.Join(tmpDir, "notadir"), []byte("file"), 0644)
		require.NoError(t, err)

		err = writePEMFile(filePath, "CERTIFICATE", []byte("test"), 0644)
		assert.Error(t, err)
	})
}

func TestFileExists(t *testing.T) {

	t.Run("Returns true for existing file", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		filePath := filepath.Join(tmpDir, "exists.txt")

		err := os.WriteFile(filePath, []byte("content"), 0644)
		require.NoError(t, err)

		exists := fileExists(filePath)
		assert.True(t, exists)
	})

	t.Run("Returns false for non-existent file", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		filePath := filepath.Join(tmpDir, "doesnotexist.txt")

		exists := fileExists(filePath)
		assert.False(t, exists)
	})

	t.Run("Returns true for directory", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)

		exists := fileExists(tmpDir)
		assert.True(t, exists, "fileExists returns true for directories (os.Stat succeeds)")
	})

	t.Run("Returns false for empty path", func(t *testing.T) {
		exists := fileExists("")
		assert.False(t, exists)
	})
}

func TestIsExpiringSoon(t *testing.T) {

	t.Run("Returns true for empty certificate", func(t *testing.T) {
		cert := tls.Certificate{}
		expiring := isExpiringSoon(cert)
		assert.True(t, expiring, "empty certificate should be considered expiring")
	})

	t.Run("Returns true for expired certificate", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "test"},
			NotBefore:    time.Now().Add(-48 * time.Hour),
			NotAfter:     time.Now().Add(-24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
		}

		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		require.NoError(t, err)

		tlsCert := tls.Certificate{
			Certificate: [][]byte{certDER},
		}

		expiring := isExpiringSoon(tlsCert)
		assert.True(t, expiring, "expired certificate should be considered expiring")
	})

	t.Run("Returns true for certificate expiring in 29 days", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "test"},
			NotBefore:    time.Now().Add(-1 * time.Minute),
			NotAfter:     time.Now().Add(29 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
		}

		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		require.NoError(t, err)

		tlsCert := tls.Certificate{
			Certificate: [][]byte{certDER},
		}

		expiring := isExpiringSoon(tlsCert)
		assert.True(t, expiring, "certificate expiring in 29 days should be considered expiring")
	})

	t.Run("Returns false for certificate expiring in 31 days", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "test"},
			NotBefore:    time.Now().Add(-1 * time.Minute),
			NotAfter:     time.Now().Add(31 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
		}

		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		require.NoError(t, err)

		tlsCert := tls.Certificate{
			Certificate: [][]byte{certDER},
		}

		expiring := isExpiringSoon(tlsCert)
		assert.False(t, expiring, "certificate expiring in 31 days should not be considered expiring")
	})

	t.Run("Returns false for certificate with long validity", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "test"},
			NotBefore:    time.Now().Add(-1 * time.Minute),
			NotAfter:     time.Now().Add(90 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
		}

		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		require.NoError(t, err)

		tlsCert := tls.Certificate{
			Certificate: [][]byte{certDER},
		}

		expiring := isExpiringSoon(tlsCert)
		assert.False(t, expiring, "certificate with 90-day validity should not be considered expiring")
	})

	t.Run("Returns true for malformed certificate", func(t *testing.T) {
		tlsCert := tls.Certificate{
			Certificate: [][]byte{[]byte("invalid cert data")},
		}

		expiring := isExpiringSoon(tlsCert)
		assert.True(t, expiring, "malformed certificate should be considered expiring")
	})
}

func TestIsCurveP256(t *testing.T) {

	t.Run("Returns true for P-256 public key", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		isP256 := isCurveP256(&key.PublicKey)
		assert.True(t, isP256)
	})

	t.Run("Returns false for P-384 public key", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		require.NoError(t, err)

		isP256 := isCurveP256(&key.PublicKey)
		assert.False(t, isP256)
	})

	t.Run("Returns false for P-521 public key", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		require.NoError(t, err)

		isP256 := isCurveP256(&key.PublicKey)
		assert.False(t, isP256)
	})

	t.Run("Returns false for non-ECDSA public key", func(t *testing.T) {
		// Use a non-ECDSA public key type
		var nonECKey interface{} = "not an ecdsa key"
		isP256 := isCurveP256(nonECKey)
		assert.False(t, isP256)
	})

	t.Run("Returns false for nil", func(t *testing.T) {
		isP256 := isCurveP256(nil)
		assert.False(t, isP256)
	})
}

func TestLoadCACertificate(t *testing.T) {

	t.Run("Loads valid PEM certificate", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		certPath := filepath.Join(tmpDir, "ca.crt")

		// Generate a test certificate
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		template := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "Test CA"},
			NotBefore:             time.Now().Add(-1 * time.Minute),
			NotAfter:              time.Now().Add(365 * 24 * time.Hour),
			KeyUsage:              x509.KeyUsageCertSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		}

		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		require.NoError(t, err)

		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
		err = os.WriteFile(certPath, certPEM, 0644)
		require.NoError(t, err)

		// Load the certificate using the method
		var loadedCert *x509.Certificate
		pki := &PKIAuthority{}
		err = pki.loadCACertificate(certPath, &loadedCert)
		require.NoError(t, err)

		assert.NotNil(t, loadedCert, "loaded certificate should not be nil")
		assert.Equal(t, "Test CA", loadedCert.Subject.CommonName)
		assert.True(t, loadedCert.IsCA)
	})

	t.Run("Returns error for non-existent file", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		certPath := filepath.Join(tmpDir, "doesnotexist.crt")

		var loadedCert *x509.Certificate
		pki := &PKIAuthority{}
		err := pki.loadCACertificate(certPath, &loadedCert)
		assert.Error(t, err)
	})

	t.Run("Returns error for invalid PEM", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		certPath := filepath.Join(tmpDir, "invalid.crt")

		err := os.WriteFile(certPath, []byte("not valid PEM"), 0644)
		require.NoError(t, err)

		var loadedCert *x509.Certificate
		pki := &PKIAuthority{}
		err = pki.loadCACertificate(certPath, &loadedCert)
		assert.Error(t, err)
	})

	t.Run("Returns error for malformed certificate", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		certPath := filepath.Join(tmpDir, "malformed.crt")

		// Write PEM with invalid DER data
		invalidPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid DER")})
		err := os.WriteFile(certPath, invalidPEM, 0644)
		require.NoError(t, err)

		var loadedCert *x509.Certificate
		pki := &PKIAuthority{}
		err = pki.loadCACertificate(certPath, &loadedCert)
		assert.Error(t, err)
	})
}

func TestLoadCAPrivateKey(t *testing.T) {

	t.Run("Returns error when secret manager is nil", func(t *testing.T) {
		pki := &PKIAuthority{
			secretManager: nil,
		}

		var loadedKey *ecdsa.PrivateKey
		err := pki.loadCAPrivateKey("root", &loadedKey)
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrPKIPrivateKeyRequired)
	})

	t.Run("Returns error when key not found in keystore", func(t *testing.T) {
		mockSM := &mockSecretManager{
			getKeyFunc: func(caType string) ([]byte, error) {
				return nil, constants.ErrKeyStoreKeyNotFound
			},
		}

		pki := &PKIAuthority{
			secretManager: mockSM,
		}

		var loadedKey *ecdsa.PrivateKey
		err := pki.loadCAPrivateKey("nonexistent", &loadedKey)
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrKeyStoreKeyNotFound)
	})

	t.Run("Returns error for malformed key DER", func(t *testing.T) {
		mockSM := &mockSecretManager{
			getKeyFunc: func(caType string) ([]byte, error) {
				return []byte("invalid DER"), nil
			},
		}

		pki := &PKIAuthority{
			secretManager: mockSM,
		}

		var loadedKey *ecdsa.PrivateKey
		err := pki.loadCAPrivateKey("root", &loadedKey)
		assert.Error(t, err)
	})
}

func TestNewPKIAuthority(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("Uses default PKI dir when pkiDir is empty", func(t *testing.T) {
		dataDir := testutil.TempDir(t)

		pki := newPKIAuthority(dataDir, "", nil, nil, logger)
		expectedPKIDir := filepath.Join(dataDir, constants.PkiDirname)
		assert.Equal(t, expectedPKIDir, pki.pkiDir)
	})

	t.Run("Uses custom PKI dir when provided", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		customPKIDir := filepath.Join(dataDir, "custom-pki")

		pki := newPKIAuthority(dataDir, customPKIDir, nil, nil, logger)
		assert.Equal(t, customPKIDir, pki.pkiDir)
	})

	t.Run("Initializes all fields", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")

		pki := newPKIAuthority(dataDir, pkiDir, nil, nil, logger)
		assert.Equal(t, pkiDir, pki.pkiDir)
		assert.Nil(t, pki.db)
		assert.Nil(t, pki.secretManager)
		assert.Equal(t, logger, pki.logger)
		assert.Nil(t, pki.rootCert)
		assert.Nil(t, pki.rootKey)
	})
}

func TestPKIAuthority_PathGetters(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	dataDir := testutil.TempDir(t)
	pkiDir := filepath.Join(dataDir, "pki")

	pki := newPKIAuthority(dataDir, pkiDir, nil, nil, logger)

	t.Run("TrustBundlePath returns correct path", func(t *testing.T) {
		expected := filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
		assert.Equal(t, expected, pki.TrustBundlePath())
	})

	t.Run("RootCAPath returns correct path", func(t *testing.T) {
		expected := filepath.Join(pkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCA)
		assert.Equal(t, expected, pki.RootCAPath())
	})

	t.Run("BinariesDir returns correct path", func(t *testing.T) {
		expected := filepath.Join(pkiDir, constants.PkiSubdirBinaries)
		assert.Equal(t, expected, pki.BinariesDir())
	})

	t.Run("PKIDir returns correct path", func(t *testing.T) {
		assert.Equal(t, pkiDir, pki.PKIDir())
	})
}

func TestPKIAuthority_VerifyCertificate_Unit(t *testing.T) {

	t.Run("Returns error for nil certificate", func(t *testing.T) {
		pki := &PKIAuthority{}
		err := pki.VerifyCertificate(nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrPKINoCertificate)
	})
}

func TestPKIAuthority_RevokeCertificate(t *testing.T) {

	t.Run("Returns error when database is nil", func(t *testing.T) {
		pki := &PKIAuthority{db: nil}
		err := pki.RevokeCertificate("123", "test reason")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrPKIDatabaseNotAvailable)
	})
}

func TestPKIAuthority_GenerateCRL_Unit(t *testing.T) {

	t.Run("Returns error when database is nil", func(t *testing.T) {
		pki := &PKIAuthority{db: nil}
		_, err := pki.GenerateCRL()
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrPKIDatabaseNotAvailable)
	})
}

func TestPKIAuthority_TLSConfig_Unit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("Returns valid TLS config", func(t *testing.T) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "Test Root CA"},
			NotBefore:    time.Now(),
			NotAfter:     time.Now().Add(365 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageCertSign,
			IsCA:         true,
		}
		certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		cert, _ := x509.ParseCertificate(certDER)

		pki := &PKIAuthority{
			logger:       logger,
			rootCert:     cert,
			operatorCert: cert,
		}

		config := pki.TLSConfig()
		assert.NotNil(t, config)
		assert.Equal(t, uint16(tls.VersionTLS13), config.MinVersion)
		assert.Equal(t, tls.RequireAndVerifyClientCert, config.ClientAuth)
		assert.NotNil(t, config.ClientCAs)
		assert.NotNil(t, config.GetCertificate)
	})
}

func TestPKIAuthority_SignCSR_Unit(t *testing.T) {

	t.Run("Returns error for gateway-peer when CA not loaded", func(t *testing.T) {
		pki := &PKIAuthority{
			gatewayPeerCert: nil,
		}

		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		csr := &x509.CertificateRequest{
			Subject: pkix.Name{CommonName: "test"},
		}
		csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csr, key)
		csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

		_, _, err := pki.SignCSR(string(csrPEM), "gateway-peer", "", "", "", "", "gateway-id")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrPKIGatewayPeerCANotLoaded)
	})

	t.Run("Returns error for operator when CA not loaded", func(t *testing.T) {
		pki := &PKIAuthority{
			operatorCert: nil,
		}

		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		csr := &x509.CertificateRequest{
			Subject: pkix.Name{CommonName: "test"},
		}
		csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csr, key)
		csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

		_, _, err := pki.SignCSR(string(csrPEM), "operator", "org-id", "op-id", "session-id", "", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrPKIOperatorCANotLoaded)
	})
}

func TestPKIAuthority_SignDelegatedCSR(t *testing.T) {

	t.Run("Returns error when operator CA not loaded", func(t *testing.T) {
		pki := &PKIAuthority{
			operatorCert: nil,
		}

		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		csr := &x509.CertificateRequest{
			Subject: pkix.Name{CommonName: "test"},
		}
		csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csr, key)
		csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

		_, _, err := pki.SignDelegatedCSR(string(csrPEM), "app-name", "user-id")
		assert.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrPKIOperatorCANotLoaded)
	})
}
