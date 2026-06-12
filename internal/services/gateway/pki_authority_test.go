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
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression test markers - standardized constants for documenting known issues
// These markers identify tests that lock down current (broken) behavior to prevent regressions
// when fixes are implemented in later phases.
const (
	// RegressionMarkerAfterFix indicates the expected behavior after a fix is implemented
	RegressionMarkerAfterFix = "REGRESSION: AFTER FIX"
	// RegressionMarkerBeforeFix indicates the current (broken) behavior before a fix
	RegressionMarkerBeforeFix = "REGRESSION: BEFORE FIX"
	// RegressionMarkerIssue identifies a specific issue being tracked (e.g., C1, C2, H2)
	RegressionMarkerIssue = "REGRESSION: ISSUE"
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

// testPKIContext holds the common test infrastructure for PKI tests.
type testPKIContext struct {
	pki     *PKIAuthority
	pkiDir  string
	dataDir string
	db      *CanonicalDBService
	sm      *SecretManager
	logger  *slog.Logger
}

// setupTestPKI creates a complete test PKI infrastructure with initialized hierarchy.
// Returns a context struct with all components and a cleanup function.
// This helper eliminates the repeated setup code across all tests.
func setupTestPKI(t *testing.T) *testPKIContext {
	t.Helper()

	dataDir := tempDir(t)
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	secretsDir := tempDir(t)

	// Clean pkiDir to avoid stale certificates from previous test runs
	os.RemoveAll(pkiDir)
	require.NoError(t, os.MkdirAll(pkiDir, 0755))

	db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
	require.NoError(t, err, "failed to open test database")
	t.Cleanup(func() { db.Close() })

	sm, err := NewSecretManager(db.db, secretsDir, logger)
	require.NoError(t, err, "failed to create secret manager")

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err, "failed to initialize PKI hierarchy")

	return &testPKIContext{
		pki:     pki,
		pkiDir:  pkiDir,
		dataDir: dataDir,
		db:      db,
		sm:      sm,
		logger:  logger,
	}
}

// loadCertificate reads and parses a PEM-encoded certificate from the given path.
func loadCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()

	certPEM, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read certificate from %s", path)

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block, "failed to decode PEM from %s", path)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "failed to parse certificate from %s", path)

	return cert
}

// loadTrustBundle reads a PEM-encoded trust bundle and returns a cert pool.
func loadTrustBundle(t *testing.T, path string) *x509.CertPool {
	t.Helper()

	bundlePEM, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read trust bundle from %s", path)

	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM(bundlePEM)
	require.True(t, ok, "failed to parse trust bundle from %s", path)

	return pool
}

// parsePEMCertificate parses a PEM-encoded certificate from bytes.
func parsePEMCertificate(t *testing.T, pemData []byte) *x509.Certificate {
	t.Helper()

	block, _ := pem.Decode(pemData)
	require.NotNil(t, block, "failed to decode PEM certificate")

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "failed to parse certificate")

	return cert
}

func TestPKIAuthority_VerifyCertificate(t *testing.T) {
	t.Parallel()
	t.Run("Nil certificate is rejected", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)
		err := ctx.pki.VerifyCertificate(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no certificate provided")
	})

	t.Run("Valid certificate is accepted", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		cert := loadCertificate(t, filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.crt"))
		err := ctx.pki.VerifyCertificate(cert)
		require.NoError(t, err)
	})

	t.Run("Revoked certificate is rejected", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		cert := loadCertificate(t, filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.crt"))
		err := ctx.pki.RevokeCertificate(cert.SerialNumber.String(), "test revocation")
		require.NoError(t, err)

		err = ctx.pki.VerifyCertificate(cert)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "certificate is revoked")
	})
}

