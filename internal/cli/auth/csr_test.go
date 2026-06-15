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
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCSR_Success(t *testing.T) {
	t.Parallel()
	csrPEM, privKey, err := GenerateCSR("test-common-name")

	require.NoError(t, err)
	require.NotNil(t, privKey)
	assert.NotEmpty(t, csrPEM)

	// Verify it's a valid PEM block
	block, _ := pem.Decode([]byte(csrPEM))
	require.NotNil(t, block)
	assert.Equal(t, "CERTIFICATE REQUEST", block.Type)

	// Verify it's a valid CSR
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, "test-common-name", csr.Subject.CommonName)
	assert.Equal(t, []string{"g8e"}, csr.Subject.Organization)
}

func TestGenerateCSR_DifferentCommonNames(t *testing.T) {
	t.Parallel()
	testCases := []string{
		"operator-1",
		"cli-device",
		"test-node.example.com",
	}

	for _, cn := range testCases {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			csrPEM, privKey, err := GenerateCSR(cn)

			require.NoError(t, err)
			require.NotNil(t, privKey)
			assert.NotEmpty(t, csrPEM)

			block, _ := pem.Decode([]byte(csrPEM))
			require.NotNil(t, block)
			csr, err := x509.ParseCertificateRequest(block.Bytes)
			require.NoError(t, err)
			assert.Equal(t, cn, csr.Subject.CommonName)
		})
	}
}
