// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package storage

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	vault "github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func newIntegrationAuditStore(t *testing.T) *SQLAuditStore {
	t.Helper()

	fileSvc, _ := newTestFileSvc(t, testutil.TempDir(t))
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, fileSvc, privKey)

	config := DefaultAuditStoreConfig()
	config.EncryptionVault = testVault

	ass, err := NewSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ass.Close()) })
	return ass
}

func sampleActionReceiptRecord(t *testing.T, sessionID string) *models.ActionReceiptRecord {
	t.Helper()

	now := time.Now().UTC()
	return &models.ActionReceiptRecord{
		TransactionID:     "tx-integration-1",
		TransactionHash:   "hash-integration-1",
		InvestigationID:   "investigation-integration-1",
		OperatorID:        "operator-integration-1",
		OperatorSessionID: sessionID,
		RequestorUserID:   "user-integration-1",
		ActingAppID:       "app-integration-1",
		ActionType:        constants.ActionTypeExecuteBash,
		TargetResource:    "localhost",
		Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:     "completed successfully",
		StateRootBefore:   "root-before",
		StateRootAfter:    "root-after",
		ExecutedAt:        now,
		SignerKeyID:       "signer-1",
		Signature:         "signature-1",
		Timestamp:         now,
		ActionReceipt: &operatorv1.ActionReceipt{
			TransactionId:   "tx-integration-1",
			TransactionHash: "hash-integration-1",
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Signature:       "signature-1",
		},
	}
}

func TestNewSQLAuditStore_NilLoggerUsesDefault(t *testing.T) {
	fileSvc, _ := newTestFileSvc(t, testutil.TempDir(t))
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, fileSvc, privKey)

	config := DefaultAuditStoreConfig()
	config.EncryptionVault = testVault

	ass, err := NewSQLAuditStore(config, nil, fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ass.Close()) })

	require.NotNil(t, ass.CommitmentLedger())
	require.NotNil(t, ass.GetEncryptionVault())
	assert.NotEmpty(t, ass.GetDataDir())
	ass.Wait()
}

func TestSQLAuditStore_IntegrationLifecycle(t *testing.T) {
	ass := newIntegrationAuditStore(t)

	require.NoError(t, ass.CreateSession("session-1", constants.SessionTypeOperator, "Integration Session", "user-1"))
	session, err := ass.GetOperatorSession("session-1")
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "session-1", session.ID)

	event := &Event{
		OperatorSessionID: "session-1",
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		ContentText:       "integration event",
		CommandRaw:        "echo hello",
		CommandExitCode:   0,
		CommandStdout:     "hello",
		CommandStderr:     "",
	}
	eventID, err := ass.RecordEvent(event)
	require.NoError(t, err)
	require.Positive(t, eventID)

	require.NoError(t, ass.RecordEvents([]*Event{
		{
			OperatorSessionID: "session-1",
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       "batch event",
			CommandRaw:        "echo batch",
			CommandExitCode:   0,
			CommandStdout:     "batch",
		},
	}))

	events, err := ass.GetEvents("session-1", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 2)

	listedEvents, err := ass.ListEvents("session-1", 10, 0)
	require.NoError(t, err)
	require.Len(t, listedEvents, 2)

	mutation := &FileMutationLog{
		EventID:          eventID,
		Filepath:         "data/example.txt",
		Operation:        FileMutationWrite,
		LedgerHashBefore: "before",
		LedgerHashAfter:  "after",
		DiffStat:         "+1",
	}
	require.NoError(t, ass.RecordFileMutation(mutation))
	mutations, err := ass.GetFileMutations(eventID)
	require.NoError(t, err)
	require.Len(t, mutations, 1)
	assert.Equal(t, "data/example.txt", mutations[0].Filepath)

	allMutations, err := ass.ListFileMutations(10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, allMutations)

	sessions, err := ass.ListSessions(10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, sessions)

	record := sampleActionReceiptRecord(t, "session-1")
	require.NoError(t, ass.RecordActionReceipt(record))

	persisted, err := ass.GetActionReceipt(record.TransactionID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, record.TransactionID, persisted.TransactionID)
	require.NotNil(t, persisted.ActionReceipt)
	assert.True(t, proto.Equal(record.ActionReceipt, persisted.ActionReceipt))

	correlated, err := ass.GetActionReceiptByInvestigationID(record.InvestigationID, record.ActionType)
	require.NoError(t, err)
	require.NotNil(t, correlated)
	assert.Equal(t, record.TransactionID, correlated.TransactionID)

	receipts, err := ass.ListActionReceipts("session-1", 10, 0)
	require.NoError(t, err)
	require.Len(t, receipts, 1)

	sinceReceipts, err := ass.ListActionReceiptsSince(time.Now().UTC().Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, sinceReceipts, 1)

	missing, err := ass.GetActionReceipt("missing-transaction")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestSQLAuditStore_DocSetAndDocDelete(t *testing.T) {
	ass := newIntegrationAuditStore(t)

	record := sampleActionReceiptRecord(t, "")
	record.OperatorSessionID = ""
	payload, err := json.Marshal(record)
	require.NoError(t, err)

	require.NoError(t, ass.DocSet("ignored", "ignored", payload))
	require.NoError(t, ass.DocDelete("ignored", "ignored"))

	persisted, err := ass.GetActionReceipt(record.TransactionID)
	require.NoError(t, err)
	require.NotNil(t, persisted)

	err = ass.DocSet("ignored", "ignored", json.RawMessage("{invalid"))
	require.Error(t, err)
}

