// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package shared

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// Flag names shared by every destructive clean/reset command.
const (
	FlagSkipBackup = "skip-backup"
	FlagYes        = "yes"
)

// PreCleanBackup is the backup a destructive command offers before it wipes
// state. Run returns a one-line summary for the operator.
type PreCleanBackup struct {
	Prompt string
	Run    func(ctx context.Context) (string, error)
}

// DestructiveOptions describes one destructive operation to confirm.
type DestructiveOptions struct {
	// Effects are the bullet lines listed under the warning.
	Effects []string
	// AssumeYes skips the confirmation and the backup question (--yes/--force);
	// the backup still runs unless SkipBackup is set.
	AssumeYes bool
	// SkipBackup opts out of the backup entirely (--skip-backup).
	SkipBackup bool
	// Backup is offered before proceeding. Nil means there is nothing to back up.
	Backup *PreCleanBackup
}

// AddSkipBackupFlag registers --skip-backup on a destructive command.
func AddSkipBackupFlag(cmd *cobra.Command, target *bool) {
	cmd.Flags().BoolVar(target, FlagSkipBackup, false, "Do not offer or take an evaluation-evidence backup before the destructive operation")
}

// ConfirmDestructive prints the warning, asks for confirmation, then offers
// the backup. It reports whether the caller should proceed. A backup that
// fails aborts with ErrDestructiveBackup, so state is never wiped after a
// backup the operator believed had succeeded; --skip-backup is the explicit
// way past it. End of input on a prompt counts as "no".
func ConfirmDestructive(cmd *cobra.Command, opts DestructiveOptions) (bool, error) {
	cmd.Println("WARNING: This is a destructive operation:")
	for _, effect := range opts.Effects {
		cmd.Printf("  - %s\n", effect)
	}
	cmd.Println()

	// One reader for both prompts: a second bufio.Reader on piped stdin would
	// find the first one already consumed the whole input.
	reader := bufio.NewReader(cmd.InOrStdin())

	if !opts.AssumeYes && !askYesNo(cmd, reader, "Continue?", false) {
		cmd.Println("Aborted")
		return false, nil
	}

	if opts.Backup == nil || opts.SkipBackup {
		return true, nil
	}
	if !opts.AssumeYes && !askYesNo(cmd, reader, opts.Backup.Prompt, true) {
		cmd.Println("Skipping backup.")
		return true, nil
	}

	summary, err := opts.Backup.Run(CommandContext(cmd))
	switch {
	case errors.Is(err, constants.ErrEvaluationBackupEmpty):
		cmd.Println("No evaluation evidence found; nothing to back up.")
	case err != nil:
		return false, fmt.Errorf("%w: %w", constants.ErrDestructiveBackup, err)
	default:
		cmd.Println(summary)
	}
	return true, nil
}

func askYesNo(cmd *cobra.Command, reader *bufio.Reader, prompt string, defaultYes bool) bool {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	cmd.Printf("%s %s: ", prompt, hint)
	input, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(input))
	if answer == "" {
		// A bare newline takes the default; bare EOF (no input at all) never does.
		return defaultYes && err == nil
	}
	return answer == "y" || answer == "yes"
}

// DefaultEvalBackupDir is where evaluation backups go unless --output-dir says
// otherwise: eval/backups under the project root, outside the .g8e/ runtime tree.
func DefaultEvalBackupDir(cfg *config.Config) string {
	return filepath.Join(cfg.ProjectRoot, filepath.FromSlash(constants.EvaluationBackupDefaultDir))
}

// EvalEvidenceBackup returns the standard pre-clean backup: a snapshot of host
// evaluation evidence in the default backup directory, skipped when the
// evidence is identical to the newest existing snapshot.
func EvalEvidenceBackup(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *PreCleanBackup {
	return &PreCleanBackup{
		Prompt: "Back up evaluation evidence to " + constants.EvaluationBackupDefaultDir + " first?",
		Run: func(ctx context.Context) (string, error) {
			cfg, err := configLoader("")
			if err != nil {
				return "", fmt.Errorf("load config: %w", err)
			}
			fileSvc, err := fileSvcFactory(cfg.ProjectRoot, slog.Default())
			if err != nil {
				return "", fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			report, err := evaluation.NewEvalBackup(fileSvc, time.Now).CreateIfChanged(ctx, DefaultEvalBackupDir(cfg))
			if err != nil {
				return "", err
			}
			if report.Unchanged {
				return fmt.Sprintf("Evaluation evidence already backed up in %s", report.SnapshotDir), nil
			}
			return fmt.Sprintf("Backed up evaluation evidence (%d file(s), %d bytes) to %s", len(report.Files), report.TotalBytes, report.SnapshotDir), nil
		},
	}
}
