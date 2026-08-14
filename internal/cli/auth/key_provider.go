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

package auth

import (
	"context"
	"crypto/ecdsa"
)

// FileKeyProvider is the default KeyProvider implementation for the
// EnrollmentCoordinator. It generates file-backed ECDSA P-256 keys and
// CSRs on all platforms. The --tpm flag and the runtime.GOOS branch were
// removed in Section 7: every platform now uses exportable file-backed
// software keys. Windows Certificate Store import of the signed cert is
// handled separately by ImportCertificateToWindowsStore (build-tag
// resolved) and is NOT part of key generation.
type FileKeyProvider struct{}

// GenerateCLIKeyAndCSR generates an ECDSA P-256 key pair and PEM-encoded
// CSR for the given common name. The returned key is file-backed
// (exportable PEM) on every platform.
func (FileKeyProvider) GenerateCLIKeyAndCSR(_ context.Context, commonName string) (string, *ecdsa.PrivateKey, error) {
	return GenerateCSR(commonName)
}

// Compile-time assertion that FileKeyProvider satisfies KeyProvider.
var _ KeyProvider = FileKeyProvider{}
