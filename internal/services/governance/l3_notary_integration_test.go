// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutboundL3Notary_VerifyL3Proof_NoApproval(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "test-tx-hash-123"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	tx := &models.SuspendedTransaction{
		TransactionHash: txHash,
		Envelope:        []byte("{}"),
		CreatedAt:       time.Now().UTC(),
		ExpiresAt:       time.Now().UTC().Add(1 * time.Hour),
		ToolName:        "test_tool",
		UserID:          userID,
		OperatorID:      "op-123",
		Approved:        false,
	}

	err = store.StoreSuspendedTransaction(context.Background(), tx)
	require.NoError(t, err)

	proof := &commonv1.L3Proof{
		CliSignature: "valid-signature",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_ExpiredApproval(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "test-tx-hash-123"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	expiredTime := time.Now().UTC().Add(-31 * time.Minute)
	tx := &models.SuspendedTransaction{
		TransactionHash:   txHash,
		Envelope:          []byte("{}"),
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(1 * time.Hour),
		ToolName:          "test_tool",
		UserID:            userID,
		OperatorID:        "op-123",
		Approved:          true,
		ApprovedAt:        &expiredTime,
		ApprovedBy:        "approver-123",
		ApprovalSignature: "signature-123",
	}

	err = store.StoreSuspendedTransaction(context.Background(), tx)
	require.NoError(t, err)

	proof := &commonv1.L3Proof{
		CliSignature: "signature-123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_MissingSignature(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "test-tx-hash-123"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	now := time.Now().UTC()
	tx := &models.SuspendedTransaction{
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
		ApprovalSignature: "signature-123",
	}

	err = store.StoreSuspendedTransaction(context.Background(), tx)
	require.NoError(t, err)

	proof := &commonv1.L3Proof{
		CliSignature: "",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_InvalidSignatureEncoding(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "test-tx-hash-123"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	now := time.Now().UTC()
	tx := &models.SuspendedTransaction{
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
		ApprovalSignature: "signature-123",
	}

	err = store.StoreSuspendedTransaction(context.Background(), tx)
	require.NoError(t, err)

	proof := &commonv1.L3Proof{
		CliSignature: "not-valid-hex!!!",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_InvalidSignatureLength(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "test-tx-hash-123"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	now := time.Now().UTC()
	tx := &models.SuspendedTransaction{
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
		ApprovalSignature: "signature-123",
	}

	err = store.StoreSuspendedTransaction(context.Background(), tx)
	require.NoError(t, err)

	shortSig := hex.EncodeToString([]byte("short"))
	proof := &commonv1.L3Proof{
		CliSignature: shortSig,
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_SignatureMismatch(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "test-tx-hash-123"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	now := time.Now().UTC()
	_, wrongPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	wrongSig := hex.EncodeToString(ed25519.Sign(wrongPrivKey, []byte(txHash)))

	// Store a different public key so ed25519.Verify fails
	_, otherPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	otherPubKeyHex := hex.EncodeToString(otherPrivKey.Public().(ed25519.PublicKey))

	tx := &models.SuspendedTransaction{
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
		ApprovalSignature: wrongSig,
		ApprovalPublicKey: otherPubKeyHex,
	}

	err = store.StoreSuspendedTransaction(context.Background(), tx)
	require.NoError(t, err)

	proof := &commonv1.L3Proof{
		CliSignature: wrongSig,
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_FingerprintMismatch(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "test-tx-hash-123"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	now := time.Now().UTC()
	_, sigPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(sigPrivKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(sigPrivKey.Public().(ed25519.PublicKey))

	tx := &models.SuspendedTransaction{
		TransactionHash:         txHash,
		Envelope:                []byte("{}"),
		CreatedAt:               now,
		ExpiresAt:               now.Add(1 * time.Hour),
		ToolName:                "test_tool",
		UserID:                  userID,
		OperatorID:              "op-123",
		Approved:                true,
		ApprovedAt:              &now,
		ApprovedBy:              "approver-123",
		ApprovalSignature:       signature,
		ExpectedCertFingerprint: "cert-fingerprint-abc",
		ApprovalPublicKey:       pubKeyHex,
	}

	err = store.StoreSuspendedTransaction(context.Background(), tx)
	require.NoError(t, err)

	proof := &commonv1.L3Proof{
		CliSignature:        signature,
		MtlsCertFingerprint: "cert-fingerprint-xyz",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_ValidProof(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "test-tx-hash-123"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	now := time.Now().UTC()
	pubKey, sigPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(sigPrivKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	tx := &models.SuspendedTransaction{
		TransactionHash:         txHash,
		Envelope:                []byte("{}"),
		CreatedAt:               now,
		ExpiresAt:               now.Add(1 * time.Hour),
		ToolName:                "test_tool",
		UserID:                  userID,
		OperatorID:              "op-123",
		Approved:                true,
		ApprovedAt:              &now,
		ApprovedBy:              "approver-123",
		ApprovalSignature:       signature,
		ExpectedCertFingerprint: "cert-fingerprint-abc",
		ApprovalPublicKey:       pubKeyHex,
	}

	err = store.StoreSuspendedTransaction(context.Background(), tx)
	require.NoError(t, err)

	proof := &commonv1.L3Proof{
		CliSignature:        signature,
		MtlsCertFingerprint: "cert-fingerprint-abc",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_TransactionNotFound(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "nonexistent-tx-hash"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	proof := &commonv1.L3Proof{
		CliSignature: "signature-123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_UserIDMismatch(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "test-tx-hash-123"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	now := time.Now().UTC()
	tx := &models.SuspendedTransaction{
		TransactionHash:   txHash,
		Envelope:          []byte("{}"),
		CreatedAt:         now,
		ExpiresAt:         now.Add(1 * time.Hour),
		ToolName:          "test_tool",
		UserID:            "different-user-456",
		OperatorID:        "op-123",
		Approved:          true,
		ApprovedAt:        &now,
		ApprovedBy:        "approver-123",
		ApprovalSignature: "signature-123",
	}

	err = store.StoreSuspendedTransaction(context.Background(), tx)
	require.NoError(t, err)

	proof := &commonv1.L3Proof{
		CliSignature: "signature-123",
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_NoFingerprintCheckWhenNotSet(t *testing.T) {
	logger := slog.Default()
	tmpDir := testutil.TempDir(t)
	config := &storage.SuspendedTransactionConfig{
		DBPath: filepath.Join(tmpDir, "test.db"),
	}

	store, err := storage.NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	defer store.Close()
	notary := NewOutboundL3Notary(store, logger)

	txHash := "test-tx-hash-123"
	userID := "user-123"
	cliSessionID := "cli-session-456"

	now := time.Now().UTC()
	pubKey, sigPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signature := hex.EncodeToString(ed25519.Sign(sigPrivKey, []byte(txHash)))
	pubKeyHex := hex.EncodeToString(pubKey)

	tx := &models.SuspendedTransaction{
		TransactionHash:         txHash,
		Envelope:                []byte("{}"),
		CreatedAt:               now,
		ExpiresAt:               now.Add(1 * time.Hour),
		ToolName:                "test_tool",
		UserID:                  userID,
		OperatorID:              "op-123",
		Approved:                true,
		ApprovedAt:              &now,
		ApprovedBy:              "approver-123",
		ApprovalSignature:       signature,
		ExpectedCertFingerprint: "",
		ApprovalPublicKey:       pubKeyHex,
	}

	err = store.StoreSuspendedTransaction(context.Background(), tx)
	require.NoError(t, err)

	proof := &commonv1.L3Proof{
		CliSignature: signature,
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.NoError(t, err)
	assert.True(t, allowed)
}
