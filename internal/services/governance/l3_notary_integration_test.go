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
	"crypto/ed25519"
	"encoding/hex"
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

func TestOutboundL3Notary_VerifyL3Proof_NoApproval(t *testing.T) {
	logger := slog.Default()
	tmpDir := t.TempDir()
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
	tmpDir := t.TempDir()
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
	tmpDir := t.TempDir()
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
	tmpDir := t.TempDir()
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
	tmpDir := t.TempDir()
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
	tmpDir := t.TempDir()
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

	_, wrongPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	wrongSig := hex.EncodeToString(ed25519.Sign(wrongPrivKey, []byte(txHash)))

	proof := &commonv1.L3Proof{
		CliSignature: wrongSig,
	}

	allowed, err := notary.VerifyL3Proof(context.Background(), userID, txHash, cliSessionID, proof)
	require.Error(t, err)
	assert.False(t, allowed)
}

func TestOutboundL3Notary_VerifyL3Proof_FingerprintMismatch(t *testing.T) {
	logger := slog.Default()
	tmpDir := t.TempDir()
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
	tmpDir := t.TempDir()
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
	tmpDir := t.TempDir()
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
	tmpDir := t.TempDir()
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
	tmpDir := t.TempDir()
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
