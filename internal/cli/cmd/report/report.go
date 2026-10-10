// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/paths"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/reporting"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "report",
		Aliases: []string{"reports"},
		Short:   "Generate CSV evidence reports from all persistent stores",
		Long: `Generate flat, deterministic CSV files from every g8e persistent store.
Each file contains one record type with cryptographic proof fields.
A verification pass independently re-validates receipt signatures,
the commitment hash chain, and the git merkle root.`,
	}

	cmd.AddCommand(
		reportAllCmd(),
		reportVerifyCmd(),
	)

	return cmd
}

type reportFlags struct {
	outDir     string
	dataDir    string
	runtimeDir string
	ledgerDir  string
	// masterKeyFile selects an operator-provisioned master key instead of the
	// OS key store, matching the runtime that wrote the vault.
	masterKeyFile string
}

func (f *reportFlags) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.outDir, "out", "", "Output directory (default: reports/<timestamp>)")
	cmd.Flags().StringVar(&f.dataDir, "data-dir", "", "Data directory (default: "+paths.Infra.DataDir+")")
	cmd.Flags().StringVar(&f.runtimeDir, "runtime-dir", "", "Runtime directory (default: "+paths.Infra.RuntimeDir+")")
	cmd.Flags().StringVar(&f.ledgerDir, "ledger-dir", "", "Ledger base directory (default: <runtime-dir>/data/ledger)")
	cmd.Flags().StringVar(&f.masterKeyFile, "master-key-file", "", "Absolute path to a provisioned base64 32-byte master key outside the runtime root; Linux mode 0400 or 0600. Nonblank flag overrides G8E_MASTER_KEY_FILE; blank uses environment, then OS key store. No fallback on failure.")
}

func (f *reportFlags) resolveOptions() (reporting.Options, error) {
	runtimeDir := f.runtimeDir
	if runtimeDir == "" {
		runtimeDir = paths.Infra.RuntimeDir
	}
	runtimeDir, err := filepath.Abs(runtimeDir)
	if err != nil {
		return reporting.Options{}, fmt.Errorf("%w: runtime dir: %w", constants.ErrPathValidation, err)
	}
	baseDir := filepath.Dir(runtimeDir)
	expectedDataDir := filepath.Join(runtimeDir, constants.DataDirname)
	if f.dataDir != "" {
		dataDir, absErr := filepath.Abs(f.dataDir)
		if absErr != nil || dataDir != expectedDataDir {
			return reporting.Options{}, fmt.Errorf("%w: data dir must be %s for runtime %s", constants.ErrPathValidation, expectedDataDir, runtimeDir)
		}
	}
	expectedLedgerDir := filepath.Join(expectedDataDir, constants.LedgerDirname)
	if f.ledgerDir != "" {
		ledgerDir, absErr := filepath.Abs(f.ledgerDir)
		if absErr != nil || ledgerDir != expectedLedgerDir {
			return reporting.Options{}, fmt.Errorf("%w: ledger dir must be %s for runtime %s", constants.ErrPathValidation, expectedLedgerDir, runtimeDir)
		}
	}
	outDir := f.outDir
	if outDir == "" {
		outDir = filepath.Join(constants.ReportsDirname, time.Now().UTC().Format("2006-01-02T150405Z"))
	}

	fileSvc, err := fs.NewRuntimeFileService(baseDir, slog.Default())
	if err != nil {
		return reporting.Options{}, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	// The keystore is opened without Initialize so a report never generates a
	// master key. Without a usable key store the report is metadata-only.
	ks, err := keystore.NewWithFS(fileSvc, slog.Default(), keystore.Options{MasterKeyFile: f.masterKeyFile})
	if err != nil {
		slog.Default().Warn("Keystore unavailable; report will be metadata-only", "error", err)
		ks = nil
	}

	return reporting.Options{
		FileSvc:  fileSvc,
		Keystore: ks,
		OutDir:   outDir,
		Logger:   slog.Default(),
	}, nil
}

func reportAllCmd() *cobra.Command {
	var f reportFlags

	cmd := &cobra.Command{
		Use:   "all",
		Short: "Export all stores to CSV and run verification",
		Long: `Export all persistent stores (audit vault, receipts, ledger, secrets) to
deterministic CSV files and run verification checks. Output files are written
to the reports directory.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := f.resolveOptions()
			if err != nil {
				return err
			}

			result, err := reporting.Run(cmd.Context(), opts)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrReportWriteFailed, err)
			}

			cmd.Printf("Reports written to: %s\n", result.OutDir)
			cmd.Printf("Vault unlocked: %v\n", result.VaultUnlocked)
			cmd.Printf("Files written: %d\n", len(result.Files))

			if result.FailCount > 0 {
				cmd.Printf("Verification FAILED: %d check(s) failed\n", result.FailCount)
				return fmt.Errorf("%w: %d verification check(s) failed", constants.ErrReportVerificationFailed, result.FailCount)
			}

			cmd.Println("Verification PASSED")
			return nil
		},
	}

	f.addFlags(cmd)
	return cmd
}

func reportVerifyCmd() *cobra.Command {
	var f reportFlags

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Run verification checks and write verification_summary.csv",
		Long: `Run verification checks against the persistent stores and write a
verification_summary.csv file to the reports directory. Checks include hash
integrity, receipt signature validation, and ledger consistency.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := f.resolveOptions()
			if err != nil {
				return err
			}

			result, err := reporting.Run(cmd.Context(), opts)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrReportWriteFailed, err)
			}

			if result.FailCount > 0 {
				cmd.Printf("Verification FAILED: %d check(s) failed — see %s/verification_summary.csv\n",
					result.FailCount, result.OutDir)
				return fmt.Errorf("%w: %d verification check(s) failed", constants.ErrReportVerificationFailed, result.FailCount)
			}

			cmd.Printf("Verification PASSED — see %s/verification_summary.csv\n", result.OutDir)
			return nil
		},
	}

	f.addFlags(cmd)
	return cmd
}
