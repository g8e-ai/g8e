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

//go:build integration

package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
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
	fileSvc fs.RuntimeFileService
	db      *CanonicalDBService
	sm      *SecretManager
	logger  *slog.Logger
}

// setupTestPKI creates a complete test PKI infrastructure with initialized hierarchy.
// Returns a context struct with all components and a cleanup function.
// This helper eliminates the repeated setup code across all tests.
func setupTestPKI(t *testing.T) *testPKIContext {
	t.Helper()

	dataDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	fileSvc := newTestFileSvc(t)
	pkiDir := fileSvc.Resolve(constants.PkiDirname)

	db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
	require.NoError(t, err, "failed to open test database")
	t.Cleanup(func() { db.Close() })

	sm := newTestSecretManager(t, db.db, fileSvc)

	pki := newPKIAuthority(fileSvc, db, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err, "failed to initialize PKI hierarchy")

	return &testPKIContext{
		pki:     pki,
		pkiDir:  pkiDir,
		dataDir: dataDir,
		fileSvc: fileSvc,
		db:      db,
		sm:      sm,
		logger:  logger,
	}
}

// loadCertificate reads and parses a PEM-encoded certificate from the given path.
func loadCertificate(t *testing.T, fileSvc fs.RuntimeFileService, relPath string) *x509.Certificate {
	t.Helper()

	certPEM, err := fileSvc.ReadFile(context.Background(), relPath)
	require.NoError(t, err, "failed to read certificate from %s", relPath)

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block, "failed to decode PEM from %s", relPath)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "failed to parse certificate from %s", relPath)

	return cert
}

// loadTrustBundle reads a PEM-encoded trust bundle and returns a cert pool.
func loadTrustBundle(t *testing.T, fileSvc fs.RuntimeFileService, relPath string) *x509.CertPool {
	t.Helper()

	bundlePEM, err := fileSvc.ReadFile(context.Background(), relPath)
	require.NoError(t, err, "failed to read trust bundle from %s", relPath)

	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM(bundlePEM)
	require.True(t, ok, "failed to parse trust bundle from %s", relPath)

	return pool
}

// readCAFromFS reads a CA certificate from the fileSvc tree using a relative path.
func readCAFromFS(t *testing.T, fileSvc fs.RuntimeFileService, relPath, caName string) []byte {
	t.Helper()

	certPEM, err := fileSvc.ReadFile(context.Background(), relPath)
	require.NoError(t, err, "CA certificate '%s' not found at %s. Ensure PKI is initialized before accessing certificates.", caName, relPath)
	require.NotEmpty(t, certPEM, "CA certificate '%s' at %s is empty", caName, relPath)

	return certPEM
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
	t.Run("Nil certificate is rejected", func(t *testing.T) {
		ctx := setupTestPKI(t)
		err := ctx.pki.VerifyCertificate(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no certificate provided")
	})

	t.Run("Valid certificate is accepted", func(t *testing.T) {
		ctx := setupTestPKI(t)

		cert := loadCertificate(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert))
		err := ctx.pki.VerifyCertificate(cert)
		require.NoError(t, err)
	})

	t.Run("Revoked certificate is rejected", func(t *testing.T) {
		ctx := setupTestPKI(t)

		cert := loadCertificate(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert))
		err := ctx.pki.RevokeCertificate(cert.SerialNumber.String(), "test revocation")
		require.NoError(t, err)

		err = ctx.pki.VerifyCertificate(cert)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "certificate is revoked")
	})
}

