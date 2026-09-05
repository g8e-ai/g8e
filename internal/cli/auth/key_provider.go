// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"crypto/ecdsa"
)

// FileKeyProvider is the production KeyProvider implementation for the
// EnrollmentCoordinator. It generates exportable file-backed ECDSA P-256
// keys and CSRs on every platform. Windows Certificate Store import of the
// signed certificate is handled separately by ImportCertificateToWindowsStore
// (build-tag resolved) and is not part of key generation.
type FileKeyProvider struct{}

// GenerateCLIKeyAndCSR generates an ECDSA P-256 key pair and PEM-encoded
// CSR for the given common name. The returned key is file-backed
// (exportable PEM) on every platform.
func (FileKeyProvider) GenerateCLIKeyAndCSR(_ context.Context, commonName string) (string, *ecdsa.PrivateKey, error) {
	return GenerateCSR(commonName)
}

// Compile-time assertion that FileKeyProvider satisfies KeyProvider.
var _ KeyProvider = FileKeyProvider{}
