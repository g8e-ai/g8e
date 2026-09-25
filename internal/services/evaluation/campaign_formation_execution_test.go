// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type harnessCampaignFormationRunner struct {
	harness *FormationHarness
}

func (r *harnessCampaignFormationRunner) RunHeterogeneousFormation(ctx context.Context, binding FormationBindingRequest, _ FormationRunContext, initialState []byte) (*FormationRunResult, error) {
	formation, err := BindHeterogeneousStack(binding)
	if err != nil {
		return nil, err
	}
	return r.harness.RunBoundFormation(ctx, formation, initialState)
}

func heterogeneousAssignmentExecutionRequest(t *testing.T, stack *evalv1.HeterogeneousStackDefinition, variants []*evalv1.ModelVariant) AssignmentExecutionRequest {
	t.Helper()
	return AssignmentExecutionRequest{
		Assignment: &evalv1.EvaluationAssignment{
			AssignmentId: "assignment-heterogeneous-1",
			RunId:        "run-heterogeneous-1",
			CampaignId:   "campaign-heterogeneous-1",
			ScenarioId:   "instruction-exact-format",
			Lane:         evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM,
			Target: &evalv1.EvaluationAssignment_Heterogeneous{
				Heterogeneous: &evalv1.HeterogeneousAssignmentTarget{Stack: stack},
			},
		},
		AttemptID: "attempt-heterogeneous-1",
		ScenarioInput: ScenarioInputFixture{
			ScenarioID: "instruction-exact-format",
			UserPrompt: "Reply with exactly: NORTH-STAR-OK",
		},
		GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		Binding: CampaignExecutionBinding{
			InferenceOperatorSessionID: "infer-session",
			DataOperatorID:             "data-op",
			DataOperatorSessionID:      "data-session",
			ModelRegistryDigest:        "d" + repeatHex('d', 63),
			ModelRegistry:              InferenceVariantsFromEvalRegistry(variants),
		},
	}
}

func TestBindHeterogeneousStack_BindsRegistryVariants(t *testing.T) {
	variants := testHeterogeneousVariants()
	stackSet, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: "north-star-heterogeneous",
		Seed:       11,
		Variants:   variants,
	})
	require.NoError(t, err)
	stack := stackSet.Stacks[0]

	formation, err := BindHeterogeneousStack(FormationBindingRequest{
		FormationID: stack.GetStackId(),
		Stack:       stack,
		Variants:    variants,
	})
	require.NoError(t, err)
	assert.True(t, formation.RelaxedValidation)
	assert.Equal(t, stack.GetPrimarySlot().GetVariantId(), formation.Primary.VariantID)
	assert.NotEmpty(t, formation.Primary.ModelDigest)
}

func TestCampaignFormationExecutor_ExecutesHeterogeneousAssignment(t *testing.T) {
	variants := testHeterogeneousVariants()
	stackSet, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: "north-star-heterogeneous",
		Seed:       11,
		Variants:   variants,
	})
	require.NoError(t, err)
	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)
	executor := NewCampaignFormationExecutor(variants, &harnessCampaignFormationRunner{harness: harness}, nil, func(prefix string) string { return prefix + "-1" })
	req := heterogeneousAssignmentExecutionRequest(t, stackSet.Stacks[0], variants)

	result, err := executor.ExecuteAssignment(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, result.GetLifecycleStatus())
	assert.Len(t, result.GetModelInferences(), 3)
	assert.Equal(t, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE, result.GetModelInferences()[0].GetModelRole())
	assert.Equal(t, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT, result.GetModelInferences()[1].GetModelRole())
	assert.Equal(t, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY, result.GetModelInferences()[2].GetModelRole())
}

func TestCampaignAssignmentRouter_RoutesHeterogeneousAssignments(t *testing.T) {
	variants := testHeterogeneousVariants()
	stackSet, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: "north-star-heterogeneous",
		Seed:       11,
		Variants:   variants,
	})
	require.NoError(t, err)
	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)
	heterogeneous := NewCampaignFormationExecutor(variants, &harnessCampaignFormationRunner{harness: harness}, nil, func(prefix string) string { return prefix + "-1" })
	router := NewCampaignAssignmentRouter(&recordingCampaignExecutor{label: "homogeneous"}, heterogeneous)
	req := heterogeneousAssignmentExecutionRequest(t, stackSet.Stacks[0], variants)

	result, err := router.ExecuteAssignment(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, result.GetLifecycleStatus())
}

type recordingCampaignExecutor struct {
	label string
	calls int
}

func (e *recordingCampaignExecutor) ExecuteAssignment(context.Context, AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error) {
	if e != nil {
		e.calls++
	}
	return nil, assert.AnError
}

func TestImportAssignmentResultFromFormationRun_MaterializesRoleTelemetry(t *testing.T) {
	variants := testHeterogeneousVariants()
	stackSet, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: "north-star-heterogeneous",
		Seed:       11,
		Variants:   variants,
	})
	require.NoError(t, err)
	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)
	formation, err := BindHeterogeneousStack(FormationBindingRequest{Stack: stackSet.Stacks[0], Variants: variants})
	require.NoError(t, err)
	formationResult, err := harness.RunBoundFormation(context.Background(), formation, []byte("initial"))
	require.NoError(t, err)

	req := heterogeneousAssignmentExecutionRequest(t, stackSet.Stacks[0], variants)
	result, err := ImportAssignmentResultFromFormationRun(req, formationResult, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	assert.Len(t, result.GetModelInferences(), 3)
	assert.NotNil(t, result.GetScoredInferenceSpanNanos())
}
