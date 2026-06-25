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
	"crypto/x509"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/protocol"
)

// VerifyCLICertificate verifies that a CLI certificate is valid for the given CLI session.
// This is used during request authentication to validate the mTLS certificate.
func VerifyCLICertificate(pki *PKIAuthority, cert *x509.Certificate, cliSessionID, userID string) error {
	if cert == nil {
		return constants.ErrCLIL3CertNil
	}

	// Check certificate expiry
	if time.Now().After(cert.NotAfter) {
		return constants.ErrCLIL3CertExpired
	}
	if time.Now().Before(cert.NotBefore) {
		return constants.ErrCLIL3CertNotYetValid
	}

	// Verify SPIFFE URI SAN matches the expected CLI session
	wid := protocol.NewWorkloadIdentity()
	match := false
	for _, uri := range cert.URIs {
		if wid.MatchesCLI(uri.String(), userID, cliSessionID) {
			match = true
			break
		}
	}
	if !match {
		return constants.ErrCLIL3SPIFFESANMismatch
	}

	// Verify certificate validity via PKI authority (fail-closed)
	if pki == nil {
		return constants.ErrCLIL3PKINotConfigured
	}
	if err := pki.VerifyCertificate(cert); err != nil {
		return fmt.Errorf("cli cert: verify certificate: %w", err)
	}

	return nil
}

// ExtractUserIDFromCert extracts the user ID from a certificate's SPIFFE URI SAN.
func ExtractUserIDFromCert(cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", constants.ErrCLIL3CertNil
	}

	wid := protocol.NewWorkloadIdentity()
	for _, uri := range cert.URIs {
		if userID, ok := wid.ExtractUserID(uri.String()); ok {
			return userID, nil
		}
	}

	return "", constants.ErrCLIL3NoUserIDInCert
}
