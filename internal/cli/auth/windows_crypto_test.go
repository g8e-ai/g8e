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
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestGenerateWindowsCSR(t *testing.T) {
	tests := []struct {
		name       string
		commonName string
		useTPM     bool
	}{
		{name: "software-backed", commonName: "test-g8e-windows", useTPM: false},
		{name: "tpm-requested-fallback", commonName: "test-g8e-windows-tpm", useTPM: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			csr, privKey, err := GenerateWindowsCSR(tt.commonName, tt.useTPM)
			require.NoError(t, err)
			assert.NotEmpty(t, csr)
			assert.NotNil(t, privKey)
			assert.Contains(t, csr, "CERTIFICATE REQUEST")
		})
	}
}

func TestTrustRootCAInWindowsStore_InvalidPEM(t *testing.T) {
	err := TrustRootCAInWindowsStore("not a valid PEM")
	assert.ErrorIs(t, err, constants.ErrPEMDecodeFailed)
}

func TestTrustRootCAInWindowsStore_EmptyInput(t *testing.T) {
	err := TrustRootCAInWindowsStore("")
	assert.ErrorIs(t, err, constants.ErrPEMDecodeFailed)
}
