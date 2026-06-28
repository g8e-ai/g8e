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

package governance

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/storage"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestL3ApprovalPipeline_CLI_Browser_Passkey verifies the full CLI → browser →
// approval → L3 verification pipeline using the layered authorization model:
//
//  1. CLI receives a suspended transaction (stored in suspended store)
//  2. Browser completes WebAuthn ceremony — approval recorded with passkey fields
//  3. CLI polls status endpoint and detects approval
//  4. L3 notary verifies the proof using the layered model (passkey Layer 1 + mTLS Layer 2)
func TestL3ApprovalPipeline_CLI_Browser_Passkey(t *testing.T) {
	logger := slog.Default()
	tmpDir := t.TempDir()

	storeCfg := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "suspended.db"),
	}
	store, err := storage.NewSuspendedTransactionService(storeCfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	userID := "pipeline-user-001"
	cliSessionID := "pipeline-cli-session-001"
	txHash := "pipeline-tx-hash-abc123"
	certFingerprint := "pipeline-cert-fingerprint-xyz"

	// Step 1: CLI receives a suspended transaction
	tx := &models.SuspendedTransaction{
		TransactionHash: txHash,
		Envelope:        []byte(`{"tool":"test_tool"}`),
		CreatedAt:       time.Now().UTC(),
		ExpiresAt:       time.Now().UTC().Add(1 * time.Hour),
		ToolName:        "test_tool",
		ToolArguments:   []byte(`{}`),
		UserID:          userID,
		OperatorID:      "op-001",
		Approved:        false,
	}
	err = store.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	// Verify transaction is initially unapproved
	loaded, ok, err := store.GetSuspendedTransaction(ctx, txHash)
	require.NoError(t, err)
	require.True(t, ok)
	assert.False(t, loaded.Approved)

	// Step 2: Browser completes WebAuthn ceremony — record approval with passkey fields
	approvalProof := models.ApprovalProof{
		ApprovedBy:        userID,
		CredentialID:      "base64-credential-id",
		ClientDataJSON:    "base64-client-data",
		AuthenticatorData: "base64-authenticator-data",
		Signature:         "base64-passkey-signature",
		CertFingerprint:   certFingerprint,
	}
	err = store.ApproveSuspendedTransaction(ctx, txHash, approvalProof)
	require.NoError(t, err)

	// Step 3: CLI polls status — transaction is now approved
	loaded, ok, err = store.GetSuspendedTransaction(ctx, txHash)
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, loaded.Approved)
	assert.NotNil(t, loaded.ApprovedAt)
	assert.Equal(t, userID, loaded.ApprovedBy)
	assert.Equal(t, "base64-credential-id", loaded.PasskeyCredentialID)
	assert.Equal(t, certFingerprint, loaded.ExpectedCertFingerprint)

	// Step 4: L3 notary verifies the proof using the layered model
	// Passkey verifier (mock) validates the WebAuthn assertion — Layer 1
	// CLI session verifier (mock) validates mTLS transport — Layer 2
	passkeyVerifier := &mockL3Notary{result: true}
	cliVerifier := &mockCLISessionVerifier{result: nil}
	notary := NewGatewayL3Notary(store, cliVerifier, passkeyVerifier, logger)

	l3Proof := &commonv1.L3Proof{
		CredentialId:        "base64-credential-id",
		ClientDataJson:      "base64-client-data",
		AuthenticatorData:   "base64-authenticator-data",
		Signature:           "base64-passkey-signature",
		MtlsCertFingerprint: certFingerprint,
	}

	allowed, err := notary.VerifyL3Proof(ctx, userID, txHash, cliSessionID, l3Proof)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.True(t, passkeyVerifier.called)
	assert.True(t, cliVerifier.called)
	assert.Equal(t, userID, cliVerifier.calledUserID)
	assert.Equal(t, cliSessionID, cliVerifier.calledSessionID)
	assert.Equal(t, certFingerprint, cliVerifier.calledFingerprint)
}

