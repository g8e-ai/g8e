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
		"qwen2.5:14b-instruct-q4_K_M":     '1',
		"gemma2:2b-instruct-q4_K_M":       '2',
		"llama3.2:1b-instruct-q4_K_M":     '3',
		"llama3.1:8b-instruct-q4_K_M":     '4',
		"phi3.5:3.8b-mini-instruct-q4_K_M": '5',
		"qwen2.5:0.5b-instruct-q4_K_M":    '6',
		"gemma2:9b-instruct-q4_K_M":       '7',
		"qwen2.5-coder:7b-instruct-q4_K_M": '8',
		"gemini-1.5-pro":                  '0',
		"qwen2.5:1.5b-instruct-q4_K_M":    '9',
	}
	return FormationCatalogFixtureVariants(func(tag string) string {
		ch := digestByTag[tag]
		if ch == 0 {
			ch = 'a'
		}
		return repeatHex(ch, 64)
	})
}

func TestGenerateFormationCatalogStackSet_ReturnsFiveCatalogFormations(t *testing.T) {
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
	assert.Len(t, set.VariantIDs, 10)
	require.NoError(t, ValidateHeterogeneousStackSet(set))

	stackIDs := make([]string, 0, len(set.Stacks))
	for _, stack := range set.Stacks {
		stackIDs = append(stackIDs, stack.GetStackId())
	}
	assert.ElementsMatch(t, []string{
		"code-logic-edge",
		"enterprise-polyglot",
		"heavy-reasoner",
		"hybrid-delegator",
		"ultra-light-speedster",
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
	formation, err := topologies.Formation("enterprise-polyglot")
	require.NoError(t, err)
	stack, err := formation.ToStackDefinition()
	require.NoError(t, err)

	bound, err := ResolveFormationBinding(FormationBindingRequest{
		FormationID: stack.GetStackId(),
		Stack:       stack,
		Variants:    testFormationCatalogVariants(),
	})
	require.NoError(t, err)
	assert.Equal(t, "enterprise-polyglot", bound.ID)
	assert.False(t, bound.RelaxedValidation)
	assert.Equal(t, repeatHex('4', 64), bound.Primary.ModelDigest)
	assert.Equal(t, "freeze-llama31-8b", bound.Primary.VariantID)
}

func TestResolveFormationBinding_FallsBackToHeterogeneousStack(t *testing.T) {
	stackSet, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: "north-star-heterogeneous",
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

	stackSet, err := controller.GenerateFormationCatalogStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId(), 17)
	require.NoError(t, err)
	require.NotNil(t, stackSet)
	assert.Equal(t, FormationCatalogStackGenerationRule, stackSet.GenerationRule)

	loaded, err := store.LoadHeterogeneousStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)
	assert.Equal(t, stackSet.SetDigest, loaded.SetDigest)
}
