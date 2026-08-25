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

	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

func TestFsListArgs_BuildsValidJSON(t *testing.T) {
	got := fsListArgs(".")

	var entry clientpkg.FSPathArgs
	require.NoError(t, json.Unmarshal([]byte(got), &entry))

	assert.Equal(t, ".", entry.Path)
}

func TestFsListArgs_SpecialCharsEscaped(t *testing.T) {
	got := fsListArgs(`hello "world"`)

	var entry clientpkg.FSPathArgs
	require.NoError(t, json.Unmarshal([]byte(got), &entry))

	assert.Equal(t, `hello "world"`, entry.Path)
}

func TestFsListMap_BuildsTypedArgs(t *testing.T) {
	got := fsListMap(".")

	var _ clientpkg.ToolArgs = got
	assert.Equal(t, ".", got.Path)

	b, err := json.Marshal(got)
	require.NoError(t, err)
	var asMap map[string]any
	require.NoError(t, json.Unmarshal(b, &asMap))
	assert.Equal(t, ".", asMap["path"])
}
