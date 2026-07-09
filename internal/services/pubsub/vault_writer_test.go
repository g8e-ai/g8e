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
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutionVault is a simple mock for testing VaultWriter
type mockExecutionVault struct{}

func (m *mockExecutionVault) StoreExecution(ctx context.Context, record *models.ExecutionRecord) error {
	return nil
}

func (m *mockExecutionVault) GetExecution(ctx context.Context, executionID string) (*models.ExecutionRecord, error) {
	return nil, nil
}

func (m *mockExecutionVault) StoreFileDiff(ctx context.Context, record *models.FileDiffRecord) error {
	return nil
}

func (m *mockExecutionVault) GetFileDiff(ctx context.Context, diffID string) (*models.FileDiffRecord, error) {
	return nil, nil
}

func (m *mockExecutionVault) GetFileDiffsBySession(ctx context.Context, operatorSessionID string, limit int) ([]*models.FileDiffRecord, error) {
	return nil, nil
}

func (m *mockExecutionVault) Close() error {
	return nil
}

func (m *mockExecutionVault) Wait() {}

// configurableExecutionVault is a mock where each method's return values can be set.
type configurableExecutionVault struct {
	storeExecutionErr     error
	storeFileDiffErr      error
	getFileDiffResult     *models.FileDiffRecord
	getFileDiffErr        error
	getFileDiffsBySession []*models.FileDiffRecord
	getFileDiffsBySessErr error
}

func (m *configurableExecutionVault) StoreExecution(ctx context.Context, record *models.ExecutionRecord) error {
	return m.storeExecutionErr
}

func (m *configurableExecutionVault) GetExecution(ctx context.Context, executionID string) (*models.ExecutionRecord, error) {
	return nil, nil
}

func (m *configurableExecutionVault) StoreFileDiff(ctx context.Context, record *models.FileDiffRecord) error {
	return m.storeFileDiffErr
}

func (m *configurableExecutionVault) GetFileDiff(ctx context.Context, diffID string) (*models.FileDiffRecord, error) {
	return m.getFileDiffResult, m.getFileDiffErr
}

func (m *configurableExecutionVault) GetFileDiffsBySession(ctx context.Context, operatorSessionID string, limit int) ([]*models.FileDiffRecord, error) {
	return m.getFileDiffsBySession, m.getFileDiffsBySessErr
}

func (m *configurableExecutionVault) Close() error {
	return nil
}

func (m *configurableExecutionVault) Wait() {}

func TestNewVaultWriter(t *testing.T) {
	t.Run("creates service successfully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewVaultWriter(cfg, logger, nil)
		require.NotNil(t, svc)
		assert.Equal(t, cfg, svc.config)
		assert.Equal(t, logger, svc.logger)
	})

	t.Run("creates service with all dependencies", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockVault := &mockExecutionVault{}
		svc := NewVaultWriter(cfg, logger, mockVault)
		require.NotNil(t, svc)
		assert.Equal(t, mockVault, svc.executionVault)
	})
}

func TestVaultWriter_WriteExecution(t *testing.T) {
	t.Run("handles nil vault gracefully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewVaultWriter(cfg, logger, nil)

		params := executionWriteParams{
			id:         "exec-1",
			command:    "ls -la",
			exitCode:   0,
			durationMs: 1000,
			stdout:     "file1\nfile2",
			stderr:     "",
			vaultMode:  constants.VaultModeRaw,
		}

		svc.WriteExecution(context.Background(), params)
		// Should not panic
	})

	t.Run("writes execution with vault", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockVault := &mockExecutionVault{}
		svc := NewVaultWriter(cfg, logger, mockVault)

		params := executionWriteParams{
			id:              "exec-1",
			command:         "ls -la",
			exitCode:        0,
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

		svc.WriteExecution(context.Background(), params)
		// Should attempt to write (will fail due to mock, but should not panic)
	})
}

func TestVaultWriter_WriteFileDiff(t *testing.T) {
	t.Run("handles nil vault gracefully", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewVaultWriter(cfg, logger, nil)

		params := fileDiffWriteParams{
			diffID:           "diff-1",
			filePath:         filepath.Join(tmpDir, "test.txt"),
			operation:        "write",
			ledgerHashBefore: "hash-before",
			ledgerHashAfter:  "hash-after",
			diffStat:         "1 file changed",
			diffContent:      "diff content",
		}

		svc.WriteFileDiff(context.Background(), params)
		// Should not panic
	})

	t.Run("writes file diff with vault", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockVault := &mockExecutionVault{}
		svc := NewVaultWriter(cfg, logger, mockVault)

		params := fileDiffWriteParams{
			diffID:            "diff-1",
			filePath:          filepath.Join(tmpDir, "test.txt"),
			operation:         "write",
			ledgerHashBefore:  "hash-before",
			ledgerHashAfter:   "hash-after",
			diffStat:          "1 file changed",
			diffContent:       "diff content",
			caseID:            "case-1",
			operatorSessionID: "session-1",
		}

		svc.WriteFileDiff(context.Background(), params)
		// Should attempt to write
	})
}

func TestVaultWriter_WriteExecution_VaultError(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	failingVault := &configurableExecutionVault{
		storeExecutionErr: fmt.Errorf("disk full"),
	}
	svc := NewVaultWriter(cfg, logger, failingVault)

	params := executionWriteParams{
		id:         "exec-err",
		command:    "ls",
		exitCode:   0,
		durationMs: 10,
		stdout:     "out",
		vaultMode:  constants.VaultModeRaw,
	}

	svc.WriteExecution(context.Background(), params)
}

func TestVaultWriter_WriteFileDiff_VaultError(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	failingVault := &configurableExecutionVault{
		storeFileDiffErr: fmt.Errorf("disk full"),
	}
	svc := NewVaultWriter(cfg, logger, failingVault)

	params := fileDiffWriteParams{
		diffID:      "diff-err",
		filePath:    filepath.Join(tmpDir, "test.txt"),
		operation:   "write",
		diffContent: "diff",
	}

	svc.WriteFileDiff(context.Background(), params)
}

func TestVaultWriter_StoreFileDiffFromLedger(t *testing.T) {
	t.Run("skips when ledger is nil", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewVaultWriter(cfg, logger, nil)

		svc.StoreFileDiffFromLedger(context.Background(), filepath.Join(tmpDir, "test.txt"), "write", "event-1", "session-1", "case-1", nil)
		// Should not panic
	})

	t.Run("handles insufficient history", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		ledger := &storage.GitLedgerService{}
		svc := NewVaultWriter(cfg, logger, nil)

		svc.StoreFileDiffFromLedger(context.Background(), filepath.Join(tmpDir, "test.txt"), "write", "event-1", "session-1", "case-1", ledger)
		// Should handle gracefully
	})
}
