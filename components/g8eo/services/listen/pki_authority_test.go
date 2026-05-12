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

package listen

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPKIAuthority_EnsurePKI(t *testing.T) {
	t.Run("Full PKI hierarchy initialization", func(t *testing.T) {
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()

		pki := newPKIAuthority(dataDir, pkiDir, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify directory structure
		dirs := []string{
			filepath.Join(pkiDir, "root"),
			filepath.Join(pkiDir, "authorities"),
			filepath.Join(pkiDir, "issued", "hub"),
			filepath.Join(pkiDir, "issued", "apps"),
			filepath.Join(pkiDir, "trust"),
			filepath.Join(pkiDir, "revocation"),
		}
		for _, dir := range dirs {
			info, err := os.Stat(dir)
			require.NoError(t, err, "directory %s should exist", dir)
			assert.True(t, info.IsDir(), "%s should be a directory", dir)
		}
	})

	t.Run("Root CA generation", func(t *testing.T) {
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()

		pki := newPKIAuthority(dataDir, pkiDir, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify root CA files exist
		rootCertPath := filepath.Join(pkiDir, "root", "root_ca.crt")
		rootKeyPath := filepath.Join(pkiDir, "root", "root_ca.key")

		certPEM, err := os.ReadFile(rootCertPath)
		require.NoError(t, err)
		assert.NotEmpty(t, certPEM)

		keyPEM, err := os.ReadFile(rootKeyPath)
		require.NoError(t, err)
		assert.NotEmpty(t, keyPEM)

		// Verify file permissions
		certInfo, err := os.Stat(rootCertPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), certInfo.Mode().Perm())

		keyInfo, err := os.Stat(rootKeyPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), keyInfo.Mode().Perm())
	})

	t.Run("Intermediate CA generation", func(t *testing.T) {
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()

		pki := newPKIAuthority(dataDir, pkiDir, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify all intermediate CAs
		intermediates := []string{"hub_ca", "operator_ca", "bootstrap_ca"}
		for _, name := range intermediates {
			certPath := filepath.Join(pkiDir, "authorities", name+".crt")
			keyPath := filepath.Join(pkiDir, "authorities", name+".key")

			_, err := os.Stat(certPath)
			require.NoError(t, err, "%s cert should exist", name)

			_, err = os.Stat(keyPath)
			require.NoError(t, err, "%s key should exist", name)
		}
	})

	t.Run("Service certificate generation", func(t *testing.T) {
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()

		pki := newPKIAuthority(dataDir, pkiDir, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify operator-listen service certificate
		serviceCertPath := filepath.Join(pkiDir, "issued", "hub", "operator-listen.crt")
		serviceKeyPath := filepath.Join(pkiDir, "issued", "hub", "operator-listen.key")
		serviceChainPath := filepath.Join(pkiDir, "issued", "hub", "operator-listen.chain.pem")

		_, err = os.Stat(serviceCertPath)
		require.NoError(t, err)

		_, err = os.Stat(serviceKeyPath)
		require.NoError(t, err)

		_, err = os.Stat(serviceChainPath)
		require.NoError(t, err)
	})

	t.Run("Trust bundle generation", func(t *testing.T) {
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()

		pki := newPKIAuthority(dataDir, pkiDir, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify trust bundles
		bundles := []string{"root.pem", "hub-bundle.pem", "operator-bundle.pem", "bootstrap-bundle.pem"}
		for _, bundle := range bundles {
			bundlePath := filepath.Join(pkiDir, "trust", bundle)
			_, err := os.Stat(bundlePath)
			require.NoError(t, err, "trust bundle %s should exist", bundle)
		}

		// Verify trust domain metadata
		trustDomainPath := filepath.Join(pkiDir, "trust", "trust-domain.json")
		_, err = os.Stat(trustDomainPath)
		require.NoError(t, err, "trust domain metadata should exist")
	})

	t.Run("No root-level ca.crt mirror", func(t *testing.T) {
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()

		pki := newPKIAuthority(dataDir, pkiDir, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify that ca.crt does NOT exist at the PKI root
		legacyPath := filepath.Join(pkiDir, "ca.crt")
		_, err = os.Stat(legacyPath)
		assert.True(t, os.IsNotExist(err), "legacy ca.crt should not exist at PKI root")
	})
}

func TestPKIAuthority_ChainValidity(t *testing.T) {
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()

	pki := newPKIAuthority(dataDir, pkiDir, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("Root CA is self-signed", func(t *testing.T) {
		rootCertPEM, err := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
		require.NoError(t, err)

		block, _ := pem.Decode(rootCertPEM)
		require.NotNil(t, block)

		rootCert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		// Verify self-signed: Issuer equals Subject
		assert.Equal(t, rootCert.Issuer.CommonName, rootCert.Subject.CommonName)
		assert.True(t, rootCert.IsCA)
		assert.Equal(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, rootCert.KeyUsage)
	})

	t.Run("Intermediate CA chain validity", func(t *testing.T) {
		rootCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
		hubCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "authorities", "hub_ca.crt"))

		rootBlock, _ := pem.Decode(rootCertPEM)
		hubBlock, _ := pem.Decode(hubCertPEM)

		rootCert, _ := x509.ParseCertificate(rootBlock.Bytes)
		hubCert, _ := x509.ParseCertificate(hubBlock.Bytes)

		// Verify hub is signed by root
		assert.Equal(t, rootCert.Subject.CommonName, hubCert.Issuer.CommonName)
		assert.True(t, hubCert.IsCA)
		assert.Equal(t, int(1), hubCert.MaxPathLen)
	})

	t.Run("Service certificate chain validity", func(t *testing.T) {
		hubCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "authorities", "hub_ca.crt"))
		serviceCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-listen.crt"))

		hubBlock, _ := pem.Decode(hubCertPEM)
		serviceBlock, _ := pem.Decode(serviceCertPEM)

		hubCert, _ := x509.ParseCertificate(hubBlock.Bytes)
		serviceCert, _ := x509.ParseCertificate(serviceBlock.Bytes)

		// Verify service cert is signed by hub intermediate
		assert.Equal(t, hubCert.Subject.CommonName, serviceCert.Issuer.CommonName)
		assert.False(t, serviceCert.IsCA)
		assert.Equal(t, x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment, serviceCert.KeyUsage)
	})
}

func TestPKIAuthority_IssuerSeparation(t *testing.T) {
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()

	pki := newPKIAuthority(dataDir, pkiDir, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("Distinct intermediate CAs", func(t *testing.T) {
		hubCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "authorities", "hub_ca.crt"))
		operatorCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "authorities", "operator_ca.crt"))
		bootstrapCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "authorities", "bootstrap_ca.crt"))

		hubBlock, _ := pem.Decode(hubCertPEM)
		operatorBlock, _ := pem.Decode(operatorCertPEM)
		bootstrapBlock, _ := pem.Decode(bootstrapCertPEM)

		hubCert, _ := x509.ParseCertificate(hubBlock.Bytes)
		operatorCert, _ := x509.ParseCertificate(operatorBlock.Bytes)
		bootstrapCert, _ := x509.ParseCertificate(bootstrapBlock.Bytes)

		// Verify each has a distinct CommonName
		assert.NotEqual(t, hubCert.Subject.CommonName, operatorCert.Subject.CommonName)
		assert.NotEqual(t, hubCert.Subject.CommonName, bootstrapCert.Subject.CommonName)
		assert.NotEqual(t, operatorCert.Subject.CommonName, bootstrapCert.Subject.CommonName)

		// Verify all are signed by the same root
		rootCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
		rootBlock, _ := pem.Decode(rootCertPEM)
		rootCert, _ := x509.ParseCertificate(rootBlock.Bytes)

		assert.Equal(t, rootCert.Subject.CommonName, hubCert.Issuer.CommonName)
		assert.Equal(t, rootCert.Subject.CommonName, operatorCert.Issuer.CommonName)
		assert.Equal(t, rootCert.Subject.CommonName, bootstrapCert.Issuer.CommonName)
	})
}

