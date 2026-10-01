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

	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
)

func sampleVersionInfo() serve.VersionInfo {
	return serve.VersionInfo{
		Version:             "2.2.6",
		BuildID:             "build-42",
		BuildTime:           "2026-10-01T00:00:00Z",
		Platform:            "linux_amd64",
		SourceRevision:      "abc123",
		SourceTreeStateHash: "tree-hash",
	}
}

func TestVersionInfoFromCmd_ReturnsMetadataStoredOnTheCommandContext(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "version"}
	cmd.SetContext(ContextWithVersionInfo(context.Background(), sampleVersionInfo()))

	assert.Equal(t, sampleVersionInfo(), VersionInfoFromCmd(cmd))
}

func TestVersionInfoFromCmd_ReturnsZeroValueWhenNothingWasAttached(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  func() *cobra.Command
	}{
		{name: "nil command", cmd: func() *cobra.Command { return nil }},
		{name: "command without a context", cmd: func() *cobra.Command { return &cobra.Command{Use: "version"} }},
		{name: "context without version info", cmd: func() *cobra.Command {
			cmd := &cobra.Command{Use: "version"}
			cmd.SetContext(context.Background())
			return cmd
		}},
		{name: "context holding version info under an unrelated key", cmd: func() *cobra.Command {
			cmd := &cobra.Command{Use: "version"}
			cmd.SetContext(context.WithValue(context.Background(), commandContextKey{}, sampleVersionInfo()))
			return cmd
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, serve.VersionInfo{}, VersionInfoFromCmd(tt.cmd()))
		})
	}
}

func TestContextWithVersionInfo_TreatsANilParentAsBackground(t *testing.T) {
	t.Parallel()

	var parent context.Context
	cmd := &cobra.Command{Use: "version"}
	cmd.SetContext(ContextWithVersionInfo(parent, sampleVersionInfo()))

	assert.Equal(t, sampleVersionInfo(), VersionInfoFromCmd(cmd))
}

func TestContextWithVersionInfo_KeepsParentValuesAndCancellation(t *testing.T) {
	t.Parallel()

	parent, cancel := context.WithCancel(context.WithValue(context.Background(), commandContextKey{}, "parent-value"))

	ctx := ContextWithVersionInfo(parent, sampleVersionInfo())

	assert.Equal(t, "parent-value", ctx.Value(commandContextKey{}))
	assert.NoError(t, ctx.Err())
	cancel()
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestContextWithVersionInfo_InnerValueShadowsAnEarlierOne(t *testing.T) {
	t.Parallel()

	first := sampleVersionInfo()
	second := sampleVersionInfo()
	second.Version = "9.9.9"
	cmd := &cobra.Command{Use: "version"}
	cmd.SetContext(ContextWithVersionInfo(ContextWithVersionInfo(context.Background(), first), second))

	assert.Equal(t, "9.9.9", VersionInfoFromCmd(cmd).Version)
}
