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
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func newCommitmentTestActuator(t *testing.T) (*L5Actuator, *storage.SQLAuditStore) {
	t.Helper()
	actuator, _ := newTestActuator(t)
	_, auditorPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	actuator.AuditorSigningKey = auditorPriv
	actuator.AuditorKeyID = hex.EncodeToString(auditorPriv.Public().(ed25519.PublicKey))
	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := fileSvc.Resolve(constants.VaultDirname)
	require.NoError(t, fileSvc.MkdirAll(context.Background(), constants.VaultDirname, constants.PermDirPrivate))
	vHeader, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vHeader.Save(vaultDir))
	testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: slog.Default()})
	require.NoError(t, err)
	require.NoError(t, testVault.Unlock(privKey))
	t.Cleanup(func() { require.NoError(t, testVault.Close()) })
	auditStore, err := storage.NewSQLAuditStore(&storage.AuditStoreConfig{
		DBPath:          constants.TestCommitmentLedgerDBFilename,
		MaxDBSizeMB:     100,
		RetentionDays:   1,
		EncryptionVault: testVault,
	}, slog.Default(), fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, auditStore.Close()) })
	actuator.SQLAuditStore = auditStore
	require.NoError(t, auditStore.CreateSession("test-operator-session", "operator", "Test Session", "test-user"))
	return actuator, auditStore
}

func TestL5ActuatorExecutePersistsReceiptAndCommitment(t *testing.T) {
	actuator, auditStore := newCommitmentTestActuator(t)

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
		TransactionHash:   "test-hash-1234567890abcdef",
		OperatorId:        "test-operator",
		OperatorSessionId: "test-operator-session",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
	}

	// Execute
	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Verify RecordActionReceipt was called by querying the audit store
	persistedReceipt, err := auditStore.GetActionReceipt(envelope.Id)
	require.NoError(t, err)
	require.NotNil(t, persistedReceipt, "Receipt should be persisted in audit store")

	// Verify persisted receipt matches the returned receipt
	require.Equal(t, envelope.Id, persistedReceipt.TransactionID)
	require.Equal(t, envelope.TransactionHash, persistedReceipt.TransactionHash)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, persistedReceipt.Status)
	require.Equal(t, "completed", persistedReceipt.ResultSummary)
	require.NotEmpty(t, persistedReceipt.Signature, "Persisted receipt should have signature")

	commitments, err := auditStore.CommitmentLedger().ListCommitments()
	require.NoError(t, err)
	require.Len(t, commitments, 1)
	require.Equal(t, envelope.Id, commitments[0].TransactionID)
	require.Equal(t, envelope.TransactionHash, commitments[0].TransactionHash)
	require.Equal(t, actuator.AuditorKeyID, commitments[0].AuditorKeyID)
	require.NotEmpty(t, commitments[0].WardenIntentSignatureDigest)
	require.NotEmpty(t, commitments[0].Signature)
}

func TestL5ActuatorExecuteCommitmentFailureStopsBeforeHandler(t *testing.T) {
	actuator, auditStore := newCommitmentTestActuator(t)
	handler := actuator.ExecutionHandler.(*mockExecutionHandler)
	actuator.AuditorKeyID = strings.Repeat("0", ed25519.PublicKeySize*2)
	vt := &VerifiedTransaction{
		Envelope: &govtypes.GovernanceEnvelope{
			Id:                uuid.NewString(),
			TransactionHash:   "test-hash-commitment-failure",
			OperatorId:        "test-operator",
			OperatorSessionId: "test-operator-session",
			ActionType:        string(constants.ActionTypeExecuteBash),
			TargetResource:    "localhost",
		},
		ActionType: constants.ActionTypeExecuteBash,
	}

	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrL5ActuatorCommitmentPersist))
	assert.Nil(t, receipt)
	assert.False(t, handler.executed)
	commitments, listErr := auditStore.CommitmentLedger().ListCommitments()
	require.NoError(t, listErr)
	assert.Empty(t, commitments)
}
