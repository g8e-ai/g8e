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

//go:build integration

package gateway

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testValidCOSEKey() []byte {
	coseKey := map[int]any{
		1:  2,  // kty: EC2
		3:  -7, // alg: ES256
		-1: 1,  // crv: P-256
		-2: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}, // x
		-3: []byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40}, // y
	}
	data, _ := cbor.Marshal(coseKey)
	return data
}

func testCredential(id string) models.PasskeyCredential {
	return models.PasskeyCredential{
		ID:              []byte(id),
		PublicKey:       testValidCOSEKey(),
		AttestationType: "none",
		CreatedAtUnixMs: time.Now().UnixMilli(),
	}
}

func newPasskeyServiceForTest(t *testing.T) (*PasskeyService, *models.User) {
	t.Helper()

	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	user, err := NewUserService(db, logger).CreateUser()
	require.NoError(t, err)

	svc, err := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"}, nil, nil, 0)
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
			ok, err := svc.VerifyL3Proof(context.Background(), tc.userID, tc.transactionHash, "", tc.proof)
			require.Error(t, err)
			assert.False(t, ok)
		})
	}
}

func TestPasskeyServiceVerifyL3ProofRejectsUsersWithoutPasskeys(t *testing.T) {
	t.Parallel()
	svc, user := newPasskeyServiceForTest(t)

	ok, err := svc.VerifyL3Proof(context.Background(), user.ID, strings.Repeat("a", 64), "", &commonv1.L3Proof{
		CredentialId:      base64.RawURLEncoding.EncodeToString([]byte("credential-id-123456")),
		ClientDataJson:    base64.RawURLEncoding.EncodeToString([]byte(`{"type":"webauthn.get"}`)),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 37))),
		Signature:         base64.RawURLEncoding.EncodeToString([]byte("signature")),
	})

	require.Error(t, err)
	assert.False(t, ok)
}

func TestPasskeyServiceVerifyL3ProofRejectsUnregisteredCredential(t *testing.T) {
	t.Parallel()
	svc, user := newPasskeyServiceForTest(t)

	// Add a dummy credential
	err := svc.addCredential(user.ID, testCredential("real-credential-id"))
	require.NoError(t, err)

	ok, err := svc.VerifyL3Proof(context.Background(), user.ID, "tx-hash", "", &commonv1.L3Proof{
		CredentialId:      base64.RawURLEncoding.EncodeToString([]byte("wrong-credential-id")),
		ClientDataJson:    base64.RawURLEncoding.EncodeToString([]byte(`{"type":"webauthn.get","challenge":"dngtZWFzaA"}`)),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 37))),
		Signature:         base64.RawURLEncoding.EncodeToString([]byte("signature")),
	})

	require.Error(t, err)
	assert.False(t, ok)
}

func TestPasskeyServiceVerifyL3ProofRejectsMismatchedChallenge(t *testing.T) {
	t.Parallel()
	svc, user := newPasskeyServiceForTest(t)

	// Add a dummy credential (we won't get to signature verification if challenge check fails first)
	// Wait, webauthn.ValidateLogin checks the challenge inside clientDataJSON against the one in sessionData.
	credID := []byte("real-credential-id")
	err := svc.addCredential(user.ID, testCredential("real-credential-id"))
	require.NoError(t, err)

	// Challenge in clientDataJSON is base64 of "tx-hash-1"
	// but we provide "tx-hash-2" to VerifyL3Proof
	txHash1 := "tx-hash-1"
	txHash2 := "tx-hash-2"
	clientData := fmt.Sprintf(`{"type":"webauthn.get","challenge":"%s","origin":"localhost"}`,
		base64.RawURLEncoding.EncodeToString([]byte(txHash1)))

	ok, err := svc.VerifyL3Proof(context.Background(), user.ID, txHash2, "", &commonv1.L3Proof{
		CredentialId:      base64.RawURLEncoding.EncodeToString(credID),
		ClientDataJson:    base64.RawURLEncoding.EncodeToString([]byte(clientData)),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 37))),
		Signature:         base64.RawURLEncoding.EncodeToString([]byte("signature")),
	})

	require.Error(t, err)
	assert.False(t, ok)
}

func TestPasskeyService_GenerateRegistrationChallenge(t *testing.T) {
	t.Parallel()

	t.Run("Success - generates challenge for user", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		challenge, err := svc.GenerateRegistrationChallenge(user.ID, "test-user")
		require.NoError(t, err)
		require.NotNil(t, challenge)
		require.NotEmpty(t, challenge.Response.Challenge)
	})

	t.Run("Error - user not found", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		_, err := svc.GenerateRegistrationChallenge("non-existent-user", "test-user")
		require.Error(t, err)
	})
}

func TestPasskeyService_VerifyRegistration(t *testing.T) {
	t.Parallel()

	t.Run("Error - user not found", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		_, err := svc.VerifyRegistration("non-existent-user", []byte("{}"))
		require.Error(t, err)
	})

	t.Run("Error - session not found", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		_, err := svc.VerifyRegistration(user.ID, []byte("{}"))
		require.Error(t, err)
	})
}

