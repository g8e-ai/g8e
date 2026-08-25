// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSuspendedStore is an in-memory implementation of storage.SuspendedTransactionStore
// for Tier 1 unit testing without SQLite.
type fakeSuspendedStore struct {
	txs map[string]*models.SuspendedTransaction
}

func (f *fakeSuspendedStore) StoreSuspendedTransaction(_ context.Context, tx *models.SuspendedTransaction) error {
	if f.txs == nil {
		f.txs = make(map[string]*models.SuspendedTransaction)
	}
	f.txs[tx.TransactionHash] = tx
	return nil
}

func (f *fakeSuspendedStore) GetSuspendedTransaction(_ context.Context, txHash string) (*models.SuspendedTransaction, bool, error) {
	tx, ok := f.txs[txHash]
	return tx, ok, nil
}

func (f *fakeSuspendedStore) ListSuspendedTransactions(_ context.Context, userID string) ([]*models.SuspendedTransaction, error) {
	var result []*models.SuspendedTransaction
	for _, tx := range f.txs {
		if userID == "" || tx.UserID == userID {
			result = append(result, tx)
		}
	}
	return result, nil
}

func (f *fakeSuspendedStore) ApproveSuspendedTransaction(_ context.Context, txHash string, proof models.ApprovalProof) error {
	if tx, ok := f.txs[txHash]; ok {
		tx.Approved = true
		tx.ApprovedBy = proof.ApprovedBy
		tx.ApprovalSignature = proof.CliSignature
		tx.ExpectedCertFingerprint = proof.CertFingerprint
		tx.ApprovalPublicKey = proof.ApprovalPublicKey
		tx.PasskeyCredentialID = proof.CredentialID
		tx.PasskeyClientDataJSON = proof.ClientDataJSON
		tx.PasskeyAuthenticatorData = proof.AuthenticatorData
		tx.PasskeySignature = proof.Signature
		now := time.Now().UTC()
		tx.ApprovedAt = &now
	}
	return nil
}

func (f *fakeSuspendedStore) DeleteSuspendedTransaction(_ context.Context, txHash string) error {
	delete(f.txs, txHash)
	return nil
}

func (f *fakeSuspendedStore) CleanupExpiredSuspendedTransactions(_ context.Context) (int64, error) {
	var count int64
	for hash, tx := range f.txs {
		if tx.ExpiresAt.Before(time.Now()) {
			delete(f.txs, hash)
			count++
		}
	}
	return count, nil
}

func (f *fakeSuspendedStore) GetExpiredSuspendedTransactions(_ context.Context) ([]*models.SuspendedTransaction, error) {
	var expired []*models.SuspendedTransaction
	for _, tx := range f.txs {
		if tx.ExpiresAt.Before(time.Now()) {
			expired = append(expired, tx)
		}
	}
	return expired, nil
}

func setupApprovedTx(txHash, userID string, pubKeyHex, signature string) *models.SuspendedTransaction {
	now := time.Now().UTC()
	return &models.SuspendedTransaction{
		TransactionHash:   txHash,
		Envelope:          []byte("{}"),
		CreatedAt:         now,
		ExpiresAt:         now.Add(1 * time.Hour),
		ToolName:          "test_tool",
		UserID:            userID,
		OperatorID:        "op-123",
		Approved:          true,
		ApprovedAt:        &now,
		ApprovedBy:        "approver-123",
		ApprovalSignature: signature,
		ApprovalPublicKey: pubKeyHex,
	}
}

func TestVerifyL3Proof_ValidSignature(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-valid"
	userID := "user-valid"

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(privKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	store := &fakeSuspendedStore{}
	store.StoreSuspendedTransaction(context.Background(), setupApprovedTx(txHash, userID, pubKeyHex, signature))

	notary := NewOutboundL3Notary(store, slog.Default())
	proof := &commonv1.L3Proof{CliSignature: signature}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "cli-session", proof)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestVerifyL3Proof_SignatureFromWrongKey(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-wrong-key"
	userID := "user-wrong-key"

	_, correctPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	correctPubKeyHex := hex.EncodeToString(correctPrivKey.Public().(ed25519.PublicKey))

	_, wrongPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	wrongSig := hex.EncodeToString(ed25519.Sign(wrongPrivKey, []byte(txHash)))

	store := &fakeSuspendedStore{}
	// Store the correct public key but present a signature from a different key.
	// The stored approval signature matches the wrong sig so the equality check passes,
	// but ed25519.Verify must fail.
	store.StoreSuspendedTransaction(context.Background(), setupApprovedTx(txHash, userID, correctPubKeyHex, wrongSig))

	notary := NewOutboundL3Notary(store, slog.Default())
	proof := &commonv1.L3Proof{CliSignature: wrongSig}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "cli-session", proof)
	require.Error(t, err)
	assert.False(t, allowed)
	assert.True(t, errors.Is(err, constants.ErrCLIL3SignatureVerificationFailed))
}

