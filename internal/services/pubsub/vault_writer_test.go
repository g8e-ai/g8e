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

package pubsub

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutionVault is a simple mock for testing VaultWriter
type mockExecutionVault struct {
	enabled bool
}

func (m *mockExecutionVault) StoreExecution(record *models.ExecutionRecord) error {
	return nil
}

func (m *mockExecutionVault) GetExecution(executionID string) (*models.ExecutionRecord, error) {
	return nil, nil
}

func (m *mockExecutionVault) StoreFileDiff(record *models.FileDiffRecord) error {
	return nil
}

func (m *mockExecutionVault) GetFileDiff(diffID string) (*models.FileDiffRecord, error) {
	return nil, nil
}

func (m *mockExecutionVault) GetFileDiffsBySession(operatorSessionID string, limit int) ([]*models.FileDiffRecord, error) {
	return nil, nil
}

func (m *mockExecutionVault) IsEnabled() bool {
	return m.enabled
}

func (m *mockExecutionVault) IsEncryptionEnabled() bool {
	return true
}

func (m *mockExecutionVault) Close() error {
	return nil
}

func (m *mockExecutionVault) Wait() {}

func TestNewVaultWriter(t *testing.T) {
	t.Run("creates service successfully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewVaultWriter(cfg, logger, nil, nil)
		require.NotNil(t, svc)
		assert.Equal(t, cfg, svc.config)
		assert.Equal(t, logger, svc.logger)
	})

	t.Run("creates service with all dependencies", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockVault := &mockExecutionVault{enabled: true}
		svc := NewVaultWriter(cfg, logger, nil, mockVault)
		require.NotNil(t, svc)
		assert.Equal(t, mockVault, svc.executionVault)
	})
}

func TestVaultWriter_WriteExecution(t *testing.T) {
	t.Run("skips when local store not enabled", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewVaultWriter(cfg, logger, nil, nil)

		params := executionWriteParams{
			id:         "exec-1",
			command:    "ls -la",
			exitCode:   intPtr(0),
			durationMs: 1000,
			stdout:     "file1\nfile2",
			stderr:     "",
			vaultMode:  constants.VaultModeRaw,
		}

		svc.WriteExecution(params)
		// Should not panic
	})

	t.Run("writes execution when local store enabled", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockVault := &mockExecutionVault{enabled: true}
		svc := NewVaultWriter(cfg, logger, nil, mockVault)

		params := executionWriteParams{
			id:              "exec-1",
			command:         "ls -la",
			exitCode:        intPtr(0),
			durationMs:      1000,
			stdout:          "file1\nfile2",
			stderr:          "",
			stdoutSize:      12,
			stderrSize:      0,
			caseID:          "case-1",
			taskID:          "task-1",
			investigationID: "investigation-1",
			vaultMode:       constants.VaultModeRaw,
		}

		svc.WriteExecution(params)
		// Should attempt to write (will fail due to mock, but should not panic)
	})
}

func TestVaultWriter_WriteFileDiff(t *testing.T) {
	t.Run("skips when local store not enabled", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewVaultWriter(cfg, logger, nil, nil)

		params := fileDiffWriteParams{
			diffID:           "diff-1",
			filePath:         "/tmp/test.txt",
			operation:        "write",
			ledgerHashBefore: "hash-before",
			ledgerHashAfter:  "hash-after",
			diffStat:         "1 file changed",
			diffContent:      "diff content",
		}

		svc.WriteFileDiff(params)
		// Should not panic
	})

	t.Run("writes file diff when local store enabled", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockVault := &mockExecutionVault{enabled: true}
		svc := NewVaultWriter(cfg, logger, nil, mockVault)

		params := fileDiffWriteParams{
			diffID:            "diff-1",
			filePath:          "/tmp/test.txt",
			operation:         "write",
			ledgerHashBefore:  "hash-before",
			ledgerHashAfter:   "hash-after",
			diffStat:          "1 file changed",
			diffContent:       "diff content",
			caseID:            "case-1",
			operatorSessionID: "session-1",
		}

		svc.WriteFileDiff(params)
		// Should attempt to write
	})
}

func TestVaultWriter_StoreFileDiffFromLedger(t *testing.T) {
	t.Run("skips when ledger is nil", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewVaultWriter(cfg, logger, nil, nil)

		svc.StoreFileDiffFromLedger("/tmp/test.txt", "write", "event-1", "session-1", "case-1", nil)
		// Should not panic
	})

	t.Run("handles insufficient history", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		ledger := &storage.GitLedgerService{}
		svc := NewVaultWriter(cfg, logger, nil, nil)

		svc.StoreFileDiffFromLedger("/tmp/test.txt", "write", "event-1", "session-1", "case-1", ledger)
		// Should handle gracefully
	})
}

func intPtr(i int) *int {
	return &i
}