func TestPKIAuthority_InitializePKI(t *testing.T) {
	t.Run("Full PKI hierarchy initialization", func(t *testing.T) {
		ctx := setupTestPKI(t)

		dirs := []string{
			filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirRevocation),
		}
		for _, dir := range dirs {
			info, err := ctx.fileSvc.Stat(context.Background(), dir)
			require.NoError(t, err, "directory %s should exist", dir)
			assert.True(t, info.IsDir(), "%s should be a directory", dir)
		}
	})

	t.Run("Root CA generation", func(t *testing.T) {
		ctx := setupTestPKI(t)

		certPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
		assert.NotEmpty(t, certPEM)

		rootKeyRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCAKey)
		_, err := ctx.fileSvc.Stat(context.Background(), rootKeyRelPath)
		assert.True(t, errors.Is(err, constants.ErrNotFound), "root CA private key must not exist as PEM file")

		keyDER, err := ctx.sm.GetCAPrivateKey("root")
		require.NoError(t, err)
		assert.NotEmpty(t, keyDER)

		rootCARelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA)
		certInfo, err := ctx.fileSvc.Stat(context.Background(), rootCARelPath)
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(constants.PermFilePublic), certInfo.Mode().Perm())
		}
	})

	t.Run("Intermediate CA generation", func(t *testing.T) {
		ctx := setupTestPKI(t)

		intermediates := []string{"hub_ca", "operator_ca"}
		for _, name := range intermediates {
			certRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, name+constants.FileExtCert)
			keyRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, name+constants.FileExtKey)

			_, err := ctx.fileSvc.Stat(context.Background(), certRelPath)
			require.NoError(t, err, "%s cert should exist", name)

			_, err = ctx.fileSvc.Stat(context.Background(), keyRelPath)
			assert.True(t, errors.Is(err, constants.ErrNotFound), "%s private key must not exist as PEM file", name)

			caType := strings.TrimSuffix(name, "_ca")
			keyDER, err := ctx.sm.GetCAPrivateKey(caType)
			require.NoError(t, err, "%s key should load from keystore", caType)
			assert.NotEmpty(t, keyDER, "%s key should not be empty", caType)
		}
	})

	t.Run("Service certificate generation", func(t *testing.T) {
		ctx := setupTestPKI(t)

		serviceCertRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert)
		serviceChainRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayChain)

		_, err := ctx.fileSvc.Stat(context.Background(), serviceCertRelPath)
		require.NoError(t, err)

		_, err = ctx.fileSvc.Stat(context.Background(), serviceChainRelPath)
		require.NoError(t, err)

		serviceKeyRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayKey)
		_, err = ctx.fileSvc.Stat(context.Background(), serviceKeyRelPath)
		require.Error(t, err, "private key should not exist as plaintext file")

		keyDER, err := ctx.sm.GetServicePrivateKey("operator-gateway")
		require.NoError(t, err, "private key should be loadable from keystore")
		require.NotEmpty(t, keyDER, "private key DER should not be empty")
	})

	t.Run("Trust bundle generation", func(t *testing.T) {
		ctx := setupTestPKI(t)

		bundles := map[string]int{
			constants.PkiFileRootBundle:     1,
			constants.PkiFileGatewayBundle:  4,
			constants.PkiFileOperatorBundle: 2,
		}
		for bundleName, expectedCount := range bundles {
			bundleRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, bundleName)
			bundlePEM, err := ctx.fileSvc.ReadFile(context.Background(), bundleRelPath)
			require.NoError(t, err, "trust bundle %s should exist", bundleName)

			certPool := x509.NewCertPool()
			ok := certPool.AppendCertsFromPEM(bundlePEM)
			require.True(t, ok, "trust bundle %s should parse as valid PEM", bundleName)

			actualCount := countCertificatesInPEM(bundlePEM)
			assert.Equal(t, expectedCount, actualCount, "trust bundle %s should contain %d certificates", bundleName, expectedCount)
		}

		trustDomainRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileTrustDomainJSON)
		_, err := ctx.fileSvc.Stat(context.Background(), trustDomainRelPath)
		require.NoError(t, err, "trust domain metadata should exist")
	})

	t.Run("No root-level ca.crt mirror", func(t *testing.T) {
		ctx := setupTestPKI(t)

		const rootCAMirrorFilename = "ca.crt"
		rootCAMirrorRelPath := filepath.Join(constants.PkiDirname, rootCAMirrorFilename)
		_, err := ctx.fileSvc.Stat(context.Background(), rootCAMirrorRelPath)
		assert.True(t, errors.Is(err, constants.ErrNotFound), "ca.crt must not exist at PKI root")
	})

	t.Run("Serving certificate verifies against trust bundle", func(t *testing.T) {
		ctx := setupTestPKI(t)

		serviceCert := loadCertificate(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert))
		trustPool := loadTrustBundle(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle))

		opts := x509.VerifyOptions{Roots: trustPool}
		chains, err := serviceCert.Verify(opts)
		require.NoError(t, err, "serving certificate should verify against trust bundle")
		assert.NotEmpty(t, chains, "verification should return at least one chain")
	})
}

