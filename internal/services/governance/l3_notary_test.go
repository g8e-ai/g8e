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

package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
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
	cliVerifier := &mockCLISessionVerifier{result: ErrCLISessionDenied}
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

func TestGatewayL3Notary_NoPasskeyVerifierFallsBackToCLIPath(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-no-passkey-verifier"
	userID := "user-no-passkey-verifier"

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(privKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	store := &fakeSuspendedStore{}
	store.StoreSuspendedTransaction(context.Background(), setupApprovedTx(txHash, userID, pubKeyHex, signature))

	notary := NewCLIL3Notary(store, nil, slog.Default())

	// Proof without mtls_cert_fingerprint — without a passkey verifier, the CLI path
	// should handle it (and fail because cliVerifier is nil but fingerprint is required).
	webauthnProof := &commonv1.L3Proof{
		CredentialId:      "cred-id",
		ClientDataJson:    "client-data",
		AuthenticatorData: "auth-data",
		Signature:         "sig",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "cli-session", webauthnProof)
	require.Error(t, err)
	assert.False(t, allowed)
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

func TestCLINotary_InvokesCLISessionVerifier(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-cli-invokes-verifier"
	userID := "user-cli-invokes-verifier"

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(privKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	store := &trackingSuspendedStore{
		fakeSuspendedStore: &fakeSuspendedStore{},
	}
	store.StoreSuspendedTransaction(context.Background(), setupApprovedTx(txHash, userID, pubKeyHex, signature))

	cliVerifier := &mockCLISessionVerifier{result: nil}
	notary := NewCLIL3Notary(store, cliVerifier, slog.Default())

	proof := &commonv1.L3Proof{
		CliSignature:        signature,
		MtlsCertFingerprint: "abc123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "cli-session", proof)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.True(t, cliVerifier.called, "CLI session verifier must be invoked by cliNotary")
	assert.True(t, store.getCalled, "suspended store must be accessed by cliNotary")
}

func TestCLINotary_CLISessionDeniedReturnsFalse(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-cli-denied"
	userID := "user-cli-denied"

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(privKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	store := &trackingSuspendedStore{
		fakeSuspendedStore: &fakeSuspendedStore{},
	}
	store.StoreSuspendedTransaction(context.Background(), setupApprovedTx(txHash, userID, pubKeyHex, signature))

	cliVerifier := &mockCLISessionVerifier{result: ErrCLISessionDenied}
	notary := NewCLIL3Notary(store, cliVerifier, slog.Default())

	proof := &commonv1.L3Proof{
		CliSignature:        signature,
		MtlsCertFingerprint: "abc123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "cli-session", proof)
	require.NoError(t, err)
	assert.False(t, allowed, "denied CLI session should return false without error")
	assert.True(t, cliVerifier.called)
	assert.False(t, store.getCalled, "suspended store should not be accessed when CLI session is denied")
}

func TestCLINotary_CLISessionErrorPropagates(t *testing.T) {
	t.Parallel()

	txHash := "test-tx-hash-cli-error"
	userID := "user-cli-error"

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(privKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	store := &trackingSuspendedStore{
		fakeSuspendedStore: &fakeSuspendedStore{},
	}
	store.StoreSuspendedTransaction(context.Background(), setupApprovedTx(txHash, userID, pubKeyHex, signature))

	expectedErr := errors.New("system error")
	cliVerifier := &mockCLISessionVerifier{result: expectedErr}
	notary := NewCLIL3Notary(store, cliVerifier, slog.Default())

	proof := &commonv1.L3Proof{
		CliSignature:        signature,
		MtlsCertFingerprint: "abc123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, "cli-session", proof)
	require.Error(t, err)
	assert.False(t, allowed)
	assert.ErrorIs(t, err, expectedErr)
	assert.False(t, store.getCalled, "suspended store should not be accessed when CLI session check errors")
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

func TestDemoL3Notary_AutoApprovesNonNullProof(t *testing.T) {
	t.Parallel()

	notary := NewDemoL3Notary(slog.Default())
	proof := &commonv1.L3Proof{CliSignature: "any-sig"}

	allowed, err := notary.VerifyL3Proof(context.Background(), "demo-user", "demo-tx-hash", "demo-session", proof)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestDemoL3Notary_RejectsNilProof(t *testing.T) {
	t.Parallel()

	notary := NewDemoL3Notary(slog.Default())

	allowed, err := notary.VerifyL3Proof(context.Background(), "demo-user", "demo-tx-hash", "demo-session", nil)
	require.Error(t, err)
	assert.False(t, allowed)
	assert.True(t, errors.Is(err, constants.ErrGatewayL3ProofRequired))
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