func TestVerifyL3Proof_TamperedTransactionHash(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-tampered"
	userID := "user-tampered"

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	// Sign the original hash
	signature := hex.EncodeToString(ed25519.Sign(privKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	store := &fakeSuspendedStore{}
	store.StoreSuspendedTransaction(context.Background(), setupApprovedTx(txHash, userID, pubKeyHex, signature))

	notary := NewOutboundL3Notary(store, slog.Default())
	// Present proof with a different transaction hash than what was signed
	proof := &commonv1.L3Proof{CliSignature: signature}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, "different-hash", "cli-session", proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestVerifyL3Proof_MissingPublicKeyInStore(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-no-pubkey"
	userID := "user-no-pubkey"

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(privKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	store := &fakeSuspendedStore{}
	tx := setupApprovedTx(txHash, userID, pubKeyHex, signature)
	tx.ApprovalPublicKey = "" // simulate missing public key
	store.StoreSuspendedTransaction(context.Background(), tx)

	notary := NewOutboundL3Notary(store, slog.Default())
	proof := &commonv1.L3Proof{CliSignature: signature}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "cli-session", proof)
	require.Error(t, err)
	assert.False(t, allowed)
	assert.True(t, errors.Is(err, constants.ErrCLIL3PublicKeyMissing))
}

func TestVerifyL3Proof_InvalidPublicKeyEncoding(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-bad-pubkey"
	userID := "user-bad-pubkey"

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(privKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	store := &fakeSuspendedStore{}
	tx := setupApprovedTx(txHash, userID, pubKeyHex, signature)
	tx.ApprovalPublicKey = "not-valid-hex!!"
	store.StoreSuspendedTransaction(context.Background(), tx)

	notary := NewOutboundL3Notary(store, slog.Default())
	proof := &commonv1.L3Proof{CliSignature: signature}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "cli-session", proof)
	require.Error(t, err)
	assert.False(t, allowed)
	assert.True(t, errors.Is(err, constants.ErrCLIL3PublicKeyInvalid))
}

// mockL3Notary is a test double for the L3Notary interface, used to verify
// the dispatch logic in NewGatewayL3Notary.
type mockL3Notary struct {
	called       bool
	calledUserID string
	calledTxHash string
	calledProof  *commonv1.L3Proof
	result       bool
	err          error
}

func (m *mockL3Notary) VerifyL3Proof(_ context.Context, userID, transactionHash, _ string, proof *commonv1.L3Proof) (bool, error) {
	m.called = true
	m.calledUserID = userID
	m.calledTxHash = transactionHash
	m.calledProof = proof
	return m.result, m.err
}

// VerifyPasskeyProof delegates to VerifyL3Proof so mockL3Notary also satisfies
// PasskeyVerifier for tests that wire it as the passkey delegate of
// NewGatewayL3Notary.
func (m *mockL3Notary) VerifyPasskeyProof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return m.VerifyL3Proof(ctx, userID, transactionHash, cliSessionID, proof)
}

func TestGatewayL3Notary_DispatchesToPasskeyVerifierForWebAuthnProofs(t *testing.T) {
	t.Parallel()

	passkeyMock := &mockL3Notary{result: true}
	notary := NewGatewayL3Notary(nil, passkeyMock, slog.Default())

	webauthnProof := &commonv1.L3Proof{
		CredentialId:      "cred-id",
		ClientDataJson:    "client-data",
		AuthenticatorData: "auth-data",
		Signature:         "sig",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), "user-1", "tx-hash-1", "session-1", webauthnProof)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.True(t, passkeyMock.called)
	assert.Equal(t, "user-1", passkeyMock.calledUserID)
	assert.Equal(t, "tx-hash-1", passkeyMock.calledTxHash)
	assert.Equal(t, webauthnProof, passkeyMock.calledProof)
}

func TestGatewayL3Notary_RejectsMTLSOnlyProofWhenPasskeyRequired(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-mtls-only"
	userID := "user-mtls-only"

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(privKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	store := &fakeSuspendedStore{}
	store.StoreSuspendedTransaction(context.Background(), setupApprovedTx(txHash, userID, pubKeyHex, signature))

	passkeyMock := &mockL3Notary{result: false, err: errors.New("should not be called")}
	notary := NewGatewayL3Notary(nil, passkeyMock, slog.Default())

	mtlsProof := &commonv1.L3Proof{
		CliSignature:        signature,
		MtlsCertFingerprint: "abc123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "cli-session", mtlsProof)
	require.Error(t, err)
	assert.False(t, allowed)
	assert.True(t, errors.Is(err, constants.ErrPasskeyProofRequired))
	assert.False(t, passkeyMock.called)
}

func TestGatewayL3Notary_DualLayerPasskeyAndMTLS(t *testing.T) {
	t.Parallel()

	passkeyMock := &mockL3Notary{result: true}
	cliVerifier := &mockCLISessionVerifier{result: nil}
	notary := NewGatewayL3Notary(cliVerifier, passkeyMock, slog.Default())

	dualProof := &commonv1.L3Proof{
		CredentialId:        "cred-id",
		ClientDataJson:      "client-data",
		AuthenticatorData:   "auth-data",
		Signature:           "passkey-sig",
		CliSignature:        "cli-sig",
		MtlsCertFingerprint: "abc123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), "user-1", "tx-hash-1", "cli-session-1", dualProof)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.True(t, passkeyMock.called)
	assert.True(t, cliVerifier.called)
	assert.Equal(t, "user-1", cliVerifier.calledUserID)
	assert.Equal(t, "cli-session-1", cliVerifier.calledSessionID)
	assert.Equal(t, "abc123", cliVerifier.calledFingerprint)
}

func TestGatewayL3Notary_DualLayerCLISessionDeniedReturnsFalse(t *testing.T) {
	t.Parallel()

	passkeyMock := &mockL3Notary{result: true}
	cliVerifier := &mockCLISessionVerifier{result: constants.ErrCLISessionDenied}
	notary := NewGatewayL3Notary(cliVerifier, passkeyMock, slog.Default())

	dualProof := &commonv1.L3Proof{
		CredentialId:        "cred-id",
		ClientDataJson:      "client-data",
		AuthenticatorData:   "auth-data",
		Signature:           "passkey-sig",
		MtlsCertFingerprint: "abc123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), "user-1", "tx-hash-1", "cli-session-1", dualProof)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.True(t, passkeyMock.called)
	assert.True(t, cliVerifier.called)
}

func TestGatewayL3Notary_PasskeyVerifierErrorPropagates(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("passkey verification failed")
	passkeyMock := &mockL3Notary{result: false, err: expectedErr}
	notary := NewGatewayL3Notary(nil, passkeyMock, slog.Default())

	webauthnProof := &commonv1.L3Proof{
		CredentialId:      "cred-id",
		ClientDataJson:    "client-data",
		AuthenticatorData: "auth-data",
		Signature:         "sig",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), "user-1", "tx-hash-1", "session-1", webauthnProof)
	assert.False(t, allowed)
	assert.ErrorIs(t, err, expectedErr)
	assert.True(t, passkeyMock.called)
}

func TestGatewayL3Notary_NilProofReturnsError(t *testing.T) {
	t.Parallel()

	passkeyMock := &mockL3Notary{}
	notary := NewGatewayL3Notary(nil, passkeyMock, slog.Default())

	allowed, err := notary.VerifyL3Proof(context.Background(), "user-1", "tx-hash-1", "session-1", nil)
	require.Error(t, err)
	assert.False(t, allowed)
	assert.False(t, passkeyMock.called)
	assert.True(t, errors.Is(err, constants.ErrGatewayL3ProofRequired))
}

// mockCLISessionVerifier is a test double for the CLISessionVerifier interface.
type mockCLISessionVerifier struct {
	called            bool
	calledUserID      string
	calledSessionID   string
	calledFingerprint string
	result            error
}

func (m *mockCLISessionVerifier) VerifyCLISession(userID, cliSessionID, certFingerprint string) error {
	m.called = true
	m.calledUserID = userID
	m.calledSessionID = cliSessionID
	m.calledFingerprint = certFingerprint
	return m.result
}

// trackingSuspendedStore wraps fakeSuspendedStore and tracks whether
// GetSuspendedTransaction was called, for composition matrix tests.
type trackingSuspendedStore struct {
	*fakeSuspendedStore
	getCalled bool
}

func (t *trackingSuspendedStore) GetSuspendedTransaction(ctx context.Context, txHash string) (*models.SuspendedTransaction, bool, error) {
	t.getCalled = true
	return t.fakeSuspendedStore.GetSuspendedTransaction(ctx, txHash)
}

func TestCompositionMatrix_OutboundNotary(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-outbound-matrix"
	userID := "user-outbound-matrix"

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(privKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	store := &trackingSuspendedStore{
		fakeSuspendedStore: &fakeSuspendedStore{},
	}
	store.StoreSuspendedTransaction(context.Background(), setupApprovedTx(txHash, userID, pubKeyHex, signature))

	notary := NewOutboundL3Notary(store, slog.Default())

	proof := &commonv1.L3Proof{
		CliSignature:        signature,
		MtlsCertFingerprint: "abc123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "cli-session", proof)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.True(t, store.getCalled, "outboundNotary must access suspended store")
}

func TestCompositionMatrix_GatewayNotary_DoesNotAccessSuspendedStore(t *testing.T) {
	t.Parallel()

	passkeyMock := &mockL3Notary{result: true}
	cliVerifier := &mockCLISessionVerifier{result: nil}
	notary := NewGatewayL3Notary(cliVerifier, passkeyMock, slog.Default())

	proof := &commonv1.L3Proof{
		CredentialId:        "cred-id",
		ClientDataJson:      "client-data",
		AuthenticatorData:   "auth-data",
		Signature:           "sig",
		MtlsCertFingerprint: "abc123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), "user-1", "tx-hash-1", "cli-session-1", proof)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.True(t, passkeyMock.called, "gatewayNotary must invoke passkey verifier")
	assert.True(t, cliVerifier.called, "gatewayNotary must invoke CLI session verifier when mTLS fingerprint present")
}

func TestCompositionMatrix_GatewayNotary_NoMTLSFingerprintSkipsCLIVerifier(t *testing.T) {
	t.Parallel()

	passkeyMock := &mockL3Notary{result: true}
	cliVerifier := &mockCLISessionVerifier{result: nil}
	notary := NewGatewayL3Notary(cliVerifier, passkeyMock, slog.Default())

	proof := &commonv1.L3Proof{
		CredentialId:      "cred-id",
		ClientDataJson:    "client-data",
		AuthenticatorData: "auth-data",
		Signature:         "sig",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), "user-1", "tx-hash-1", "session-1", proof)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.True(t, passkeyMock.called, "gatewayNotary must invoke passkey verifier")
	assert.False(t, cliVerifier.called, "gatewayNotary must NOT invoke CLI session verifier without mTLS fingerprint")
}

// mockPasskeyVerifier is a test double for the PasskeyVerifier interface,
// distinct from mockL3Notary, used to verify the passkeyL3Notary adapter
// delegates correctly.
type mockPasskeyVerifier struct {
	called       bool
	calledUserID string
	calledTxHash string
	result       bool
	err          error
}

func (m *mockPasskeyVerifier) VerifyPasskeyProof(_ context.Context, userID, transactionHash, _ string, _ *commonv1.L3Proof) (bool, error) {
	m.called = true
	m.calledUserID = userID
	m.calledTxHash = transactionHash
	return m.result, m.err
}

func TestPasskeyL3Notary_DelegatesToPasskeyVerifier(t *testing.T) {
	t.Parallel()

	verifier := &mockPasskeyVerifier{result: true}
	notary := NewGatewayL3Notary(nil, verifier, slog.Default())

	proof := &commonv1.L3Proof{
		CredentialId:      "cred-id",
		ClientDataJson:    "client-data",
		AuthenticatorData: "auth-data",
		Signature:         "sig",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), "user-1", "tx-hash-1", "session-1", proof)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.True(t, verifier.called, "passkeyL3Notary must delegate to the PasskeyVerifier")
	assert.Equal(t, "user-1", verifier.calledUserID)
	assert.Equal(t, "tx-hash-1", verifier.calledTxHash)
}

func TestPasskeyL3Notary_PropagatesVerifierError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("passkey verification failed")
	verifier := &mockPasskeyVerifier{result: false, err: expectedErr}
	notary := NewGatewayL3Notary(nil, verifier, slog.Default())

	proof := &commonv1.L3Proof{
		CredentialId:      "cred-id",
		ClientDataJson:    "client-data",
		AuthenticatorData: "auth-data",
		Signature:         "sig",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), "user-1", "tx-hash-1", "session-1", proof)
	assert.False(t, allowed)
	assert.ErrorIs(t, err, expectedErr)
	assert.True(t, verifier.called)
}