func TestPKIAuthority_InitializePKI(t *testing.T) {
	t.Parallel()
	t.Run("Full PKI hierarchy initialization", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		dirs := []string{
			filepath.Join(ctx.pkiDir, "root"),
			filepath.Join(ctx.pkiDir, "authorities"),
			filepath.Join(ctx.pkiDir, "issued", "hub"),
			filepath.Join(ctx.pkiDir, "trust"),
			filepath.Join(ctx.pkiDir, "revocation"),
		}
		for _, dir := range dirs {
			info, err := os.Stat(dir)
			require.NoError(t, err, "directory %s should exist", dir)
			assert.True(t, info.IsDir(), "%s should be a directory", dir)
		}
	})

	t.Run("Root CA generation", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		certPEM := testutil.ReadRootCA(t, ctx.pkiDir)
		assert.NotEmpty(t, certPEM)

		rootKeyPath := filepath.Join(ctx.pkiDir, "root", "root_ca.key")
		_, err := os.Stat(rootKeyPath)
		assert.True(t, os.IsNotExist(err), "root CA private key must not exist as PEM file")

		keyDER, err := ctx.sm.GetCAPrivateKey("root")
		require.NoError(t, err)
		assert.NotEmpty(t, keyDER)

		paths := testutil.GetPKICertPaths(ctx.pkiDir)
		certInfo, err := os.Stat(paths.RootCA)
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0644), certInfo.Mode().Perm())
		}
	})

	t.Run("Intermediate CA generation", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		intermediates := []string{"hub_ca", "operator_ca"}
		for _, name := range intermediates {
			certPath := filepath.Join(ctx.pkiDir, "authorities", name+".crt")
			keyPath := filepath.Join(ctx.pkiDir, "authorities", name+".key")

			_, err := os.Stat(certPath)
			require.NoError(t, err, "%s cert should exist", name)

			_, err = os.Stat(keyPath)
			assert.True(t, os.IsNotExist(err), "%s private key must not exist as PEM file", name)

			caType := strings.TrimSuffix(name, "_ca")
			keyDER, err := ctx.sm.GetCAPrivateKey(caType)
			require.NoError(t, err, "%s key should load from keystore", caType)
			assert.NotEmpty(t, keyDER, "%s key should not be empty", caType)
		}
	})

	t.Run("Service certificate generation", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		serviceCertPath := filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.crt")
		serviceChainPath := filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.chain.pem")

		_, err := os.Stat(serviceCertPath)
		require.NoError(t, err)

		_, err = os.Stat(serviceChainPath)
		require.NoError(t, err)

		serviceKeyPath := filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.key")
		_, err = os.Stat(serviceKeyPath)
		require.Error(t, err, "private key should not exist as plaintext file")

		keyDER, err := ctx.sm.GetServicePrivateKey("operator-gateway")
		require.NoError(t, err, "private key should be loadable from keystore")
		require.NotEmpty(t, keyDER, "private key DER should not be empty")
	})

	t.Run("Trust bundle generation", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		bundles := map[string]int{
			"root.pem":            1,
			"g8eg-ca-bundle.pem":  4,
			"operator-bundle.pem": 2,
		}
		for bundleName, expectedCount := range bundles {
			bundlePath := filepath.Join(ctx.pkiDir, "trust", bundleName)
			bundlePEM, err := os.ReadFile(bundlePath)
			require.NoError(t, err, "trust bundle %s should exist", bundleName)

			certPool := x509.NewCertPool()
			ok := certPool.AppendCertsFromPEM(bundlePEM)
			require.True(t, ok, "trust bundle %s should parse as valid PEM", bundleName)

			actualCount := countCertificatesInPEM(bundlePEM)
			assert.Equal(t, expectedCount, actualCount, "trust bundle %s should contain %d certificates", bundleName, expectedCount)
		}

		trustDomainPath := filepath.Join(ctx.pkiDir, "trust", "trust-domain.json")
		_, err := os.Stat(trustDomainPath)
		require.NoError(t, err, "trust domain metadata should exist")
	})

	t.Run("No root-level ca.crt mirror", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		rootCAPath := filepath.Join(ctx.pkiDir, "ca.crt")
		_, err := os.Stat(rootCAPath)
		assert.True(t, os.IsNotExist(err), "ca.crt must not exist at PKI root")
	})

	t.Run("Serving certificate verifies against trust bundle", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		serviceCert := loadCertificate(t, filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.crt"))
		trustPool := loadTrustBundle(t, filepath.Join(ctx.pkiDir, "trust", "g8eg-ca-bundle.pem"))

		opts := x509.VerifyOptions{Roots: trustPool}
		chains, err := serviceCert.Verify(opts)
		require.NoError(t, err, "serving certificate should verify against trust bundle")
		assert.NotEmpty(t, chains, "verification should return at least one chain")
	})
}

