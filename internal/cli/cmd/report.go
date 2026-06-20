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

package cmd

import (
	"fmt"
	"log/slog"
	"os"
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

func reportAllCmd() *cobra.Command {
	var outDir string
	var dataDir string
	var runtimeDir string
	var ledgerDir string

	cmd := &cobra.Command{
		Use:   "all",
		Short: "Export all stores to CSV and run verification",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := paths.Init(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPathValidation, err)
			}

			if dataDir == "" {
				dataDir = paths.Infra.DataDir
			}
			if runtimeDir == "" {
				runtimeDir = paths.Infra.RuntimeDir
			}
			if outDir == "" {
				outDir = filepath.Join(constants.ReportsDirname, time.Now().UTC().Format("2006-01-02T150405Z"))
			}

			opts := reporting.Options{
				DataDir:      dataDir,
				RuntimeDir:   runtimeDir,
				LedgerDir:    ledgerDir,
				VaultDir:     paths.Infra.VaultDir,
				VaultKeyPath: paths.Infra.VaultKeyPath,
				OutDir:       outDir,
				Logger:       slog.Default(),
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

	cmd.Flags().StringVar(&outDir, "out", "", "Output directory (default: reports/<timestamp>)")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory (default: "+paths.Infra.DataDir+")")
	cmd.Flags().StringVar(&runtimeDir, "runtime-dir", "", "Runtime directory (default: "+paths.Infra.RuntimeDir+")")
	cmd.Flags().StringVar(&ledgerDir, "ledger-dir", "", "Ledger base directory (default: <runtime-dir>/ledger)")

	return cmd
}

func reportVerifyCmd() *cobra.Command {
	var outDir string
	var dataDir string
	var runtimeDir string
	var ledgerDir string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Run verification checks and write verification_summary.csv",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := paths.Init(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPathValidation, err)
			}

			if dataDir == "" {
				dataDir = paths.Infra.DataDir
			}
			if runtimeDir == "" {
				runtimeDir = paths.Infra.RuntimeDir
			}
			if outDir == "" {
				outDir = filepath.Join(constants.ReportsDirname, time.Now().UTC().Format("2006-01-02T150405Z"))
			}

			if err := os.MkdirAll(outDir, 0755); err != nil {
				return fmt.Errorf("%w: %s: %w", constants.ErrReportOutputDirFailed, outDir, err)
			}

			opts := reporting.Options{
				DataDir:      dataDir,
				RuntimeDir:   runtimeDir,
				LedgerDir:    ledgerDir,
				VaultDir:     paths.Infra.VaultDir,
				VaultKeyPath: paths.Infra.VaultKeyPath,
				OutDir:       outDir,
				Logger:       slog.Default(),
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

	cmd.Flags().StringVar(&outDir, "out", "", "Output directory (default: reports/<timestamp>)")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory (default: "+paths.Infra.DataDir+")")
	cmd.Flags().StringVar(&runtimeDir, "runtime-dir", "", "Runtime directory (default: "+paths.Infra.RuntimeDir+")")
	cmd.Flags().StringVar(&ledgerDir, "ledger-dir", "", "Ledger base directory (default: <runtime-dir>/ledger)")

	return cmd
}
