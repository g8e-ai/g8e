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
