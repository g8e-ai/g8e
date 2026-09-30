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
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestConfirmDestructive_EmptyEvidenceIsNotAFailure(t *testing.T) {
	cmd, out := destructiveCmd("")
	calls := 0

	proceed, err := ConfirmDestructive(cmd, DestructiveOptions{AssumeYes: true, Backup: countingBackup(&calls, constants.ErrEvaluationBackupEmpty)})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Contains(t, out.String(), "nothing to back up")
}
