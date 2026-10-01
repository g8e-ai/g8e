// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package shared

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func destructiveCmd(input string) (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "clean"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(input))
	return cmd, &out
}

func countingBackup(calls *int, err error) *PreCleanBackup {
	return &PreCleanBackup{
		Prompt: "Back up first?",
		Run: func(context.Context) (string, error) {
			*calls++
			return "Backed up.", err
		},
	}
}

func TestConfirmDestructive_DeclineAbortsWithoutBackup(t *testing.T) {
	cmd, out := destructiveCmd("n\n")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{Effects: []string{"wipe it"}, Backup: countingBackup(&calls, nil)})

	require.NoError(t, err)
	assert.False(t, proceed)
	assert.Zero(t, calls)
	assert.Contains(t, out.String(), "wipe it")
	assert.Contains(t, out.String(), "Aborted")
}

func TestConfirmDestructive_EndOfInputAborts(t *testing.T) {
	cmd, _ := destructiveCmd("")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{Backup: countingBackup(&calls, nil)})

	require.NoError(t, err)
	assert.False(t, proceed)
	assert.Zero(t, calls)
}

func TestConfirmDestructive_BackupPromptDefaultsToYes(t *testing.T) {
	cmd, out := destructiveCmd("y\n\n")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{Backup: countingBackup(&calls, nil)})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Equal(t, 1, calls)
	assert.Contains(t, out.String(), "Back up first? [Y/n]")
	assert.Contains(t, out.String(), "Backed up.")
}

func TestConfirmDestructive_DecliningBackupStillProceeds(t *testing.T) {
	cmd, out := destructiveCmd("y\nn\n")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{Backup: countingBackup(&calls, nil)})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Zero(t, calls)
	assert.Contains(t, out.String(), "Skipping backup.")
}

func TestConfirmDestructive_EndOfInputAtBackupPromptSkipsBackup(t *testing.T) {
	cmd, _ := destructiveCmd("y\n")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{Backup: countingBackup(&calls, nil)})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Zero(t, calls)
}

func TestConfirmDestructive_AssumeYesStillBacksUp(t *testing.T) {
	cmd, out := destructiveCmd("")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{AssumeYes: true, Backup: countingBackup(&calls, nil)})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Equal(t, 1, calls)
	assert.NotContains(t, out.String(), "Continue?")
}

func TestConfirmDestructive_SkipBackupOptsOut(t *testing.T) {
	cmd, out := destructiveCmd("y\n")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{SkipBackup: true, Backup: countingBackup(&calls, nil)})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Zero(t, calls)
	assert.NotContains(t, out.String(), "Back up first?")
}

func TestConfirmDestructive_BackupFailureAborts(t *testing.T) {
	cmd, _ := destructiveCmd("")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{AssumeYes: true, Backup: countingBackup(&calls, errors.New("disk full"))})

	require.ErrorIs(t, err, constants.ErrDestructiveBackup)
	assert.False(t, proceed)
}

// failingReader returns err on every read, simulating a broken stdin.
type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestConfirmDestructive_ListsEveryEffectUnderTheWarning(t *testing.T) {
	cmd, out := destructiveCmd("n\n")

	_, err := ConfirmDestructive(cmd, DestructiveOptions{Effects: []string{"drop the vault", "delete the ledger"}})

	require.NoError(t, err)
	assert.Contains(t, out.String(), "WARNING: This is a destructive operation:\n  - drop the vault\n  - delete the ledger\n")
}

func TestConfirmDestructive_AcceptedAnswersProceed(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "YES\n", "  yes  \n", "y"} {
		t.Run(strconv.Quote(answer), func(t *testing.T) {
			cmd, _ := destructiveCmd(answer)

			proceed, err := ConfirmDestructive(cmd, DestructiveOptions{})

			require.NoError(t, err)
			assert.True(t, proceed, "answer %q", answer)
		})
	}
}

func TestConfirmDestructive_UnrecognisedOrNegativeAnswersAbort(t *testing.T) {
	for _, answer := range []string{"n\n", "no\n", "maybe\n", "yep\n", "\n", "yes please\n"} {
		t.Run(strconv.Quote(answer), func(t *testing.T) {
			cmd, out := destructiveCmd(answer)

			proceed, err := ConfirmDestructive(cmd, DestructiveOptions{})

			require.NoError(t, err)
			assert.False(t, proceed, "answer %q", answer)
			assert.Contains(t, out.String(), "Aborted")
		})
	}
}