func TestPKIAuthority_URISAN(t *testing.T) {
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()

	pki := newPKIAuthority(dataDir, pkiDir, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("Service certificate has SPIFFE URI SAN", func(t *testing.T) {
		serviceCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-listen.crt"))
		block, _ := pem.Decode(serviceCertPEM)
		serviceCert, _ := x509.ParseCertificate(block.Bytes)

		// Verify URI SANs exist
		assert.NotEmpty(t, serviceCert.URIs)

		// Verify SPIFFE workload identity
		expectedURI := "spiffe://g8e.local/hub/operator-listen"
		found := false
		for _, uri := range serviceCert.URIs {
			if uri.String() == expectedURI {
				found = true
				break
			}
		}
		assert.True(t, found, "service certificate should have SPIFFE URI SAN")
	})
}

func TestPKIAuthority_ValidityPeriods(t *testing.T) {
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()

	pki := newPKIAuthority(dataDir, pkiDir, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("Root CA validity period", func(t *testing.T) {
		rootCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
		block, _ := pem.Decode(rootCertPEM)
		rootCert, _ := x509.ParseCertificate(block.Bytes)

		duration := rootCert.NotAfter.Sub(rootCert.NotBefore)
		expectedDuration := time.Duration(rootValidityDays) * 24 * time.Hour

		// Allow 1 minute tolerance
		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})

	t.Run("Intermediate CA validity period", func(t *testing.T) {
		hubCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "authorities", "hub_ca.crt"))
		block, _ := pem.Decode(hubCertPEM)
		hubCert, _ := x509.ParseCertificate(block.Bytes)

		duration := hubCert.NotAfter.Sub(hubCert.NotBefore)
		expectedDuration := time.Duration(intermediateValidityDays) * 24 * time.Hour

		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})

	t.Run("Service certificate validity period", func(t *testing.T) {
		serviceCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-listen.crt"))
		block, _ := pem.Decode(serviceCertPEM)
		serviceCert, _ := x509.ParseCertificate(block.Bytes)

		duration := serviceCert.NotAfter.Sub(serviceCert.NotBefore)
		expectedDuration := time.Duration(serviceValidityDays) * 24 * time.Hour

		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})
}

