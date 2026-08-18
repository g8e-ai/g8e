// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyCAFingerprint_Match(t *testing.T) {
	t.Parallel()
	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")

	// Compute the actual fingerprint
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)
	hash := sha256.Sum256(block.Bytes)
	expectedFP := hex.EncodeToString(hash[:])

	// Test with hex fingerprint (no prefix)
	err := VerifyCAFingerprint([]byte(certPEM), expectedFP)
	require.NoError(t, err)
}

func TestVerifyCAFingerprint_Mismatch(t *testing.T) {
	t.Parallel()
	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")

	err := VerifyCAFingerprint([]byte(certPEM), "deadbeef")
	require.Error(t, err)
	assert.Error(t, err)
}

func TestVerifyCAFingerprint_EmptyPin(t *testing.T) {
	t.Parallel()
	certPEM, _ := testutil.GenerateTestCertificate(t, "test-cert")

	// Empty fingerprint should pass (no verification)
	err := VerifyCAFingerprint([]byte(certPEM), "")
	require.NoError(t, err)
}

func TestVerifyCAFingerprint_InvalidPEM(t *testing.T) {
	t.Parallel()
	err := VerifyCAFingerprint([]byte("not valid pem"), "deadbeef")
	require.Error(t, err)
	assert.Error(t, err)
}

func TestVerifyCAFingerprint_NonCertificatePEM(t *testing.T) {
	t.Parallel()
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: []byte("dummy"),
	})

	err := VerifyCAFingerprint(keyPEM, "deadbeef")
	require.Error(t, err)
	assert.Error(t, err)
}
