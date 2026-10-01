// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package shared

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

type commandContextKey struct{}

func TestCommandContext_ReturnsTheContextAttachedToTheCommand(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), commandContextKey{}, "attached")
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(ctx)

	assert.Same(t, ctx, CommandContext(cmd))
}

func TestCommandContext_FallsBackToBackgroundForACommandThatNeverRan(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "run"}
	assert.Nil(t, cmd.Context(), "precondition: a fresh command has no context")

	got := CommandContext(cmd)

	assert.Equal(t, context.Background(), got)
	assert.NoError(t, got.Err())
	assert.Nil(t, got.Value(commandContextKey{}))
}

func TestCommandContext_PropagatesCancellationOfTheCommandContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(ctx)

	cancel()

	assert.ErrorIs(t, CommandContext(cmd).Err(), context.Canceled)
}
