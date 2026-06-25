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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"github.com/stretchr/testify/require"
)

func newTestSuspendedStore(t *testing.T) *storage.SuspendedTransactionService {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_suspended.db")
	sts, err := storage.NewSuspendedTransactionService(&storage.SuspendedTransactionConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { sts.Close() })
	return sts
}

func TestCLIL3Notary_VerifyL3Proof_RejectsMissingInputs(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(db, logger)
	cliSessionSvc := NewCLISessionService(db, logger)
	cliVerifier := NewCLISessionVerifier(db, nil, logger, userSvc, cliSessionSvc)
	notary := governance.NewCLIL3Notary(nil, cliVerifier, logger)

	tests := []struct {
		name            string
		userID          string
		transactionHash string
		proof           *commonv1.L3Proof
		wantErr         string
	}{
		{
			name:            "missing user_id",
			userID:          "",
			transactionHash: "tx-hash",
			proof:           &commonv1.L3Proof{MtlsCertFingerprint: "abc123", CliSignature: "sig"},
			wantErr:         "user_id is required",
		},
		{
			name:            "missing transaction_hash",
			userID:          "user-1",
			transactionHash: "",
			proof:           &commonv1.L3Proof{MtlsCertFingerprint: "abc123", CliSignature: "sig"},
			wantErr:         "transaction_hash is required",
		},
		{
			name:            "missing proof",
			userID:          "user-1",
			transactionHash: "tx-hash",
			proof:           nil,
			wantErr:         "L3 proof is required",
		},
		{
			name:            "missing mtls_cert_fingerprint",
			userID:          "user-1",
			transactionHash: "tx-hash",
			proof:           &commonv1.L3Proof{CliSignature: "sig"},
			wantErr:         "mtls_cert_fingerprint is required",
		},
		{
			name:            "missing cli_signature",
			userID:          "user-1",
			transactionHash: "tx-hash",
			proof:           &commonv1.L3Proof{MtlsCertFingerprint: "abc123"},
			wantErr:         "cli_signature required",
		},
		{
			name:            "invalid fingerprint format",
			userID:          "user-1",
			transactionHash: "tx-hash",
			proof:           &commonv1.L3Proof{MtlsCertFingerprint: "not-hex!", CliSignature: "sig"},
			wantErr:         "invalid mtls_cert_fingerprint format",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ok, err := notary.VerifyL3Proof(context.Background(), tc.userID, tc.transactionHash, "", tc.proof)
			require.Error(t, err)
			require.False(t, ok)
		})
	}
}

func TestCLIL3Notary_VerifyL3Proof_RejectsInactiveUser(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(db, logger)
	cliSessionSvc := NewCLISessionService(db, logger)
	cliVerifier := NewCLISessionVerifier(db, nil, logger, userSvc, cliSessionSvc)
	notary := governance.NewCLIL3Notary(nil, cliVerifier, logger)

	// Create a disabled user
	userID := "disabled-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusDisabled,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "", &commonv1.L3Proof{MtlsCertFingerprint: validFingerprint, CliSignature: "sig"})
	require.Error(t, err)
	require.False(t, ok)
}

func TestCLIL3Notary_VerifyL3Proof_AcceptsActiveUser(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Create PKI (required for fail-closed revocation check)
	sm, _ := NewSecretManager(db.db, secretsDir, logger)
	pki := newPKIAuthority(dbDir, filepath.Join(dbDir, "pki"), db, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	cliSessionSvc := NewCLISessionService(db, logger)
	suspendedStore := newTestSuspendedStore(t)
	cliVerifier := NewCLISessionVerifier(db, pki, logger, userSvc, cliSessionSvc)
	notary := governance.NewCLIL3Notary(suspendedStore, cliVerifier, logger)

	// Create an active user
	userID := "active-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	// Generate Ed25519 key pair for signing
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	// Create a CLI session with a known fingerprint (no cert serial to avoid revocation lookup)
	cliSessionID := "cli-session-123"
	cliSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: "operator-session-123",
		CertFingerprint:   validFingerprint,
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(24 * time.Hour),
		AbsoluteExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		IdleExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       "csr",
	}
	cliSessionBytes, _ := json.Marshal(cliSession)
	require.NoError(t, db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes))

	// Store an approved suspended transaction with the public key
	sig := ed25519.Sign(privKey, []byte(txHash))
	approvedAt := time.Now().UTC()
	suspendedTx := &models.SuspendedTransaction{
		TransactionHash:         txHash,
		UserID:                  userID,
		Approved:                true,
		ApprovedAt:              &approvedAt,
		ApprovedBy:              userID,
		ApprovalSignature:       hex.EncodeToString(sig),
		ExpectedCertFingerprint: validFingerprint,
		ApprovalPublicKey:       hex.EncodeToString(pubKey),
	}
	err = suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)
	require.NoError(t, err)

	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, &commonv1.L3Proof{
		MtlsCertFingerprint: validFingerprint,
		CliSignature:        hex.EncodeToString(sig),
	})
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCLIL3Notary_VerifyL3Proof_RejectsInvalidSignature(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	sm, _ := NewSecretManager(db.db, secretsDir, logger)
	pki := newPKIAuthority(dbDir, filepath.Join(dbDir, "pki"), db, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	cliSessionSvc := NewCLISessionService(db, logger)
	suspendedStore := newTestSuspendedStore(t)
	cliVerifier := NewCLISessionVerifier(db, pki, logger, userSvc, cliSessionSvc)
	notary := governance.NewCLIL3Notary(suspendedStore, cliVerifier, logger)

	userID := "active-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	// Generate two key pairs: one for storing, one for signing (mismatch)
	storePubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, signPrivKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	cliSessionID := "cli-session-sig"
	cliSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: "operator-session-123",
		CertFingerprint:   validFingerprint,
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(24 * time.Hour),
		AbsoluteExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		IdleExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       "csr",
	}
	cliSessionBytes, _ := json.Marshal(cliSession)
	require.NoError(t, db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes))

	// Store approved transaction with a DIFFERENT public key than the signer
	badSig := ed25519.Sign(signPrivKey, []byte(txHash))
	approvedAt := time.Now().UTC()
	suspendedTx := &models.SuspendedTransaction{
		TransactionHash:         txHash,
		UserID:                  userID,
		Approved:                true,
		ApprovedAt:              &approvedAt,
		ApprovedBy:              userID,
		ApprovalSignature:       hex.EncodeToString(badSig),
		ExpectedCertFingerprint: validFingerprint,
		ApprovalPublicKey:       hex.EncodeToString(storePubKey),
	}
	err = suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)
	require.NoError(t, err)

	// Signature was made with a different key → ed25519.Verify should fail
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, &commonv1.L3Proof{
		MtlsCertFingerprint: validFingerprint,
		CliSignature:        hex.EncodeToString(badSig),
	})
	require.Error(t, err)
	require.False(t, ok)
}

