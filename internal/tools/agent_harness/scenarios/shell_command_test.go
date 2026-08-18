// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
)

func TestShellCommandArgs_BuildsValidJSON(t *testing.T) {
	got := shellCommandArgs("cloudop", "provision", "10.73.0.50:9100", "vm-01", "MODERATE")

	var cmd clientpkg.ShellCommandArgs
	require.NoError(t, json.Unmarshal([]byte(got), &cmd))

	assert.Equal(t, "cloudop", cmd.Command)
	assert.Equal(t, []string{"provision", "10.73.0.50:9100", "vm-01", "MODERATE"}, cmd.Args)
	assert.Equal(t, 10, cmd.Timeout)
}

func TestShellCommandArgs_NoArgs(t *testing.T) {
	got := shellCommandArgs("ping")

	var cmd clientpkg.ShellCommandArgs
	require.NoError(t, json.Unmarshal([]byte(got), &cmd))

	assert.Equal(t, "ping", cmd.Command)
	assert.Empty(t, cmd.Args)
	assert.Equal(t, 10, cmd.Timeout)
}

func TestShellCommandArgs_SpecialCharsEscaped(t *testing.T) {
	got := shellCommandArgs("echo", "hello \"world\"")

	var cmd clientpkg.ShellCommandArgs
	require.NoError(t, json.Unmarshal([]byte(got), &cmd))

	assert.Equal(t, "hello \"world\"", cmd.Args[0])
}

func TestShellCommandMap_BuildsTypedArgs(t *testing.T) {
	got := shellCommandMap("cloudop", "provision", "10.73.0.50:9100", "vm-01", "MODERATE")

	assert.Equal(t, "cloudop", got.Command)
	assert.Equal(t, 10, got.Timeout)
	assert.Equal(t, []string{"provision", "10.73.0.50:9100", "vm-01", "MODERATE"}, got.Args)

	// Verify it satisfies ToolArgs and serializes to the same JSON-RPC shape as
	// the previous map[string]any representation.
	var _ clientpkg.ToolArgs = got
	b, err := json.Marshal(got)
	require.NoError(t, err)
	var asMap map[string]any
	require.NoError(t, json.Unmarshal(b, &asMap))
	assert.Equal(t, "cloudop", asMap["command"])
	assert.Equal(t, float64(10), asMap["timeout"])
}

func TestShellCommandMap_NoArgs(t *testing.T) {
	got := shellCommandMap("ping")

	assert.Equal(t, "ping", got.Command)
	assert.Equal(t, 10, got.Timeout)
	assert.Empty(t, got.Args)
}
