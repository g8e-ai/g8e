// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testutil

import (
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateTestCSR(t *testing.T) {
	tests := []struct {
		name        string
		commonName  string
		wantPEMType string
	}{
		{
			name:        "basic CSR",
			commonName:  "test-operator",
			wantPEMType: "CERTIFICATE REQUEST",
		},
		{
			name:        "CSR with special characters",
			commonName:  "test-operator-123.example.com",
			wantPEMType: "CERTIFICATE REQUEST",
		},
		{
			name:        "CSR with underscores",
			commonName:  "test_operator_cluster",
			wantPEMType: "CERTIFICATE REQUEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			csrPEM := GenerateTestCSR(t, tt.commonName)

			// Verify PEM encoding
			block, _ := pem.Decode([]byte(csrPEM))
			require.NotNil(t, block, "PEM decoding failed")
			require.Equal(t, tt.wantPEMType, block.Type, "PEM block type mismatch")

			// Verify CSR structure
			csr, err := x509.ParseCertificateRequest(block.Bytes)
			require.NoError(t, err, "CSR parsing failed")
			require.Equal(t, tt.commonName, csr.Subject.CommonName, "CommonName mismatch")
		})
	}
}

func TestGenerateTestCSR_Deterministic(t *testing.T) {
	// Test that multiple calls with the same CN produce valid CSRs
	cn := "test-deterministic"
	csr1 := GenerateTestCSR(t, cn)
	csr2 := GenerateTestCSR(t, cn)

	// Both should be valid PEM
	block1, _ := pem.Decode([]byte(csr1))
	require.NotNil(t, block1)

	block2, _ := pem.Decode([]byte(csr2))
	require.NotNil(t, block2)

	// Both should parse as valid CSRs
	_, err := x509.ParseCertificateRequest(block1.Bytes)
	require.NoError(t, err)

	_, err = x509.ParseCertificateRequest(block2.Bytes)
	require.NoError(t, err)

	// CommonName should match in both
	csrParsed1, err := x509.ParseCertificateRequest(block1.Bytes)
	require.NoError(t, err)
	require.Equal(t, cn, csrParsed1.Subject.CommonName)

	csrParsed2, err := x509.ParseCertificateRequest(block2.Bytes)
	require.NoError(t, err)
	require.Equal(t, cn, csrParsed2.Subject.CommonName)
}

func TestGenerateTestCSR_PEMFormat(t *testing.T) {
	csrPEM := GenerateTestCSR(t, "test-pem-format")

	// Verify PEM header/footer format
	require.True(t, strings.HasPrefix(csrPEM, "-----BEGIN CERTIFICATE REQUEST-----"), "Missing PEM header")
	require.Contains(t, csrPEM, "-----END CERTIFICATE REQUEST-----", "Missing PEM footer")
	require.Contains(t, csrPEM, "\n", "PEM should contain newlines")
}