func TestParseStoredActionReceipt_InvalidCanonicalPayload(t *testing.T) {
	receipt, err := parseStoredActionReceipt(sql.NullString{String: "{not-canonical", Valid: true})
	require.Error(t, err)
	assert.Nil(t, receipt)

	empty, err := parseStoredActionReceipt(sql.NullString{})
	require.NoError(t, err)
	assert.Nil(t, empty)
}

func TestSQLAuditStore_RecordActionReceiptWithoutSessionAutoCreatesRow(t *testing.T) {
	ass := newIntegrationAuditStore(t)

	record := sampleActionReceiptRecord(t, "")
	record.TransactionID = "tx-auto-session"
	record.OperatorSessionID = "auto-session"
	record.ActionReceipt = &operatorv1.ActionReceipt{
		TransactionId:   record.TransactionID,
		TransactionHash: record.TransactionHash,
		Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
	}
	body, err := compliancev1.MarshalCanonical(record.ActionReceipt)
	require.NoError(t, err)
	record.ActionReceipt = nil
	require.NoError(t, ass.RecordActionReceipt(record))

	_, err = ass.db.ExecWithRetry(`UPDATE receipts SET receipt_json = ? WHERE transaction_id = ?`, body, record.TransactionID)
	require.NoError(t, err)

	persisted, err := ass.GetActionReceipt(record.TransactionID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.NotNil(t, persisted.ActionReceipt)
	assert.Equal(t, record.TransactionID, persisted.ActionReceipt.GetTransactionId())
}

func TestSQLAuditStore_VerifyChain_AppendAndTamper(t *testing.T) {
	ass := newIntegrationAuditStore(t)
	require.NoError(t, ass.CreateSession("chain-session", constants.SessionTypeOperator, "Chain Session", "user-1"))

	events := []*Event{
		{
			OperatorSessionID: "chain-session",
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       "first",
			CommandExitCode:   constants.ExitCodeNone,
		},
		{
			OperatorSessionID: "chain-session",
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.AIMsg,
			ContentText:       "second",
			CommandExitCode:   constants.ExitCodeNone,
		},
	}
	for _, event := range events {
		_, err := ass.RecordEvent(event)
		require.NoError(t, err)
	}

	require.NoError(t, ass.VerifyChain(context.Background(), 0))

	_, err := ass.db.ExecWithRetry(`UPDATE events SET content_digest = ? WHERE seq = 1`, "tampered")
	require.NoError(t, err)
	err = ass.VerifyChain(context.Background(), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch")
}

func TestSQLAuditStore_VerifyChain_BackfillLegacyRows(t *testing.T) {
	fileSvc, _ := newTestFileSvc(t, testutil.TempDir(t))
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, fileSvc, privKey)

	config := DefaultAuditStoreConfig()
	config.EncryptionVault = testVault

	ass, err := NewSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ass.Close()) })

	require.NoError(t, ass.CreateSession("legacy-session", constants.SessionTypeOperator, "Legacy", "user-legacy"))
	_, err = ass.db.ExecWithRetry(`
		INSERT INTO events (operator_session_id, timestamp, type, content_text, command_exit_code, stored_locally)
		VALUES (?, ?, ?, ?, ?, 1)
	`, "legacy-session", "2026-01-01T00:00:00.000000Z", string(constants.Event.Operator.Audit.UserMsg), "legacy payload", constants.ExitCodeNone)
	require.NoError(t, err)

	require.NoError(t, MigrateEventChainColumns(ass.db, ass.logger, ass.encryptionVault))
	require.NoError(t, ass.VerifyChain(context.Background(), 0))
}

