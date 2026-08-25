// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	storage "github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
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
	t.Run("returns non-nil service with config and logger", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewVaultWriter(cfg, logger, nil)
		require.NotNil(t, svc)
		assert.Equal(t, cfg, svc.config)
		assert.Equal(t, logger, svc.logger)
	})

	t.Run("wires execution vault into service", func(t *testing.T) {
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
		tmpDir := testutil.TempDir(t)
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
		tmpDir := testutil.TempDir(t)
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
	tmpDir := testutil.TempDir(t)
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
		tmpDir := testutil.TempDir(t)
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewVaultWriter(cfg, logger, nil)

		svc.StoreFileDiffFromLedger(context.Background(), filepath.Join(tmpDir, "test.txt"), "write", "event-1", "session-1", "case-1", nil)
		// Should not panic
	})

	t.Run("handles insufficient history", func(t *testing.T) {
		t.Parallel()
		tmpDir := testutil.TempDir(t)
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		ledger := &storage.GitLedgerService{}
		svc := NewVaultWriter(cfg, logger, nil)

		svc.StoreFileDiffFromLedger(context.Background(), filepath.Join(tmpDir, "test.txt"), "write", "event-1", "session-1", "case-1", ledger)
		// Should handle gracefully
	})
}