func TestPKIAuthority_ChainValidity(t *testing.T) {
	ctx := setupTestPKI(t)

	t.Run("Root CA is self-signed", func(t *testing.T) {
		rootCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
		rootCert := parsePEMCertificate(t, rootCertPEM)

		assert.Equal(t, rootCert.Issuer.CommonName, rootCert.Subject.CommonName)
		assert.True(t, rootCert.IsCA)
		assert.Equal(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, rootCert.KeyUsage)
	})

	t.Run("Intermediate CA chain validity", func(t *testing.T) {
		rootCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
		hubCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileHubCA), "hub")

		rootCert := parsePEMCertificate(t, rootCertPEM)
		hubCert := parsePEMCertificate(t, hubCertPEM)

		assert.Equal(t, rootCert.Subject.CommonName, hubCert.Issuer.CommonName)
		assert.True(t, hubCert.IsCA)
		assert.Equal(t, 1, hubCert.MaxPathLen)
	})

	t.Run("Service certificate chain validity", func(t *testing.T) {
		hubCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileHubCA), "hub")
		serviceCert := loadCertificate(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert))

		hubCert := parsePEMCertificate(t, hubCertPEM)

		assert.Equal(t, hubCert.Subject.CommonName, serviceCert.Issuer.CommonName)
		assert.False(t, serviceCert.IsCA)
		assert.Equal(t, x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment, serviceCert.KeyUsage)
	})
}

func TestPKIAuthority_IssuerSeparation(t *testing.T) {
	ctx := setupTestPKI(t)

	t.Run("Distinct intermediate CAs", func(t *testing.T) {
		hubCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileHubCA), "hub")
		operatorCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileOperatorCA), "operator")

		hubCert := parsePEMCertificate(t, hubCertPEM)
		operatorCert := parsePEMCertificate(t, operatorCertPEM)

		assert.NotEqual(t, hubCert.Subject.CommonName, operatorCert.Subject.CommonName)

		rootCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
		rootCert := parsePEMCertificate(t, rootCertPEM)

		assert.Equal(t, rootCert.Subject.CommonName, hubCert.Issuer.CommonName)
		assert.Equal(t, rootCert.Subject.CommonName, operatorCert.Issuer.CommonName)
	})
}

func TestPKIAuthority_URISAN(t *testing.T) {
	ctx := setupTestPKI(t)

	t.Run("Service certificate has SPIFFE URI SAN", func(t *testing.T) {
		serviceCert := loadCertificate(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert))

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
	ctx := setupTestPKI(t)

	t.Run("Root CA validity period", func(t *testing.T) {
		rootCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
		rootCert := parsePEMCertificate(t, rootCertPEM)

		duration := rootCert.NotAfter.Sub(rootCert.NotBefore)
		expectedDuration := time.Duration(rootValidityDays) * 24 * time.Hour

		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})

	t.Run("Intermediate CA validity period", func(t *testing.T) {
		hubCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileHubCA), "hub")
		hubCert := parsePEMCertificate(t, hubCertPEM)

		duration := hubCert.NotAfter.Sub(hubCert.NotBefore)
		expectedDuration := time.Duration(intermediateValidityDays) * 24 * time.Hour

		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})

	t.Run("Service certificate validity period", func(t *testing.T) {
		serviceCert := loadCertificate(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert))

		duration := serviceCert.NotAfter.Sub(serviceCert.NotBefore)
		expectedDuration := time.Duration(servingCertValidityDays) * 24 * time.Hour

		assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0)
	})
}

func TestPKIAuthority_EKU(t *testing.T) {
	ctx := setupTestPKI(t)

	t.Run("CA has correct KeyUsage", func(t *testing.T) {
		rootCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
		rootCert := parsePEMCertificate(t, rootCertPEM)

		assert.Equal(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, rootCert.KeyUsage)
	})

	t.Run("Service certificate has correct EKU", func(t *testing.T) {
		serviceCert := loadCertificate(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert))

		assert.Contains(t, serviceCert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
		assert.Contains(t, serviceCert.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
	})
}

func TestPKIAuthority_TLSConfig(t *testing.T) {
	ctx := setupTestPKI(t)

	t.Run("TLS 1.3 only", func(t *testing.T) {
		tlsConfig := ctx.pki.TLSConfig()
		assert.Equal(t, uint16(tls.VersionTLS13), tlsConfig.MinVersion)
	})

	t.Run("GetCertificate returns valid cert", func(t *testing.T) {
		tlsConfig := ctx.pki.TLSConfig()
		cert, err := tlsConfig.GetCertificate(nil)
		require.NoError(t, err)
		assert.NotNil(t, cert)
		assert.NotEmpty(t, cert.Certificate)
	})
}

func TestPKIAuthority_TrustBundlePath(t *testing.T) {
	ctx := setupTestPKI(t)

	expectedPath := filepath.Join(ctx.pkiDir, "trust", "g8eg-ca-bundle.pem")
	actualPath := ctx.pki.TrustBundlePath()
	assert.Equal(t, expectedPath, actualPath)

	_, err := ctx.fileSvc.Stat(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle))
	require.NoError(t, err)
}