// TestL3ApprovalPipeline_CLI_Browser_PasskeyDenied verifies that the pipeline
// rejects the L3 proof when the passkey verifier denies the WebAuthn assertion.
func TestL3ApprovalPipeline_CLI_Browser_PasskeyDenied(t *testing.T) {
	logger := slog.Default()
	tmpDir := t.TempDir()

	storeCfg := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "suspended.db"),
	}
	store, err := storage.NewSuspendedTransactionService(storeCfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	userID := "pipeline-user-002"
	cliSessionID := "pipeline-cli-session-002"
	txHash := "pipeline-tx-hash-denied"

	tx := &models.SuspendedTransaction{
		TransactionHash: txHash,
		Envelope:        []byte(`{"tool":"test_tool"}`),
		CreatedAt:       time.Now().UTC(),
		ExpiresAt:       time.Now().UTC().Add(1 * time.Hour),
		ToolName:        "test_tool",
		ToolArguments:   []byte(`{}`),
		UserID:          userID,
		OperatorID:      "op-002",
		Approved:        false,
	}
	err = store.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	approvalProof := models.ApprovalProof{
		ApprovedBy:        userID,
		CredentialID:      "base64-credential-id",
		ClientDataJSON:    "base64-client-data",
		AuthenticatorData: "base64-authenticator-data",
		Signature:         "base64-passkey-signature",
	}
	err = store.ApproveSuspendedTransaction(ctx, txHash, approvalProof)
	require.NoError(t, err)

	// Passkey verifier denies the assertion
	passkeyVerifier := &mockL3Notary{result: false}
	cliVerifier := &mockCLISessionVerifier{result: nil}
	notary := NewGatewayL3Notary(store, cliVerifier, passkeyVerifier, logger)

	l3Proof := &commonv1.L3Proof{
		CredentialId:        "base64-credential-id",
		ClientDataJson:      "base64-client-data",
		AuthenticatorData:   "base64-authenticator-data",
		Signature:           "base64-passkey-signature",
		MtlsCertFingerprint: "fingerprint-xyz",
	}

	allowed, err := notary.VerifyL3Proof(ctx, userID, txHash, cliSessionID, l3Proof)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.True(t, passkeyVerifier.called)
	assert.False(t, cliVerifier.called, "CLI verifier should not be called when passkey fails")
}

// TestL3ApprovalPipeline_BrowserOnly_NoMTLS verifies the browser-only flow
// where no mTLS certificate fingerprint is present (pure web session approval).
func TestL3ApprovalPipeline_BrowserOnly_NoMTLS(t *testing.T) {
	logger := slog.Default()
	tmpDir := t.TempDir()

	storeCfg := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "suspended.db"),
	}
	store, err := storage.NewSuspendedTransactionService(storeCfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	userID := "pipeline-user-003"
	txHash := "pipeline-tx-hash-browser-only"

	tx := &models.SuspendedTransaction{
		TransactionHash: txHash,
		Envelope:        []byte(`{"tool":"test_tool"}`),
		CreatedAt:       time.Now().UTC(),
		ExpiresAt:       time.Now().UTC().Add(1 * time.Hour),
		ToolName:        "test_tool",
		ToolArguments:   []byte(`{}`),
		UserID:          userID,
		OperatorID:      "op-003",
		Approved:        false,
	}
	err = store.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	approvalProof := models.ApprovalProof{
		ApprovedBy:        userID,
		CredentialID:      "browser-credential-id",
		ClientDataJSON:    "browser-client-data",
		AuthenticatorData: "browser-authenticator-data",
		Signature:         "browser-passkey-signature",
	}
	err = store.ApproveSuspendedTransaction(ctx, txHash, approvalProof)
	require.NoError(t, err)

	// Browser-only proof: no mTLS fingerprint, so CLI verifier should not be called
	passkeyVerifier := &mockL3Notary{result: true}
	cliVerifier := &mockCLISessionVerifier{result: nil}
	notary := NewGatewayL3Notary(store, cliVerifier, passkeyVerifier, logger)

	l3Proof := &commonv1.L3Proof{
		CredentialId:      "browser-credential-id",
		ClientDataJson:    "browser-client-data",
		AuthenticatorData: "browser-authenticator-data",
		Signature:         "browser-passkey-signature",
	}

	allowed, err := notary.VerifyL3Proof(ctx, userID, txHash, "", l3Proof)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.True(t, passkeyVerifier.called)
	assert.False(t, cliVerifier.called, "CLI verifier should not be called for browser-only proof")
}
