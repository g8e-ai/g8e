// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package storage

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
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