func TestPKIAuthority_ChainValidity(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	t.Run("Root CA is self-signed", func(t *testing.T) {
		t.Parallel()
		rootCertPEM := testutil.ReadRootCA(t, ctx.pkiDir)
		rootCert := parsePEMCertificate(t, rootCertPEM)

		assert.Equal(t, rootCert.Issuer.CommonName, rootCert.Subject.CommonName)
		assert.True(t, rootCert.IsCA)
		assert.Equal(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, rootCert.KeyUsage)
	})

	t.Run("Intermediate CA chain validity", func(t *testing.T) {
		t.Parallel()
		rootCertPEM := testutil.ReadRootCA(t, ctx.pkiDir)
		hubCertPEM := testutil.ReadHubCA(t, ctx.pkiDir)

		rootCert := parsePEMCertificate(t, rootCertPEM)
		hubCert := parsePEMCertificate(t, hubCertPEM)

		assert.Equal(t, rootCert.Subject.CommonName, hubCert.Issuer.CommonName)
		assert.True(t, hubCert.IsCA)
		assert.Equal(t, 1, hubCert.MaxPathLen)
	})

	t.Run("Service certificate chain validity", func(t *testing.T) {
		t.Parallel()
		hubCertPEM := testutil.ReadHubCA(t, ctx.pkiDir)
		serviceCert := loadCertificate(t, filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.crt"))

		hubCert := parsePEMCertificate(t, hubCertPEM)

		assert.Equal(t, hubCert.Subject.CommonName, serviceCert.Issuer.CommonName)
		assert.False(t, serviceCert.IsCA)
		assert.Equal(t, x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment, serviceCert.KeyUsage)
	})
}

func TestPKIAuthority_IssuerSeparation(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	t.Run("Distinct intermediate CAs", func(t *testing.T) {
		t.Parallel()
		hubCertPEM := testutil.ReadHubCA(t, ctx.pkiDir)
		operatorCertPEM := testutil.ReadOperatorCA(t, ctx.pkiDir)

		hubCert := parsePEMCertificate(t, hubCertPEM)
		operatorCert := parsePEMCertificate(t, operatorCertPEM)

		assert.NotEqual(t, hubCert.Subject.CommonName, operatorCert.Subject.CommonName)

		rootCertPEM := testutil.ReadRootCA(t, ctx.pkiDir)
		rootCert := parsePEMCertificate(t, rootCertPEM)

		assert.Equal(t, rootCert.Subject.CommonName, hubCert.Issuer.CommonName)
		assert.Equal(t, rootCert.Subject.CommonName, operatorCert.Issuer.CommonName)
	})
}

func TestPKIAuthority_URISAN(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	t.Run("Service certificate has SPIFFE URI SAN", func(t *testing.T) {
		t.Parallel()
		serviceCert := loadCertificate(t, filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.crt"))

		assert.NotEmpty(t, serviceCert.URIs)

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
	ctx := setupTestPKI(t)

	t.Run("Root CA validity period", func(t *testing.T) {
		t.Parallel()
		rootCertPEM := testutil.ReadRootCA(t, ctx.pkiDir)
		rootCert := parsePEMCertificate(t, rootCertPEM)

		duration := rootCert.NotAfter.Sub(rootCert.NotBefore)
		expectedDuration := time.Duration(rootValidityDays) * 24 * time.Hour

		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})

	t.Run("Intermediate CA validity period", func(t *testing.T) {
		t.Parallel()
		hubCertPEM := testutil.ReadHubCA(t, ctx.pkiDir)
		hubCert := parsePEMCertificate(t, hubCertPEM)

		duration := hubCert.NotAfter.Sub(hubCert.NotBefore)
		expectedDuration := time.Duration(intermediateValidityDays) * 24 * time.Hour

		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})

	t.Run("Service certificate validity period", func(t *testing.T) {
		t.Parallel()
		serviceCert := loadCertificate(t, filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.crt"))

		duration := serviceCert.NotAfter.Sub(serviceCert.NotBefore)
		expectedDuration := time.Duration(servingCertValidityDays) * 24 * time.Hour

		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})
}

func TestPKIAuthority_EKU(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	t.Run("CA has correct KeyUsage", func(t *testing.T) {
		t.Parallel()
		rootCertPEM := testutil.ReadRootCA(t, ctx.pkiDir)
		rootCert := parsePEMCertificate(t, rootCertPEM)

		assert.Equal(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, rootCert.KeyUsage)
	})

	t.Run("Service certificate has correct EKU", func(t *testing.T) {
		t.Parallel()
		serviceCert := loadCertificate(t, filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.crt"))

		assert.Contains(t, serviceCert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
		assert.Contains(t, serviceCert.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
	})
}

func TestPKIAuthority_TLSConfig(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	t.Run("TLS 1.3 only", func(t *testing.T) {
		t.Parallel()
		tlsConfig := ctx.pki.TLSConfig()
		assert.Equal(t, uint16(tls.VersionTLS13), tlsConfig.MinVersion)
	})

	t.Run("GetCertificate returns valid cert", func(t *testing.T) {
		t.Parallel()
		tlsConfig := ctx.pki.TLSConfig()
		cert, err := tlsConfig.GetCertificate(nil)
		require.NoError(t, err)
		assert.NotNil(t, cert)
		assert.NotEmpty(t, cert.Certificate)
	})
}

func TestPKIAuthority_TrustBundlePath(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	expectedPath := filepath.Join(ctx.pkiDir, "trust", "g8eg-ca-bundle.pem")
	actualPath := ctx.pki.TrustBundlePath()
	assert.Equal(t, expectedPath, actualPath)

	_, err := os.Stat(actualPath)
	require.NoError(t, err)
}

func TestPKIAuthority_PKIDir(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)
	assert.Equal(t, ctx.pkiDir, ctx.pki.PKIDir())
}

func TestPKIAuthority_ReuseExisting(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	rootCertPEM1 := testutil.ReadRootCA(t, ctx.pkiDir)
	cert1 := parsePEMCertificate(t, rootCertPEM1)
	serial1 := cert1.SerialNumber

	pki2 := newPKIAuthority(ctx.dataDir, ctx.pkiDir, ctx.db, ctx.sm, ctx.logger)
	err := pki2.InitializePKI(nil)
	require.NoError(t, err)

	rootCertPEM2 := testutil.ReadRootCA(t, ctx.pkiDir)
	cert2 := parsePEMCertificate(t, rootCertPEM2)
	serial2 := cert2.SerialNumber

	assert.Equal(t, serial1, serial2, "should reuse existing root CA")
}

// Phase 0 regression tests for current buggy behavior
// These tests lock down the current (broken) behavior so regressions are visible
// when we fix the issues in later phases.

func TestPKIAuthority_Phase0Regression_C1_ServiceCertRenewal(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	serviceCert := loadCertificate(t, filepath.Join(ctx.pkiDir, "issued", "hub", "operator-gateway.crt"))

	duration := serviceCert.NotAfter.Sub(serviceCert.NotBefore)
	expectedDuration := time.Duration(servingCertValidityDays) * 24 * time.Hour
	assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0, RegressionMarkerAfterFix+": service cert has 90-day TTL")

	assert.Equal(t, 90, int(servingCertValidityDays), RegressionMarkerAfterFix+": servingCertValidityDays is 90 days")
}

func TestPKIAuthority_Phase0Regression_C2_OperatorSerialBlank(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	csr := testutil.GenerateTestCSRP256(t, "test-operator")
	certPEM, _, err := ctx.pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
	require.NoError(t, err)

	cert := parsePEMCertificate(t, []byte(certPEM))
	issuedSerial := cert.SerialNumber.String()
	assert.NotEmpty(t, issuedSerial, "issued cert should have a serial")

	// Regression marker: operator_cert_serial is blanked in completeRegistration
	_ = RegressionMarkerBeforeFix
}

func TestPKIAuthority_Phase0Regression_H2_CurveInconsistency(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	rootCertPEM := testutil.ReadRootCA(t, ctx.pkiDir)
	rootCert := parsePEMCertificate(t, rootCertPEM)
	assert.Equal(t, elliptic.P256(), rootCert.PublicKey.(*ecdsa.PublicKey).Curve, RegressionMarkerAfterFix+": root CA uses P-256")

	hubCertPEM := testutil.ReadHubCA(t, ctx.pkiDir)
	hubCert := parsePEMCertificate(t, hubCertPEM)
	assert.Equal(t, elliptic.P256(), hubCert.PublicKey.(*ecdsa.PublicKey).Curve, RegressionMarkerAfterFix+": intermediate CA uses P-256")

	csr := testutil.GenerateTestCSRP256(t, "test-operator")
	certPEM, _, err := ctx.pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
	require.NoError(t, err)

	leafCert := parsePEMCertificate(t, []byte(certPEM))
	assert.Equal(t, elliptic.P256(), leafCert.PublicKey.(*ecdsa.PublicKey).Curve, RegressionMarkerAfterFix+": leaf certs use P-256")
}

func TestPKIAuthority_Phase0Regression_C3_LeafCertTTL(t *testing.T) {
	t.Parallel()
	ctx := setupTestPKI(t)

	csr := testutil.GenerateTestCSRP256(t, "test-operator")
	certPEM, _, err := ctx.pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
	require.NoError(t, err)

	leafCert := parsePEMCertificate(t, []byte(certPEM))

	duration := leafCert.NotAfter.Sub(leafCert.NotBefore)
	expectedDuration := time.Duration(leafCertValidityDays) * 24 * time.Hour
	assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0, RegressionMarkerAfterFix+": leaf cert has 7-day TTL")

	assert.Equal(t, 7, int(leafCertValidityDays), RegressionMarkerAfterFix+": leaf cert TTL is 7 days")
}

func TestPKIAuthority_SignCSR(t *testing.T) {
	t.Run("Auto-bootstrap creates valid PKI hierarchy", func(t *testing.T) {
		ctx := setupTestPKI(t)
		defer ctx.db.Close()

		// Verify PKI directory structure exists
		testutil.RequirePKIInitialized(t, ctx.pkiDir)

		// Verify we can sign a CSR using the production flow
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		certPEM, chainPEM, err := ctx.pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
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

		// Verify the chain contains the Operator CA and root CA
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
		assert.GreaterOrEqual(t, chainBlocks, 2, "chain should contain at least Operator CA and root CA")
	})

	t.Run("Auto-bootstrap creates distinct test CA per test", func(t *testing.T) {
		ctx1 := setupTestPKI(t)
		defer ctx1.db.Close()

		ctx2 := setupTestPKI(t)
		defer ctx2.db.Close()

		// Verify each test gets a distinct PKI directory
		assert.NotEqual(t, ctx1.dataDir, ctx2.dataDir, "each test should get a distinct data directory")

		// Verify root CA serials are different (distinct test CAs)
		rootCertPEM1 := testutil.ReadRootCA(t, ctx1.pkiDir)
		block1, _ := pem.Decode(rootCertPEM1)
		cert1, _ := x509.ParseCertificate(block1.Bytes)

		rootCertPEM2 := testutil.ReadRootCA(t, ctx2.pkiDir)
		block2, _ := pem.Decode(rootCertPEM2)
		cert2, _ := x509.ParseCertificate(block2.Bytes)

		assert.NotEqual(t, cert1.SerialNumber, cert2.SerialNumber, "each test should get a distinct test CA")
	})

	t.Run("Auto-bootstrap documents C1-C5 current behavior", func(t *testing.T) {
		ctx := setupTestPKI(t)
		defer ctx.db.Close()

		// Document current behavior for all critical issues
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		certPEM, _, err := ctx.pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
		require.NoError(t, err)

		block, _ := pem.Decode([]byte(certPEM))
		require.NotNil(t, block)
		cert, _ := x509.ParseCertificate(block.Bytes)

		// C1: Leaf cert has 7-day TTL ("+RegressionMarkerAfterFix+")
		leafDuration := cert.NotAfter.Sub(cert.NotBefore)
		expectedLeafDuration := time.Duration(leafCertValidityDays) * 24 * time.Hour
		assert.InDelta(t, expectedLeafDuration.Hours(), leafDuration.Hours(), 1.0, "C1: leaf has 7-day TTL, not 90-day")
	})
}

func TestPKIAuthority_GenerateCRL(t *testing.T) {
	t.Run("GenerateCRL creates standard X.509 CRL", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		certPEM, _, err := ctx.pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
		require.NoError(t, err)

		cert := parsePEMCertificate(t, []byte(certPEM))
		err = ctx.pki.RevokeCertificate(cert.SerialNumber.String(), "test revocation")
		require.NoError(t, err)

		crlDER, err := ctx.pki.GenerateCRL()
		require.NoError(t, err)
		assert.NotNil(t, crlDER)

		crl, err := x509.ParseRevocationList(crlDER)
		require.NoError(t, err)

		assert.Len(t, crl.RevokedCertificateEntries, 1)
		assert.Equal(t, cert.SerialNumber, crl.RevokedCertificateEntries[0].SerialNumber)

		err = crl.CheckSignatureFrom(ctx.pki.operatorCert)
		require.NoError(t, err, "CRL signature should verify with Operator CA")
	})

	t.Run("GenerateCRL handles empty revocation list", func(t *testing.T) {
		t.Parallel()
		ctx := setupTestPKI(t)

		crlDER, err := ctx.pki.GenerateCRL()
		require.NoError(t, err)
		assert.NotNil(t, crlDER)

		crl, err := x509.ParseRevocationList(crlDER)
		require.NoError(t, err)

		assert.Empty(t, crl.RevokedCertificateEntries)
	})
}

func TestPKIAuthority_Phase5_CurveEnforcement(t *testing.T) {
	t.Run("Phase5: SignCSR rejects P-384 CSR", func(t *testing.T) {
		t.Parallel()
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		secretsDir := tempDir(t)
		db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, secretsDir, logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
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

		_, _, err = pki.SignCSR(csrPEM, "operator", "org-123", "op-456", "", "session-789", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must use P-256 curve")
	})

	t.Run("Phase5: SignCSR accepts P-256 CSR", func(t *testing.T) {
		t.Parallel()
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		secretsDir := tempDir(t)
		db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, secretsDir, logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Generate a P-256 CSR (should be accepted)
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
		require.NoError(t, err)
		assert.NotEmpty(t, certPEM)
	})

	t.Run("Phase5: All CA and service certs use P-256", func(t *testing.T) {
		t.Parallel()
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		secretsDir := tempDir(t)
		db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, secretsDir, logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Check root CA curve
		assert.True(t, isCurveP256(pki.rootCert.PublicKey), "root CA must use P-256")

		// Check hub intermediate CA curve
		assert.True(t, isCurveP256(pki.hubCert.PublicKey), "hub CA must use P-256")

		// Check Operator intermediate CA curve
		assert.True(t, isCurveP256(pki.operatorCert.PublicKey), "operator CA must use P-256")

		// Check service cert curve
		x509Cert, err := x509.ParseCertificate(pki.serviceCert.Certificate[0])
		require.NoError(t, err)
		assert.True(t, isCurveP256(x509Cert.PublicKey), "service cert must use P-256")
	})

	t.Run("Phase5: Public certificates have 0644 permissions", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("Unix file permissions not supported on Windows")
		}
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		secretsDir := tempDir(t)
		db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, secretsDir, logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
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
		if runtime.GOOS == "windows" {
			t.Skip("Unix file permissions not supported on Windows")
		}
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		secretsDir := tempDir(t)
		db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, secretsDir, logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Check sensitive chain file has 0600 permissions
		chainFile := filepath.Join(pkiDir, "issued", "hub", "operator-gateway.chain.pem")
		info, err := os.Stat(chainFile)
		require.NoError(t, err, "chain file should exist")
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "chain file should have 0600 permissions")
	})

	t.Run("Phase5: issued/apps directory is not created", func(t *testing.T) {
		t.Parallel()
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		secretsDir := tempDir(t)
		db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, secretsDir, logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Verify issued/apps directory does not exist
		appsDir := filepath.Join(pkiDir, "issued", "apps")
		_, err = os.Stat(appsDir)
		assert.True(t, os.IsNotExist(err), "issued/apps directory should not be created")
	})
}

func TestPKIAuthority_Phase5_Permissions(t *testing.T) {
	t.Run("Phase5: Public certificates have 0644 permissions", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("Unix file permissions not supported on Windows")
		}
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		secretsDir := tempDir(t)
		db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, secretsDir, logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
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
		if runtime.GOOS == "windows" {
			t.Skip("Unix file permissions not supported on Windows")
		}
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		secretsDir := tempDir(t)
		db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, secretsDir, logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Check sensitive chain file has 0600 permissions
		chainFile := filepath.Join(pkiDir, "issued", "hub", "operator-gateway.chain.pem")
		info, err := os.Stat(chainFile)
		require.NoError(t, err, "chain file should exist")
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "chain file should have 0600 permissions")
	})
}

