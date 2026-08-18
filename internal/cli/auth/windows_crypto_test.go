// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows
// +build windows

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestImportCertificateToWindowsStore_InvalidPEM verifies that importing
// invalid PEM data returns an error. The full import path requires
// PowerShell and the .NET X509Store, so only the error path is unit-tested.
func TestImportCertificateToWindowsStore_InvalidPEM(t *testing.T) {
	err := ImportCertificateToWindowsStore("not-a-cert")
	assert.Error(t, err)
}
