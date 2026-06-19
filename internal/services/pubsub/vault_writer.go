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
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
)

// VaultWriter owns consolidated vault persistence for command executions and file diffs.
// All data is encrypted at rest in the consolidated execution vault.
// Both writes are best-effort - failures are logged but never propagate to callers.
type VaultWriter struct {
	config         *config.Config
	logger         *slog.Logger
	executionVault storage.ExecutionVault
}

// NewVaultWriter creates a VaultWriter. The ExecutionVault is optional - a nil
// vault is treated as disabled, matching the IsEnabled() pattern used elsewhere.
func NewVaultWriter(
	cfg *config.Config,
	logger *slog.Logger,
	executionVault storage.ExecutionVault,
) *VaultWriter {
	return &VaultWriter{
		config:         cfg,
		logger:         logger,
		executionVault: executionVault,
	}
}

// executionWriteParams carries the fields shared between a command execution and a file
// operation when writing to the dual vault.
type executionWriteParams struct {
	id              string
	command         string
	exitCode        *int
	durationMs      int64
	stdout          string
	stderr          string
	stdoutSize      int
	stderrSize      int
	caseID          string
	taskID          string
	investigationID string
	vaultMode       constants.VaultMode
}

// WriteExecution persists a command execution result to the consolidated vault.
// All data is encrypted at rest.
func (vw *VaultWriter) WriteExecution(ctx context.Context, p executionWriteParams) {
	if vw.executionVault != nil {
		execRecord := &models.ExecutionRecord{
			ID:               p.id,
			TimestampUTC:     time.Now().UTC(),
			Command:          p.command,
			ExitCode:         p.exitCode,
			DurationMs:       p.durationMs,
			StdoutCompressed: []byte(p.stdout),
			StderrCompressed: []byte(p.stderr),
			StdoutSize:       p.stdoutSize,
			StderrSize:       p.stderrSize,
			CaseID:           p.caseID,
			TaskID:           p.taskID,
			InvestigationID:  p.investigationID,
			OperatorID:       vw.config.OperatorID,
		}
		if err := vw.executionVault.StoreExecution(ctx, execRecord); err != nil {
			vw.logger.Warn("vault_writer: failed to store execution in consolidated vault", "error", fmt.Errorf("store execution: %w", err))
		} else {
			vw.logger.Info("Execution stored in consolidated vault (encrypted at rest)",
				"execution_id", p.id,
				"stdout_size", p.stdoutSize,
				"stderr_size", p.stderrSize)
		}
	}
}

// fileDiffWriteParams carries the fields needed to persist a file diff to the dual vault.
type fileDiffWriteParams struct {
	diffID            string
	timestamp         time.Time
	filePath          string
	operation         string
	ledgerHashBefore  string
	ledgerHashAfter   string
	diffStat          string
	diffContent       string
	caseID            string
	operatorSessionID string
}

// WriteFileDiff persists a file diff to the consolidated vault.
// All data is encrypted at rest.
func (vw *VaultWriter) WriteFileDiff(ctx context.Context, p fileDiffWriteParams) {
	if vw.executionVault != nil {
		diffRecord := &models.FileDiffRecord{
			ID:                p.diffID,
			TimestampUTC:      p.timestamp,
			FilePath:          p.filePath,
			Operation:         p.operation,
			LedgerHashBefore:  p.ledgerHashBefore,
			LedgerHashAfter:   p.ledgerHashAfter,
			DiffStat:          p.diffStat,
			DiffCompressed:    []byte(p.diffContent),
			DiffSize:          len(p.diffContent),
			OperatorSessionID: p.operatorSessionID,
			CaseID:            p.caseID,
			OperatorID:        vw.config.OperatorID,
		}
		if err := vw.executionVault.StoreFileDiff(ctx, diffRecord); err != nil {
			vw.logger.Warn("vault_writer: failed to store file diff in consolidated vault", "error", fmt.Errorf("store file diff: %w", err))
		} else {
			vw.logger.Info("File diff stored in consolidated vault (encrypted at rest)",
				"diff_id", p.diffID,
				"file_path", p.filePath,
				"diff_size", len(p.diffContent))
		}
	}
}

// StoreFileDiffFromLedger fetches the two most recent ledger commits for filePath, computes
// the diff, and writes it to both vaults. Called after a successful file mutation audit event.
func (vw *VaultWriter) StoreFileDiffFromLedger(ctx context.Context, filePath, operation, eventID, operatorSessionID, caseID string, ledger *storage.GitLedgerService) {
	if ledger == nil {
		return
	}

	history, err := ledger.GetFileHistory(filePath, 2, operatorSessionID)
	if err != nil || len(history) < 2 {
		vw.logger.Info("No file history available for diff computation",
			"file_path", filePath,
			"history_len", len(history))
		return
	}

	hashBefore := history[1].CommitHash
	hashAfter := history[0].CommitHash

	diffContent := ledger.GetDiffContent(hashBefore, hashAfter, operatorSessionID)
	if diffContent == "" {
		vw.logger.Info("No diff content available", "file_path", filePath)
		return
	}

	vw.WriteFileDiff(ctx, fileDiffWriteParams{
		diffID:            fmt.Sprintf("diff_%s_%d", eventID, time.Now().UnixNano()),
		timestamp:         time.Now().UTC(),
		filePath:          filePath,
		operation:         operation,
		ledgerHashBefore:  hashBefore,
		ledgerHashAfter:   hashAfter,
		diffStat:          ledger.GetDiffStat(hashBefore, hashAfter, operatorSessionID),
		diffContent:       diffContent,
		caseID:            caseID,
		operatorSessionID: operatorSessionID,
	})
}