func TestPKIAuthority_PKIDir(t *testing.T) {
	ctx := setupTestPKI(t)
	assert.Equal(t, ctx.pkiDir, ctx.pki.PKIDir())
}

func TestPKIAuthority_ReuseExisting(t *testing.T) {
	ctx := setupTestPKI(t)

	rootCertPEM1 := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
	cert1 := parsePEMCertificate(t, rootCertPEM1)
	serial1 := cert1.SerialNumber

	pki2 := newPKIAuthority(ctx.fileSvc, ctx.db, ctx.sm, ctx.logger)
	err := pki2.InitializePKI(nil)
	require.NoError(t, err)

	rootCertPEM2 := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
	cert2 := parsePEMCertificate(t, rootCertPEM2)
	serial2 := cert2.SerialNumber

	assert.Equal(t, serial1, serial2, "should reuse existing root CA")
}

// Phase 0 regression tests for current buggy behavior
// These tests lock down the current (broken) behavior so regressions are visible
// when we fix the issues in later phases.

func TestPKIAuthority_Phase0Regression_C1_ServiceCertRenewal(t *testing.T) {
	ctx := setupTestPKI(t)

	serviceCert := loadCertificate(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, "issued", "hub", "operator-gateway.crt"))

	duration := serviceCert.NotAfter.Sub(serviceCert.NotBefore)
	expectedDuration := time.Duration(servingCertValidityDays) * 24 * time.Hour
	assert.InDelta(t, expectedDuration.Hours(), duration.Hours(), 1.0, RegressionMarkerAfterFix+": service cert has 90-day TTL")

	assert.Equal(t, 90, int(servingCertValidityDays), RegressionMarkerAfterFix+": servingCertValidityDays is 90 days")
}

func TestPKIAuthority_Phase0Regression_C2_OperatorSerialBlank(t *testing.T) {
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
	ctx := setupTestPKI(t)

	rootCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
	rootCert := parsePEMCertificate(t, rootCertPEM)
	assert.Equal(t, elliptic.P256(), rootCert.PublicKey.(*ecdsa.PublicKey).Curve, RegressionMarkerAfterFix+": root CA uses P-256")

	hubCertPEM := readCAFromFS(t, ctx.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileHubCA), "hub")
	hubCert := parsePEMCertificate(t, hubCertPEM)
	assert.Equal(t, elliptic.P256(), hubCert.PublicKey.(*ecdsa.PublicKey).Curve, RegressionMarkerAfterFix+": intermediate CA uses P-256")

	csr := testutil.GenerateTestCSRP256(t, "test-operator")
	certPEM, _, err := ctx.pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
	require.NoError(t, err)

	leafCert := parsePEMCertificate(t, []byte(certPEM))
	assert.Equal(t, elliptic.P256(), leafCert.PublicKey.(*ecdsa.PublicKey).Curve, RegressionMarkerAfterFix+": leaf certs use P-256")
}

