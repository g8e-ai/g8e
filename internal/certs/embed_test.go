// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCAPEM creates a self-signed CA certificate and returns its PEM bytes.
// Uses ECDSA P-256 for speed. The certificate is valid for 1 hour.
func generateTestCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

// Tests for new DI types

func TestTrustStore_GetRawCA_ReturnsStoredValue(t *testing.T) {
	ts := NewTrustStore(nil)
	assert.Nil(t, ts.GetRawCA())
}

func TestTrustStore_SetCA_StoresBytes(t *testing.T) {
	ts := NewTrustStore(nil)
	pem := []byte("fake-pem-data")
	ts.SetCA(pem)
	assert.Equal(t, pem, ts.GetRawCA())
}

func TestTrustStore_SetCA_OverwritesPreviousValue(t *testing.T) {
	ts := NewTrustStore(nil)
	ts.SetCA([]byte("first"))
	ts.SetCA([]byte("second"))
	assert.Equal(t, []byte("second"), ts.GetRawCA())
}

func TestTrustStore_SetCA_NilClearsValue(t *testing.T) {
	ts := NewTrustStore(nil)
	ts.SetCA([]byte("some-ca"))
	ts.SetCA(nil)
	assert.Nil(t, ts.GetRawCA())
}

func TestTrustStore_GetRootCAs_WhenCANotSet(t *testing.T) {
	ts := NewTrustStore(nil)
	pool, err := ts.GetRootCAs()
	assert.Nil(t, pool)
	require.Error(t, err)
	assert.Error(t, err)
}

func TestTrustStore_GetRootCAs_InvalidPEM(t *testing.T) {
	ts := NewTrustStore(nil)
	ts.SetCA([]byte("not-a-valid-pem-block"))
	pool, err := ts.GetRootCAs()
	assert.Nil(t, pool)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestTrustStore_GetRootCAs_ValidPEM(t *testing.T) {
	ts := NewTrustStore(nil)
	caBytes := generateTestCAPEM(t)
	ts.SetCA(caBytes)

	pool, err := ts.GetRootCAs()
	require.NoError(t, err)
	assert.NotNil(t, pool)
}

func TestClientIdentity_GetCertificate_WhenNotSet(t *testing.T) {
	ci := NewClientIdentity(tls.Certificate{})
	cert, ok := ci.GetCertificate()
	assert.False(t, ok)
	assert.Equal(t, tls.Certificate{}, cert)
}

func TestClientIdentity_SetCertificate_StoresCert(t *testing.T) {
	ci := NewClientIdentity(tls.Certificate{})
	cert := tls.Certificate{Certificate: [][]byte{[]byte("test")}}
	ci.SetCertificate(cert)

	retrieved, ok := ci.GetCertificate()
	assert.True(t, ok)
	assert.Equal(t, cert, retrieved)
}

func TestClientIdentity_SetCertificate_Overwrites(t *testing.T) {
	ci := NewClientIdentity(tls.Certificate{})
	ci.SetCertificate(tls.Certificate{Certificate: [][]byte{[]byte("first")}})
	ci.SetCertificate(tls.Certificate{Certificate: [][]byte{[]byte("second")}})

	retrieved, _ := ci.GetCertificate()
	assert.Equal(t, [][]byte{[]byte("second")}, retrieved.Certificate)
}

func TestTLSConfig_GetTLSConfig_WhenCANotSet(t *testing.T) {
	ts := NewTrustStore(nil)
	ci := NewClientIdentity(tls.Certificate{})
	tc := NewTLSConfig(ts, ci)

	cfg, err := tc.GetTLSConfig()
	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.Error(t, err)
}

func TestTLSConfig_GetTLSConfig_WithValidCA(t *testing.T) {
	ts := NewTrustStore(nil)
	ci := NewClientIdentity(tls.Certificate{})
	tc := NewTLSConfig(ts, ci)

	caBytes := generateTestCAPEM(t)
	ts.SetCA(caBytes)

	cfg, err := tc.GetTLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	assert.NotNil(t, cfg.RootCAs)
	assert.NotContains(t, cfg.CurvePreferences, tls.X25519)
	assert.Contains(t, cfg.CurvePreferences, tls.X25519MLKEM768)
	assert.Contains(t, cfg.CurvePreferences, tls.CurveP384)
	assert.Contains(t, cfg.CurvePreferences, tls.CurveP256)
}

func TestTLSConfig_GetTLSConfig_WithClientCert(t *testing.T) {
	ts := NewTrustStore(nil)
	ci := NewClientIdentity(tls.Certificate{})
	tc := NewTLSConfig(ts, ci)

	caBytes := generateTestCAPEM(t)
	ts.SetCA(caBytes)

	cert := tls.Certificate{Certificate: [][]byte{[]byte("test-cert")}}
	ci.SetCertificate(cert)

	cfg, err := tc.GetTLSConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Certificates, 1)
	assert.Equal(t, cert, cfg.Certificates[0])
}

func TestTLSConfig_GetTLSConfig_WithoutClientCert(t *testing.T) {
	ts := NewTrustStore(nil)
	ci := NewClientIdentity(tls.Certificate{})
	tc := NewTLSConfig(ts, ci)

	caBytes := generateTestCAPEM(t)
	ts.SetCA(caBytes)

	cfg, err := tc.GetTLSConfig()
	require.NoError(t, err)
	assert.Empty(t, cfg.Certificates)
}

// TestFIPSCurvePreferences_ExcludesX25519 is the FIPS 140-3 regression guard.
// X25519 is not SP 800-56A rev3 compliant and is excluded from Go's FIPS TLS
// mode, so it must never appear in any g8e TLS CurvePreferences list. Every
// g8e TLS configuration references FIPSCurvePreferences, so this single
// assertion covers the client (embed.go), server (gateway_certs.go), CLI
// mTLS (cli/auth/tls.go), and bootstrap transport (bootstrap.go) paths.
func TestFIPSCurvePreferences_ExcludesX25519(t *testing.T) {
	curves := FIPSCurvePreferences()
	assert.NotContains(t, curves, tls.X25519,
		"tls.X25519 is excluded from Go's FIPS TLS mode and must not appear in g8e CurvePreferences")
	assert.Contains(t, curves, tls.X25519MLKEM768)
	assert.Contains(t, curves, tls.CurveP384)
	assert.Contains(t, curves, tls.CurveP256)
}
