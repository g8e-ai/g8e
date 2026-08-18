// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/reporting"
)

func reportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate CSV evidence reports from all persistent stores",
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
}

func (f *reportFlags) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.outDir, "out", "", "Output directory (default: reports/<timestamp>)")
	cmd.Flags().StringVar(&f.dataDir, "data-dir", "", "Data directory (default: "+paths.Infra.DataDir+")")
	cmd.Flags().StringVar(&f.runtimeDir, "runtime-dir", "", "Runtime directory (default: "+paths.Infra.RuntimeDir+")")
	cmd.Flags().StringVar(&f.ledgerDir, "ledger-dir", "", "Ledger base directory (default: <runtime-dir>/ledger)")
}

func (f *reportFlags) resolveOptions() (reporting.Options, error) {
	if err := paths.Init(); err != nil {
		return reporting.Options{}, fmt.Errorf("%w: %w", constants.ErrPathValidation, err)
	}

	dataDir := f.dataDir
	if dataDir == "" {
		dataDir = paths.Infra.DataDir
	}
	runtimeDir := f.runtimeDir
	if runtimeDir == "" {
		runtimeDir = paths.Infra.RuntimeDir
	}
	outDir := f.outDir
	if outDir == "" {
		outDir = filepath.Join(constants.ReportsDirname, time.Now().UTC().Format("2006-01-02T150405Z"))
	}

	return reporting.Options{
		DataDir:                    dataDir,
		RuntimeDir:                 runtimeDir,
		LedgerDir:                  f.ledgerDir,
		VaultDir:                   paths.Infra.VaultDir,
		VaultKeyPath:               paths.Infra.VaultKeyPath,
		OutDir:                     outDir,
		ExecutionVaultDBPath:       paths.Infra.ExecutionVaultDBPath,
		ReplayStoreDBPath:          paths.Infra.ReplayStoreDBPath,
		SuspendedTransactionDBPath: paths.Infra.SuspendedTransactionsDBPath,
		Logger:                     slog.Default(),
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
