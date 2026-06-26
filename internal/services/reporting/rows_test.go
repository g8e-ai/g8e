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

package reporting

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReceiptRow(t *testing.T) {
	t.Run("Columns returns expected headers", func(t *testing.T) {
		cols := ReceiptRow{}.Columns()
		assert.Contains(t, cols, "transaction_id")
		assert.Contains(t, cols, "transaction_hash")
		assert.Contains(t, cols, "action_type")
		assert.Contains(t, cols, "status")
		assert.Contains(t, cols, "executed_at_utc")
		assert.Len(t, cols, 12)
	})

	t.Run("Record returns values in column order", func(t *testing.T) {
		r := ReceiptRow{
			TransactionID:   "tx-001",
			TransactionHash: "hash123",
			OperatorID:      "op-1",
			SessionID:       "sess-1",
			ActionType:      "FS_READ",
			TargetResource:  "/path/file",
			Status:          "OK",
			StateRootBefore: "root-a",
			StateRootAfter:  "root-b",
			SignerKeyID:     "key-1",
			Signature:       "sig-1",
			ExecutedAtUTC:   "2026-01-01T00:00:00Z",
		}
		rec := r.Record()
		require.Len(t, rec, 12)
		assert.Equal(t, "tx-001", rec[0])
		assert.Equal(t, "hash123", rec[1])
		assert.Equal(t, "FS_READ", rec[4])
		assert.Equal(t, "2026-01-01T00:00:00Z", rec[11])
	})
}

func TestSessionRow(t *testing.T) {
	t.Run("Columns and Record match", func(t *testing.T) {
		r := SessionRow{
			ID:           "sess-1",
			SessionType:  "operator",
			Title:        "Test Session",
			UserIdentity: "user@example.com",
			CreatedAtUTC: "2026-01-01T00:00:00Z",
		}
		cols := SessionRow{}.Columns()
		rec := r.Record()
		require.Len(t, cols, 5)
		require.Len(t, rec, 5)
		assert.Equal(t, "sess-1", rec[0])
		assert.Equal(t, "Test Session", rec[2])
	})
}

func TestEventRow(t *testing.T) {
	t.Run("Columns returns expected headers", func(t *testing.T) {
		cols := EventRow{}.Columns()
		assert.Contains(t, cols, "id")
		assert.Contains(t, cols, "type")
		assert.Contains(t, cols, "command_raw")
		assert.Contains(t, cols, "exit_code")
		assert.Len(t, cols, 12)
	})

	t.Run("Record with non-None exit code", func(t *testing.T) {
		r := EventRow{
			ID:           42,
			SessionID:    "sess-1",
			TimestampUTC: "2026-01-01T00:00:00Z",
			Type:         "COMMAND_EXECUTION",
			CommandRaw:   "ls -la",
			ExitCode:     0,
			DurationMs:   150,
		}
		rec := r.Record()
		require.Len(t, rec, 12)
		assert.Equal(t, "42", rec[0])
		assert.Equal(t, "0", rec[5])
		assert.Equal(t, "150", rec[6])
	})

	t.Run("Record with ExitCodeNone produces empty string", func(t *testing.T) {
		r := EventRow{
			ID:       1,
			ExitCode: constants.ExitCodeNone,
		}
		rec := r.Record()
		assert.Equal(t, "", rec[5])
	})
}

func TestFileMutationRow(t *testing.T) {
	t.Run("Columns and Record match", func(t *testing.T) {
		r := FileMutationRow{
			ID:               1,
			EventID:          10,
			Filepath:         "/test/file",
			Operation:        "WRITE",
			LedgerHashBefore: "hash-before",
			LedgerHashAfter:  "hash-after",
			DiffStat:         "1 file changed",
		}
		cols := FileMutationRow{}.Columns()
		rec := r.Record()
		require.Len(t, cols, 7)
		require.Len(t, rec, 7)
		assert.Equal(t, "1", rec[0])
		assert.Equal(t, "10", rec[1])
		assert.Equal(t, "WRITE", rec[3])
	})
}

func TestExecutionRow(t *testing.T) {
	t.Run("Record with non-None exit code", func(t *testing.T) {
		r := ExecutionRow{
			ID:           "exec-1",
			TimestampUTC: "2026-01-01T00:00:00Z",
			Command:      "echo hello",
			ExitCode:     0,
			DurationMs:   50,
			StdoutHash:   "out-hash",
			StdoutSize:   "11",
			OperatorID:   "op-1",
		}
		rec := r.Record()
		require.Len(t, rec, 12)
		assert.Equal(t, "exec-1", rec[0])
		assert.Equal(t, "0", rec[3])
		assert.Equal(t, "50", rec[4])
	})

	t.Run("Record with ExitCodeNone produces empty string", func(t *testing.T) {
		r := ExecutionRow{
			ID:       "exec-2",
			ExitCode: constants.ExitCodeNone,
		}
		rec := r.Record()
		assert.Equal(t, "", rec[3])
	})
}

