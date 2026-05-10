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
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	sentinel "github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
)

// VaultWriter owns dual-vault persistence for command executions and file diffs.
// Raw vault receives unscrubbed data; scrubbed vault receives sentinel-processed data.
// Both writes are best-effort — failures are logged but never propagate to callers.
type VaultWriter struct {
	config     *config.Config
	logger     *slog.Logger
	sentinel   *sentinel.Sentinel
	rawVault   *storage.RawVaultService
	localStore *storage.LocalStoreService
}

// NewVaultWriter creates a VaultWriter. All service dependencies are optional — a nil
// service is treated as disabled, matching the IsEnabled() pattern used elsewhere.
func NewVaultWriter(
	cfg *config.Config,
	logger *slog.Logger,
	s *sentinel.Sentinel,
	rawVault *storage.RawVaultService,
	localStore *storage.LocalStoreService,
) *VaultWriter {
	return &VaultWriter{
		config:     cfg,
		logger:     logger,
		sentinel:   s,
		rawVault:   rawVault,
		localStore: localStore,
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
	vaultMode       string
}

// WriteExecution persists a command execution result to the dual vault.
// Raw vault write is skipped when vaultMode == scrubbed.
// Scrubbed vault always receives sentinel-processed output.
func (vw *VaultWriter) WriteExecution(p executionWriteParams) {
	if vw.rawVault != nil && vw.rawVault.IsEnabled() && p.vaultMode != constants.Status.VaultMode.Scrubbed {
		rawRecord := &storage.RawExecutionRecord{
			ID:               p.id,
			TimestampUTC:     time.Now().UTC(),
			Command:          p.command,
			ExitCode:         p.exitCode,
			DurationMs:       p.durationMs,
			StdoutCompressed: []byte(p.stdout),
			StderrCompressed: []byte(p.stderr),
			StdoutHash:       vw.rawVault.HashString(p.stdout),
			StderrHash:       vw.rawVault.HashString(p.stderr),
			StdoutSize:       p.stdoutSize,
			StderrSize:       p.stderrSize,
			CaseID:           p.caseID,
			TaskID:           p.taskID,
			InvestigationID:  p.investigationID,
			OperatorID:       vw.config.OperatorID,
		}
		if err := vw.rawVault.StoreRawExecution(rawRecord); err != nil {
			vw.logger.Warn("Failed to store raw execution in raw vault", "error", err)
		} else {
			vw.logger.Info("Raw execution stored in raw vault (customer data)",
				"execution_id", p.id,
				"stdout_size", p.stdoutSize,
				"stderr_size", p.stderrSize)
		}
	} else if p.vaultMode == constants.Status.VaultMode.Scrubbed {
		vw.logger.Info("Raw vault storage skipped (sentinel_mode=scrubbed)", "execution_id", p.id)
	}

	if vw.localStore != nil && vw.localStore.IsEnabled() {
		scrubbedStdout := p.stdout
		scrubbedStderr := p.stderr
		if vw.sentinel != nil && vw.sentinel.IsEnabled() {
			scrubbedStdout = vw.sentinel.ScrubText(p.stdout)
			scrubbedStderr = vw.sentinel.ScrubText(p.stderr)
		}

		execRecord := &storage.ExecutionRecord{
			ID:               p.id,
			TimestampUTC:     time.Now().UTC(),
			Command:          p.command,
			ExitCode:         p.exitCode,
			DurationMs:       p.durationMs,
			StdoutCompressed: []byte(scrubbedStdout),
			StderrCompressed: []byte(scrubbedStderr),
			StdoutSize:       p.stdoutSize,
			StderrSize:       p.stderrSize,
			CaseID:           p.caseID,
			TaskID:           p.taskID,
			InvestigationID:  p.investigationID,
			OperatorID:       vw.config.OperatorID,
		}
		if err := vw.localStore.StoreExecution(execRecord); err != nil {
			vw.logger.Warn("Failed to store scrubbed execution in scrubbed vault", "error", err)
		} else {
			vw.logger.Info("Scrubbed execution stored in scrubbed vault (AI-accessible)",
				"execution_id", p.id,
				"stdout_size", p.stdoutSize,
				"scrubbed_stdout_size", len(scrubbedStdout))
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

// WriteFileDiff persists a file diff to the dual vault.
func (vw *VaultWriter) WriteFileDiff(p fileDiffWriteParams) {
	if vw.rawVault != nil && vw.rawVault.IsEnabled() {
		rawRecord := &storage.RawFileDiffRecord{
			ID:                p.diffID,
			TimestampUTC:      p.timestamp,
			FilePath:          p.filePath,
			Operation:         p.operation,
			LedgerHashBefore:  p.ledgerHashBefore,
			LedgerHashAfter:   p.ledgerHashAfter,
			DiffStat:          p.diffStat,
			DiffCompressed:    []byte(p.diffContent),
			DiffHash:          vw.rawVault.HashString(p.diffContent),
			DiffSize:          len(p.diffContent),
			OperatorSessionID: p.operatorSessionID,
			CaseID:            p.caseID,
			OperatorID:        vw.config.OperatorID,
		}
		if err := vw.rawVault.StoreRawFileDiff(rawRecord); err != nil {
			vw.logger.Warn("Failed to store raw file diff in raw vault", "error", err)
		} else {
			vw.logger.Info("Raw file diff stored in raw vault (customer data)",
				"diff_id", p.diffID,
				"file_path", p.filePath,
				"diff_size", len(p.diffContent))
		}
	}

	if vw.localStore != nil && vw.localStore.IsEnabled() {
		scrubbedDiff := p.diffContent
		if vw.sentinel != nil && vw.sentinel.IsEnabled() {
			scrubbedDiff = vw.sentinel.ScrubText(p.diffContent)
		}

		scrubbedRecord := &storage.FileDiffRecord{
			ID:                p.diffID,
			TimestampUTC:      p.timestamp,
			FilePath:          p.filePath,
			Operation:         p.operation,
			LedgerHashBefore:  p.ledgerHashBefore,
			LedgerHashAfter:   p.ledgerHashAfter,
			DiffStat:          p.diffStat,
			DiffCompressed:    []byte(scrubbedDiff),
			DiffSize:          len(p.diffContent),
			OperatorSessionID: p.operatorSessionID,
			CaseID:            p.caseID,
			OperatorID:        vw.config.OperatorID,
		}
		if err := vw.localStore.StoreFileDiff(scrubbedRecord); err != nil {
			vw.logger.Warn("Failed to store scrubbed file diff in scrubbed vault", "error", err)
		} else {
			vw.logger.Info("Scrubbed file diff stored in scrubbed vault (AI-accessible)",
				"diff_id", p.diffID,
				"file_path", p.filePath,
				"raw_size", len(p.diffContent),
				"scrubbed_size", len(scrubbedDiff))
		}
	}
}

// StoreFileDiffFromLedger fetches the two most recent ledger commits for filePath, computes
// the diff, and writes it to both vaults. Called after a successful file mutation audit event.
func (vw *VaultWriter) StoreFileDiffFromLedger(filePath, operation, eventID, operatorSessionID, caseID string, ledger *storage.LedgerService) {
	if ledger == nil {
		return
	}

	history, err := ledger.GetFileHistory(filePath, 2)
	if err != nil || len(history) < 2 {
		vw.logger.Info("No file history available for diff computation",
			"file_path", filePath,
			"history_len", len(history))
		return
	}

	hashBefore := history[1].CommitHash
	hashAfter := history[0].CommitHash

	diffContent := ledger.GetDiffContent(hashBefore, hashAfter)
	if diffContent == "" {
		vw.logger.Info("No diff content available", "file_path", filePath)
		return
	}

	vw.WriteFileDiff(fileDiffWriteParams{
		diffID:            fmt.Sprintf("diff_%s_%d", eventID, time.Now().UnixNano()),
		timestamp:         time.Now().UTC(),
		filePath:          filePath,
		operation:         operation,
		ledgerHashBefore:  hashBefore,
		ledgerHashAfter:   hashAfter,
		diffStat:          ledger.GetDiffStat(hashBefore, hashAfter),
		diffContent:       diffContent,
		caseID:            caseID,
		operatorSessionID: operatorSessionID,
	})
}
