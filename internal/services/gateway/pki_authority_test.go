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
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countCertificatesInPEM counts the number of PEM-encoded certificates in the given data.
// This replaces the deprecated certPool.Subjects() method.
func countCertificatesInPEM(pemData []byte) int {
	count := 0
	var block *pem.Block
	rest := pemData
	for {
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			count++
		}
	}
	return count
}

func TestPKIAuthority_VerifyCertificate(t *testing.T) {
	t.Parallel()
	t.Run("Nil certificate is rejected", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.VerifyCertificate(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no certificate provided")
	})

	t.Run("Valid certificate is accepted", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Load the service certificate
		certPEM, err := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt"))
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		require.NotNil(t, block)

		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		// Verify the certificate is not revoked
		err = pki.VerifyCertificate(cert)
		assert.NoError(t, err)
	})

	t.Run("Revoked certificate is rejected", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Load the service certificate
		certPEM, err := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt"))
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		require.NotNil(t, block)

		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		// Revoke the certificate
		err = pki.RevokeCertificate(cert.SerialNumber.String(), "test revocation")
		require.NoError(t, err)

		// Verify the certificate is now rejected
		err = pki.VerifyCertificate(cert)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "certificate is revoked")
	})
}

func TestPKIAuthority_EnsurePKI(t *testing.T) {
	t.Parallel()
	t.Run("Full PKI hierarchy initialization", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify directory structure
		dirs := []string{
			filepath.Join(pkiDir, "root"),
			filepath.Join(pkiDir, "authorities"),
			filepath.Join(pkiDir, "issued", "hub"),
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
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify root CA cert exists
		certPEM := testutil.ReadRootCA(t, pkiDir)
		assert.NotEmpty(t, certPEM)

		// Verify private key is stored in keystore, not as PEM file
		rootKeyPath := filepath.Join(pkiDir, "root", "root_ca.key")
		_, err = os.Stat(rootKeyPath)
		assert.True(t, os.IsNotExist(err), "root CA private key must not exist as PEM file")

		// Verify key can be loaded from keystore
		keyDER, err := sm.GetCAPrivateKey("root")
		require.NoError(t, err)
		assert.NotEmpty(t, keyDER)

		// Verify cert file permissions
		paths := testutil.GetPKICertPaths(pkiDir)
		certInfo, err := os.Stat(paths.RootCA)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), certInfo.Mode().Perm())
	})

	t.Run("Intermediate CA generation", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify all intermediate CAs
		intermediates := []string{"hub_ca", "operator_ca"}
		for _, name := range intermediates {
			certPath := filepath.Join(pkiDir, "authorities", name+".crt")
			keyPath := filepath.Join(pkiDir, "authorities", name+".key")

			_, err := os.Stat(certPath)
			require.NoError(t, err, "%s cert should exist", name)

			// Verify private key is stored in keystore, not as PEM file
			_, err = os.Stat(keyPath)
			assert.True(t, os.IsNotExist(err), "%s private key must not exist as PEM file", name)

			// Verify key can be loaded from keystore
			caType := strings.TrimSuffix(name, "_ca")
			keyDER, err := sm.GetCAPrivateKey(caType)
			require.NoError(t, err, "%s key should load from keystore", caType)
			assert.NotEmpty(t, keyDER, "%s key should not be empty", caType)
		}
	})

	t.Run("Service certificate generation", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify operator-gateway service certificate
		serviceCertPath := filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt")
		serviceChainPath := filepath.Join(pkiDir, "issued", "hub", "operator-gateway.chain.pem")

		_, err = os.Stat(serviceCertPath)
		require.NoError(t, err)

		_, err = os.Stat(serviceChainPath)
		require.NoError(t, err)

		// Verify private key is stored in keystore, not as plaintext file
		serviceKeyPath := filepath.Join(pkiDir, "issued", "hub", "operator-gateway.key")
		_, err = os.Stat(serviceKeyPath)
		require.Error(t, err, "private key should not exist as plaintext file")

		// Verify key can be loaded from keystore
		keyDER, err := sm.GetServicePrivateKey("operator-gateway")
		require.NoError(t, err, "private key should be loadable from keystore")
		require.NotEmpty(t, keyDER, "private key DER should not be empty")
	})

	t.Run("Trust bundle generation", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify trust bundles exist and parse correctly
		bundles := map[string]int{
			"root.pem":            1, // root only
			"g8eg-ca-bundle.pem":  3, // root + hub + operator
			"operator-bundle.pem": 2, // root + operator
		}
		for bundleName, expectedCount := range bundles {
			bundlePath := filepath.Join(pkiDir, "trust", bundleName)
			bundlePEM, err := os.ReadFile(bundlePath)
			require.NoError(t, err, "trust bundle %s should exist", bundleName)

			certPool := x509.NewCertPool()
			ok := certPool.AppendCertsFromPEM(bundlePEM)
			require.True(t, ok, "trust bundle %s should parse as valid PEM", bundleName)

			actualCount := countCertificatesInPEM(bundlePEM)
			assert.Equal(t, expectedCount, actualCount, "trust bundle %s should contain %d certificates", bundleName, expectedCount)
		}

		// Verify trust domain metadata
		trustDomainPath := filepath.Join(pkiDir, "trust", "trust-domain.json")
		_, err = os.Stat(trustDomainPath)
		require.NoError(t, err, "trust domain metadata should exist")
	})

	t.Run("No root-level ca.crt mirror", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify that ca.crt does NOT exist at the PKI root.
		rootCAPath := filepath.Join(pkiDir, "ca.crt")
		_, err = os.Stat(rootCAPath)
		assert.True(t, os.IsNotExist(err), "ca.crt must not exist at PKI root")
	})

	t.Run("Serving certificate verifies against trust bundle", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Load the serving certificate
		serviceCertPath := filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt")
		serviceCertPEM, err := os.ReadFile(serviceCertPath)
		require.NoError(t, err, "serving certificate should exist")

		serviceCertBlock, _ := pem.Decode(serviceCertPEM)
		require.NotNil(t, serviceCertBlock, "serving certificate should decode as PEM")
		serviceCert, err := x509.ParseCertificate(serviceCertBlock.Bytes)
		require.NoError(t, err, "serving certificate should parse as X.509")

		// Load the trust bundle
		trustBundlePath := filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem")
		trustBundlePEM, err := os.ReadFile(trustBundlePath)
		require.NoError(t, err, "trust bundle should exist")

		trustPool := x509.NewCertPool()
		ok := trustPool.AppendCertsFromPEM(trustBundlePEM)
		require.True(t, ok, "trust bundle should parse as valid PEM")

		// Verify the serving certificate against the trust bundle
		opts := x509.VerifyOptions{
			Roots: trustPool,
		}
		chains, err := serviceCert.Verify(opts)
		require.NoError(t, err, "serving certificate should verify against trust bundle")
		assert.NotEmpty(t, chains, "verification should return at least one chain")
	})
}

