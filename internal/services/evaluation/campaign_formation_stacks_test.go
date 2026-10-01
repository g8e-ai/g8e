// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License 2.0.

package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func testFormationCatalogVariants() []*evalv1.ModelVariant {
	return formationCatalogTestVariants()
}

func formationCatalogTestVariants() []*evalv1.ModelVariant {
	digestByTag := map[string]byte{
		"qwen3.5:9b":     '1',
		"ministral-3:3b": '2',
		"gemma3:1b":      '3',
		"deepseek-r1:7b": '4',
		"llama3.2:1b":    '5',
		"gemma4:e4b":     '6',
		"qwen2.5:7b":     '7',
		"llama3.1:8b":    '8',
		"phi4-mini:3.8b": '9',
		"qwen3.5:4b":     'b',
		"gemma4:e2b":     'c',
	}
	return FormationCatalogFixtureVariants(func(tag string) string {
		ch := digestByTag[tag]
		if ch == 0 {
			ch = 'a'
		}
		return repeatHex(ch, 64)
	})
}

func TestGenerateFormationCatalogStackSet_ReturnsEveryCatalogFormation(t *testing.T) {
	set, err := GenerateFormationCatalogStackSet(FormationCatalogStackGenerationRequest{
		CampaignID: "eval-formations-benchmark",
		Seed:       17,
		Variants:   testFormationCatalogVariants(),
	})
	require.NoError(t, err)
	assert.Equal(t, FormationCatalogStackGenerationRule, set.GenerationRule)
	assert.Len(t, set.Stacks, 5)
	assert.Equal(t, 5, set.Coverage.HypothesisStackCount)
	assert.Equal(t, 0, set.Coverage.CoverageStackCount)
	assert.Len(t, set.VariantIDs, 11)
	require.NoError(t, ValidateHeterogeneousStackSet(set))

	stackIDs := make([]string, 0, len(set.Stacks))
	for _, stack := range set.Stacks {
		stackIDs = append(stackIDs, stack.GetStackId())
	}
	assert.ElementsMatch(t, []string{
		"deepseek-reasoning-tower",
		"gemma-cascade",
		"llama-meta-stack",
		"qwen-powerhouse",
		"ultra-efficient-speedster",
	}, stackIDs)
}

func TestGenerateFormationCatalogStackSet_IsDeterministic(t *testing.T) {
	req := FormationCatalogStackGenerationRequest{
		CampaignID: "eval-formations-benchmark",
		Seed:       11,
		Variants:   testFormationCatalogVariants(),
	}
	left, err := GenerateFormationCatalogStackSet(req)
	require.NoError(t, err)
	right, err := GenerateFormationCatalogStackSet(req)
	require.NoError(t, err)
	assert.Equal(t, left.SetDigest, right.SetDigest)
}

