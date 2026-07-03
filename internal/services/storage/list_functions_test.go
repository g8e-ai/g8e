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

package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// ─── AuditStore List tests ───────────────────────────────────────────────────

func TestListSessions_NilStore(t *testing.T) {
	t.Parallel()
	var ass *SQLAuditStore
	sessions, err := ass.ListSessions(10, 0)
	require.Error(t, err)
	assert.Nil(t, sessions)
	assert.ErrorIs(t, err, constants.ErrAuditStoreDisabled)
}

func TestListSessions_NilDB(t *testing.T) {
	t.Parallel()
	ass := &SQLAuditStore{db: nil}
	sessions, err := ass.ListSessions(10, 0)
	require.Error(t, err)
	assert.Nil(t, sessions)
	assert.ErrorIs(t, err, constants.ErrAuditStoreDisabled)
}

func TestListEvents_NilStore(t *testing.T) {
	t.Parallel()
	var ass *SQLAuditStore
	events, err := ass.ListEvents("", 10, 0)
	require.Error(t, err)
	assert.Nil(t, events)
	assert.ErrorIs(t, err, constants.ErrAuditStoreDisabled)
}

func TestListEvents_NilDB(t *testing.T) {
	t.Parallel()
	ass := &SQLAuditStore{db: nil}
	events, err := ass.ListEvents("session-1", 10, 0)
	require.Error(t, err)
	assert.Nil(t, events)
	assert.ErrorIs(t, err, constants.ErrAuditStoreDisabled)
}

func TestListFileMutations_NilStore(t *testing.T) {
	t.Parallel()
	var ass *SQLAuditStore
	mutations, err := ass.ListFileMutations(10, 0)
	require.Error(t, err)
	assert.Nil(t, mutations)
	assert.ErrorIs(t, err, constants.ErrAuditStoreDisabled)
}

func TestListFileMutations_NilDB(t *testing.T) {
	t.Parallel()
	ass := &SQLAuditStore{db: nil}
	mutations, err := ass.ListFileMutations(10, 0)
	require.Error(t, err)
	assert.Nil(t, mutations)
	assert.ErrorIs(t, err, constants.ErrAuditStoreDisabled)
}

// ─── ExecutionVault List tests ───────────────────────────────────────────────

func TestListExecutions_NilService(t *testing.T) {
	t.Parallel()
	var ev *ExecutionVaultService
	records, err := ev.ListExecutions(context.Background(), 10, 0)
	require.Error(t, err)
	assert.Nil(t, records)
	assert.ErrorIs(t, err, constants.ErrLedgerDisabled)
}

func TestListExecutions_NilDB(t *testing.T) {
	t.Parallel()
	ev := &ExecutionVaultService{db: nil}
	records, err := ev.ListExecutions(context.Background(), 10, 0)
	require.Error(t, err)
	assert.Nil(t, records)
	assert.ErrorIs(t, err, constants.ErrLedgerDisabled)
}

