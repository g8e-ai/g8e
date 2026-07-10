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
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
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

func TestVerifyCLICertificate_NilCert(t *testing.T) {
	err := VerifyCLICertificate(&PKIAuthority{}, nil, "session-123", "user-456")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIL3CertNil)
}

func TestVerifyCLICertificate_ExpiredCert(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-48*time.Hour),
		time.Now().Add(-24*time.Hour),
		[]string{"spiffe://g8e.local/cli/user-456/session-123"},
	)

	err := VerifyCLICertificate(&PKIAuthority{}, cert, "session-123", "user-456")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIL3CertExpired)
}

func TestVerifyCLICertificate_NotYetValidCert(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(24*time.Hour),
		time.Now().Add(48*time.Hour),
		[]string{"spiffe://g8e.local/cli/user-456/session-123"},
	)

	err := VerifyCLICertificate(&PKIAuthority{}, cert, "session-123", "user-456")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIL3CertNotYetValid)
}

func TestVerifyCLICertificate_SPIFFESANMismatch(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{"spiffe://g8e.local/cli/wrong-user/wrong-session"},
	)

	err := VerifyCLICertificate(&PKIAuthority{}, cert, "session-123", "user-456")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIL3SPIFFESANMismatch)
}

func TestVerifyCLICertificate_NoURIsInCert(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		nil,
	)

	err := VerifyCLICertificate(&PKIAuthority{}, cert, "session-123", "user-456")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIL3SPIFFESANMismatch)
}

func TestVerifyCLICertificate_MultipleURIsOneMatching(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{
			"spiffe://g8e.local/operator/org-1/op-1/sess-1",
			"spiffe://g8e.local/cli/user-456/session-123",
		},
	)

	// PKI is nil so we expect ErrCLIL3PKINotConfigured, which proves the SPIFFE match succeeded.
	err := VerifyCLICertificate(nil, cert, "session-123", "user-456")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIL3PKINotConfigured)
}

func TestVerifyCLICertificate_NilPKI(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{"spiffe://g8e.local/cli/user-456/session-123"},
	)

	err := VerifyCLICertificate(nil, cert, "session-123", "user-456")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIL3PKINotConfigured)
}

func TestVerifyCLICertificate_PKIVerifyFails(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{"spiffe://g8e.local/cli/user-456/session-123"},
	)

	// PKIAuthority with nil db causes VerifyCertificate to fail with ErrPKIDatabaseNotAvailable.
	pki := &PKIAuthority{}
	err := VerifyCLICertificate(pki, cert, "session-123", "user-456")
	require.Error(t, err)
	assert.Contains(t, err.Error(), constants.ErrCLIL3CertVerificationFailed.Error())
	assert.ErrorIs(t, err, constants.ErrPKIDatabaseNotAvailable)
}

func TestVerifyCLICertificate_UserIDMismatch(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{"spiffe://g8e.local/cli/user-456/session-123"},
	)

	err := VerifyCLICertificate(&PKIAuthority{}, cert, "session-123", "different-user")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIL3SPIFFESANMismatch)
}

func TestVerifyCLICertificate_SessionIDMismatch(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{"spiffe://g8e.local/cli/user-456/session-123"},
	)

	err := VerifyCLICertificate(&PKIAuthority{}, cert, "different-session", "user-456")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIL3SPIFFESANMismatch)
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

// TestVerifyCLICertificate_ErrorWrapping ensures the PKI verification failure
// wraps the underlying error so callers can unwrap it.
func TestVerifyCLICertificate_ErrorWrapping(t *testing.T) {
	cert := makeTestCLICert(t,
		time.Now().Add(-1*time.Minute),
		time.Now().Add(24*time.Hour),
		[]string{"spiffe://g8e.local/cli/user-456/session-123"},
	)

	pki := &PKIAuthority{}
	err := VerifyCLICertificate(pki, cert, "session-123", "user-456")
	require.Error(t, err)

	inner := errors.Unwrap(err)
	require.Error(t, inner, "wrapped error should contain an inner error")
}
