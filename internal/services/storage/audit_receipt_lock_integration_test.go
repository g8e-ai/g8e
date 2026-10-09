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
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestReceiptPreparationDoesNotWaitForLedger(t *testing.T) {
	for _, invalid := range []string{"canonical JSON", "locked vault"} {
		t.Run(invalid, func(t *testing.T) {
			store := newIntegrationAuditStore(t)
			record := sampleActionReceiptRecord(t, "session")
			if invalid == "canonical JSON" {
				record.ActionReceipt.ResultSummary = string([]byte{0xff})
			} else {
				store.encryptionVault.Lock()
			}
			store.commitmentLedger.mu.Lock()
			done := make(chan error, 1)
			go func() {
				done <- store.RecordActionReceiptWithCommitment(record, func(string) ([]byte, string, error) {
					return nil, "", constants.ErrNotFound
				})
			}()
			var err error
			select {
			case err = <-done:
				store.commitmentLedger.mu.Unlock()
			case <-time.After(time.Second):
				store.commitmentLedger.mu.Unlock()
				err = <-done // Join even when the regression is present.
				t.Error("receipt preparation waited for the occupied ledger")
			}
			require.Error(t, err)
			if invalid == "locked vault" {
				require.ErrorIs(t, err, constants.ErrAuditStoreVaultLocked)
			}
			var count int
			require.NoError(t, store.db.QueryRow("SELECT count(*) FROM receipts").Scan(&count))
			require.Zero(t, count)
		})
	}
}

type receiptLogObserver struct {
	slog.Handler
	observe func(slog.Record)
}

func (h receiptLogObserver) Handle(_ context.Context, record slog.Record) error {
	h.observe(record)
	return nil
}

func (h receiptLogObserver) Enabled(context.Context, slog.Level) bool { return true }

func TestReceiptSuccessLogsFollowCommitAndUnlock(t *testing.T) {
	store := newIntegrationAuditStore(t)
	var messages []string
	logger := slog.New(receiptLogObserver{
		Handler: slog.DiscardHandler,
		observe: func(record slog.Record) {
			messages = append(messages, record.Message)
			unlocked := store.commitmentLedger.mu.TryLock()
			if unlocked {
				store.commitmentLedger.mu.Unlock()
			}
			assert.True(t, unlocked, "log handler must not serialize receipt writers: %s", record.Message)
			var count int
			err := store.db.QueryRow("SELECT count(*) FROM commitment_ledger").Scan(&count)
			assert.NoError(t, err)
			assert.Equal(t, 1, count, "success logging must follow commit")
		},
	})
	store.logger = logger
	store.commitmentLedger.logger = logger
	record := sampleActionReceiptRecord(t, "session")
	require.NoError(t, store.RecordActionReceiptWithCommitment(record, func(prior string) ([]byte, string, error) {
		payload, err := compliancev1.MarshalCanonical(&operatorv1.CommitmentAttestation{
			TransactionId: record.TransactionID, TransactionHash: record.TransactionHash,
			PriorCommitmentHash: prior, Hash: "commitment-hash",
		})
		return payload, "commitment-hash", err
	}))
	require.Contains(t, messages, "ActionReceipt recorded")
	require.Contains(t, messages, "Commitment appended to ledger")
	require.NoError(t, store.VerifyChain(t.Context(), 0))
}

func TestReceiptCommitFailureRollsBackWithoutSuccessLogs(t *testing.T) {
	store := newIntegrationAuditStore(t)
	// Fail COMMIT after the commitment INSERT has succeeded, rather than
	// failing the builder before the old in-transaction success log ran.
	_, err := store.db.Exec(`
		CREATE TABLE commit_failure (
			session_id TEXT REFERENCES sessions(id) DEFERRABLE INITIALLY DEFERRED
		);
		CREATE TRIGGER fail_commit AFTER INSERT ON commitment_ledger BEGIN
			INSERT INTO commit_failure (session_id) VALUES ('missing-session');
		END;
	`)
	require.NoError(t, err)
	var messages []string
	logger := slog.New(receiptLogObserver{
		Handler: slog.DiscardHandler,
		observe: func(record slog.Record) { messages = append(messages, record.Message) },
	})
	store.logger, store.commitmentLedger.logger = logger, logger
	record := sampleActionReceiptRecord(t, "session")
	err = store.RecordActionReceiptWithCommitment(record, func(prior string) ([]byte, string, error) {
		payload, err := compliancev1.MarshalCanonical(&operatorv1.CommitmentAttestation{
			TransactionId: record.TransactionID, TransactionHash: record.TransactionHash,
			PriorCommitmentHash: prior, Hash: "commitment-hash",
		})
		return payload, "commitment-hash", err
	})
	require.ErrorIs(t, err, constants.ErrAuditStoreRecordReceiptFailed)
	require.Empty(t, messages, "a rolled-back write must not emit success logs")
	for _, table := range []string{"receipts", "events", "commitment_ledger", "sessions"} {
		var count int
		require.NoError(t, store.db.QueryRow("SELECT count(*) FROM "+table).Scan(&count))
		require.Zero(t, count, table)
	}
	require.NoError(t, store.VerifyChain(t.Context(), 0))
}

func TestReceiptConcurrentStagesPreserveBothChains(t *testing.T) {
	store := newIntegrationAuditStore(t)
	const writers = 32
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		record := sampleActionReceiptRecord(t, "shared-session")
		record.TransactionID = fmt.Sprintf("receipt-%d", i)
		record.ActionReceipt.TransactionId = record.TransactionID
		record.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING
		record.ActionReceipt.Status = record.Status
		wg.Go(func() {
			<-start
			err := store.RecordActionReceiptWithCommitment(record, func(prior string) ([]byte, string, error) {
				hash := "commitment-" + record.TransactionID
				payload, err := compliancev1.MarshalCanonical(&operatorv1.CommitmentAttestation{
					TransactionId: record.TransactionID, TransactionHash: record.TransactionHash,
					PriorCommitmentHash: prior, Hash: hash,
				})
				return payload, hash, err
			})
			if err == nil {
				record.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED
				record.ActionReceipt.Status = record.Status
				err = store.RecordActionReceipt(record)
			}
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	rows, err := store.commitmentLedger.ListCommitments()
	require.NoError(t, err)
	require.Len(t, rows, writers)
	prior := ""
	for _, row := range rows {
		require.Equal(t, prior, row.PriorCommitmentHash)
		prior = row.Hash
		record, err := store.GetActionReceipt(row.TransactionID)
		require.NoError(t, err)
		require.NotNil(t, record)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, record.Status)
	}
	var count int
	require.NoError(t, store.db.QueryRow("SELECT count(*) FROM events").Scan(&count))
	require.Equal(t, writers*2, count)
	require.NoError(t, store.VerifyChain(t.Context(), 0))
}
