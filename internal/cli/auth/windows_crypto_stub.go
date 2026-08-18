// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !windows
// +build !windows

package auth

import (
	"github.com/g8e-ai/g8e/internal/constants"
)

// ImportCertificateToWindowsStore is a stub for non-Windows platforms.
func ImportCertificateToWindowsStore(certPEM string) error {
	return constants.ErrWindowsCertStoreImport
}