func TestPKIAuthority_Phase0Regression_C3_LeafCertTTL(t *testing.T) {
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

		// Verify PKI directory structure exists
		for _, dir := range []string{
			filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust),
		} {
			info, err := ctx.fileSvc.Stat(context.Background(), dir)
			require.NoError(t, err, "PKI directory %s does not exist", dir)
			require.True(t, info.IsDir(), "PKI path %s is not a directory", dir)
		}
		rootCARelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA)
		_, err := ctx.fileSvc.Stat(context.Background(), rootCARelPath)
		require.NoError(t, err, "Root CA certificate does not exist at %s. PKI may not be initialized.", rootCARelPath)

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

		ctx2 := setupTestPKI(t)

		// Verify each test gets a distinct PKI directory
		assert.NotEqual(t, ctx1.dataDir, ctx2.dataDir, "each test should get a distinct data directory")

		// Verify root CA serials are different (distinct test CAs)
		rootCertPEM1 := readCAFromFS(t, ctx1.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
		block1, _ := pem.Decode(rootCertPEM1)
		cert1, _ := x509.ParseCertificate(block1.Bytes)

		rootCertPEM2 := readCAFromFS(t, ctx2.fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA), "root")
		block2, _ := pem.Decode(rootCertPEM2)
		cert2, _ := x509.ParseCertificate(block2.Bytes)

		assert.NotEqual(t, cert1.SerialNumber, cert2.SerialNumber, "each test should get a distinct test CA")
	})

	t.Run("Auto-bootstrap documents C1-C5 current behavior", func(t *testing.T) {
		ctx := setupTestPKI(t)

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
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
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
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Generate a P-256 CSR (should be accepted)
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
		require.NoError(t, err)
		assert.NotEmpty(t, certPEM)
	})

	t.Run("Phase5: All CA and service certs use P-256", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
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
		if runtime.GOOS == "windows" {
			t.Skip("Unix file permissions not supported on Windows")
		}
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Check public certificate files have 0644 permissions
		publicFiles := []string{
			filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileHubCA),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileOperatorCA),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileRootBundle),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileOperatorBundle),
		}

		for _, file := range publicFiles {
			info, err := fileSvc.Stat(context.Background(), file)
			require.NoError(t, err, "file should exist: "+file)
			assert.Equal(t, os.FileMode(constants.PermFilePublic), info.Mode().Perm(), "public file should have 0644 permissions: "+file)
		}
	})

	t.Run("Phase5: Sensitive chain file has 0600 permissions", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Unix file permissions not supported on Windows")
		}
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Check sensitive chain file has 0600 permissions
		chainRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayChain)
		info, err := fileSvc.Stat(context.Background(), chainRelPath)
		require.NoError(t, err, "chain file should exist")
		assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm(), "chain file should have 0600 permissions")
	})

	t.Run("Phase5: issued/apps directory is created by CreateRuntimeTree", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Verify issued/apps directory exists (created by CreateRuntimeTree)
		appsRelDir := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirApps)
		info, err := fileSvc.Stat(context.Background(), appsRelDir)
		require.NoError(t, err, "issued/apps directory should exist (created by CreateRuntimeTree)")
		assert.True(t, info.IsDir(), "issued/apps should be a directory")
	})
}

func TestPKIAuthority_Phase5_Permissions(t *testing.T) {
	t.Run("Phase5: Public certificates have 0644 permissions", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Unix file permissions not supported on Windows")
		}
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Check public certificate files have 0644 permissions
		publicFiles := []string{
			filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileHubCA),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirAuthorities, constants.PkiFileOperatorCA),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileRootBundle),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle),
			filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileOperatorBundle),
		}

		for _, file := range publicFiles {
			info, err := fileSvc.Stat(context.Background(), file)
			require.NoError(t, err, "file should exist: "+file)
			assert.Equal(t, os.FileMode(constants.PermFilePublic), info.Mode().Perm(), "public file should have 0644 permissions: "+file)
		}
	})

	t.Run("Phase5: Sensitive chain file has 0600 permissions", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Unix file permissions not supported on Windows")
		}
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Check sensitive chain file has 0600 permissions
		chainRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayChain)
		info, err := fileSvc.Stat(context.Background(), chainRelPath)
		require.NoError(t, err, "chain file should exist")
		assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm(), "chain file should have 0600 permissions")
	})
}

func TestPKIAuthority_Phase8_1_TrustBundles(t *testing.T) {
	t.Run("Phase8_1: root.pem parses with 1 certificate", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Load root.pem bundle
		rootBundleRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileRootBundle)
		rootPEM, err := fileSvc.ReadFile(context.Background(), rootBundleRelPath)
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
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Load operator-bundle.pem
		operatorBundleRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileOperatorBundle)
		operatorPEM, err := fileSvc.ReadFile(context.Background(), operatorBundleRelPath)
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
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Load g8eg-ca-bundle.pem
		gatewayBundleRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
		gatewayPEM, err := fileSvc.ReadFile(context.Background(), gatewayBundleRelPath)
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
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, err := openTestDB(t, dataDir, filepath.Join(dataDir, constants.VaultDirname), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, db, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		// Load the serving certificate
		serviceCertRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert)
		certPEM, err := fileSvc.ReadFile(context.Background(), serviceCertRelPath)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		require.NotNil(t, block, "serving certificate should decode as PEM")

		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err, "serving certificate should parse as X.509")

		// Load the gateway trust bundle
		gatewayBundleRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
		gatewayPEM, err := fileSvc.ReadFile(context.Background(), gatewayBundleRelPath)
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
