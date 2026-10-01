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

	"github.com/g8e-ai/g8e/v2/internal/constants"
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
	assert.Len(t, assignments, 78)
	inventory := &ModelInventoryFreeze{
		CampaignID:           "north-star-smoke",
		RegistryDigest:       repeatHex('c', 64),
		Variants:             variants,
		HomogeneousCellCount: 78,
	}
	require.NoError(t, ValidateHomogeneousAssignmentMatrix(catalog, inventory, 1, assignments))
}

func TestBuildHomogeneousAssignmentMatrix_RunsModelByModel(t *testing.T) {
	catalog, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	variants := []*evalv1.ModelVariant{
		{VariantId: "qwen3-4b", ProviderClass: "ollama", ServedModelTag: "qwen3:4b", ModelDigest: repeatHex('a', 64)},
		{VariantId: "gemma3-4b", ProviderClass: "ollama", ServedModelTag: "gemma3:4b", ModelDigest: repeatHex('b', 64)},
		{VariantId: "llama3-2-3b", ProviderClass: "ollama", ServedModelTag: "llama3.2:3b", ModelDigest: repeatHex('c', 64)},
	}
	assignments, err := BuildHomogeneousAssignmentMatrix(HomogeneousScheduleRequest{
		CampaignID:      "model-by-model",
		RunID:           "run-1",
		Catalog:         catalog,
		Variants:        variants,
		RepetitionCount: 2,
		QueuedAt:        time.Unix(1_700_000_000, 0).UTC(),
	})
	require.NoError(t, err)

	// Each model's assignments must form one contiguous block, so the provider
	// loads every model exactly once.
	blocks := make([]string, 0, len(variants))
	for _, assignment := range assignments {
		tag := assignment.GetHomogeneous().GetCandidateVariant().GetServedModelTag()
		if len(blocks) == 0 || blocks[len(blocks)-1] != tag {
			blocks = append(blocks, tag)
		}
	}
	assert.Equal(t, []string{"gemma3:4b", "llama3.2:3b", "qwen3:4b"}, blocks)

	// The store lists assignments from files named by hash; re-sorting a
	// reversed copy must reproduce the scheduled order exactly.
	reversed := make([]*evalv1.EvaluationAssignment, len(assignments))
	for i, assignment := range assignments {
		reversed[len(assignments)-1-i] = assignment
	}
	sortAssignmentsDeterministic(reversed)
	assert.Equal(t, assignments, reversed)
}

func TestBuildHomogeneousAssignmentMatrix_RejectsDuplicateIdentity(t *testing.T) {
	catalog := testScenarioCatalog()
	variant := testModelVariant()
	first, err := buildHomogeneousAssignment("campaign", "run", catalog.Scenarios[0], variant, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY, 1, time.Unix(0, 0).UTC())
	require.NoError(t, err)
	duplicate := first
	inventory := &ModelInventoryFreeze{Variants: []*evalv1.ModelVariant{variant}, HomogeneousCellCount: 3}
	assert.Error(t, ValidateHomogeneousAssignmentMatrix(catalog, inventory, 1, []*evalv1.EvaluationAssignment{first, duplicate}))
}

func TestBuildHomogeneousAssignmentMatrix_SchedulesEachScenarioOnlyForItsEligibleRoles(t *testing.T) {
	catalog, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	variant := &evalv1.ModelVariant{VariantId: "qwen3-4b", ProviderClass: "ollama", ServedModelTag: "qwen3:4b", ModelDigest: repeatHex('a', 64)}
	assignments, err := BuildHomogeneousAssignmentMatrix(HomogeneousScheduleRequest{
		CampaignID:      "role-eligibility",
		RunID:           "run-1",
		Catalog:         catalog,
		Variants:        []*evalv1.ModelVariant{variant},
		RepetitionCount: 1,
		QueuedAt:        time.Unix(1_700_000_000, 0).UTC(),
	})
	require.NoError(t, err)

	rolesByScenario := make(map[string][]evalv1.ModelCampaignRole)
	for _, assignment := range assignments {
		rolesByScenario[assignment.GetScenarioId()] = append(rolesByScenario[assignment.GetScenarioId()], assignment.GetHomogeneous().GetDesignatedRole())
	}
	const (
		primary   = evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY
		assistant = evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT
		lite      = evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE
	)
	assert.ElementsMatch(t, []evalv1.ModelCampaignRole{primary, assistant}, rolesByScenario["tool-arg-file-path"])
	assert.ElementsMatch(t, []evalv1.ModelCampaignRole{lite}, rolesByScenario["instruction-classify-severity"])
	assert.ElementsMatch(t, []evalv1.ModelCampaignRole{lite}, rolesByScenario["route-lite-triage"])
	assert.ElementsMatch(t, []evalv1.ModelCampaignRole{primary}, rolesByScenario["final-response-diagnosis"])
	assert.ElementsMatch(t, []evalv1.ModelCampaignRole{assistant}, rolesByScenario["route-handoff-assistant"])
	assert.Len(t, rolesByScenario, len(catalog.GetScenarios()))
}

func TestBuildHomogeneousAssignmentMatrix_RejectsScenarioWithoutEligibleRoles(t *testing.T) {
	catalog := testScenarioCatalog()
	catalog.Scenarios[0].EligibleRoles = nil

	_, err := BuildHomogeneousAssignmentMatrix(HomogeneousScheduleRequest{
		CampaignID: "campaign",
		RunID:      "run",
		Catalog:    catalog,
		Variants:   []*evalv1.ModelVariant{testModelVariant()},
	})

	require.ErrorIs(t, err, constants.ErrEvaluationScenarioRolesUnassigned)
}

func TestValidateHomogeneousAssignmentMatrix_RejectsAssignmentForIneligibleRole(t *testing.T) {
	catalog := testScenarioCatalog()
	catalog.Scenarios = catalog.Scenarios[:1]
	variant := testModelVariant()
	liteOnly, err := buildHomogeneousAssignment("campaign", "run", catalog.Scenarios[0], variant, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY, 1, time.Unix(0, 0).UTC())
	require.NoError(t, err)
	inventory := &ModelInventoryFreeze{Variants: []*evalv1.ModelVariant{variant}}

	err = ValidateHomogeneousAssignmentMatrix(catalog, inventory, 1, []*evalv1.EvaluationAssignment{liteOnly})

	require.ErrorIs(t, err, constants.ErrEvaluationRoleNotEligible)
}
