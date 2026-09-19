// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestModelRegistryFreezeLookupModelVariant(t *testing.T) {
	freeze := &ModelRegistryFreeze{
		CampaignID: "eval-init-qwen3-4b",
		Digest:     "digest-1",
		Variants: []*operatorv1.InferenceModelVariant{
			{Model: "qwen3:4b", Digest: "abc"},
		},
	}
	variant, err := freeze.LookupModelVariant("qwen3:4b")
	require.NoError(t, err)
	assert.Equal(t, "qwen3:4b", variant.GetModel())

	_, err = freeze.LookupModelVariant("missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelNotFound)

	_, err = (*ModelRegistryFreeze)(nil).LookupModelVariant("qwen3:4b")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestMaterializeCampaignInventory_WritesInventoryFile(t *testing.T) {
	root := t.TempDir()
	freeze, relPath, err := MaterializeCampaignInventory(MaterializeCampaignInventoryRequest{
		ProjectRoot: root,
		CampaignID:  "eval-init-gemma3-4b",
		Variants:    []*evalv1.ModelVariant{testModelVariant()},
	})
	require.NoError(t, err)
	require.NotNil(t, freeze)
	assert.Equal(t, "eval-init-gemma3-4b", freeze.CampaignID)
	assert.Contains(t, relPath, "eval-init-gemma3-4b.json")
}