func TestPKIAuthority_Phase8_1_TrustBundles(t *testing.T) {
	t.Run("Phase8_1: root.pem parses with 1 certificate", func(t *testing.T) {
		t.Parallel()
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		secretsDir := tempDir(t)
		db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, secretsDir, logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
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
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		secretsDir := tempDir(t)
		db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, secretsDir, logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Load operator-bundle.pem
		operatorBundlePath := filepath.Join(pkiDir, "trust", "operator-bundle.pem")
		operatorPEM, err := os.ReadFile(operatorBundlePath)
		require.NoError(t, err)

		// Parse with x509.NewCertPool().AppendCertsFromPEM
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(operatorPEM)
		assert.True(t, ok, "operator-bundle.pem should parse as valid PEM bundle")

		// Verify it contains exactly 2 certificates (root + Operator intermediate)
		certCount := countCertificatesInPEM(operatorPEM)
		assert.Equal(t, 2, certCount, "operator-bundle.pem should contain exactly 2 certificates (root + Operator intermediate)")
	})

	t.Run("Phase8_1: g8eg-ca-bundle.pem parses with 3 certificates", func(t *testing.T) {
		t.Parallel()
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, err := OpenCanonicalDBService(dataDir, tempDir(t), filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, tempDir(t), logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Load g8eg-ca-bundle.pem
		gatewayBundlePath := filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem")
		gatewayPEM, err := os.ReadFile(gatewayBundlePath)
		require.NoError(t, err)

		// Parse with x509.NewCertPool().AppendCertsFromPEM
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(gatewayPEM)
		assert.True(t, ok, "g8eg-ca-bundle.pem should parse as valid PEM bundle")

		// Verify it contains exactly 4 certificates (root + hub intermediate + Operator intermediate + gateway peer intermediate)
		certCount := countCertificatesInPEM(gatewayPEM)
		assert.Equal(t, 4, certCount, "g8eg-ca-bundle.pem should contain exactly 4 certificates (root + hub + Operator + gateway peer intermediates)")
	})

	t.Run("Phase8_1: serving certificate verifies against g8eg-ca-bundle.pem", func(t *testing.T) {
		t.Parallel()
		dataDir := tempDir(t)
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, err := OpenCanonicalDBService(dataDir, tempDir(t), filepath.Join(dataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm, err := NewSecretManager(db.db, tempDir(t), logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.InitializePKI(nil)
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
		require.NoError(t, err, "serving certificate should verify against g8eg-ca-bundle.pem")
		assert.NotEmpty(t, chains, "verification should return at least one valid chain")
	})
}
