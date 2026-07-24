// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellCommandArgs_BuildsValidJSON(t *testing.T) {
	got := shellCommandArgs("cloudop", "provision", "10.73.0.50:9100", "vm-01", "MODERATE")

	var cmd shellCommandJSON
	require.NoError(t, json.Unmarshal([]byte(got), &cmd))

	assert.Equal(t, "cloudop", cmd.Command)
	assert.Equal(t, []string{"provision", "10.73.0.50:9100", "vm-01", "MODERATE"}, cmd.Args)
	assert.Equal(t, 10, cmd.Timeout)
}

func TestShellCommandArgs_NoArgs(t *testing.T) {
	got := shellCommandArgs("ping")

	var cmd shellCommandJSON
	require.NoError(t, json.Unmarshal([]byte(got), &cmd))

	assert.Equal(t, "ping", cmd.Command)
	assert.Empty(t, cmd.Args)
	assert.Equal(t, 10, cmd.Timeout)
}

func TestShellCommandArgs_SpecialCharsEscaped(t *testing.T) {
	got := shellCommandArgs("echo", "hello \"world\"")

	var cmd shellCommandJSON
	require.NoError(t, json.Unmarshal([]byte(got), &cmd))

	assert.Equal(t, "hello \"world\"", cmd.Args[0])
}