func TestPasskeyService_GenerateAuthenticationChallenge(t *testing.T) {
	t.Parallel()

	t.Run("Success - generates challenge for user with credentials", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		// Add a credential
		err := svc.addCredential(user.ID, testCredential("cred-id"))
		require.NoError(t, err)

		challenge, err := svc.GenerateAuthenticationChallenge(user.ID)
		require.NoError(t, err)
		require.NotNil(t, challenge)
		require.NotEmpty(t, challenge.Response.Challenge)
	})

	t.Run("Error - user not found", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		_, err := svc.GenerateAuthenticationChallenge("non-existent-user")
		require.Error(t, err)
	})

	t.Run("Error - user has no passkeys", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		_, err := svc.GenerateAuthenticationChallenge(user.ID)
		require.Error(t, err)
	})
}

func TestPasskeyService_VerifyAuthentication(t *testing.T) {
	t.Parallel()

	t.Run("Error - user not found", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		_, err := svc.VerifyAuthentication("non-existent-user", []byte("{}"))
		require.Error(t, err)
	})

	t.Run("Error - session not found", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		_, err := svc.VerifyAuthentication(user.ID, []byte("{}"))
		require.Error(t, err)
	})
}

func TestPasskeyService_GenerateApprovalChallenge(t *testing.T) {
	t.Parallel()

	t.Run("Success - generates approval challenge", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		// Add a credential
		err := svc.addCredential(user.ID, testCredential("cred-id"))
		require.NoError(t, err)

		challenge, err := svc.GenerateApprovalChallenge(user.ID, "transaction-hash-123")
		require.NoError(t, err)
		require.NotNil(t, challenge)
		require.NotEmpty(t, challenge.Response.Challenge)
	})

	t.Run("Error - user not found", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		_, err := svc.GenerateApprovalChallenge("non-existent-user", "tx-hash")
		require.Error(t, err)
	})
}

func TestPasskeyService_ListCredentials(t *testing.T) {
	t.Parallel()

	t.Run("Success - lists credentials for user", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		// Add credentials
		err := svc.addCredential(user.ID, testCredential("cred-1"))
		require.NoError(t, err)

		err = svc.addCredential(user.ID, testCredential("cred-2"))
		require.NoError(t, err)

		creds, err := svc.listCredentials(user.ID)
		require.NoError(t, err)
		require.Len(t, creds, 2)
	})

	t.Run("Success - returns nil for non-existent user", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		creds, err := svc.listCredentials("non-existent-user")
		require.NoError(t, err)
		require.Nil(t, creds)
	})

	t.Run("Success - returns empty list for user with no credentials", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		creds, err := svc.listCredentials(user.ID)
		require.NoError(t, err)
		require.Empty(t, creds)
	})
}

func TestPasskeyService_RevokeCredential(t *testing.T) {
	t.Parallel()

	t.Run("Success - revokes credential", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		// Add credentials
		err := svc.addCredential(user.ID, testCredential("cred-1"))
		require.NoError(t, err)

		err = svc.addCredential(user.ID, testCredential("cred-2"))
		require.NoError(t, err)

		// Revoke one credential
		found, remaining, err := svc.revokeCredential(user.ID, base64.RawURLEncoding.EncodeToString([]byte("cred-1")))
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, 1, remaining)

		// Verify it was revoked
		creds, err := svc.listCredentials(user.ID)
		require.NoError(t, err)
		require.Len(t, creds, 1)
	})

	t.Run("Success - credential not found", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		found, remaining, err := svc.revokeCredential(user.ID, "non-existent-cred")
		require.NoError(t, err)
		require.False(t, found)
		require.Equal(t, 0, remaining)
	})

	t.Run("Success - returns nil for non-existent user", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		found, remaining, err := svc.revokeCredential("non-existent-user", "cred-id")
		require.NoError(t, err)
		require.False(t, found)
		require.Equal(t, 0, remaining)
	})
}

func TestPasskeyService_getUser(t *testing.T) {
	t.Parallel()

	t.Run("Success - retrieves user", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		retrieved, err := svc.getUser(user.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		require.Equal(t, user.ID, retrieved.ID)
	})

	t.Run("Success - returns nil for non-existent user", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		retrieved, err := svc.getUser("non-existent-user")
		require.NoError(t, err)
		require.Nil(t, retrieved)
	})
}

func TestPasskeyService_addCredential(t *testing.T) {
	t.Parallel()

	t.Run("Success - adds credential", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		err := svc.addCredential(user.ID, testCredential("cred-1"))
		require.NoError(t, err)

		creds, err := svc.listCredentials(user.ID)
		require.NoError(t, err)
		require.Len(t, creds, 1)
	})

	t.Run("Error - user not found", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		err := svc.addCredential("non-existent-user", testCredential("cred-1"))
		require.Error(t, err)
	})
}

