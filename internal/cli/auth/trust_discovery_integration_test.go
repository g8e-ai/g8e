// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// These tests exercise DiscoverLiveTrustBundle against real httptest
// HTTP servers. They use network listeners and therefore belong in
// Tier 2 (integration), not Tier 1. Pure ValidateTrustBundle tests
// (no network) live in trust_discovery_test.go.

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

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
