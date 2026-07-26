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
