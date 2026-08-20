// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestSignPlatformAppCSRCreatesNormalValidityDualSANIdentity(t *testing.T) {
	controller, _, _ := setupTestPKIController(t)
	csr := testutil.GenerateTestCSRP256(t, "requester-controlled-name")

	certPEM, chainPEM, err := controller.pki.SignPlatformAppCSR(csr, "g8ed", "owner-user")
	require.NoError(t, err)
	assert.NotEmpty(t, chainPEM)

	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.Len(t, cert.URIs, 2)
	assert.Equal(t, "spiffe://g8e.local/app/g8ed", cert.URIs[0].String())
	assert.Equal(t, "spiffe://g8e.local/user/owner-user", cert.URIs[1].String())
	assert.Empty(t, cert.DNSNames)
	assert.Empty(t, cert.IPAddresses)
	assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
	assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
	assert.Greater(t, cert.NotAfter.Sub(cert.NotBefore), 24*time.Hour)
}
