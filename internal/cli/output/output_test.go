// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONEnabled_GlobalFlag(t *testing.T) {
	root := &cobra.Command{Use: "g8e"}
	root.PersistentFlags().Bool("json", false, "Emit machine-readable JSON output")
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)

	t.Run("disabled by default", func(t *testing.T) {
		assert.False(t, JSONEnabled(child))
	})

	t.Run("enabled before subcommand", func(t *testing.T) {
		root.SetArgs([]string{"--json", "child"})
		require.NoError(t, root.ParseFlags([]string{"--json"}))
		assert.True(t, JSONEnabled(child))
	})

	t.Run("enabled after subcommand", func(t *testing.T) {
		child.SetArgs([]string{"--json"})
		require.NoError(t, child.ParseFlags([]string{"--json"}))
		assert.True(t, JSONEnabled(child))
	})
}

func TestWriteJSON_PrettyPrints(t *testing.T) {
	var buf bytes.Buffer
	err := WriteJSON(&buf, map[string]any{"operators": []any{}})
	require.NoError(t, err)
	assert.Equal(t, "{\n  \"operators\": []\n}\n", buf.String())
}

func TestWriteRawJSON_PrettyPrintsCompactPayload(t *testing.T) {
	var buf bytes.Buffer
	err := WriteRawJSON(&buf, []byte(`{"operators":[{"id":"op-1"}]}`))
	require.NoError(t, err)
	assert.True(t, strings.Contains(buf.String(), "\n  \"operators\""))
	assert.True(t, strings.HasSuffix(buf.String(), "\n"))
}

func TestWriteRawJSON_InvalidJSONPassthrough(t *testing.T) {
	var buf bytes.Buffer
	err := WriteRawJSON(&buf, []byte("not-json"))
	require.NoError(t, err)
	assert.Equal(t, "not-json\n", buf.String())
}
