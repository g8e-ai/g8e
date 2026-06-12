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

package gateway

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCompositeL3Verifier(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	verifier := NewCompositeL3Verifier(nil, nil, logger)
	require.NotNil(t, verifier)
	assert.Equal(t, logger, verifier.logger)
}

func TestCompositeL3Verifier_VerifyL3Proof_NilProof(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	verifier := NewCompositeL3Verifier(nil, nil, logger)

	_, err := verifier.VerifyL3Proof(context.Background(), "user-123", "tx-hash-456", "cli-session-789", nil)
	require.ErrorIs(t, err, ErrL3ProofRequired)
}

func TestCompositeL3Verifier_VerifyL3Proof_MTLSProof_NoCLIVerifier(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	verifier := NewCompositeL3Verifier(nil, nil, logger)

	proof := &commonv1.L3Proof{
		MtlsCertFingerprint: "cert-fp-123",
	}

	_, err := verifier.VerifyL3Proof(context.Background(), "user-123", "tx-hash-456", "cli-session-789", proof)
	require.ErrorIs(t, err, ErrCLIL3NotaryNotConfigured)
}

func TestCompositeL3Verifier_VerifyL3Proof_WebAuthnProof_NoPasskeyVerifier(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	verifier := NewCompositeL3Verifier(nil, nil, logger)

	proof := &commonv1.L3Proof{
		// No mtls_cert_fingerprint, so it should try passkey
		CredentialId: "cred-id-123",
	}

	_, err := verifier.VerifyL3Proof(context.Background(), "user-123", "tx-hash-456", "", proof)
	require.ErrorIs(t, err, ErrPasskeyL3NotaryNotConfigured)
}

func TestCompositeL3Verifier_VerifyL3Proof_MTLSProof_DelegatesToCLI(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	cliSessionSvc := NewCLISessionService(db, logger)
	cliL3 := NewCLIL3Notary(db, nil, logger, userSvc, cliSessionSvc)
	verifier := NewCompositeL3Verifier(nil, cliL3, logger)

	proof := &commonv1.L3Proof{
		MtlsCertFingerprint: "cert-fp-456",
	}

	// This will fail because we don't have a valid CLI session set up,
	// but it should delegate to the CLI verifier
	_, err := verifier.VerifyL3Proof(context.Background(), "user-123", "tx-hash-789", "cli-session-101", proof)
	require.Error(t, err)
	// The error should come from the CLI verifier, not about missing verifier
	assert.NotErrorIs(t, err, ErrCLIL3NotaryNotConfigured)
}

func TestCompositeL3Verifier_VerifyL3Proof_WebAuthnProof_DelegatesToPasskey(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{
		RpID:   "localhost",
		RpName: "g8e Test",
	})
	verifier := NewCompositeL3Verifier(passkey, nil, logger)

	proof := &commonv1.L3Proof{
		CredentialId: "cred-id-789",
	}

	// This will fail because we don't have a valid passkey set up,
	// but it should delegate to the passkey verifier
	_, err := verifier.VerifyL3Proof(context.Background(), "user-123", "tx-hash-101", "", proof)
	require.Error(t, err)
	// The error should come from the passkey verifier, not about missing verifier
	assert.NotErrorIs(t, err, ErrPasskeyL3NotaryNotConfigured)
}
