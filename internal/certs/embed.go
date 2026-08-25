// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package certs

import (
	"crypto/tls"
	"crypto/x509"
	"sync"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// FIPSCurvePreferences returns the FIPS 140-3 compliant TLS key agreement
// curve set used by every g8e TLS configuration. X25519 is excluded because it
// is not SP 800-56A rev3 compliant and is omitted from Go's FIPS TLS mode.
// X25519MLKEM768 is the FIPS 203 validated post-quantum hybrid (preferred),
// followed by the classical ECDH curves P-384 and P-256.
//
// The function returns a fresh slice on each call to prevent consumers from
// mutating a shared package-level slice.
func FIPSCurvePreferences() []tls.CurveID {
	return []tls.CurveID{
		tls.X25519MLKEM768,
		tls.CurveP384,
		tls.CurveP256,
	}
}

// TrustStore holds the CA trust bundle for TLS verification.
// It replaces the package-level serverCAPEM global with an injectable type.
type TrustStore struct {
	mu    sync.RWMutex
	caPEM []byte
}

// NewTrustStore creates a new TrustStore with optional initial CA PEM.
func NewTrustStore(caPEM []byte) *TrustStore {
	return &TrustStore{caPEM: caPEM}
}

// SetCA stores the PEM-encoded CA certificate.
func (ts *TrustStore) SetCA(pem []byte) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.caPEM = pem
}

// GetRawCA returns the current PEM bytes.
func (ts *TrustStore) GetRawCA() []byte {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.caPEM
}

// GetRootCAs returns a certificate pool containing the CA.
func (ts *TrustStore) GetRootCAs() (*x509.CertPool, error) {
	ts.mu.RLock()
	pem := ts.caPEM
	ts.mu.RUnlock()

	if len(pem) == 0 {
		return nil, constants.ErrEmptyTrustBundle
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, constants.ErrCAParseFailed
	}
	return pool, nil
}

// ClientIdentity holds the mTLS client certificate for outbound connections.
// It replaces the package-level clientCert global with an injectable type.
type ClientIdentity struct {
	mu   sync.RWMutex
	cert tls.Certificate
}

// NewClientIdentity creates a new ClientIdentity with optional initial certificate.
func NewClientIdentity(cert tls.Certificate) *ClientIdentity {
	return &ClientIdentity{cert: cert}
}

// SetCertificate stores the mTLS client certificate.
func (ci *ClientIdentity) SetCertificate(cert tls.Certificate) {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.cert = cert
}

// GetCertificate returns the mTLS client certificate and a boolean indicating if it's set.
func (ci *ClientIdentity) GetCertificate() (tls.Certificate, bool) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.cert, len(ci.cert.Certificate) > 0
}

// TLSConfig combines TrustStore and ClientIdentity to produce a complete TLS configuration.
type TLSConfig struct {
	trustStore     *TrustStore
	clientIdentity *ClientIdentity
}

// NewTLSConfig creates a new TLSConfig with the given trust store and client identity.
func NewTLSConfig(trustStore *TrustStore, clientIdentity *ClientIdentity) *TLSConfig {
	return &TLSConfig{
		trustStore:     trustStore,
		clientIdentity: clientIdentity,
	}
}

// GetTLSConfig returns a TLS configuration that trusts the CA and includes the client certificate if set.
func (tc *TLSConfig) GetTLSConfig() (*tls.Config, error) {
	rootCAs, err := tc.trustStore.GetRootCAs()
	if err != nil {
		return nil, err
	}

	cfg := &tls.Config{
		RootCAs:          rootCAs,
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: FIPSCurvePreferences(),
	}

	if cert, ok := tc.clientIdentity.GetCertificate(); ok {
		cfg.Certificates = []tls.Certificate{cert}
	}

	return cfg, nil
}
