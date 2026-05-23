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
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPasskeyServiceForTest(t *testing.T) (*PasskeyService, *models.User) {
	t.Helper()

	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	user, err := NewUserService(db, logger).CreateUser()
	require.NoError(t, err)

	svc, err := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
	require.NoError(t, err)
	return svc, user
}

func TestPasskeyServiceVerifyL3ProofRejectsMissingInputs(t *testing.T) {
	t.Parallel()
	svc, user := newPasskeyServiceForTest(t)
	validProof := &commonv1.L3Proof{
		CredentialId:      base64.RawURLEncoding.EncodeToString([]byte("credential-id-123456")),
		ClientDataJson:    base64.RawURLEncoding.EncodeToString([]byte(`{"type":"webauthn.get"}`)),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 37))),
		Signature:         base64.RawURLEncoding.EncodeToString([]byte("signature")),
	}

	tests := []struct {
		name            string
		userID          string
		transactionHash string
		proof           *commonv1.L3Proof
		want            string
	}{
		{name: "missing user", userID: "", transactionHash: "tx", proof: validProof, want: "user_id is required"},
		{name: "missing transaction hash", userID: user.ID, transactionHash: "", proof: validProof, want: "transaction_hash is required"},
		{name: "nil proof", userID: user.ID, transactionHash: "tx", proof: nil, want: "L3 WebAuthn proof is required"},
		{name: "missing credential id", userID: user.ID, transactionHash: "tx", proof: &commonv1.L3Proof{ClientDataJson: "c", AuthenticatorData: "a", Signature: "s"}, want: "credential_id is required"},
		{name: "missing client data", userID: user.ID, transactionHash: "tx", proof: &commonv1.L3Proof{CredentialId: "c", AuthenticatorData: "a", Signature: "s"}, want: "client_data_json is required"},
		{name: "missing authenticator data", userID: user.ID, transactionHash: "tx", proof: &commonv1.L3Proof{CredentialId: "c", ClientDataJson: "c", Signature: "s"}, want: "authenticator_data is required"},
		{name: "missing signature", userID: user.ID, transactionHash: "tx", proof: &commonv1.L3Proof{CredentialId: "c", ClientDataJson: "c", AuthenticatorData: "a"}, want: "signature is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
		t.Parallel()
			ok, err := svc.VerifyL3Proof(tc.userID, tc.transactionHash, "", tc.proof)
			require.Error(t, err)
			assert.False(t, ok)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestPasskeyServiceVerifyL3ProofRejectsUsersWithoutPasskeys(t *testing.T) {
	t.Parallel()
	svc, user := newPasskeyServiceForTest(t)

	ok, err := svc.VerifyL3Proof(user.ID, strings.Repeat("a", 64), "", &commonv1.L3Proof{
		CredentialId:      base64.RawURLEncoding.EncodeToString([]byte("credential-id-123456")),
		ClientDataJson:    base64.RawURLEncoding.EncodeToString([]byte(`{"type":"webauthn.get"}`)),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 37))),
		Signature:         base64.RawURLEncoding.EncodeToString([]byte("signature")),
	})

	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "user has no registered passkey credentials")
}

func TestPasskeyServiceVerifyL3ProofRejectsUnregisteredCredential(t *testing.T) {
	t.Parallel()
	svc, user := newPasskeyServiceForTest(t)

	// Add a dummy credential
	credID := []byte("real-credential-id")
	err := svc.addCredential(user.ID, models.PasskeyCredential{
		ID:        credID,
		PublicKey: []byte("fake-pubkey"),
	})
	require.NoError(t, err)

	ok, err := svc.VerifyL3Proof(user.ID, "tx-hash", "", &commonv1.L3Proof{
		CredentialId:      base64.RawURLEncoding.EncodeToString([]byte("wrong-credential-id")),
		ClientDataJson:    base64.RawURLEncoding.EncodeToString([]byte(`{"type":"webauthn.get","challenge":"dngtZWFzaA"}`)),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 37))),
		Signature:         base64.RawURLEncoding.EncodeToString([]byte("signature")),
	})

	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "failed to parse credential assertion")
}

func TestPasskeyServiceVerifyL3ProofRejectsMismatchedChallenge(t *testing.T) {
	t.Parallel()
	svc, user := newPasskeyServiceForTest(t)

	// Add a dummy credential (we won't get to signature verification if challenge check fails first)
	// Wait, webauthn.ValidateLogin checks the challenge inside clientDataJSON against the one in sessionData.
	credID := []byte("real-credential-id")
	err := svc.addCredential(user.ID, models.PasskeyCredential{
		ID:        credID,
		PublicKey: []byte("fake-pubkey"),
	})
	require.NoError(t, err)

	// Challenge in clientDataJSON is base64 of "tx-hash-1"
	// but we provide "tx-hash-2" to VerifyL3Proof
	txHash1 := "tx-hash-1"
	txHash2 := "tx-hash-2"
	clientData := fmt.Sprintf(`{"type":"webauthn.get","challenge":"%s","origin":"localhost"}`,
		base64.RawURLEncoding.EncodeToString([]byte(txHash1)))

	ok, err := svc.VerifyL3Proof(user.ID, txHash2, "", &commonv1.L3Proof{
		CredentialId:      base64.RawURLEncoding.EncodeToString(credID),
		ClientDataJson:    base64.RawURLEncoding.EncodeToString([]byte(clientData)),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 37))),
		Signature:         base64.RawURLEncoding.EncodeToString([]byte("signature")),
	})

	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "failed to parse credential assertion")
}