func TestFileDiffRow(t *testing.T) {
	t.Run("Columns and Record match", func(t *testing.T) {
		r := FileDiffRow{
			ID:               "diff-1",
			TimestampUTC:     "2026-01-01T00:00:00Z",
			FilePath:         "/test/file",
			Operation:        "CREATE",
			LedgerHashBefore: "before",
			LedgerHashAfter:  "after",
			DiffStat:         "1 file",
			DiffHash:         "diff-hash",
			DiffSize:         "100",
			SessionID:        "sess-1",
			CaseID:           "case-1",
			OperatorID:       "op-1",
		}
		cols := FileDiffRow{}.Columns()
		rec := r.Record()
		require.Len(t, cols, 12)
		require.Len(t, rec, 12)
		assert.Equal(t, "diff-1", rec[0])
		assert.Equal(t, "CREATE", rec[3])
	})
}

func TestCommitmentRow(t *testing.T) {
	t.Run("Columns and Record match", func(t *testing.T) {
		r := CommitmentRow{
			Seq:                 1,
			CommittedAtUTC:      "2026-01-01T00:00:00Z",
			TransactionID:       "tx-1",
			TransactionHash:     "hash-1",
			PriorCommitmentHash: "prior-1",
			Hash:                "commit-hash-1",
			StateRootAtCommit:   "root-1",
			ActionType:          "FS_WRITE",
			TargetResource:      "/file",
			AuditorKeyID:        "key-1",
			Signature:           "sig-1",
		}
		cols := CommitmentRow{}.Columns()
		rec := r.Record()
		require.Len(t, cols, 11)
		require.Len(t, rec, 11)
		assert.Equal(t, "1", rec[0])
		assert.Equal(t, "tx-1", rec[2])
	})
}

func TestLedgerCommitRow(t *testing.T) {
	t.Run("Columns and Record match", func(t *testing.T) {
		r := LedgerCommitRow{
			CommitHash:   "abc123",
			ParentHash:   "def456",
			TimestampUTC: "2026-01-01T00:00:00Z",
			Message:      "test commit",
			FilesChanged: "2",
			DiffStat:     "2 files changed",
		}
		cols := LedgerCommitRow{}.Columns()
		rec := r.Record()
		require.Len(t, cols, 6)
		require.Len(t, rec, 6)
		assert.Equal(t, "abc123", rec[0])
		assert.Equal(t, "test commit", rec[3])
	})
}

func TestLedgerMerkleRootRow(t *testing.T) {
	t.Run("Columns and Record match", func(t *testing.T) {
		r := LedgerMerkleRootRow{
			MerkleRoot:    "root-hash",
			CapturedAtUTC: "2026-01-01T00:00:00Z",
		}
		cols := LedgerMerkleRootRow{}.Columns()
		rec := r.Record()
		require.Len(t, cols, 2)
		require.Len(t, rec, 2)
		assert.Equal(t, "root-hash", rec[0])
	})
}

func TestReplayNonceRow(t *testing.T) {
	t.Run("Columns and Record match", func(t *testing.T) {
		r := ReplayNonceRow{
			Nonce:         "nonce-123",
			Status:        "used",
			ReservedAtUTC: "2026-01-01T00:00:00Z",
			UsedAtUTC:     "2026-01-01T00:01:00Z",
			ExpiresAtUTC:  "2026-01-01T00:05:00Z",
		}
		cols := ReplayNonceRow{}.Columns()
		rec := r.Record()
		require.Len(t, cols, 5)
		require.Len(t, rec, 5)
		assert.Equal(t, "nonce-123", rec[0])
		assert.Equal(t, "used", rec[1])
	})
}

func TestSuspendedTxRow(t *testing.T) {
	t.Run("Columns and Record match", func(t *testing.T) {
		r := SuspendedTxRow{
			TxHash:            "tx-hash",
			UserID:            "user-1",
			ActionType:        "FS_WRITE",
			TargetResource:    "/file",
			Status:            "pending",
			CreatedAtUTC:      "2026-01-01T00:00:00Z",
			ExpiresAtUTC:      "2026-01-01T01:00:00Z",
			ApprovedBy:        "admin",
			ApprovalSignature: "sig",
		}
		cols := SuspendedTxRow{}.Columns()
		rec := r.Record()
		require.Len(t, cols, 9)
		require.Len(t, rec, 9)
		assert.Equal(t, "tx-hash", rec[0])
		assert.Equal(t, "pending", rec[4])
	})
}

func TestVerificationRow(t *testing.T) {
	t.Run("Columns and Record match", func(t *testing.T) {
		r := VerificationRow{
			Check:   "commitment_chain",
			Scope:   "commitment_ledger",
			Subject: "all",
			Result:  "PASS",
			Detail:  "5 commitments verified",
		}
		cols := VerificationRow{}.Columns()
		rec := r.Record()
		require.Len(t, cols, 5)
		require.Len(t, rec, 5)
		assert.Equal(t, "PASS", rec[3])
	})
}

func TestManifestRow(t *testing.T) {
	t.Run("Columns and Record match", func(t *testing.T) {
		r := ManifestRow{
			File:           "receipts.csv",
			RecordType:     "receipts",
			RowCount:       "10",
			SHA256:         "abc123",
			GeneratedAtUTC: "2026-01-01T00:00:00Z",
			VaultUnlocked:  "true",
		}
		cols := ManifestRow{}.Columns()
		rec := r.Record()
		require.Len(t, cols, 6)
		require.Len(t, rec, 6)
		assert.Equal(t, "receipts.csv", rec[0])
		assert.Equal(t, "true", rec[5])
	})
}
