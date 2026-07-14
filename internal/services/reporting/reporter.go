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

// Package reporting provides read-only CSV export of all g8e persistent stores.
// It emits one file per logical record type plus a cryptographic verification pass.
package reporting

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pathutil"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
)

// Options configures a reporting run.
type Options struct {
	// DataDir is the .g8e/data directory (audit store + commitment ledger).
	DataDir string
	// RuntimeDir is the .g8e runtime directory (execution vault, replay store).
	RuntimeDir string
	// LedgerDir is the base directory for the git ledger (default: RuntimeDir/ledger).
	LedgerDir string
	// VaultDir is the vault data directory (for DEK key).
	VaultDir string
	// VaultKeyPath is the path to the hex-encoded vault key file.
	VaultKeyPath string
	// OutDir is the directory to write CSV files into.
	OutDir string
	// ExecutionVaultDBPath is the precomputed path to the execution vault DB.
	// If empty, defaults to RuntimeDir/execution_vault.db.
	ExecutionVaultDBPath string
	// ReplayStoreDBPath is the precomputed path to the replay store DB.
	// If empty, defaults to RuntimeDir/replay_store.db.
	ReplayStoreDBPath string
	// SuspendedTransactionDBPath is the precomputed path to the suspended transaction DB.
	// If empty, defaults to DataDir/suspended_transactions.db.
	SuspendedTransactionDBPath string
	// Logger is optional; if nil, slog.Default() is used.
	Logger *slog.Logger
}

// RunResult summarises a completed reporting run.
type RunResult struct {
	OutDir        string
	VaultUnlocked bool
	FailCount     int
	Files         []FileResult
}

