// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"crypto/x509"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/protocol"
)

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