func TestPKIAuthority_EKU(t *testing.T) {
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()

	pki := newPKIAuthority(dataDir, pkiDir, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("CA has correct KeyUsage", func(t *testing.T) {
		rootCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
		block, _ := pem.Decode(rootCertPEM)
		rootCert, _ := x509.ParseCertificate(block.Bytes)

		// CAs should have CertSign and CRLSign
		assert.Equal(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, rootCert.KeyUsage)
	})

	t.Run("Service certificate has correct EKU", func(t *testing.T) {
		serviceCertPEM, _ := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-listen.crt"))
		block, _ := pem.Decode(serviceCertPEM)
		serviceCert, _ := x509.ParseCertificate(block.Bytes)

		// Service cert should have both ServerAuth and ClientAuth
		assert.Contains(t, serviceCert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
		assert.Contains(t, serviceCert.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
	})
}

func TestPKIAuthority_TLSConfig(t *testing.T) {
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()

	pki := newPKIAuthority(dataDir, pkiDir, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("TLS 1.3 only", func(t *testing.T) {
		tlsConfig := pki.TLSConfig()
		assert.Equal(t, uint16(tls.VersionTLS13), tlsConfig.MinVersion)
	})

	t.Run("GetCertificate returns valid cert", func(t *testing.T) {
		tlsConfig := pki.TLSConfig()
		cert, err := tlsConfig.GetCertificate(nil)
		require.NoError(t, err)
		assert.NotNil(t, cert)
		assert.NotEmpty(t, cert.Certificate)
	})
}

func TestPKIAuthority_TrustBundlePath(t *testing.T) {
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()

	pki := newPKIAuthority(dataDir, pkiDir, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	expectedPath := filepath.Join(pkiDir, "trust", "hub-bundle.pem")
	actualPath := pki.TrustBundlePath()
	assert.Equal(t, expectedPath, actualPath)

	// Verify the file exists
	_, err = os.Stat(actualPath)
	require.NoError(t, err)
}

func TestPKIAuthority_PKIDir(t *testing.T) {
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()

	pki := newPKIAuthority(dataDir, pkiDir, logger)
	assert.Equal(t, pkiDir, pki.PKIDir())
}

func TestPKIAuthority_ReuseExisting(t *testing.T) {
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()

	// First initialization
	pki1 := newPKIAuthority(dataDir, pkiDir, logger)
	err := pki1.EnsurePKI(nil)
	require.NoError(t, err)

	// Read root cert fingerprint
	rootCertPEM1, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
	block1, _ := pem.Decode(rootCertPEM1)
	cert1, _ := x509.ParseCertificate(block1.Bytes)
	serial1 := cert1.SerialNumber

	// Second initialization should reuse existing
	pki2 := newPKIAuthority(dataDir, pkiDir, logger)
	err = pki2.EnsurePKI(nil)
	require.NoError(t, err)

	// Verify same cert is used (not regenerated)
	rootCertPEM2, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
	block2, _ := pem.Decode(rootCertPEM2)
	cert2, _ := x509.ParseCertificate(block2.Bytes)
	serial2 := cert2.SerialNumber

	assert.Equal(t, serial1, serial2, "should reuse existing root CA")
}
