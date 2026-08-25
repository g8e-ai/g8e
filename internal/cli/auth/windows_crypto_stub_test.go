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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestImportCertificateToWindowsStore_NonWindowsReturnsErrWindowsCertStoreImport(t *testing.T) {
	err := ImportCertificateToWindowsStore("dummy")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrWindowsCertStoreImport)
}
