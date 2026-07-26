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
