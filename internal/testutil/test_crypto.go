// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/require"
)

// GenerateTestCSR generates a test CSR for the given common name using RSA.
func GenerateTestCSR(t *testing.T, commonName string) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: commonName,
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privateKey)
	require.NoError(t, err)

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEM)
}

// GenerateTestCSRP256 generates a test CSR for the given common name using ECDSA P-256.
// This matches the actual CLI behavior for CSR generation (see internal/cli/auth/client.go).
func GenerateTestCSRP256(t *testing.T, commonName string) string {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: commonName,
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privateKey)
	require.NoError(t, err)

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEM)
}

// GenerateTestCA generates a minimal valid self-signed CA certificate and returns
// the certificate PEM. The certificate has IsCA=true and the required key usages
// so it can be used as a RootCAs trust anchor in TLS configs.
func GenerateTestCA(t *testing.T, commonName string) string {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}))
}

// GenerateTestCertificate generates a minimal valid test certificate and returns
// the certificate PEM and private key PEM.
func GenerateTestCertificate(t *testing.T, commonName string) (certPEM string, keyPEM string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	certPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}))

	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	require.NoError(t, err)

	keyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	}))

	return certPEM, keyPEM
}

// GenerateTestSignedCert generates a leaf certificate signed by the given parent
// CA certificate/key and returns the cert PEM and the leaf's ECDSA private key.
// The leaf has client+server auth key usages and is NOT a CA. Use this with
// GenerateTestCA to build a root + leaf pair where the leaf chains to the root.
func GenerateTestSignedCert(t *testing.T, commonName string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (certPEM string, key *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, parent, &key.PublicKey, parentKey)
	require.NoError(t, err)
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return certPEM, key
}

// GenerateTestSignedCertWithExpiry generates a leaf certificate signed by the
// given parent CA with a custom NotAfter. Returns the cert PEM and the leaf's
// ECDSA private key. Use this to test expiry-based rotation/recovery decisions.
func GenerateTestSignedCertWithExpiry(t *testing.T, commonName string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, notAfter time.Time) (certPEM string, key *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, parent, &key.PublicKey, parentKey)
	require.NoError(t, err)
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return certPEM, key
}

// ParseTestCert decodes a PEM-encoded certificate and returns the parsed
// x509.Certificate. Fails the test on decode/parse error.
func ParseTestCert(t *testing.T, pemStr string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(pemStr))
	require.NotNil(t, block, "failed to decode PEM")
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}

// GenerateTestECPrivateKey generates a minimal valid EC private key PEM.
func GenerateTestECPrivateKey(t *testing.T) string {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	require.NoError(t, err)

	keyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	}))

	return keyPEM
}

// PKICertPaths holds the standard PKI directory structure paths.
type PKICertPaths struct {
	RootCA          string
	HubCA           string
	OperatorCA      string
	BootstrapCA     string
	TrustBundle     string
	HubBundle       string
	OperatorBundle  string
	BootstrapBundle string
}

// GetPKICertPaths returns the standard PKI certificate paths for a given PKI directory.
func GetPKICertPaths(pkiDir string) PKICertPaths {
	return PKICertPaths{
		RootCA:          filepath.Join(pkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCA),
		HubCA:           filepath.Join(pkiDir, constants.PkiSubdirAuthorities, constants.PkiFileHubCA),
		OperatorCA:      filepath.Join(pkiDir, constants.PkiSubdirAuthorities, constants.PkiFileOperatorCA),
		BootstrapCA:     filepath.Join(pkiDir, constants.PkiSubdirAuthorities, constants.PkiFileBootstrapCA),
		TrustBundle:     filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileRootBundle),
		HubBundle:       filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle),
		OperatorBundle:  filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileOperatorBundle),
		BootstrapBundle: filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileBootstrapBundle),
	}
}

// ReadCACert reads a CA certificate from the given path with graceful error handling.
func ReadCACert(t *testing.T, path, caName string) []byte {
	t.Helper()

	certPEM, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			require.NoError(t, err, "CA certificate '%s' not found at %s. Ensure PKI is initialized before accessing certificates.", caName, path)
		}
		require.NoError(t, err, "failed to read CA certificate '%s' from %s", caName, path)
	}

	if len(certPEM) == 0 {
		require.NotEmpty(t, certPEM, "CA certificate '%s' at %s is empty", caName, path)
	}

	return certPEM
}

// ReadRootCA reads the root CA certificate from the PKI directory.
func ReadRootCA(t *testing.T, pkiDir string) []byte {
	t.Helper()
	paths := GetPKICertPaths(pkiDir)
	return ReadCACert(t, paths.RootCA, "root")
}

// ReadOperatorCA reads the Operator CA certificate from the PKI directory.
func ReadOperatorCA(t *testing.T, pkiDir string) []byte {
	t.Helper()
	paths := GetPKICertPaths(pkiDir)
	return ReadCACert(t, paths.OperatorCA, "operator")
}