func TestListExecutions_Empty(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)
	records, err := ev.ListExecutions(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestListExecutions_WithData(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	for i := 0; i < 3; i++ {
		record := &models.ExecutionRecord{
			ID:                fmt.Sprintf("exec-list-%d", i),
			TimestampUTC:      time.Now().UTC(),
			Command:           fmt.Sprintf("echo test%d", i),
			ExitCode:          0,
			DurationMs:        int64(i * 10),
			StdoutCompressed:  []byte("output"),
			StdoutSize:        6,
		}
		require.NoError(t, ev.StoreExecution(context.Background(), record))
	}
	ev.Wait()

	records, err := ev.ListExecutions(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Len(t, records, 3)
}

func TestListExecutions_DefaultLimit(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	for i := 0; i < 3; i++ {
		record := &models.ExecutionRecord{
			ID:                fmt.Sprintf("exec-default-%d", i),
			TimestampUTC:      time.Now().UTC(),
			Command:           "test",
			StdoutCompressed:  []byte("out"),
			StdoutSize:        3,
		}
		require.NoError(t, ev.StoreExecution(context.Background(), record))
	}
	ev.Wait()

	records, err := ev.ListExecutions(context.Background(), 0, 0)
	require.NoError(t, err)
	assert.Len(t, records, 3)
}

func TestListExecutions_Pagination(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	for i := 0; i < 5; i++ {
		record := &models.ExecutionRecord{
			ID:                fmt.Sprintf("exec-page-%d", i),
			TimestampUTC:      time.Now().UTC(),
			Command:           "test",
			StdoutCompressed:  []byte("out"),
			StdoutSize:        3,
		}
		require.NoError(t, ev.StoreExecution(context.Background(), record))
	}
	ev.Wait()

	page1, err := ev.ListExecutions(context.Background(), 2, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := ev.ListExecutions(context.Background(), 2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
}

func TestListFileDiffs_NilService(t *testing.T) {
	t.Parallel()
	var ev *ExecutionVaultService
	records, err := ev.ListFileDiffs(context.Background(), 10, 0)
	require.Error(t, err)
	assert.Nil(t, records)
	assert.ErrorIs(t, err, constants.ErrLedgerDisabled)
}

func TestListFileDiffs_NilDB(t *testing.T) {
	t.Parallel()
	ev := &ExecutionVaultService{db: nil}
	records, err := ev.ListFileDiffs(context.Background(), 10, 0)
	require.Error(t, err)
	assert.Nil(t, records)
	assert.ErrorIs(t, err, constants.ErrLedgerDisabled)
}

func TestListFileDiffs_Empty(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)
	records, err := ev.ListFileDiffs(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestListFileDiffs_WithData(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	for i := 0; i < 3; i++ {
		record := &models.FileDiffRecord{
			ID:                fmt.Sprintf("diff-list-%d", i),
			TimestampUTC:      time.Now().UTC(),
			FilePath:          fmt.Sprintf("/test/file%d.txt", i),
			Operation:         "write",
			DiffCompressed:    []byte("diff content"),
			DiffSize:          12,
			OperatorSessionID: "session-1",
		}
		require.NoError(t, ev.StoreFileDiff(context.Background(), record))
	}
	ev.Wait()

	records, err := ev.ListFileDiffs(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Len(t, records, 3)
}

func TestListFileDiffs_DefaultLimit(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	for i := 0; i < 3; i++ {
		record := &models.FileDiffRecord{
			ID:                fmt.Sprintf("diff-default-%d", i),
			TimestampUTC:      time.Now().UTC(),
			FilePath:          "/test/file.txt",
			Operation:         "write",
			DiffCompressed:    []byte("diff"),
			DiffSize:          4,
		}
		require.NoError(t, ev.StoreFileDiff(context.Background(), record))
	}
	ev.Wait()

	records, err := ev.ListFileDiffs(context.Background(), 0, 0)
	require.NoError(t, err)
	assert.Len(t, records, 3)
}

func TestListFileDiffs_Pagination(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	for i := 0; i < 5; i++ {
		record := &models.FileDiffRecord{
			ID:                fmt.Sprintf("diff-page-%d", i),
			TimestampUTC:      time.Now().UTC(),
			FilePath:          "/test/file.txt",
			Operation:         "write",
			DiffCompressed:    []byte("diff"),
			DiffSize:          4,
		}
		require.NoError(t, ev.StoreFileDiff(context.Background(), record))
	}
	ev.Wait()

	page1, err := ev.ListFileDiffs(context.Background(), 2, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := ev.ListFileDiffs(context.Background(), 2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
}

// ─── ReplayStore ListNonces tests ────────────────────────────────────────────

func TestListNonces_NilStore(t *testing.T) {
	t.Parallel()
	var rs *SQLReplayStore
	nonces, err := rs.ListNonces(10, 0)
	require.Error(t, err)
	assert.Nil(t, nonces)
}

func TestListNonces_NilDB(t *testing.T) {
	t.Parallel()
	rs := &SQLReplayStore{db: nil}
	nonces, err := rs.ListNonces(10, 0)
	require.Error(t, err)
	assert.Nil(t, nonces)
}

func TestListNonces_Empty(t *testing.T) {
	t.Parallel()
	rs := setupTestReplayStore(t)
	nonces, err := rs.ListNonces(10, 0)
	require.NoError(t, err)
	assert.Empty(t, nonces)
}

func TestListNonces_WithData(t *testing.T) {
	t.Parallel()
	rs := setupTestReplayStore(t)

	expiresAt := time.Now().UTC().Add(time.Hour)
	for i := 0; i < 3; i++ {
		_, err := rs.ReserveNonce(fmt.Sprintf("nonce-list-%d", i), expiresAt)
		require.NoError(t, err)
	}

	nonces, err := rs.ListNonces(10, 0)
	require.NoError(t, err)
	assert.Len(t, nonces, 3)

	for _, n := range nonces {
		assert.Equal(t, "reserved", n.Status)
		assert.False(t, n.ReservedAt.IsZero())
		assert.False(t, n.ExpiresAt.IsZero())
		assert.Nil(t, n.UsedAt)
	}
}

func TestListNonces_DefaultLimit(t *testing.T) {
	t.Parallel()
	rs := setupTestReplayStore(t)

	expiresAt := time.Now().UTC().Add(time.Hour)
	for i := 0; i < 3; i++ {
		_, err := rs.ReserveNonce(fmt.Sprintf("nonce-default-%d", i), expiresAt)
		require.NoError(t, err)
	}

	nonces, err := rs.ListNonces(0, 0)
	require.NoError(t, err)
	assert.Len(t, nonces, 3)
}

func TestListNonces_Pagination(t *testing.T) {
	t.Parallel()
	rs := setupTestReplayStore(t)

	expiresAt := time.Now().UTC().Add(time.Hour)
	for i := 0; i < 5; i++ {
		_, err := rs.ReserveNonce(fmt.Sprintf("nonce-page-%d", i), expiresAt)
		require.NoError(t, err)
	}

	page1, err := rs.ListNonces(2, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := rs.ListNonces(2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
}

// ─── CommitmentLedger ListCommitments tests ──────────────────────────────────

func TestListCommitments_NilLedger(t *testing.T) {
	t.Parallel()
	var cl *CommitmentLedger
	commitments, err := cl.ListCommitments()
	require.Error(t, err)
	assert.Nil(t, commitments)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestListCommitments_NilDB(t *testing.T) {
	t.Parallel()
	cl := &CommitmentLedger{db: nil, logger: testutil.NewTestLogger()}
	commitments, err := cl.ListCommitments()
	require.Error(t, err)
	assert.Nil(t, commitments)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestListCommitments_Empty(t *testing.T) {
	t.Parallel()
	cl, _ := setupTestCommitmentLedger(t)
	commitments, err := cl.ListCommitments()
	require.NoError(t, err)
	assert.Empty(t, commitments)
}

func TestListCommitments_WithData(t *testing.T) {
	t.Parallel()
	cl, _ := setupTestCommitmentLedger(t)

	for i := 0; i < 3; i++ {
		attestation := []byte(fmt.Sprintf(`{
			"transaction_id": "tx-%d",
			"transaction_hash": "thash-%d",
			"state_root_at_commit": "sr-%d",
			"l2_signature_digest": "l2-%d",
			"Actuator_intent_signature_digest": "act-%d",
			"human_signature_digest": "hsig-%d",
			"action_type": "write",
			"target_resource": "/file%d",
			"committed_at_unix_ms": %d,
			"auditor_key_id": "aud-%d",
			"signature": "sig-%d"
		}`, i, i, i, i, i, i, i, 1000+i, i, i))
		var priorHash string
		if i > 0 {
			priorHash = fmt.Sprintf("hash-%d", i-1)
		}
		err := cl.AppendCommitmentJSON(attestation, priorHash, fmt.Sprintf("hash-%d", i))
		require.NoError(t, err)
	}

	commitments, err := cl.ListCommitments()
	require.NoError(t, err)
	assert.Len(t, commitments, 3)

	assert.Equal(t, int64(1), commitments[0].Seq)
	assert.Equal(t, int64(3), commitments[2].Seq)
	assert.Equal(t, "tx-0", commitments[0].TransactionID)
	assert.Equal(t, "tx-2", commitments[2].TransactionID)
}

func TestListCommitments_OrderedByCommittedAt(t *testing.T) {
	t.Parallel()
	cl, _ := setupTestCommitmentLedger(t)

	attestation1 := []byte(`{
		"transaction_id": "tx-first",
		"transaction_hash": "thash-1",
		"committed_at_unix_ms": 1000,
		"action_type": "write",
		"target_resource": "/file1",
		"auditor_key_id": "aud1",
		"signature": "sig1"
	}`)
	require.NoError(t, cl.AppendCommitmentJSON(attestation1, "", "hash-1"))

	attestation2 := []byte(`{
		"transaction_id": "tx-second",
		"transaction_hash": "thash-2",
		"committed_at_unix_ms": 2000,
		"action_type": "write",
		"target_resource": "/file2",
		"auditor_key_id": "aud2",
		"signature": "sig2"
	}`)
	require.NoError(t, cl.AppendCommitmentJSON(attestation2, "hash-1", "hash-2"))

	commitments, err := cl.ListCommitments()
	require.NoError(t, err)
	assert.Len(t, commitments, 2)
	assert.Equal(t, "tx-first", commitments[0].TransactionID)
	assert.Equal(t, "tx-second", commitments[1].TransactionID)
	assert.True(t, commitments[0].CommittedAt.Before(commitments[1].CommittedAt))
}