func TestPruneChainedAuditEvents_WritesCheckpoint(t *testing.T) {
	ass := newIntegrationAuditStore(t)
	require.NoError(t, ass.CreateSession("prune-session", constants.SessionTypeOperator, "Prune", "user-prune"))

	oldTimestamp := time.Now().UTC().AddDate(0, 0, -120)
	_, err := ass.RecordEvent(&Event{
		OperatorSessionID: "prune-session",
		Timestamp:         oldTimestamp,
		Type:              constants.Event.Operator.Audit.Command,
		ContentText:       "old event",
		CommandExitCode:   constants.ExitCodeNone,
	})
	require.NoError(t, err)

	cutoff := time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339Nano)
	require.NoError(t, PruneChainedAuditEvents(context.Background(), ass.db, ass.logger, cutoff))

	var checkpointCount int
	require.NoError(t, ass.db.QueryRowWithRetry(`SELECT COUNT(*) FROM audit_chain_checkpoints`).Scan(&checkpointCount))
	assert.Positive(t, checkpointCount)

	var remaining int
	require.NoError(t, ass.db.QueryRowWithRetry(`SELECT COUNT(*) FROM events WHERE type != ?`, string(constants.EventPlatformAuditChainCheckpointed)).Scan(&remaining))
	assert.Zero(t, remaining)

	require.NoError(t, ass.VerifyChain(context.Background(), 0))
}

