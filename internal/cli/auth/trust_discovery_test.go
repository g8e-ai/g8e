// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version.0.

package auth

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestValidateTrustBundle_ValidBundleWithRootAndLeaf(t *testing.T) {
	caKey, caCert := generateTestCAWithKey(t, "test-root-ca")
	caPEM := pemEncode("CERTIFICATE", caCert.Raw)
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "test-leaf", caCert, caKey)
	bundlePEM := []byte(caPEM + leafPEM)

	result, err := ValidateTrustBundle(bundlePEM, time.Now)
	require.NoError(t, err)
	assert.Equal(t, bundlePEM, result.BundlePEM)
	require.Len(t, result.RootAnchors, 1)
	assert.Equal(t, "test-root-ca", result.RootAnchors[0].Subject.CommonName)
	assert.NotNil(t, result.PrimaryRoot)
	assert.Equal(t, platform.CertFingerprint(caCert), result.Fingerprint)
}

func TestValidateTrustBundle_EmptyBundleReturnsErrSystemTrustInvalidAnchor(t *testing.T) {
	_, err := ValidateTrustBundle([]byte{}, time.Now)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEmptyTrustBundle)
}

func TestValidateTrustBundle_MalformedPEMReturnsErrSystemTrustInvalidAnchor(t *testing.T) {
	_, err := ValidateTrustBundle([]byte("not a pem"), time.Now)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSystemTrustInvalidAnchor)
}

func TestValidateTrustBundle_ExpiredRootReturnsErrSystemTrustInvalidAnchor(t *testing.T) {
	caKey, caCert := generateTestCAWithKeyAndExpiry(t, "expired-root", time.Now().Add(-time.Hour))
	caPEM := pemEncode("CERTIFICATE", caCert.Raw)
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "test-leaf", caCert, caKey)
	bundlePEM := []byte(caPEM + leafPEM)

	_, err := ValidateTrustBundle(bundlePEM, time.Now)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSystemTrustInvalidAnchor)
}

func TestValidateTrustBundle_NoRootAnchorsReturnsErrSystemTrustInvalidAnchor(t *testing.T) {
	// A bundle with only leaf certs (no self-signed CA) has no root
	// anchors. ExtractRootAnchors returns (nil, nil) — it skips
	// non-CA certs without erroring. ValidateTrustBundle catches the
	// empty root slice and returns ErrSystemTrustInvalidAnchor.
	caKey, caCert := generateTestCAWithKey(t, "test-root-ca")
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "test-leaf", caCert, caKey)
	// Deliberately exclude the root CA PEM — only the leaf.
	bundlePEM := []byte(leafPEM)

	_, err := ValidateTrustBundle(bundlePEM, time.Now)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSystemTrustInvalidAnchor)
}

func TestValidateTrustBundle_NoChainToAnchorReturnsErrSystemTrustInvalidAnchor(t *testing.T) {
	// Two unrelated root CAs: the leaf from one won't chain to the
	// other. Build a bundle with rootA + leafB (signed by rootB, not
	// rootA). VerifyRootUsable should fail because no non-root cert
	// chains to rootA (the only root in the bundle).
	_, caCertA := generateTestCAWithKey(t, "root-a")
	caKeyB, caCertB := generateTestCAWithKey(t, "root-b")
	caAPEM := pemEncode("CERTIFICATE", caCertA.Raw)
	leafBPEM, _ := testutil.GenerateTestSignedCert(t, "leaf-b", caCertB, caKeyB)
	bundlePEM := []byte(caAPEM + leafBPEM)

	_, err := ValidateTrustBundle(bundlePEM, time.Now)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSystemTrustNoChainToAnchor)
}

func TestValidateTrustBundle_FingerprintMatchesCertFingerprint(t *testing.T) {
	caKey, caCert := generateTestCAWithKey(t, "fingerprint-root")
	caPEM := pemEncode("CERTIFICATE", caCert.Raw)
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "leaf", caCert, caKey)
	bundlePEM := []byte(caPEM + leafPEM)

	result, err := ValidateTrustBundle(bundlePEM, time.Now)
	require.NoError(t, err)
	assert.Equal(t, platform.CertFingerprint(caCert), result.Fingerprint)
}

func TestDiscoverLiveTrustBundle_FetchesAndValidatesBundle(t *testing.T) {
	caKey, caCert := generateTestCAWithKey(t, "discovery-root")
	caPEM := pemEncode("CERTIFICATE", caCert.Raw)
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "discovery-leaf", caCert, caKey)
	bundlePEM := []byte(caPEM + leafPEM)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write(bundlePEM)
	}))
	defer server.Close()

	result, err := DiscoverLiveTrustBundle(t.Context(), server.URL, time.Now)
	require.NoError(t, err)
	assert.Equal(t, bundlePEM, result.BundlePEM)
	require.Len(t, result.RootAnchors, 1)
	assert.Equal(t, "discovery-root", result.PrimaryRoot.Subject.CommonName)
	assert.Equal(t, platform.CertFingerprint(caCert), result.Fingerprint)
}

func TestDiscoverLiveTrustBundle_NetworkFailureReturnsError(t *testing.T) {
	// Use a closed server to simulate a network failure.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	server.Close()

	_, err := DiscoverLiveTrustBundle(t.Context(), server.URL, time.Now)
	require.Error(t, err)
}

