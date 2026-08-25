// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
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
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
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
		fileSvc := newTestFileSvc(t)

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
		relPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, "test-ca.crt")
		err = fileSvc.WriteFile(context.Background(), relPath, certPEM, constants.PermFilePublic)
		require.NoError(t, err)

		// Load the certificate using the method
		var loadedCert *x509.Certificate
		pki := &PKIAuthority{fileSvc: fileSvc}
		err = pki.loadCACertificate(relPath, &loadedCert)
		require.NoError(t, err)

		assert.NotNil(t, loadedCert, "loaded certificate should not be nil")
		assert.Equal(t, "Test CA", loadedCert.Subject.CommonName)
		assert.True(t, loadedCert.IsCA)
	})

	t.Run("Returns error for non-existent file", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)

		var loadedCert *x509.Certificate
		pki := &PKIAuthority{fileSvc: fileSvc}
		err := pki.loadCACertificate(filepath.Join(constants.PkiDirname, "doesnotexist.crt"), &loadedCert)
		assert.Error(t, err)
	})

	t.Run("Returns error for invalid PEM", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)

		relPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, "invalid.crt")
		err := fileSvc.WriteFile(context.Background(), relPath, []byte("not valid PEM"), constants.PermFilePublic)
		require.NoError(t, err)

		var loadedCert *x509.Certificate
		pki := &PKIAuthority{fileSvc: fileSvc}
		err = pki.loadCACertificate(relPath, &loadedCert)
		assert.Error(t, err)
	})

	t.Run("Returns error for malformed certificate", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)

		invalidPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid DER")})
		relPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, "malformed.crt")
		err := fileSvc.WriteFile(context.Background(), relPath, invalidPEM, constants.PermFilePublic)
		require.NoError(t, err)

		var loadedCert *x509.Certificate
		pki := &PKIAuthority{fileSvc: fileSvc}
		err = pki.loadCACertificate(relPath, &loadedCert)
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

	t.Run("Initializes all fields", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)

		pki := newPKIAuthority(fileSvc, nil, nil, logger)
		assert.Nil(t, pki.db)
		assert.Nil(t, pki.secretManager)
		assert.Equal(t, logger, pki.logger)
		assert.Nil(t, pki.rootCert)
		assert.Nil(t, pki.rootKey)
	})
}

func TestPKIAuthority_PathGetters(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fileSvc := newTestFileSvc(t)

	pki := newPKIAuthority(fileSvc, nil, nil, logger)

	t.Run("TrustBundlePath returns correct path", func(t *testing.T) {
		expected := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle))
		assert.Equal(t, expected, pki.TrustBundlePath())
	})

	t.Run("RootCAPath returns correct path", func(t *testing.T) {
		expected := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA))
		assert.Equal(t, expected, pki.RootCAPath())
	})

	t.Run("BinariesDir returns correct path", func(t *testing.T) {
		expected := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirBinaries))
		assert.Equal(t, expected, pki.BinariesDir())
	})

	t.Run("PKIDir returns correct path", func(t *testing.T) {
		assert.Equal(t, fileSvc.Resolve(constants.PkiDirname), pki.PKIDir())
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
		assert.NotContains(t, config.CurvePreferences, tls.X25519,
			"gateway serving TLS must not use X25519 (excluded from Go's FIPS TLS mode)")
		assert.Contains(t, config.CurvePreferences, tls.X25519MLKEM768)
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

func TestPKIAuthority_CertsUseECDSASignatures_NotEd25519(t *testing.T) {
	dataDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dataDir, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	sm := newTestSecretManager(t, db.db, fileSvc)

	pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	t.Run("Root CA uses ECDSA signature algorithm", func(t *testing.T) {
		assert.NotEqual(t, x509.PureEd25519, pki.rootCert.SignatureAlgorithm,
			"root CA must not use Ed25519 signature (excluded from Go FIPS TLS mode)")
		assert.Contains(t, pki.rootCert.SignatureAlgorithm.String(), "ECDSA",
			"root CA must use ECDSA signature algorithm")
	})

	t.Run("Hub intermediate CA uses ECDSA signature algorithm", func(t *testing.T) {
		assert.NotEqual(t, x509.PureEd25519, pki.hubCert.SignatureAlgorithm,
			"hub CA must not use Ed25519 signature (excluded from Go FIPS TLS mode)")
		assert.Contains(t, pki.hubCert.SignatureAlgorithm.String(), "ECDSA",
			"hub CA must use ECDSA signature algorithm")
	})

	t.Run("Operator intermediate CA uses ECDSA signature algorithm", func(t *testing.T) {
		assert.NotEqual(t, x509.PureEd25519, pki.operatorCert.SignatureAlgorithm,
			"operator CA must not use Ed25519 signature (excluded from Go FIPS TLS mode)")
		assert.Contains(t, pki.operatorCert.SignatureAlgorithm.String(), "ECDSA",
			"operator CA must use ECDSA signature algorithm")
	})

	t.Run("Gateway peer intermediate CA uses ECDSA signature algorithm", func(t *testing.T) {
		assert.NotEqual(t, x509.PureEd25519, pki.gatewayPeerCert.SignatureAlgorithm,
			"gateway peer CA must not use Ed25519 signature (excluded from Go FIPS TLS mode)")
		assert.Contains(t, pki.gatewayPeerCert.SignatureAlgorithm.String(), "ECDSA",
			"gateway peer CA must use ECDSA signature algorithm")
	})

	t.Run("Service certificate uses ECDSA signature algorithm", func(t *testing.T) {
		x509Cert, err := x509.ParseCertificate(pki.serviceCert.Certificate[0])
		require.NoError(t, err)
		assert.NotEqual(t, x509.PureEd25519, x509Cert.SignatureAlgorithm,
			"service cert must not use Ed25519 signature (excluded from Go FIPS TLS mode)")
		assert.Contains(t, x509Cert.SignatureAlgorithm.String(), "ECDSA",
			"service cert must use ECDSA signature algorithm")
	})

	t.Run("Leaf cert signed via SignCSR uses ECDSA signature algorithm", func(t *testing.T) {
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
		require.NoError(t, err)

		block, _ := pem.Decode([]byte(certPEM))
		require.NotNil(t, block)
		leafCert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		assert.NotEqual(t, x509.PureEd25519, leafCert.SignatureAlgorithm,
			"leaf cert must not use Ed25519 signature (excluded from Go FIPS TLS mode)")
		assert.Contains(t, leafCert.SignatureAlgorithm.String(), "ECDSA",
			"leaf cert must use ECDSA signature algorithm")
	})
}

func TestLoadCACertificate_RejectsEd25519Signature(t *testing.T) {
	fileSvc := newTestFileSvc(t)

	ed25519PubKey, ed25519PrivKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Ed25519 Test CA"},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, ed25519PubKey, ed25519PrivKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	relPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, "ed25519-test-ca.crt")
	err = fileSvc.WriteFile(context.Background(), relPath, certPEM, constants.PermFilePublic)
	require.NoError(t, err)

	var loadedCert *x509.Certificate
	pki := &PKIAuthority{fileSvc: fileSvc}
	err = pki.loadCACertificate(relPath, &loadedCert)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPKIEd25519CertRejected)
	assert.Nil(t, loadedCert, "rejected cert must not be stored")
}