func TestGenerateFormationCatalogStackSet_RejectsMissingRegistryVariant(t *testing.T) {
	variants := testFormationCatalogVariants()[:3]
	_, err := GenerateFormationCatalogStackSet(FormationCatalogStackGenerationRequest{
		CampaignID: "eval-formations-benchmark",
		Variants:   variants,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationRegistryBinding)
}

func TestGenerateFormationCatalogStackSet_SelectsNamedFormations(t *testing.T) {
	set, err := GenerateFormationCatalogStackSet(FormationCatalogStackGenerationRequest{
		CampaignID:   "eval-formations-subset",
		Seed:         3,
		Variants:     testFormationCatalogVariants(),
		FormationIDs: []string{"qwen-powerhouse", "ultra-efficient-speedster", "qwen-powerhouse"},
	})
	require.NoError(t, err)
	require.NoError(t, ValidateHeterogeneousStackSet(set))
	assert.Equal(t, 2, set.Coverage.HypothesisStackCount)

	stackIDs := make([]string, 0, len(set.Stacks))
	for _, stack := range set.Stacks {
		stackIDs = append(stackIDs, stack.GetStackId())
	}
	assert.ElementsMatch(t, []string{"qwen-powerhouse", "ultra-efficient-speedster"}, stackIDs)
	assert.Less(t, len(set.VariantIDs), 11, "only the named formations' models are bound")
}

func TestGenerateFormationCatalogStackSet_RejectsUnknownFormation(t *testing.T) {
	_, err := GenerateFormationCatalogStackSet(FormationCatalogStackGenerationRequest{
		CampaignID:   "eval-formations-subset",
		Variants:     testFormationCatalogVariants(),
		FormationIDs: []string{"no-such-formation"},
	})
	require.ErrorIs(t, err, constants.ErrFormationInvalid)
}

func TestValidateFormationCatalogStackSet_AcceptsSubsetsAndRejectsEmptySets(t *testing.T) {
	set, err := GenerateFormationCatalogStackSet(FormationCatalogStackGenerationRequest{
		CampaignID:   "eval-formations-subset",
		Variants:     testFormationCatalogVariants(),
		FormationIDs: []string{"gemma-cascade"},
	})
	require.NoError(t, err)
	require.NoError(t, validateFormationCatalogStackSet(set))

	empty := *set
	empty.Stacks = nil
	require.Error(t, validateFormationCatalogStackSet(&empty))
}

func TestGenerateFormationCatalogStackSet_RejectsEmptySovereignDigest(t *testing.T) {
	variants := testFormationCatalogVariants()
	for _, variant := range variants {
		if variant.GetProviderClass() == "ollama" {
			variant.ModelDigest = ""
			break
		}
	}
	_, err := GenerateFormationCatalogStackSet(FormationCatalogStackGenerationRequest{
		CampaignID: "eval-formations-benchmark",
		Variants:   variants,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationAttestationRequired)
}

func TestResolveFormationBinding_UsesCatalogBindForFormationStack(t *testing.T) {
	topologies, err := NewExecutionTopologies()
	require.NoError(t, err)
	formation, err := topologies.Formation("deepseek-reasoning-tower")
	require.NoError(t, err)
	stack, err := formation.ToStackDefinition()
	require.NoError(t, err)

	bound, err := ResolveFormationBinding(FormationBindingRequest{
		FormationID: stack.GetStackId(),
		Stack:       stack,
		Variants:    testFormationCatalogVariants(),
	})
	require.NoError(t, err)
	assert.Equal(t, "deepseek-reasoning-tower", bound.ID)
	assert.False(t, bound.RelaxedValidation)
	assert.Equal(t, repeatHex('4', 64), bound.Primary.ModelDigest)
	assert.Equal(t, "freeze-deepseek-r1-7b", bound.Primary.VariantID)
}

func TestResolveFormationBinding_FallsBackToHeterogeneousStack(t *testing.T) {
	stackSet, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: "heterogeneous-campaign",
		Seed:       17,
		Variants:   testHeterogeneousVariants(),
	})
	require.NoError(t, err)

	bound, err := ResolveFormationBinding(FormationBindingRequest{
		Stack:    stackSet.Stacks[0],
		Variants: testHeterogeneousVariants(),
	})
	require.NoError(t, err)
	assert.True(t, bound.RelaxedValidation)
}

func TestBindFormation_BindsAllCatalogFormationsFromServedTags(t *testing.T) {
	topologies, err := NewExecutionTopologies()
	require.NoError(t, err)
	variants := testFormationCatalogVariants()
	for _, formation := range topologies.Formations() {
		t.Run(formation.ID, func(t *testing.T) {
			stack, stackErr := formation.ToStackDefinition()
			require.NoError(t, stackErr)
			bound, bindErr := BindFormation(FormationBindingRequest{
				FormationID: formation.ID,
				Stack:       stack,
				Variants:    variants,
			})
			require.NoError(t, bindErr)
			assert.Equal(t, formation.ID, bound.ID)
			for _, role := range formation.Roles() {
				catalogModel, modelErr := formation.Model(role)
				require.NoError(t, modelErr)
				boundModel, boundErr := bound.Model(role)
				require.NoError(t, boundErr)
				assert.Equal(t, catalogModel.ServedModelTag, boundModel.ServedModelTag)
				if catalogModel.Trust == FormationTrustSovereign {
					assert.NotEmpty(t, boundModel.ModelDigest)
				}
			}
		})
	}
}

func TestCampaignController_GenerateFormationCatalogStackSet_PersistsStacks(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })

	req := testCampaignInitRequest(t)
	req.Lane = evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM
	var err error
	req.Inventory, err = MaterializeModelRegistry(req.CampaignID, testFormationCatalogVariants())
	require.NoError(t, err)

	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)

	stackSet, err := controller.GenerateFormationCatalogStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId(), 17, nil)
	require.NoError(t, err)
	require.NotNil(t, stackSet)
	assert.Equal(t, FormationCatalogStackGenerationRule, stackSet.GenerationRule)

	loaded, err := store.LoadHeterogeneousStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)
	assert.Equal(t, stackSet.SetDigest, loaded.SetDigest)
}
