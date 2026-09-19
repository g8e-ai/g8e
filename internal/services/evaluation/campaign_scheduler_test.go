// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildHomogeneousAssignmentMatrix_MaterializesFullCrossProduct(t *testing.T) {
	catalog, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	variants := []*evalv1.ModelVariant{
		{VariantId: "qwen3-4b", ProviderClass: "ollama", ServedModelTag: "qwen3:4b", ModelDigest: repeatHex('a', 64)},
		{VariantId: "gemma3-4b", ProviderClass: "ollama", ServedModelTag: "gemma3:4b", ModelDigest: repeatHex('b', 64)},
	}
	assignments, err := BuildHomogeneousAssignmentMatrix(HomogeneousScheduleRequest{
		CampaignID:      "north-star-smoke",
		RunID:           "run-1",
		Catalog:         catalog,
		Variants:        variants,
		RepetitionCount: 1,
		QueuedAt:        time.Unix(1_700_000_000, 0).UTC(),
	})
	require.NoError(t, err)
	assert.Len(t, assignments, 150)
	inventory := &ModelInventoryFreeze{
		CampaignID:           "north-star-smoke",
		RegistryDigest:       repeatHex('c', 64),
		Variants:             variants,
		HomogeneousCellCount: 150,
	}
	require.NoError(t, ValidateHomogeneousAssignmentMatrix(catalog, inventory, 1, assignments))
}

func TestBuildHomogeneousAssignmentMatrix_RejectsDuplicateIdentity(t *testing.T) {
	catalog := testScenarioCatalog()
	variant := testModelVariant()
	first, err := buildHomogeneousAssignment("campaign", "run", catalog.Scenarios[0], variant, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY, 1, time.Unix(0, 0).UTC())
	require.NoError(t, err)
	duplicate := first
	inventory := &ModelInventoryFreeze{Variants: []*evalv1.ModelVariant{variant}, HomogeneousCellCount: 6}
	assert.Error(t, ValidateHomogeneousAssignmentMatrix(catalog, inventory, 1, []*evalv1.EvaluationAssignment{first, duplicate}))
}