func TestCLIL3Notary_VerifyL3Proof_RejectsUnknownFingerprint(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(db, logger)
	cliSessionSvc := NewCLISessionService(db, logger)
	cliVerifier := NewCLISessionVerifier(db, nil, logger, userSvc, cliSessionSvc)
	notary := governance.NewCLIL3Notary(nil, cliVerifier, logger)

	// Create an active user
	userID := "active-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	// No CLI session created - verification should fail
	unknownFingerprint := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "non-existent-session", &commonv1.L3Proof{MtlsCertFingerprint: unknownFingerprint, CliSignature: "sig"})
	require.Error(t, err)
	require.False(t, ok)
}

func TestCLIL3Notary_VerifyL3Proof_RejectsRevokedCertificate(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	sm, _ := NewSecretManager(db.db, secretsDir, logger)
	pki := newPKIAuthority(dbDir, filepath.Join(dbDir, "pki"), db, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	cliSessionSvc := NewCLISessionService(db, logger)
	cliVerifier := NewCLISessionVerifier(db, pki, logger, userSvc, cliSessionSvc)
	notary := governance.NewCLIL3Notary(nil, cliVerifier, logger)

	userID := "user-123"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	// Create a CLI session with a revoked certificate serial
	cliSessionID := "cli-session-revoked"
	cliSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: "operator-session-123",
		CertFingerprint:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CertSerial:        "1234567890abcdef",
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(24 * time.Hour),
		AbsoluteExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		IdleExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       "csr",
	}
	cliSessionBytes, _ := json.Marshal(cliSession)
	require.NoError(t, db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes))

	// Revoke the certificate
	err = pki.RevokeCertificate("1234567890abcdef", "test revocation")
	require.NoError(t, err)

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, &commonv1.L3Proof{MtlsCertFingerprint: validFingerprint, CliSignature: "sig"})
	require.NoError(t, err)
	require.False(t, ok)
}

func TestGatewayL3Notary_DelegatesToCLI(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(db, logger)
	cliSessionSvc := NewCLISessionService(db, logger)
	cliVerifier := NewCLISessionVerifier(db, nil, logger, userSvc, cliSessionSvc)
	notary := governance.NewGatewayL3Notary(nil, cliVerifier, nil, logger)

	// Create an active user
	userID := "active-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	// Create a CLI session with a known fingerprint
	cliSessionID := "cli-session-456"
	cliSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: "operator-session-456",
		CertFingerprint:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CertSerial:        "1234567890abcdef",
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(24 * time.Hour),
		AbsoluteExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		IdleExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       "csr",
	}
	cliSessionBytes, _ := json.Marshal(cliSession)
	require.NoError(t, db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes))

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, &commonv1.L3Proof{MtlsCertFingerprint: validFingerprint, CliSignature: "sig"})
	require.Error(t, err)
	require.False(t, ok)
}

func TestGatewayL3Notary_DelegatesToPasskey(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	passkeyL3, user := newPasskeyServiceForTest(t)
	notary := governance.NewGatewayL3Notary(nil, nil, passkeyL3, logger)

	// Use the user already created by newPasskeyServiceForTest
	userID := user.ID

	// Add a dummy credential
	credID := []byte("real-credential-id")
	require.NoError(t, passkeyL3.addCredential(userID, models.PasskeyCredential{
		ID:        credID,
		PublicKey: []byte("fake-pubkey"),
	}))

	// Create a WebAuthn proof (no mtls_cert_fingerprint)
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	clientData := `{"type":"webauthn.get","challenge":"` + base64.RawURLEncoding.EncodeToString([]byte(txHash)) + `","origin":"localhost"}`
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "", &commonv1.L3Proof{
		CredentialId:      base64.RawURLEncoding.EncodeToString(credID),
		ClientDataJson:    base64.RawURLEncoding.EncodeToString([]byte(clientData)),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 37))),
		Signature:         base64.RawURLEncoding.EncodeToString([]byte("signature")),
	})
	// This will fail signature verification but proves delegation to passkey verifier
	require.Error(t, err)
	require.False(t, ok)
}
