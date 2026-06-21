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

package main

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/constants"
)

// probeGatewayTLS dials the gateway HTTPS port with certificate verification disabled
// solely to capture and log the raw certificate chain the gateway presents.
// This is debug-only; it does NOT establish an authenticated connection.
func probeGatewayTLS(logger *slog.Logger, endpoint string, trustStore *certs.TrustStore) {
	httpsPort := constants.Ports.OperatorHttps
	addr := fmt.Sprintf("%s:%d", endpoint, httpsPort)
	logger.Info("[TLS-DEBUG] dialing gateway (InsecureSkipVerify=true to capture chain)", "addr", addr)

	tlsCfg := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // intentional: debug-only cert chain capture // lgtm[go/disabled-certificate-check]
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			logger.Info("[TLS-DEBUG] gateway presented cert chain", "chain_len", len(rawCerts))
			for i, derBytes := range rawCerts {
				cert, err := x509.ParseCertificate(derBytes)
				if err != nil {
					logger.Warn("[TLS-DEBUG] failed to parse chain cert", "idx", i, "error", err)
					continue
				}
				fp := sha256.Sum256(derBytes)
				logger.Info("[TLS-DEBUG] gateway chain cert",
					"idx", i,
					"subject", cert.Subject.String(),
					"issuer", cert.Issuer.String(),
					"serial", cert.SerialNumber.String(),
					"not_before", cert.NotBefore.Format(time.RFC3339),
					"not_after", cert.NotAfter.Format(time.RFC3339),
					"is_ca", cert.IsCA,
					"key_algo", cert.PublicKeyAlgorithm.String(),
					"sig_algo", cert.SignatureAlgorithm.String(),
					"sha256", hex.EncodeToString(fp[:]),
				)
			}
			// Now try to verify the chain against our trust store and log the result.
			if len(rawCerts) == 0 {
				return nil
			}
			rootCAs, err := trustStore.GetRootCAs()
			if err != nil {
				logger.Warn("[TLS-DEBUG] trust store unavailable for chain verification", "error", err)
				return nil
			}
			leaf, _ := x509.ParseCertificate(rawCerts[0])
			if leaf == nil {
				return nil
			}
			// Build intermediate pool from remaining certs in the chain.
			intermediates := x509.NewCertPool()
			for _, der := range rawCerts[1:] {
				if c, err := x509.ParseCertificate(der); err == nil {
					intermediates.AddCert(c)
				}
			}
			opts := x509.VerifyOptions{
				Roots:         rootCAs,
				Intermediates: intermediates,
				CurrentTime:   time.Now(),
			}
			if net.ParseIP(endpoint) != nil {
				opts.DNSName = constants.GatewayInternalHostname
			} else {
				opts.DNSName = endpoint
			}
			chains, verifyErr := leaf.Verify(opts)
			if verifyErr != nil {
				logger.Error("[TLS-DEBUG] manual chain verification FAILED", "error", verifyErr)
			} else {
				logger.Info("[TLS-DEBUG] manual chain verification OK", "chain_count", len(chains))
			}
			return nil
		},
	}
	if net.ParseIP(endpoint) != nil {
		tlsCfg.ServerName = constants.GatewayInternalHostname
	}

	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		// The handshake will still run VerifyPeerCertificate before returning
		// an error, so the certs will have been logged.  Only log if we got
		// no cert data at all (e.g. connection refused).
		logger.Warn("[TLS-DEBUG] probe dial error (certs may still have been logged above)", "error", err)
		return
	}
	conn.Close()
	logger.Info("[TLS-DEBUG] probe dial completed cleanly")
}
