// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCA generates a self-signed CA certificate and returns its PEM and the
// private key.
func testCA(t *testing.T, cn string) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return string(pemBytes), key
}

// testLeaf generates a leaf certificate signed by parent and returns its PEM.
func testLeaf(t *testing.T, cn string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, parent, &key.PublicKey, parentKey)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// testBundle builds a four-certificate gateway-style bundle: root + hub
// intermediate + operator intermediate + gateway peer intermediate. Returns
// the concatenated PEM and the root certificate.
func testBundle(t *testing.T) (string, *x509.Certificate) {
	t.Helper()
	rootPEM, rootKey := testCA(t, "g8e-test-root")
	rootCert := mustParse(t, rootPEM)
	hubPEM, hubKey := testSignedCA(t, "g8e-test-hub", rootCert, rootKey)
	hubCert := mustParse(t, hubPEM)
	opPEM, opKey := testSignedCA(t, "g8e-test-operator", hubCert, hubKey)
	opCert := mustParse(t, opPEM)
	gwPEM := testLeaf(t, "g8e-test-gateway", opCert, opKey)
	return rootPEM + hubPEM + opPEM + gwPEM, rootCert
}

// testSignedCA generates an intermediate CA signed by parent and returns its
// PEM and private key.
func testSignedCA(t *testing.T, cn string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, parent, &key.PublicKey, parentKey)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), key
}

func mustParse(t *testing.T, pemStr string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(pemStr))
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}

func TestExtractRootAnchors_FourCertBundle(t *testing.T) {
	t.Parallel()
	bundle, _ := testBundle(t)
	roots, err := extractRootAnchors([]byte(bundle), time.Now)
	require.NoError(t, err)
	require.Len(t, roots, 1)
	assert.Equal(t, "g8e-test-root", roots[0].Subject.CommonName)
}

func TestExtractRootAnchors_EmptyBundle(t *testing.T) {
	t.Parallel()
	_, err := extractRootAnchors(nil, time.Now)
	require.Error(t, err)
}

func TestExtractRootAnchors_MalformedPEM(t *testing.T) {
	t.Parallel()
	_, err := extractRootAnchors([]byte("not a pem at all"), time.Now)
	require.Error(t, err)
}

func TestExtractRootAnchors_NonCAPEMOnly(t *testing.T) {
	t.Parallel()
	leafPEM, _, _ := testLeafAndKey(t)
	roots, err := extractRootAnchors([]byte(leafPEM), time.Now)
	require.NoError(t, err)
	assert.Empty(t, roots, "no CA certs should yield no root anchors")
}

func TestExtractRootAnchors_ExpiredCA(t *testing.T) {
	t.Parallel()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "expired-root"},
		NotBefore:             time.Now().Add(-48 * time.Hour),
		NotAfter:              time.Now().Add(-time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	expiredPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	_, err = extractRootAnchors(expiredPEM, time.Now)
	require.Error(t, err)
}

func TestExtractRootAnchors_NotYetValidCA(t *testing.T) {
	t.Parallel()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "future-root"},
		NotBefore:             time.Now().Add(time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	futurePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	_, err = extractRootAnchors(futurePEM, time.Now)
	require.Error(t, err)
}

func TestCertFingerprint_Stable(t *testing.T) {
	t.Parallel()
	caPEM, _ := testCA(t, "fp-test")
	cert := mustParse(t, caPEM)
	fp1 := certFingerprint(cert)
	fp2 := certFingerprint(cert)
	assert.Equal(t, fp1, fp2)
	assert.Len(t, fp1, 64) // SHA-256 hex = 64 chars
}

func TestParseBundleCerts_MultipleCerts(t *testing.T) {
	t.Parallel()
	ca1PEM, _ := testCA(t, "ca-one")
	ca2PEM, _ := testCA(t, "ca-two")
	certs, err := parseBundleCerts([]byte(ca1PEM + ca2PEM))
	require.NoError(t, err)
	assert.Len(t, certs, 2)
}

func TestParseBundleCerts_IgnoresNonCertificateBlocks(t *testing.T) {
	t.Parallel()
	keyPEM := generateECKeyPEM(t)
	caPEM, _ := testCA(t, "ca-with-key")
	certs, err := parseBundleCerts([]byte(keyPEM + caPEM))
	require.NoError(t, err)
	assert.Len(t, certs, 1)
}

func TestVerifyRootUsable_ValidChain(t *testing.T) {
	t.Parallel()
	rootPEM, rootKey := testCA(t, "root-verify")
	rootCert := mustParse(t, rootPEM)
	hubPEM, hubKey := testSignedCA(t, "hub-verify", rootCert, rootKey)
	leafPEM := testLeaf(t, "leaf-verify", mustParse(t, hubPEM), hubKey)
	bundle := rootPEM + hubPEM + leafPEM
	err := verifyRootUsable([]*x509.Certificate{rootCert}, []byte(bundle), time.Now)
	assert.NoError(t, err)
}

func TestVerifyRootUsable_UnrelatedRoot(t *testing.T) {
	t.Parallel()
	root1PEM, _ := testCA(t, "root-unrelated-1")
	root2PEM, root2Key := testCA(t, "root-unrelated-2")
	root2Cert := mustParse(t, root2PEM)
	hubPEM, _ := testSignedCA(t, "hub-unrelated", root2Cert, root2Key)
	bundle := root2PEM + hubPEM
	err := verifyRootUsable([]*x509.Certificate{mustParse(t, root1PEM)}, []byte(bundle), time.Now)
	require.Error(t, err)
}

// testLeafAndKey generates a self-signed leaf (not a CA) for testing.
func testLeafAndKey(t *testing.T) (string, *ecdsa.PrivateKey, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "leaf-only"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), key, key
}

func generateECKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keyBytes, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}))
}