func TestPasskeyService_setCredentials(t *testing.T) {
	t.Parallel()

	t.Run("Success - sets credentials", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		creds := []models.PasskeyCredential{
			testCredential("cred-1"),
			testCredential("cred-2"),
		}

		err := svc.setCredentials(user.ID, creds)
		require.NoError(t, err)

		retrieved, err := svc.listCredentials(user.ID)
		require.NoError(t, err)
		require.Len(t, retrieved, 2)
	})

	t.Run("Error - user not found", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		err := svc.setCredentials("non-existent-user", []models.PasskeyCredential{})
		require.Error(t, err)
	})
}

func TestPasskeyService_updateUser(t *testing.T) {
	t.Parallel()

	t.Run("Success - updates user", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		user.Status = "test-status"
		err := svc.updateUser(user.ID, user)
		require.NoError(t, err)
	})

}

// mockWebauthnClient implements webauthnClient for testing.
// It returns predictable challenges and credentials without real crypto.
type mockWebauthnClient struct {
	credential *webauthn.Credential
}

func (m *mockWebauthnClient) BeginRegistration(user webauthn.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	session := &webauthn.SessionData{Challenge: "mock-challenge", UserID: []byte(user.WebAuthnID())}
	return &protocol.CredentialCreation{}, session, nil
}

func (m *mockWebauthnClient) FinishRegistration(user webauthn.User, session webauthn.SessionData, r *http.Request) (*webauthn.Credential, error) {
	return m.credential, nil
}

func (m *mockWebauthnClient) BeginLogin(user webauthn.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	session := &webauthn.SessionData{Challenge: "mock-challenge", UserID: []byte(user.WebAuthnID())}
	return &protocol.CredentialAssertion{}, session, nil
}

func (m *mockWebauthnClient) FinishLogin(user webauthn.User, session webauthn.SessionData, r *http.Request) (*webauthn.Credential, error) {
	return m.credential, nil
}

func (m *mockWebauthnClient) ValidateLogin(user webauthn.User, session webauthn.SessionData, parsedResponse *protocol.ParsedCredentialAssertionData) (*webauthn.Credential, error) {
	return m.credential, nil
}

func newPasskeyServiceWithMock(t *testing.T) (*PasskeyService, *models.User) {
	t.Helper()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	user, err := NewUserService(db, logger).CreateUser()
	require.NoError(t, err)
	svc, err := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"}, nil, nil, 0)
	require.NoError(t, err)
	svc.webauthn = &mockWebauthnClient{
		credential: &webauthn.Credential{
			ID:              []byte("mock-cred-id"),
			PublicKey:       testValidCOSEKey(),
			AttestationType: "none",
		},
	}
	return svc, user
}

func TestPasskeyService_PurgeSessionAfterVerifyRegistration(t *testing.T) {
	t.Parallel()
	svc, user := newPasskeyServiceWithMock(t)

	_, err := svc.GenerateRegistrationChallenge(user.ID, "test-user")
	require.NoError(t, err)

	_, err = svc.getWebAuthnSession(user.ID)
	require.NoError(t, err)

	_, err = svc.VerifyRegistration(user.ID, []byte(`{"id":"mock","rawId":"mock","response":{"clientDataJSON":"{}","attestationObject":""}}`))
	require.NoError(t, err)

	_, err = svc.getWebAuthnSession(user.ID)
	require.ErrorIs(t, err, constants.ErrExpired)
}

func TestPasskeyService_PurgeSessionAfterVerifyAuthentication(t *testing.T) {
	t.Parallel()
	svc, user := newPasskeyServiceWithMock(t)

	err := svc.addCredential(user.ID, testCredential("mock-cred-id"))
	require.NoError(t, err)

	_, err = svc.GenerateAuthenticationChallenge(user.ID)
	require.NoError(t, err)

	_, err = svc.getWebAuthnSession(user.ID)
	require.NoError(t, err)

	_, err = svc.VerifyAuthentication(user.ID, []byte(`{}`))
	require.NoError(t, err)

	_, err = svc.getWebAuthnSession(user.ID)
	require.ErrorIs(t, err, constants.ErrExpired)
}

func TestPasskeyService_storeWebAuthnSession(t *testing.T) {
	t.Parallel()

	t.Run("Success - stores session", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		session := &webauthn.SessionData{
			Challenge: "test-challenge",
			UserID:    []byte(user.ID),
		}

		err := svc.storeWebAuthnSession(user.ID, session)
		require.NoError(t, err)
	})
}

func TestPasskeyService_getWebAuthnSession(t *testing.T) {
	t.Parallel()

	t.Run("Success - retrieves session", func(t *testing.T) {
		t.Parallel()
		svc, user := newPasskeyServiceForTest(t)

		session := &webauthn.SessionData{
			Challenge: "test-challenge",
			UserID:    []byte(user.ID),
		}

		err := svc.storeWebAuthnSession(user.ID, session)
		require.NoError(t, err)

		retrieved, err := svc.getWebAuthnSession(user.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		require.Equal(t, "test-challenge", retrieved.Challenge)
	})

	t.Run("Error - session not found", func(t *testing.T) {
		t.Parallel()
		svc, _ := newPasskeyServiceForTest(t)

		_, err := svc.getWebAuthnSession("non-existent-user")
		require.Error(t, err)
	})
}