func TestConfirmDestructive_ContinuePromptDefaultsToNo(t *testing.T) {
	cmd, out := destructiveCmd("\n")

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{})

	require.NoError(t, err)
	assert.False(t, proceed)
	assert.Contains(t, out.String(), "Continue? [y/N]: ")
}

func TestConfirmDestructive_ReadErrorOnStdinCountsAsNo(t *testing.T) {
	cmd, out := destructiveCmd("")
	cmd.SetIn(failingReader{err: errors.New("stdin closed")})

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{})

	require.NoError(t, err)
	assert.False(t, proceed)
	assert.Contains(t, out.String(), "Aborted")
}

func TestConfirmDestructive_NoBackupConfiguredProceedsWithoutBackupPrompt(t *testing.T) {
	cmd, out := destructiveCmd("y\n")

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.NotContains(t, out.String(), "[Y/n]")
}

func TestConfirmDestructive_AssumeYesWithoutBackupNeverReadsInput(t *testing.T) {
	cmd, out := destructiveCmd("")
	cmd.SetIn(failingReader{err: errors.New("stdin must not be read")})

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{AssumeYes: true})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.NotContains(t, out.String(), "[y/N]")
}

func TestConfirmDestructive_BackupFailureKeepsTheCauseReachable(t *testing.T) {
	cmd, _ := destructiveCmd("")
	cause := errors.New("disk full")

	_, err := ConfirmDestructive(cmd, DestructiveOptions{AssumeYes: true, Backup: countingBackup(new(int), cause)})

	require.ErrorIs(t, err, constants.ErrDestructiveBackup)
	require.ErrorIs(t, err, cause)
}

func TestConfirmDestructive_BackupRunsWithTheCommandContext(t *testing.T) {
	cmd, _ := destructiveCmd("")
	type ctxKey struct{}
	cmd.SetContext(context.WithValue(context.Background(), ctxKey{}, "from-command"))
	var seen any

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{
		AssumeYes: true,
		Backup: &PreCleanBackup{
			Prompt: "Back up?",
			Run: func(ctx context.Context) (string, error) {
				seen = ctx.Value(ctxKey{})
				return "done", nil
			},
		},
	})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Equal(t, "from-command", seen)
}

func TestConfirmDestructive_SkipBackupWinsEvenWhenAssumeYesIsSet(t *testing.T) {
	cmd, _ := destructiveCmd("")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{AssumeYes: true, SkipBackup: true, Backup: countingBackup(&calls, nil)})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Zero(t, calls)
}

func TestAddSkipBackupFlag_RegistersABooleanFlagDefaultingToFalse(t *testing.T) {
	cmd := &cobra.Command{Use: "clean"}
	var skip bool

	AddSkipBackupFlag(cmd, &skip)

	flag := cmd.Flags().Lookup(FlagSkipBackup)
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.False(t, skip)
	assert.NotEmpty(t, flag.Usage)

	require.NoError(t, cmd.Flags().Parse([]string{"--skip-backup"}))
	assert.True(t, skip)
}

func TestDefaultEvalBackupDir_IsUnderTheProjectRootOutsideTheRuntimeTree(t *testing.T) {
	cfg := &config.Config{ProjectRoot: filepath.Join(string(filepath.Separator), "work", "project"), RuntimeDir: filepath.Join(string(filepath.Separator), "work", "project", ".g8e")}

	got := DefaultEvalBackupDir(cfg)

	assert.Equal(t, filepath.Join(cfg.ProjectRoot, "eval", "backups"), got)
	assert.False(t, strings.HasPrefix(got, cfg.RuntimeDir+string(filepath.Separator)), "backups must not live inside the runtime tree that a clean wipes")
}

func TestConfirmDestructive_EmptyEvidenceIsNotAFailure(t *testing.T) {
	cmd, out := destructiveCmd("")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{AssumeYes: true, Backup: countingBackup(&calls, constants.ErrEvaluationBackupEmpty)})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Contains(t, out.String(), "nothing to back up")
}
