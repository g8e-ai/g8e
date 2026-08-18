// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeTestCLICert creates a self-signed x509 certificate with optional URI SANs
// and configurable validity period. This is a Tier 1 helper — no external deps.
func makeTestCLICert(t *testing.T, notBefore, notAfter time.Time, spiffeURIs []string) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	var uris []*url.URL
	for _, s := range spiffeURIs {
		u, err := url.Parse(s)
		require.NoError(t, err)
		uris = append(uris, u)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-cli-cert"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		URIs:         uris,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cert
}

func TestExtractUserIDFromCert_NilCert(t *testing.T) {
	userID, err := ExtractUserIDFromCert(nil)
	require.Error(t, err)
	assert.Empty(t, userID)
	assert.ErrorIs(t, err, constants.ErrCLIL3CertNil)
}

func TestExtractUserIDFromCert_NoURIs(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		nil,
	)

	userID, err := ExtractUserIDFromCert(cert)
	require.Error(t, err)
	assert.Empty(t, userID)
	assert.ErrorIs(t, err, constants.ErrCLIL3NoUserIDInCert)
}

func TestExtractUserIDFromCert_NonCLISPIFFEURI(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{"spiffe://g8e.local/operator/org-1/op-1/sess-1"},
	)

	userID, err := ExtractUserIDFromCert(cert)
	require.Error(t, err)
	assert.Empty(t, userID)
	assert.ErrorIs(t, err, constants.ErrCLIL3NoUserIDInCert)
}

func TestExtractUserIDFromCert_ValidCLISPIFFEURI(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{"spiffe://g8e.local/cli/user-789/session-abc"},
	)

	userID, err := ExtractUserIDFromCert(cert)
	require.NoError(t, err)
	assert.Equal(t, "user-789", userID)
}

func TestExtractUserIDFromCert_MultipleURIsReturnsFirstCLIMatch(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{
			"spiffe://g8e.local/operator/org-1/op-1/sess-1",
			"spiffe://g8e.local/user/user-999",
			"spiffe://g8e.local/cli/target-user/sess-xyz",
		},
	)

	userID, err := ExtractUserIDFromCert(cert)
	require.NoError(t, err)
	assert.Equal(t, "target-user", userID)
}

func TestExtractUserIDFromCert_MultipleNonCLIURIs(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{
			"spiffe://g8e.local/operator/org-1/op-1/sess-1",
			"spiffe://g8e.local/gateway/gw-1",
		},
	)

	userID, err := ExtractUserIDFromCert(cert)
	require.Error(t, err)
	assert.Empty(t, userID)
	assert.ErrorIs(t, err, constants.ErrCLIL3NoUserIDInCert)
}
