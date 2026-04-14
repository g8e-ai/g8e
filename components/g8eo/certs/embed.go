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

package certs

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
)

// serverCAMu guards serverCAPEM.
var serverCAMu sync.RWMutex

// serverCAPEM holds the PEM-encoded CA certificate fetched from the hub at
// startup via FetchAndSetCA. It is never embedded at build time.
var serverCAPEM []byte

// GetRawCA returns the current PEM bytes stored in the CA store. Intended for
// use in tests to save and restore state around SetCA calls.
func GetRawCA() []byte {
	serverCAMu.RLock()
	defer serverCAMu.RUnlock()
	return serverCAPEM
}

// SetCA stores the PEM-encoded CA certificate for use by GetTLSConfig and
// GetServerCARootCAs. Must be called before any TLS connections are made.
func SetCA(pem []byte) {
	serverCAMu.Lock()
	defer serverCAMu.Unlock()
	serverCAPEM = pem
}

// GetServerCARootCAs returns a certificate pool containing the hub CA.
func GetServerCARootCAs() (*x509.CertPool, error) {
	serverCAMu.RLock()
	pem := serverCAPEM
	serverCAMu.RUnlock()

	if len(pem) == 0 {
		return nil, fmt.Errorf("server CA not set — call certs.SetCA before making TLS connections")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("failed to parse server CA certificate")
	}
	return pool, nil
}

// GetTLSConfig returns a TLS configuration that trusts the hub CA.
// No client certificate is included — the per-operator mTLS cert is applied
// after bootstrap via rebuildTransportWithOperatorCert.
func GetTLSConfig() (*tls.Config, error) {
	rootCAs, err := GetServerCARootCAs()
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		RootCAs:    rootCAs,
		MinVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP384,
			tls.CurveP256,
		},
	}, nil
}
