// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFsListArgs_BuildsValidJSON(t *testing.T) {
	got := fsListArgs(".")

	var entry fsListJSON
	require.NoError(t, json.Unmarshal([]byte(got), &entry))

	assert.Equal(t, ".", entry.Path)
}

func TestFsListArgs_SpecialCharsEscaped(t *testing.T) {
	got := fsListArgs(`hello "world"`)

	var entry fsListJSON
	require.NoError(t, json.Unmarshal([]byte(got), &entry))

	assert.Equal(t, `hello "world"`, entry.Path)
}
