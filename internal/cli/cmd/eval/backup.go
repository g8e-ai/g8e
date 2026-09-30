// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

// defaultEvalBackupDir is where backups go unless --output-dir says otherwise:
// eval/backups under the project root, outside the .g8e/ runtime tree.
func defaultEvalBackupDir(cfg *config.Config) string {
	return filepath.Join(cfg.ProjectRoot, filepath.FromSlash(constants.EvaluationBackupDefaultDir))
}

// autoBackupEval snapshots evaluation evidence into the default backup
// directory once a run has finished, whatever its outcome: a failed or
// cancelled run's evidence is worth keeping too. A backup failure never masks
// the run's own result, so it is reported as a warning. Text output goes to
// stdout only outside JSON mode, which must stay machine-parseable.
func autoBackupEval(cmd *cobra.Command, deps nativeEvalDeps, jsonOutput bool) {
	// The run may have been cancelled, so the backup must not inherit that.
	ctx := context.WithoutCancel(cmd.Context())
	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err == nil {
		var report *evaluation.EvalBackupReport
		report, err = evaluation.NewEvalBackup(fileSvc, deps.now).CreateIfChanged(ctx, defaultEvalBackupDir(cfg))
		if err == nil {
			if jsonOutput {
				return
			}
			if report.Unchanged {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Evaluation evidence already backed up in %s\n", report.SnapshotDir)
				return
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Backed up evaluation evidence (%d file(s), %d bytes) to %s\n", len(report.Files), report.TotalBytes, report.SnapshotDir)
			return
		}
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: evaluation: automatic backup failed: %v\n", err)
}

type evalBackupJSON struct {
	SnapshotDir string                      `json:"snapshot_dir"`
	CreatedAt   string                      `json:"created_at"`
	FileCount   int                         `json:"file_count"`
	TotalBytes  int64                       `json:"total_bytes"`
	Files       []evaluation.EvalBackupFile `json:"files"`
}

type evalRestoreJSON struct {
	SnapshotDir string                      `json:"snapshot_dir"`
	Restored    []evaluation.EvalBackupFile `json:"restored"`
	Unchanged   []evaluation.EvalBackupFile `json:"unchanged"`
}

func backupEvalCmd(deps nativeEvalDeps) *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:     "backup",
		Aliases: []string{"backups"},
		Short:   "Copy evaluation evidence to a directory outside the runtime tree",
		Long: `Copy all evaluation evidence (runs, campaigns, archives, exports, the rollout
queue, and the frozen model inventory) into a new timestamped snapshot
directory beneath --output-dir (default: eval/backups under the project root),
with a SHA-256 manifest.

The destination must be outside .g8e/ so it survives a Docker volume wipe or
'./g8e docker clean'. Run and lease state is not copied. Runs made by
'g8e eval runs start' and 'resume' back up to the default directory
automatically when they finish. Use 'g8e eval restore' to put a snapshot back.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			if outputDir == "" {
				outputDir = defaultEvalBackupDir(cfg)
			}
			report, err := evaluation.NewEvalBackup(fileSvc, deps.now).Create(cmd.Context(), outputDir)
			if err != nil {
				return fmt.Errorf("evaluation: backup: %w", err)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), evalBackupJSON{
					SnapshotDir: report.SnapshotDir,
					CreatedAt:   report.CreatedAt.Format(time.RFC3339),
					FileCount:   len(report.Files),
					TotalBytes:  report.TotalBytes,
					Files:       report.Files,
				})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Backed up %d file(s), %d bytes, to %s\n", len(report.Files), report.TotalBytes, report.SnapshotDir)
			return err
		},
	}
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory outside .g8e/ to write the snapshot into (default: <project>/eval/backups)")
	return cmd
}

func restoreEvalCmd(deps nativeEvalDeps) *cobra.Command {
	var overwrite bool
	cmd := &cobra.Command{
		Use:     "restore [snapshot-dir]",
		Aliases: []string{"restores"},
		Short:   "Restore evaluation evidence from a backup snapshot",
		Long: `Verify every file in a snapshot created by 'g8e eval backup' against its
manifest, then write them back into .g8e/. Files already present with identical
content are skipped. If any existing file differs, nothing is written unless
--overwrite is passed.

Without <snapshot-dir>, the newest complete snapshot in eval/backups under the
project root is restored.

Restoring puts host evidence back; it does not rebuild the Gateway mirror. After
re-enrolling a fresh stack, run 'g8e public restore --queue'.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			var snapshotDir string
			if len(args) == 1 {
				snapshotDir = args[0]
			} else if snapshotDir, err = evaluation.LatestEvalBackupSnapshot(defaultEvalBackupDir(cfg)); err != nil {
				return fmt.Errorf("evaluation: restore: %w", err)
			}
			report, err := evaluation.NewEvalBackup(fileSvc, deps.now).Restore(cmd.Context(), snapshotDir, overwrite)
			if err != nil {
				return fmt.Errorf("evaluation: restore: %w", err)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), evalRestoreJSON{
					SnapshotDir: report.SnapshotDir,
					Restored:    nonNilBackupFiles(report.Restored),
					Unchanged:   nonNilBackupFiles(report.Unchanged),
				})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Restored %d file(s) from %s (%d already identical)\n", len(report.Restored), report.SnapshotDir, len(report.Unchanged))
			return err
		},
	}
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Replace existing files whose content differs from the backup")
	return cmd
}

func nonNilBackupFiles(files []evaluation.EvalBackupFile) []evaluation.EvalBackupFile {
	if files == nil {
		return []evaluation.EvalBackupFile{}
	}
	return files
}