// Run performs a full CSV export of all stores and returns a RunResult.
// A non-zero RunResult.FailCount means at least one verification check failed.
func Run(ctx context.Context, opts Options) (RunResult, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if err := os.MkdirAll(opts.OutDir, 0755); err != nil {
		return RunResult{}, fmt.Errorf("%w: %s: %w", constants.ErrReportOutputDirFailed, opts.OutDir, err)
	}

	// Construct fileSvc for .g8e/ file I/O. Derive base dir from opts.DataDir
	// (which is <base>/.g8e/data) so the audit store opens the same DB that
	// the commitment ledger accesses via opts.DataDir directly.
	baseDir := ""
	if opts.DataDir != "" {
		baseDir = filepath.Dir(filepath.Dir(opts.DataDir))
	}
	fileSvc, err := fs.NewRuntimeFileService(baseDir, logger)
	if err != nil {
		return RunResult{}, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	// Open vault (locked or unlocked).
	v, vaultUnlocked := openVault(opts.VaultDir, opts.VaultKeyPath, logger)

	// Open audit store (sessions, events, file_mutations, receipts, commitment_ledger).
	auditStoreCfg := storage.DefaultAuditStoreConfig()
	auditStoreCfg.EncryptionVault = v
	auditStore, err := storage.NewSQLAuditStore(auditStoreCfg, logger, fileSvc)
	if err != nil {
		return RunResult{}, fmt.Errorf("%w: audit store: %w", constants.ErrReportStoreUnavailable, err)
	}
	defer auditStore.Close()

	// Open commitment ledger (shares g8e.db with audit store — open separately read-only).
	dbPath := pathutil.ResolveDBPath(opts.DataDir, constants.DbFilename)
	mainDB, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), logger)
	if err != nil {
		return RunResult{}, fmt.Errorf("%w: main db: %w", constants.ErrReportStoreUnavailable, err)
	}
	defer mainDB.Close()
	cl := storage.NewCommitmentLedger(mainDB, logger)

	// Open execution vault.
	evCfg := storage.DefaultExecutionVaultConfig()
	if opts.ExecutionVaultDBPath != "" {
		evCfg.DBPath = opts.ExecutionVaultDBPath
	} else {
		evCfg.DBPath = filepath.Join(opts.RuntimeDir, constants.ExecutionVaultDBFilename)
	}
	ev, evErr := storage.NewExecutionVaultService(evCfg, logger, v)
	if evErr != nil {
		logger.Warn("Execution vault unavailable; executions and file_diffs will be skipped", "error", evErr)
		ev = nil
	}
	if ev != nil {
		defer ev.Close()
	}

	// Open replay store.
	rsCfg := storage.DefaultReplayStoreConfig()
	if opts.ReplayStoreDBPath != "" {
		rsCfg.DBPath = opts.ReplayStoreDBPath
	} else {
		rsCfg.DBPath = filepath.Join(opts.RuntimeDir, constants.ReplayStoreDBFilename)
	}
	rs, rsErr := storage.NewSQLReplayStore(rsCfg, logger)
	if rsErr != nil {
		logger.Warn("Replay store unavailable; replay_nonces will be skipped", "error", rsErr)
		rs = nil
	}
	if rs != nil {
		defer rs.Close()
	}

	// Open suspended transaction store.
	stsCfg := storage.DefaultSuspendedTransactionConfig()
	if opts.SuspendedTransactionDBPath != "" {
		stsCfg.DBPath = opts.SuspendedTransactionDBPath
	} else {
		stsCfg.DBPath = filepath.Join(opts.DataDir, constants.SuspendedTxFilename)
	}
	sts, stsErr := storage.NewSuspendedTransactionService(stsCfg, logger)
	if stsErr != nil {
		logger.Warn("Suspended transaction store unavailable; suspended_transactions will be skipped", "error", stsErr)
		sts = nil
	}
	if sts != nil {
		defer sts.Close()
	}

	// Open git ledger.
	var ledger *storage.GitLedgerService
	ledgerCfg := &storage.LedgerConfig{
		GitPath:         "git",
		EncryptionVault: v,
	}
	ledger, ledgerErr := storage.NewGitLedgerService(ledgerCfg, logger, fileSvc)
	if ledgerErr != nil {
		logger.Warn("Git ledger unavailable; ledger reports will be skipped", "error", ledgerErr)
		ledger = nil
	}

	now := time.Now().UTC()
	var allFiles []FileResult

	run := func(name string, fn func() (FileResult, error)) {
		if ctx.Err() != nil {
			return
		}
		res, err := fn()
		if err != nil {
			logger.Warn("Reporter failed", "name", name, "error", err)
			return
		}
		logger.Info("Wrote CSV", "file", res.Filename, "rows", res.RowCount)
		allFiles = append(allFiles, res)
	}

	// Per-store reporters.
	run("receipts", func() (FileResult, error) { return reportReceipts(ctx, opts.OutDir, auditStore) })
	run("sessions", func() (FileResult, error) { return reportSessions(ctx, opts.OutDir, auditStore) })
	run("events", func() (FileResult, error) { return reportEvents(ctx, opts.OutDir, auditStore) })
	run("file_mutations", func() (FileResult, error) { return reportFileMutations(ctx, opts.OutDir, auditStore) })
	run("commitments", func() (FileResult, error) { return reportCommitments(ctx, opts.OutDir, cl) })

	if ev != nil {
		run("executions", func() (FileResult, error) { return reportExecutions(ctx, opts.OutDir, ev) })
		run("file_diffs", func() (FileResult, error) { return reportFileDiffs(ctx, opts.OutDir, ev) })
	}

	if rs != nil {
		run("replay_nonces", func() (FileResult, error) { return reportReplayNonces(ctx, opts.OutDir, rs) })
	}

	if sts != nil {
		run("suspended_transactions", func() (FileResult, error) {
			return reportSuspendedTransactions(ctx, opts.OutDir, sts)
		})
	}

	if ledger != nil {
		run("ledger_merkle_root", func() (FileResult, error) { return reportLedgerMerkleRoot(ctx, opts.OutDir, ledger) })
		run("ledger_commits", func() (FileResult, error) { return reportLedgerCommits(ctx, opts.OutDir, ledger) })
	}

	// Verification pass.
	var failCount int
	if ctx.Err() == nil {
		verRes, vr, verErr := reportVerification(ctx, opts.OutDir, auditStore, cl, ledger)
		if verErr != nil {
			logger.Warn("Verification reporter failed", "error", verErr)
		} else {
			allFiles = append(allFiles, verRes)
			failCount = vr.FailCount
			logger.Info("Verification complete", "fail_count", failCount)
		}
	}

	// Manifest.
	if ctx.Err() == nil {
		manifestPath := filepath.Join(opts.OutDir, constants.ReportManifestFilename)
		var mRows []Row
		for _, f := range allFiles {
			mRows = append(mRows, ManifestRow{
				File:           f.Filename,
				RecordType:     recordTypeForFile(f.Filename),
				RowCount:       intStr(f.RowCount),
				SHA256:         f.SHA256,
				GeneratedAtUTC: utcRFC3339(now),
				VaultUnlocked:  boolStr(vaultUnlocked),
			})
		}
		mRes, mErr := writeCSV(manifestPath, ManifestRow{}.Columns(), mRows)
		if mErr != nil {
			logger.Warn("Failed to write manifest", "error", mErr)
		} else {
			mRes.Filename = constants.ReportManifestFilename
			allFiles = append(allFiles, mRes)
			logger.Info("Wrote manifest", "file", manifestPath, "entries", mRes.RowCount)
		}
	}

	return RunResult{
		OutDir:        opts.OutDir,
		VaultUnlocked: vaultUnlocked,
		FailCount:     failCount,
		Files:         allFiles,
	}, nil
}

// openVault tries to create and unlock the vault from the key file.
// Returns a (possibly locked) vault and whether it was successfully unlocked.
func openVault(vaultDir, keyPath string, logger *slog.Logger) (*vault.Vault, bool) {
	v, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: logger})
	if err != nil {
		logger.Warn("Could not create vault; proceeding without encryption", "error", err)
		return nil, false
	}

	if keyPath == "" {
		return v, false
	}

	data, err := os.ReadFile(keyPath)
	if err != nil {
		logger.Info("Vault key not found; proceeding in metadata-only mode", "path", keyPath)
		return v, false
	}

	keyHex := strings.TrimSpace(string(data))
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		logger.Warn("Invalid vault key encoding; proceeding in metadata-only mode", "error", err)
		return v, false
	}

	if err := v.Unlock(keyBytes); err != nil {
		logger.Warn("Vault unlock failed; proceeding in metadata-only mode", "error", err)
		return v, false
	}

	logger.Info("Vault unlocked; content columns will be populated")
	return v, true
}

// recordTypeForFile derives a human-readable record type from a CSV filename.
func recordTypeForFile(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}
