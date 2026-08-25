// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package pubsub

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/storage/storagetest"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHeartbeatService_SendAutomatic_PersistsReceiptAgainstRealAuditStore
// reproduces the operator heartbeat failure: SendAutomatic built the
// GovernanceEnvelope with only Id/TransactionHash/ActionType/Payload and never
// set OperatorId or OperatorSessionId. L5Actuator.buildReceiptRecord read the
// empty OperatorSessionId off the envelope, SQLAuditStore.RecordActionReceipt
// skipped the auto-create of the parent sessions row (guarded on non-empty
// session id), and the receipts INSERT violated
// FOREIGN KEY(operator_session_id) REFERENCES sessions(id) because no session
// with id "" existed. With PRAGMA foreign_keys = ON, SQLite returned
// "FOREIGN KEY constraint failed (787)" and the fail-closed LogReceipt path
// aborted the heartbeat.
//
// This test wires SendAutomatic to a real SQLAuditStore (real SQLite, real
// vault, foreign_keys enforced) and asserts the receipt is persisted. Before
// the fix it fails with the FK constraint error; after the fix the envelope
// carries the operator identity, the session row is auto-created, and the
// receipt is recorded.
func TestHeartbeatService_SendAutomatic_PersistsReceiptAgainstRealAuditStore(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	require.NotEmpty(t, cfg.OperatorID)
	require.NotEmpty(t, cfg.OperatorSessionId)

	logger := testutil.NewTestLogger()

	tempDir := testutil.TempDir(t)
	fileSvc := storagetest.NewTestFileSvc(t, tempDir)

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := storagetest.CreateTestVault(t, filepath.Join(tempDir, constants.VaultDirname), privKey)

	auditConfig := &storage.AuditStoreConfig{
		DBPath:          "heartbeat_test.db",
		MaxDBSizeMB:     100,
		RetentionDays:   1,
		EncryptionVault: testVault,
	}
	auditStore, err := storage.NewSQLAuditStore(auditConfig, logger, fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { auditStore.Close() })

	mockHandler := &mockExecutionHandler{
		ExecuteVerifiedTransactionFunc: func(ctx context.Context, eventType constants.EventType, cmdMsg governance.CommandMessage) (string, error) {
			return "heartbeat-completed", nil
		},
	}
	actuatorPrivKey := ed25519.NewKeyFromSeed(make([]byte, 32))
	actuator := &governance.L5Actuator{
		Logger:           logger,
		SQLAuditStore:    auditStore,
		ExecutionHandler: mockHandler,
		SigningKey:       actuatorPrivKey,
		KeyID:            "test-actuator-key",
	}

	svc := NewHeartbeatService(cfg, logger, nil)
	svc.SetContext(context.Background())
	svc.SetActuator(actuator)

	// Before the fix this returns an error wrapping
	// constants.ErrAuditStoreRecordReceiptFailed with the SQLite FK constraint
	// failure (787).
	err = svc.SendAutomatic()
	require.NoError(t, err, "heartbeat must persist its receipt without FK violation")

	// The envelope Id is generated inside SendAutomatic; recover it by querying
	// the audit store for the single heartbeat receipt recorded in this fresh DB.
	receipts, err := auditStore.ListActionReceipts("", 10, 0)
	require.NoError(t, err)
	require.Len(t, receipts, 1, "exactly one heartbeat receipt should be persisted")

	persisted := receipts[0]
	assert.Equal(t, cfg.OperatorID, persisted.OperatorID, "persisted receipt must carry the operator id")
	assert.Equal(t, cfg.OperatorSessionId, persisted.OperatorSessionID, "persisted receipt must carry the operator session id")
	assert.Equal(t, constants.ActionTypeHeartbeat, persisted.ActionType, "persisted receipt must be a heartbeat action")
}
