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

//go:build !windows
// +build !windows

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestGenerateWindowsCSR_NonWindowsReturnsErrWindowsSpecificEnrollment(t *testing.T) {
	_, _, err := GenerateWindowsCSR("test", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrWindowsSpecificEnrollment)
}

func TestImportCertificateToWindowsStore_NonWindowsReturnsErrWindowsCertStoreImport(t *testing.T) {
	err := ImportCertificateToWindowsStore("dummy")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrWindowsCertStoreImport)
}
