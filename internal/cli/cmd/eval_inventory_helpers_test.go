// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestWriteInventoryMaterializeResult_TextAndJSON(t *testing.T) {
	lines := []inventoryMaterializeLine{{
		CampaignID:     "eval-init-qwen3-4b",
		ServedModelTag: "qwen3:4b",
		RegistryDigest: "digest-1",
		CellCount:      3,
		InventoryFile:  ".g8e/eval/inventories/eval-init-qwen3-4b.json",
	}}
	command := &cobra.Command{}
	var output bytes.Buffer
	command.SetOut(&output)
	require.NoError(t, writeInventoryMaterializeResult(command, lines, false))
	assert.Contains(t, output.String(), "qwen3:4b")

	command = evalCmdWithConfig(testNativeEvalDeps(t.TempDir()))
	rootCmd := globalJSONRoot(t, command)
	output.Reset()
	rootCmd.SetOut(&output)
	require.NoError(t, writeInventoryMaterializeResult(command, lines, true))
	assert.Contains(t, output.String(), `"inventories"`)
}

func TestModelInventoryFreezeJSON_AndWriteFile(t *testing.T) {
	freeze := &evaluation.ModelInventoryFreeze{
		CampaignID:           "eval-init-qwen3-4b",
		RegistryDigest:       "digest-1",
		HomogeneousCellCount: 3,
		Variants: []*evalv1.ModelVariant{{
			VariantId:      "qwen3-4b",
			ServedModelTag: "qwen3:4b",
			ModelDigest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ProviderClass:  "ollama",
		}},
	}
	payload, err := modelInventoryFreezeJSON(freeze)
	require.NoError(t, err)
	assert.Contains(t, string(payload), "eval-init-qwen3-4b")

	_, err = modelInventoryFreezeJSON(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)

	root := t.TempDir()
	path := filepath.Join(root, "inventory.json")
	require.NoError(t, writeModelInventoryFreezeFile(path, freeze))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "qwen3:4b")
}