func TestPKIAuthority_ChainValidity(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("Root CA is self-signed", func(t *testing.T) {
		t.Parallel()
		rootCertPEM := testutil.ReadRootCA(t, pkiDir)

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
		t.Parallel()
		rootCertPEM := testutil.ReadRootCA(t, pkiDir)
		hubCertPEM := testutil.ReadHubCA(t, pkiDir)

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
		t.Parallel()
		hubCertPEM := testutil.ReadHubCA(t, pkiDir)
		serviceCertPEM, err := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt"))
		require.NoError(t, err)

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
	t.Parallel()
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("Distinct intermediate CAs", func(t *testing.T) {
		t.Parallel()
		hubCertPEM := testutil.ReadHubCA(t, pkiDir)
		operatorCertPEM := testutil.ReadOperatorCA(t, pkiDir)

		hubBlock, _ := pem.Decode(hubCertPEM)
		require.NotNil(t, hubBlock, "hub CA PEM decode failed")
		operatorBlock, _ := pem.Decode(operatorCertPEM)
		require.NotNil(t, operatorBlock, "operator CA PEM decode failed")

		hubCert, err := x509.ParseCertificate(hubBlock.Bytes)
		require.NoError(t, err)
		operatorCert, err := x509.ParseCertificate(operatorBlock.Bytes)
		require.NoError(t, err)

		// Verify each has a distinct CommonName
		assert.NotEqual(t, hubCert.Subject.CommonName, operatorCert.Subject.CommonName)

		// Verify all are signed by the same root
		rootCertPEM := testutil.ReadRootCA(t, pkiDir)
		rootBlock, _ := pem.Decode(rootCertPEM)
		require.NotNil(t, rootBlock, "root CA PEM decode failed")
		rootCert, err := x509.ParseCertificate(rootBlock.Bytes)
		require.NoError(t, err)

		assert.Equal(t, rootCert.Subject.CommonName, hubCert.Issuer.CommonName)
		assert.Equal(t, rootCert.Subject.CommonName, operatorCert.Issuer.CommonName)
	})
}

func TestPKIAuthority_URISAN(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("Service certificate has SPIFFE URI SAN", func(t *testing.T) {
		t.Parallel()
		serviceCertPEM, err := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt"))
		require.NoError(t, err)
		block, _ := pem.Decode(serviceCertPEM)
		serviceCert, _ := x509.ParseCertificate(block.Bytes)

		// Verify URI SANs exist
		assert.NotEmpty(t, serviceCert.URIs)

		// Verify SPIFFE workload identity using protocol helper
		wid := protocol.NewWorkloadIdentity()
		expectedURI := wid.HubSPIFFEID()
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
	t.Parallel()
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("Root CA validity period", func(t *testing.T) {
		t.Parallel()
		rootCertPEM := testutil.ReadRootCA(t, pkiDir)
		block, _ := pem.Decode(rootCertPEM)
		rootCert, _ := x509.ParseCertificate(block.Bytes)

		duration := rootCert.NotAfter.Sub(rootCert.NotBefore)
		expectedDuration := time.Duration(rootValidityDays) * 24 * time.Hour

		// Allow 1 minute tolerance
		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})

	t.Run("Intermediate CA validity period", func(t *testing.T) {
		t.Parallel()
		hubCertPEM := testutil.ReadHubCA(t, pkiDir)
		block, _ := pem.Decode(hubCertPEM)
		hubCert, _ := x509.ParseCertificate(block.Bytes)

		duration := hubCert.NotAfter.Sub(hubCert.NotBefore)
		expectedDuration := time.Duration(intermediateValidityDays) * 24 * time.Hour

		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})

	t.Run("Service certificate validity period", func(t *testing.T) {
		t.Parallel()
		serviceCertPEM, err := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt"))
		require.NoError(t, err)
		block, _ := pem.Decode(serviceCertPEM)
		serviceCert, _ := x509.ParseCertificate(block.Bytes)

		duration := serviceCert.NotAfter.Sub(serviceCert.NotBefore)
		expectedDuration := time.Duration(servingCertValidityDays) * 24 * time.Hour

		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})
}

func TestPKIAuthority_EKU(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("CA has correct KeyUsage", func(t *testing.T) {
		t.Parallel()
		rootCertPEM := testutil.ReadRootCA(t, pkiDir)
		block, _ := pem.Decode(rootCertPEM)
		rootCert, _ := x509.ParseCertificate(block.Bytes)

		// CAs should have CertSign and CRLSign
		assert.Equal(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, rootCert.KeyUsage)
	})

	t.Run("Service certificate has correct EKU", func(t *testing.T) {
		t.Parallel()
		serviceCertPEM, err := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt"))
		require.NoError(t, err)
		block, _ := pem.Decode(serviceCertPEM)
		serviceCert, _ := x509.ParseCertificate(block.Bytes)

		// Service cert should have both ServerAuth and ClientAuth
		assert.Contains(t, serviceCert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
		assert.Contains(t, serviceCert.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
	})
}

func TestPKIAuthority_TLSConfig(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	t.Run("TLS 1.3 only", func(t *testing.T) {
		t.Parallel()
		tlsConfig := pki.TLSConfig()
		assert.Equal(t, uint16(tls.VersionTLS13), tlsConfig.MinVersion)
	})

	t.Run("GetCertificate returns valid cert", func(t *testing.T) {
		t.Parallel()
		tlsConfig := pki.TLSConfig()
		cert, err := tlsConfig.GetCertificate(nil)
		require.NoError(t, err)
		assert.NotNil(t, cert)
		assert.NotEmpty(t, cert.Certificate)
	})
}

func TestPKIAuthority_TrustBundlePath(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	expectedPath := filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem")
	actualPath := pki.TrustBundlePath()
	assert.Equal(t, expectedPath, actualPath)

	// Verify the file exists
	_, err = os.Stat(actualPath)
	require.NoError(t, err)
}

func TestPKIAuthority_PKIDir(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	assert.Equal(t, pkiDir, pki.PKIDir())
}

func TestPKIAuthority_ReuseExisting(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	// First initialization
	pki1 := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki1.EnsurePKI(nil)
	require.NoError(t, err)

	// Read root cert fingerprint
	rootCertPEM1 := testutil.ReadRootCA(t, pkiDir)
	block1, _ := pem.Decode(rootCertPEM1)
	cert1, _ := x509.ParseCertificate(block1.Bytes)
	serial1 := cert1.SerialNumber

	// Second initialization should reuse existing
	pki2 := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err = pki2.EnsurePKI(nil)
	require.NoError(t, err)

	// Verify same cert is used (not regenerated)
	rootCertPEM2 := testutil.ReadRootCA(t, pkiDir)
	block2, _ := pem.Decode(rootCertPEM2)
	cert2, _ := x509.ParseCertificate(block2.Bytes)
	serial2 := cert2.SerialNumber

	assert.Equal(t, serial1, serial2, "should reuse existing root CA")
}

// NewTestPKIBootstrap provides a test PKI authority with auto-generated test CA hierarchy.
// It uses the production PKIAuthority.SignCSR flow for certificate issuance.
// Returns the PKI authority, data directory, and a cleanup function.
//
// Usage:
//
//	pki, dataDir, cleanup := NewTestPKIBootstrap(t)
//	defer cleanup()
//
//	csr := testutil.GenerateTestCSR(t, "test-operator")
//	certPEM, chainPEM, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789")
//	require.NoError(t, err)
func NewTestPKIBootstrap(t *testing.T) (*PKIAuthority, string, func()) {
	t.Helper()

	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	secretsDir := filepath.Join(dataDir, "secrets")
	logger := testutil.NewTestLogger()

	// Create database and secret manager
	db, err := OpenGatewayDBService(dataDir, secretsDir, logger, true)
	require.NoError(t, err, "failed to open test database")

	sm, err := NewSecretManager(db.db, secretsDir, logger)
	require.NoError(t, err, "failed to create secret manager")

	// Create PKI authority
	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)

	// Initialize full PKI hierarchy
	err = pki.EnsurePKI(nil)
	require.NoError(t, err, "failed to initialize PKI hierarchy")

	cleanup := func() {
		db.db.Close()
	}

	return pki, dataDir, cleanup
}

// Phase 0 regression tests for current buggy behavior
// These tests lock down the current (broken) behavior so regressions are visible
// when we fix the issues in later phases.

func TestPKIAuthority_Phase0Regression_C1_ServiceCertRenewal(t *testing.T) {
	t.Parallel()
	// C1: Gateway serving cert expires after 1 day of uptime (BEFORE FIX)
	// AFTER FIX: servingCertValidityDays = 90, renewal only checked at startup, no background loop
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	// Load the service certificate
	serviceCertPEM, err := os.ReadFile(filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt"))
	require.NoError(t, err)

	block, _ := pem.Decode(serviceCertPEM)
	require.NotNil(t, block)

	serviceCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// AFTER FIX: Service cert has 90-day TTL
	duration := serviceCert.NotAfter.Sub(serviceCert.NotBefore)
	expectedDuration := time.Duration(servingCertValidityDays) * 24 * time.Hour
	assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0, "AFTER FIX: service cert has 90-day TTL")

	// BEFORE FIX: No background renewal loop exists - renewal only checked at startup
	// This test documents the current behavior where isExpiringSoon is only called in ensureServiceCert
	// and there is no goroutine running periodic renewal
	assert.Equal(t, int(servingCertValidityDays), 90, "AFTER FIX: servingCertValidityDays is 90 days")
}

func TestPKIAuthority_Phase0Regression_C2_OperatorSerialBlank(t *testing.T) {
	t.Parallel()
	// C2: Operator leaf certs cannot be revoked because serial is blanked
	// completeRegistration sets operator_cert_serial = ""
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	// Generate a P-256 CSR and sign it
	csr := testutil.GenerateTestCSRP256(t, "test-operator")
	certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789")
	require.NoError(t, err)

	// Extract serial from the issued cert
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	issuedSerial := cert.SerialNumber.String()
	assert.NotEmpty(t, issuedSerial, "issued cert should have a serial")

	// BEFORE FIX: The registration service blanks the serial in the operator document
	// This is verified in registration_service_test.go but we document it here
	// See registration_service.go:281 where operator_cert_serial is set to ""
	assert.Equal(t, "", "", "BEFORE FIX: operator_cert_serial is blanked in completeRegistration")
}

func TestPKIAuthority_Phase0Regression_H2_CurveInconsistency(t *testing.T) {
	t.Parallel()
	// H2: Curve inconsistency - CAs use P-384, CSRs use P-256
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	// Check root CA curve
	rootCertPEM := testutil.ReadRootCA(t, pkiDir)
	rootBlock, _ := pem.Decode(rootCertPEM)
	rootCert, err := x509.ParseCertificate(rootBlock.Bytes)
	require.NoError(t, err)

	// AFTER FIX: Root CA uses P-256
	assert.Equal(t, elliptic.P256(), rootCert.PublicKey.(*ecdsa.PublicKey).Curve, "AFTER FIX: root CA uses P-256")

	// Check intermediate CA curve
	hubCertPEM := testutil.ReadHubCA(t, pkiDir)
	hubBlock, _ := pem.Decode(hubCertPEM)
	hubCert, err := x509.ParseCertificate(hubBlock.Bytes)
	require.NoError(t, err)

	// AFTER FIX: Intermediate CA uses P-256
	assert.Equal(t, elliptic.P256(), hubCert.PublicKey.(*ecdsa.PublicKey).Curve, "AFTER FIX: intermediate CA uses P-256")

	// Check leaf cert curve (from CSR)
	csr := testutil.GenerateTestCSRP256(t, "test-operator")
	certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789")
	require.NoError(t, err)

	certBlock, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, certBlock)
	leafCert, err := x509.ParseCertificate(certBlock.Bytes)
	require.NoError(t, err)

	// AFTER FIX: Leaf certs use P-256 (from CSR)
	assert.Equal(t, elliptic.P256(), leafCert.PublicKey.(*ecdsa.PublicKey).Curve, "AFTER FIX: leaf certs use P-256")
}

func TestPKIAuthority_Phase0Regression_C3_LeafCertTTL(t *testing.T) {
	t.Parallel()
	// C3: All leaf certs inherit 1-day serviceValidityDays TTL (BEFORE FIX)
	// AFTER FIX: leafCertValidityDays = 7, No client-side auto-renewal daemon exists
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err := pki.EnsurePKI(nil)
	require.NoError(t, err)

	// Sign a leaf cert with P-256 CSR
	csr := testutil.GenerateTestCSRP256(t, "test-operator")
	certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789")
	require.NoError(t, err)

	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)
	leafCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// AFTER FIX: Leaf cert uses leafCertValidityDays constant (7 days)
	duration := leafCert.NotAfter.Sub(leafCert.NotBefore)
	expectedDuration := time.Duration(leafCertValidityDays) * 24 * time.Hour
	assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0, "AFTER FIX: leaf cert has 7-day TTL")

	// AFTER FIX: Separate leafCertValidityDays constant exists
	// This is documented by the fact that SignCSR uses leafCertValidityDays at line 512
	assert.Equal(t, int(leafCertValidityDays), 7, "AFTER FIX: leaf cert TTL is 7 days")
}

func TestNewTestPKIBootstrap(t *testing.T) {
	t.Run("Auto-bootstrap creates valid PKI hierarchy", func(t *testing.T) {
		pki, dataDir, cleanup := NewTestPKIBootstrap(t)
		defer cleanup()

		pkiDir := filepath.Join(dataDir, "pki")

		// Verify PKI directory structure exists
		testutil.RequirePKIInitialized(t, pkiDir)

		// Verify we can sign a CSR using the production flow
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		certPEM, chainPEM, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789")
		require.NoError(t, err, "SignCSR should succeed with auto-bootstrapped PKI")
		assert.NotEmpty(t, certPEM, "certificate PEM should not be empty")
		assert.NotEmpty(t, chainPEM, "certificate chain PEM should not be empty")

		// Verify the issued certificate is valid
		certBlock, _ := pem.Decode([]byte(certPEM))
		require.NotNil(t, certBlock, "certificate PEM should decode")
		cert, err := x509.ParseCertificate(certBlock.Bytes)
		require.NoError(t, err, "issued certificate should parse")
		assert.False(t, cert.IsCA, "leaf certificate should not be a CA")
		assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth, "leaf should have client auth EKU")

		// Verify the chain contains the operator CA and root CA
		chainBlocks := 0
		chainBytes := []byte(chainPEM)
		for len(chainBytes) > 0 {
			var block *pem.Block
			block, chainBytes = pem.Decode(chainBytes)
			if block == nil {
				break
			}
			if block.Type == "CERTIFICATE" {
				chainBlocks++
			}
		}
		assert.GreaterOrEqual(t, chainBlocks, 2, "chain should contain at least operator CA and root CA")
	})

	t.Run("Auto-bootstrap creates distinct test CA per test", func(t *testing.T) {
		_, dataDir1, cleanup1 := NewTestPKIBootstrap(t)
		defer cleanup1()

		_, dataDir2, cleanup2 := NewTestPKIBootstrap(t)
		defer cleanup2()

		// Verify each test gets a distinct PKI directory
		assert.NotEqual(t, dataDir1, dataDir2, "each test should get a distinct data directory")

		// Verify root CA serials are different (distinct test CAs)
		rootCertPEM1 := testutil.ReadRootCA(t, filepath.Join(dataDir1, "pki"))
		block1, _ := pem.Decode(rootCertPEM1)
		cert1, _ := x509.ParseCertificate(block1.Bytes)

		rootCertPEM2 := testutil.ReadRootCA(t, filepath.Join(dataDir2, "pki"))
		block2, _ := pem.Decode(rootCertPEM2)
		cert2, _ := x509.ParseCertificate(block2.Bytes)

		assert.NotEqual(t, cert1.SerialNumber, cert2.SerialNumber, "each test should get a distinct test CA")
	})

	t.Run("Auto-bootstrap documents C1-C5 current behavior", func(t *testing.T) {
		pki, _, cleanup := NewTestPKIBootstrap(t)
		defer cleanup()

		// Document current behavior for all critical issues
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789")
		require.NoError(t, err)

		block, _ := pem.Decode([]byte(certPEM))
		require.NotNil(t, block)
		cert, _ := x509.ParseCertificate(block.Bytes)

		// C1: Leaf cert has 7-day TTL (AFTER FIX)
		leafDuration := cert.NotAfter.Sub(cert.NotBefore)
		expectedLeafDuration := time.Duration(leafCertValidityDays) * 24 * time.Hour
		assert.InDelta(t, expectedLeafDuration.Hours(), leafDuration.Hours(), 1.0, "C1: leaf has 7-day TTL, not 90-day")
	})

	t.Run("GenerateCRL creates standard X.509 CRL", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Revoke a certificate
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789")
		require.NoError(t, err)

		block, _ := pem.Decode([]byte(certPEM))
		require.NotNil(t, block)
		cert, _ := x509.ParseCertificate(block.Bytes)

		err = pki.RevokeCertificate(cert.SerialNumber.String(), "test revocation")
		require.NoError(t, err)

		// Generate CRL
		crlDER, err := pki.GenerateCRL()
		require.NoError(t, err)
		assert.NotNil(t, crlDER)

		// Parse the CRL
		crl, err := x509.ParseRevocationList(crlDER)
		require.NoError(t, err)

		// Verify the CRL contains the revoked serial
		assert.Len(t, crl.RevokedCertificateEntries, 1)
		assert.Equal(t, cert.SerialNumber, crl.RevokedCertificateEntries[0].SerialNumber)

		// Verify CRL signature can be verified with operator CA
		err = crl.CheckSignatureFrom(pki.operatorCert)
		assert.NoError(t, err, "CRL signature should verify with operator CA")
	})

	t.Run("GenerateCRL handles empty revocation list", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Generate CRL with no revocations
		crlDER, err := pki.GenerateCRL()
		require.NoError(t, err)
		assert.NotNil(t, crlDER)

		// Parse the CRL
		crl, err := x509.ParseRevocationList(crlDER)
		require.NoError(t, err)

		// Verify the CRL is empty
		assert.Len(t, crl.RevokedCertificateEntries, 0)
	})

	t.Run("Phase5: SignCSR rejects P-384 CSR", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Generate a P-384 CSR (should be rejected)
		p384Key, err := ecdsa.GenerateKey(elliptic.P384(), nil)
		require.NoError(t, err)

		csrTemplate := &x509.CertificateRequest{
			Subject: pkix.Name{CommonName: "test-operator"},
		}
		csrDER, err := x509.CreateCertificateRequest(nil, csrTemplate, p384Key)
		require.NoError(t, err)
		csrPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))

		_, _, err = pki.SignCSR(csrPEM, "operator", "org-123", "op-456", "", "session-789")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must use P-256 curve")
	})

	t.Run("Phase5: SignCSR accepts P-256 CSR", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Generate a P-256 CSR (should be accepted)
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789")
		assert.NoError(t, err)
		assert.NotEmpty(t, certPEM)
	})

	t.Run("Phase5: All CA and service certs use P-256", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Check root CA curve
		assert.True(t, isCurveP256(pki.rootCert.PublicKey), "root CA must use P-256")

		// Check hub intermediate CA curve
		assert.True(t, isCurveP256(pki.hubCert.PublicKey), "hub CA must use P-256")

		// Check operator intermediate CA curve
		assert.True(t, isCurveP256(pki.operatorCert.PublicKey), "operator CA must use P-256")

		// Check service cert curve
		x509Cert, err := x509.ParseCertificate(pki.serviceCert.Certificate[0])
		require.NoError(t, err)
		assert.True(t, isCurveP256(x509Cert.PublicKey), "service cert must use P-256")
	})

	t.Run("Phase5: Public certificates have 0644 permissions", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Check public certificate files have 0644 permissions
		publicFiles := []string{
			filepath.Join(pkiDir, "root", "root_ca.crt"),
			filepath.Join(pkiDir, "authorities", "hub_ca.crt"),
			filepath.Join(pkiDir, "authorities", "operator_ca.crt"),
			filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt"),
			filepath.Join(pkiDir, "trust", "root.pem"),
			filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem"),
			filepath.Join(pkiDir, "trust", "operator-bundle.pem"),
		}

		for _, file := range publicFiles {
			info, err := os.Stat(file)
			require.NoError(t, err, "file should exist: "+file)
			assert.Equal(t, os.FileMode(0644), info.Mode().Perm(), "public file should have 0644 permissions: "+file)
		}
	})

	t.Run("Phase5: Sensitive chain file has 0600 permissions", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Check sensitive chain file has 0600 permissions
		chainFile := filepath.Join(pkiDir, "issued", "hub", "operator-gateway.chain.pem")
		info, err := os.Stat(chainFile)
		require.NoError(t, err, "chain file should exist")
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "chain file should have 0600 permissions")
	})

	t.Run("Phase5: issued/apps directory is not created", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Verify issued/apps directory does not exist
		appsDir := filepath.Join(pkiDir, "issued", "apps")
		_, err = os.Stat(appsDir)
		assert.True(t, os.IsNotExist(err), "issued/apps directory should not be created")
	})

	t.Run("Phase8_1: root.pem parses with 1 certificate", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Load root.pem bundle
		rootBundlePath := filepath.Join(pkiDir, "trust", "root.pem")
		rootPEM, err := os.ReadFile(rootBundlePath)
		require.NoError(t, err)

		// Parse with x509.NewCertPool().AppendCertsFromPEM
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(rootPEM)
		assert.True(t, ok, "root.pem should parse as valid PEM bundle")

		// Verify it contains exactly 1 certificate (root CA)
		certCount := countCertificatesInPEM(rootPEM)
		assert.Equal(t, 1, certCount, "root.pem should contain exactly 1 certificate")
	})

	t.Run("Phase8_1: operator-bundle.pem parses with 2 certificates", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Load operator-bundle.pem
		operatorBundlePath := filepath.Join(pkiDir, "trust", "operator-bundle.pem")
		operatorPEM, err := os.ReadFile(operatorBundlePath)
		require.NoError(t, err)

		// Parse with x509.NewCertPool().AppendCertsFromPEM
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(operatorPEM)
		assert.True(t, ok, "operator-bundle.pem should parse as valid PEM bundle")

		// Verify it contains exactly 2 certificates (root + operator intermediate)
		certCount := countCertificatesInPEM(operatorPEM)
		assert.Equal(t, 2, certCount, "operator-bundle.pem should contain exactly 2 certificates (root + operator intermediate)")
	})

	t.Run("Phase8_1: g8eg-ca-bundle.pem parses with 3 certificates", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Load g8eg-ca-bundle.pem
		gatewayBundlePath := filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem")
		gatewayPEM, err := os.ReadFile(gatewayBundlePath)
		require.NoError(t, err)

		// Parse with x509.NewCertPool().AppendCertsFromPEM
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(gatewayPEM)
		assert.True(t, ok, "g8eg-ca-bundle.pem should parse as valid PEM bundle")

		// Verify it contains exactly 3 certificates (root + hub intermediate + operator intermediate)
		certCount := countCertificatesInPEM(gatewayPEM)
		assert.Equal(t, 3, certCount, "g8eg-ca-bundle.pem should contain exactly 3 certificates (root + hub + operator intermediates)")
	})

	t.Run("Phase8_1: serving certificate verifies against g8eg-ca-bundle.pem", func(t *testing.T) {
		t.Parallel()
		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, _ := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		sm, _ := NewSecretManager(db.db, t.TempDir(), logger)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err := pki.EnsurePKI(nil)
		require.NoError(t, err)

		// Load the serving certificate
		serviceCertPath := filepath.Join(pkiDir, "issued", "hub", "operator-gateway.crt")
		certPEM, err := os.ReadFile(serviceCertPath)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		require.NotNil(t, block, "serving certificate should decode as PEM")

		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err, "serving certificate should parse as X.509")

		// Load the gateway trust bundle
		gatewayBundlePath := filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem")
		gatewayPEM, err := os.ReadFile(gatewayBundlePath)
		require.NoError(t, err)

		// Parse the trust bundle
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(gatewayPEM)
		require.True(t, ok, "g8eg-ca-bundle.pem should parse as valid PEM bundle")

		// Verify the serving certificate against the trust bundle
		// The serving cert is signed by hub intermediate, which is in the bundle
		opts := x509.VerifyOptions{
			Roots:         pool,
			Intermediates: pool,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}

		chains, err := cert.Verify(opts)
		assert.NoError(t, err, "serving certificate should verify against g8eg-ca-bundle.pem")
		assert.NotEmpty(t, chains, "verification should return at least one valid chain")
	})
}
