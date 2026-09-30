// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

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
		Use:   "backup",
		Short: "Copy evaluation evidence to a directory outside the runtime tree",
		Long: `Copy all evaluation evidence (runs, campaigns, archives, exports, the rollout
queue, and the frozen model inventory) into a new timestamped snapshot
directory beneath --output-dir, with a SHA-256 manifest.

The destination must be outside .g8e/ so it survives a Docker volume wipe or
'./g8e docker clean'. Run and lease state is not copied. Use 'g8e eval restore'
to put a snapshot back.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
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
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory outside .g8e/ to write the snapshot into")
	_ = cmd.MarkFlagRequired("output-dir")
	return cmd
}

func restoreEvalCmd(deps nativeEvalDeps) *cobra.Command {
	var overwrite bool
	cmd := &cobra.Command{
		Use:   "restore <snapshot-dir>",
		Short: "Restore evaluation evidence from a backup snapshot",
		Long: `Verify every file in a snapshot created by 'g8e eval backup' against its
manifest, then write them back into .g8e/. Files already present with identical
content are skipped. If any existing file differs, nothing is written unless
--overwrite is passed.

Restoring puts host evidence back; it does not rebuild the Gateway mirror. After
re-enrolling a fresh stack, run 'g8e public restore --queue'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			report, err := evaluation.NewEvalBackup(fileSvc, deps.now).Restore(cmd.Context(), args[0], overwrite)
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