func TestDiscoverLiveTrustBundle_InvalidBundleReturnsFetchError(t *testing.T) {
	// Non-PEM content is rejected by FetchTrustBundleWithClient before
	// ValidateTrustBundle is called, so the error is a fetch/parse
	// error, not ErrSystemTrustInvalidAnchor.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not a certificate bundle"))
	}))
	defer server.Close()

	_, err := DiscoverLiveTrustBundle(t.Context(), server.URL, time.Now)
	require.Error(t, err)
}

func TestDiscoverLiveTrustBundle_RootOnlyNoLeafReturnsErrSystemTrustNoChainToAnchor(t *testing.T) {
	// A bundle with only a root CA and no non-root cert that chains to
	// it. VerifyRootUsable fails because there is no non-root cert to
	// verify.
	_, caCert := generateTestCAWithKey(t, "lonely-root")
	caPEM := pemEncode("CERTIFICATE", caCert.Raw)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(caPEM))
	}))
	defer server.Close()

	_, err := DiscoverLiveTrustBundle(t.Context(), server.URL, time.Now)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSystemTrustNoChainToAnchor)
}

// Ensure DiscoverLiveTrustBundle does not use InsecureSkipVerify by
// verifying the returned result's root anchors are real x509
// certificates (not nil placeholders).
func TestDiscoverLiveTrustBundle_RootAnchorsAreParsedCertificates(t *testing.T) {
	caKey, caCert := generateTestCAWithKey(t, "parsed-root")
	caPEM := pemEncode("CERTIFICATE", caCert.Raw)
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "parsed-leaf", caCert, caKey)
	bundlePEM := []byte(caPEM + leafPEM)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bundlePEM)
	}))
	defer server.Close()

	result, err := DiscoverLiveTrustBundle(t.Context(), server.URL, time.Now)
	require.NoError(t, err)
	for _, anchor := range result.RootAnchors {
		assert.NotNil(t, anchor.Raw, "root anchor must have parsed DER bytes")
		assert.True(t, anchor.IsCA, "root anchor must be a CA")
	}
}

// Ensure the bundle PEM in the result can be re-parsed independently.
func TestValidateTrustBundle_ResultBundlePEMIsReparseable(t *testing.T) {
	caKey, caCert := generateTestCAWithKey(t, "reparse-root")
	caPEM := pemEncode("CERTIFICATE", caCert.Raw)
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "reparse-leaf", caCert, caKey)
	bundlePEM := []byte(caPEM + leafPEM)

	result, err := ValidateTrustBundle(bundlePEM, time.Now)
	require.NoError(t, err)

	// Re-parse the bundle PEM independently.
	var certs []*x509.Certificate
	rest := result.BundlePEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		cert, pErr := x509.ParseCertificate(block.Bytes)
		require.NoError(t, pErr)
		certs = append(certs, cert)
	}
	assert.Len(t, certs, 2, "bundle PEM must contain exactly 2 certificates (root + leaf)")
}

// Ensure the error from DiscoverLiveTrustBundle on a server returning
// non-PEM content is a fetch/parse error (FetchTrustBundleWithClient
// validates PEM content before ValidateTrustBundle runs).
func TestDiscoverLiveTrustBundle_NonPEMContentReturnsFetchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain text, not PEM"))
	}))
	defer server.Close()

	_, err := DiscoverLiveTrustBundle(t.Context(), server.URL, time.Now)
	require.Error(t, err)
}

// Ensure the error message from ValidateTrustBundle on a missing root
// anchor chain includes the underlying error for diagnostics.
func TestValidateTrustBundle_ChainFailurePreservesUnderlyingError(t *testing.T) {
	_, caCertA := generateTestCAWithKey(t, "unrelated-root-a")
	caKeyB, caCertB := generateTestCAWithKey(t, "unrelated-root-b")
	caAPEM := pemEncode("CERTIFICATE", caCertA.Raw)
	leafBPEM, _ := testutil.GenerateTestSignedCert(t, "unrelated-leaf-b", caCertB, caKeyB)
	bundlePEM := []byte(caAPEM + leafBPEM)

	_, err := ValidateTrustBundle(bundlePEM, time.Now)
	require.Error(t, err)
	// The wrapped error must be ErrSystemTrustInvalidAnchor (the
	// outer wrap) and the underlying must be
	// ErrSystemTrustNoChainToAnchor (from VerifyRootUsable).
	assert.ErrorIs(t, err, constants.ErrSystemTrustInvalidAnchor)
	assert.ErrorIs(t, err, constants.ErrSystemTrustNoChainToAnchor)
	// The error message must mention the chain failure for diagnostics.
	assert.True(t, strings.Contains(err.Error(), "no certificate in the bundle chains"),
		"error message must mention chain failure, got: %s", err.Error())
}

// Ensure ValidateTrustBundle wraps ExtractRootAnchors errors with
// ErrSystemTrustInvalidAnchor so callers can check for the outer
// constant regardless of the underlying cause.
func TestValidateTrustBundle_EmptyBundleErrorIsWrappedAsInvalidAnchor(t *testing.T) {
	_, err := ValidateTrustBundle([]byte{}, time.Now)
	require.Error(t, err)
	// ValidateTrustBundle wraps all ExtractRootAnchors errors with
	// ErrSystemTrustInvalidAnchor, so errors.Is succeeds for both the
	// outer wrap and the underlying ErrEmptyTrustBundle.
	assert.ErrorIs(t, err, constants.ErrSystemTrustInvalidAnchor)
	assert.ErrorIs(t, err, constants.ErrEmptyTrustBundle)
}
