// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	storage "github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	"github.com/stretchr/testify/require"
)

func newTestSuspendedStore(t *testing.T) storage.SuspendedTransactionStore {
	t.Helper()
	dbPath := filepath.Join(testutil.TempDir(t), "test_suspended.db")
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

func TestGatewayL3Notary_CLIVerifier_RejectsInactiveUser(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(stores.DocStore, logger)
	cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
	cliVerifier := NewCLISessionVerifier(stores.DocStore, nil, logger, userSvc, cliSessionSvc)
	mockPasskey := testutil.NewConfigurableMockL3Notary(true)
	notary := governance.NewGatewayL3Notary(cliVerifier, mockPasskey, logger)

	// Create a disabled user
	userID := "disabled-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusDisabled,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "some-session", &commonv1.L3Proof{
		CredentialId:        "mock-credential",
		MtlsCertFingerprint: validFingerprint,
		CliSignature:        "sig",
	})
	require.Error(t, err)
	require.False(t, ok)
}

func TestOutboundL3Notary_VerifyL3Proof_AcceptsValidSignature(t *testing.T) {
	logger := testutil.NewTestLogger()
	suspendedStore := newTestSuspendedStore(t)

	// Generate Ed25519 key pair for signing
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	userID := "active-user"
	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

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
		ExpiresAt:               time.Now().UTC().Add(1 * time.Hour),
	}
	err = suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)
	require.NoError(t, err)

	notary := governance.NewOutboundL3Notary(suspendedStore, logger)
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "", &commonv1.L3Proof{
		MtlsCertFingerprint: validFingerprint,
		CliSignature:        hex.EncodeToString(sig),
	})
	require.NoError(t, err)
	require.True(t, ok)
}

func TestOutboundL3Notary_VerifyL3Proof_RejectsInvalidSignature(t *testing.T) {
	logger := testutil.NewTestLogger()
	suspendedStore := newTestSuspendedStore(t)

	// Generate two key pairs: one for storing, one for signing (mismatch)
	storePubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, signPrivKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	userID := "active-user"
	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

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
		ExpiresAt:               time.Now().UTC().Add(1 * time.Hour),
	}
	err = suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)
	require.NoError(t, err)

	notary := governance.NewOutboundL3Notary(suspendedStore, logger)
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "", &commonv1.L3Proof{
		MtlsCertFingerprint: validFingerprint,
		CliSignature:        hex.EncodeToString(badSig),
	})
	require.Error(t, err)
	require.False(t, ok)
}

func TestGatewayL3Notary_CLIVerifier_RejectsUnknownFingerprint(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(stores.DocStore, logger)
	cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
	cliVerifier := NewCLISessionVerifier(stores.DocStore, nil, logger, userSvc, cliSessionSvc)
	mockPasskey := testutil.NewConfigurableMockL3Notary(true)
	notary := governance.NewGatewayL3Notary(cliVerifier, mockPasskey, logger)

	// Create an active user
	userID := "active-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	// No CLI session created - verification should fail
	unknownFingerprint := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "non-existent-session", &commonv1.L3Proof{
		CredentialId:        "mock-credential",
		MtlsCertFingerprint: unknownFingerprint,
		CliSignature:        "sig",
	})
	require.Error(t, err)
	require.False(t, ok)
}

func TestGatewayL3Notary_CLIVerifier_RejectsRevokedCertificate(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	sm := newTestSecretManager(t, db.db, fileSvc)
	pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(stores.DocStore, logger)
	cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
	cliVerifier := NewCLISessionVerifier(stores.DocStore, pki, logger, userSvc, cliSessionSvc)
	mockPasskey := testutil.NewConfigurableMockL3Notary(true)
	notary := governance.NewGatewayL3Notary(cliVerifier, mockPasskey, logger)

	userID := "user-123"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

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
	require.NoError(t, stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes))

	// Revoke the certificate
	err = pki.RevokeCertificate("1234567890abcdef", "test revocation")
	require.NoError(t, err)

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, &commonv1.L3Proof{
		CredentialId:        "mock-credential",
		MtlsCertFingerprint: validFingerprint,
		CliSignature:        "sig",
	})
	require.NoError(t, err)
	require.False(t, ok)
}

func TestGatewayL3Notary_DelegatesToCLI(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(stores.DocStore, logger)
	cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
	cliVerifier := NewCLISessionVerifier(stores.DocStore, nil, logger, userSvc, cliSessionSvc)
	notary := governance.NewGatewayL3Notary(cliVerifier, nil, logger)

	// Create an active user
	userID := "active-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

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
	require.NoError(t, stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes))

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, &commonv1.L3Proof{MtlsCertFingerprint: validFingerprint, CliSignature: "sig"})
	require.Error(t, err)
	require.False(t, ok)
}

func TestGatewayL3Notary_DelegatesToPasskey(t *testing.T) {
	logger := testutil.NewTestLogger()
	passkeyL3, user := newPasskeyServiceForTest(t)
	notary := governance.NewGatewayL3Notary(nil, passkeyL3, logger)

	// Use the user already created by newPasskeyServiceForTest
	userID := user.ID

	// Add a dummy credential
	credID := []byte("real-credential-id")
	require.NoError(t, passkeyL3.addCredential(userID, testCredential("real-credential-id")))

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