func TestSQLAuditStore_VerifyChain_PrevHashTamper(t *testing.T) {
	ass := newIntegrationAuditStore(t)
	require.NoError(t, ass.CreateSession("prevhash-session", constants.SessionTypeOperator, "PrevHash", "user-prev"))

	for i := 0; i < 3; i++ {
		_, err := ass.RecordEvent(&Event{
			OperatorSessionID: "prevhash-session",
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       "event-" + string(rune('a'+i)),
			CommandExitCode:   constants.ExitCodeNone,
		})
		require.NoError(t, err)
	}
	require.NoError(t, ass.VerifyChain(context.Background(), 0))

	_, err := ass.db.ExecWithRetry(`UPDATE events SET prev_hash = ? WHERE seq = 2`, auditChainGenesisPrevHash)
	require.NoError(t, err)
	err = ass.VerifyChain(context.Background(), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prev_hash mismatch")
}

func TestSQLAuditStore_VerifyChain_SeqReorderTamper(t *testing.T) {
	ass := newIntegrationAuditStore(t)
	require.NoError(t, ass.CreateSession("reorder-session", constants.SessionTypeOperator, "Reorder", "user-reorder"))

	for i := 0; i < 3; i++ {
		_, err := ass.RecordEvent(&Event{
			OperatorSessionID: "reorder-session",
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       "ordered-" + string(rune('a'+i)),
			CommandExitCode:   constants.ExitCodeNone,
		})
		require.NoError(t, err)
	}
	require.NoError(t, ass.VerifyChain(context.Background(), 0))

	// Swap seq 1 and 2 while leaving stale prev_hash/hash columns — breaks linkage.
	_, err := ass.db.ExecWithRetry(`UPDATE events SET seq = 999 WHERE seq = 1`)
	require.NoError(t, err)
	_, err = ass.db.ExecWithRetry(`UPDATE events SET seq = 1 WHERE seq = 2`)
	require.NoError(t, err)
	_, err = ass.db.ExecWithRetry(`UPDATE events SET seq = 2 WHERE seq = 999`)
	require.NoError(t, err)

	err = ass.VerifyChain(context.Background(), 0)
	require.Error(t, err)
}

func TestSQLAuditStore_VerifyChain_ConcurrentAppends(t *testing.T) {
	ass := newIntegrationAuditStore(t)
	require.NoError(t, ass.CreateSession("concurrent-session", constants.SessionTypeOperator, "Concurrent", "user-concurrent"))

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := ass.RecordEvent(&Event{
				OperatorSessionID: "concurrent-session",
				Timestamp:         time.Now().UTC(),
				Type:              constants.Event.Operator.Audit.Command,
				ContentText:       "concurrent-" + string(rune('a'+n)),
				CommandExitCode:   constants.ExitCodeNone,
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var count int
	require.NoError(t, ass.db.QueryRowWithRetry(`SELECT COUNT(*) FROM events WHERE seq IS NOT NULL`).Scan(&count))
	assert.Equal(t, workers, count)
	require.NoError(t, ass.VerifyChain(context.Background(), 0))
}

func TestSQLAuditStore_VerifyChain_SurvivesVaultRekey(t *testing.T) {
	tempDir := testutil.TempDir(t)
	oldKey := []byte("audit-chain-old-vault-key!!")
	newKey := []byte("audit-chain-new-vault-key!!")

	fileSvc, _ := newTestFileSvc(t, tempDir)
	testVault := CreateTestVault(t, fileSvc, oldKey)

	config := DefaultAuditStoreConfig()
	config.EncryptionVault = testVault
	ass, err := NewSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)

	require.NoError(t, ass.CreateSession("rekey-chain-session", constants.SessionTypeOperator, "Rekey Chain", "user-rekey"))
	for i := 0; i < 2; i++ {
		_, err := ass.RecordEvent(&Event{
			OperatorSessionID: "rekey-chain-session",
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       "pre-rekey-" + string(rune('a'+i)),
			CommandExitCode:   constants.ExitCodeNone,
		})
		require.NoError(t, err)
	}
	require.NoError(t, ass.VerifyChain(context.Background(), 0))

	require.NoError(t, ass.Close())
	testVault.Close()

	fileSvcForVault, err := fs.NewRuntimeFileService(tempDir, testutil.NewTestLogger())
	require.NoError(t, err)
	rekeyedVault, err := vault.NewVault(&vault.VaultConfig{
		FileSvc: fileSvcForVault,
		Logger:  testutil.NewTestLogger(),
	})
	require.NoError(t, err)
	require.NoError(t, rekeyedVault.Rekey(oldKey, newKey))
	require.NoError(t, rekeyedVault.Unlock(newKey))
	t.Cleanup(func() { rekeyedVault.Close() })

	config.EncryptionVault = rekeyedVault
	ass2, err := NewSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ass2.Close()) })

	require.NoError(t, ass2.VerifyChain(context.Background(), 0))

	_, err = ass2.RecordEvent(&Event{
		OperatorSessionID: "rekey-chain-session",
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.AIMsg,
		ContentText:       "post-rekey",
		CommandExitCode:   constants.ExitCodeNone,
	})
	require.NoError(t, err)
	require.NoError(t, ass2.VerifyChain(context.Background(), 0))
}
