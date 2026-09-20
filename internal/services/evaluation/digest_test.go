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
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/models"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func testModelVariant() *evalv1.ModelVariant {
	return &evalv1.ModelVariant{
		VariantId:      "qwen3-4b",
		ProviderClass:  "ollama",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    repeatHex('b', 64),
		ModelFamily:    "qwen3",
	}
}

func testCampaignSpec() *evalv1.EvaluationCampaignSpec {
	return &evalv1.EvaluationCampaignSpec{
		SchemaVersion:       CampaignSchemaVersion,
		CampaignId:          "phase1a-smoke",
		CatalogRef:          &compliancev1.VersionedReference{Id: "north-star-25", Version: "1.0.0"},
		CatalogDigest:       repeatHex('a', 64),
		ModelRegistry:       []*evalv1.ModelVariant{testModelVariant()},
		ModelRegistryDigest: repeatHex('c', 64),
		GovernancePosture:   evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_DOCTRINE,
		ScenarioCount:       25,
		RepetitionCount:     1,
	}
}

func testScenarioCatalog() *evalv1.EvaluationScenarioCatalog {
	return &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: CampaignSchemaVersion,
		CatalogRef:    &compliancev1.VersionedReference{Id: "north-star-25", Version: "1.0.0"},
		Scenarios: []*evalv1.EvaluationScenarioDefinition{
			{
				ScenarioId:        "instruction-exact-1",
				ScenarioVersion:   "1.0.0",
				Category:          evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE,
				PublicDescription: "Reply exactly",
				GradingMethod:     evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
			},
			{
				ScenarioId:        "tool-select-1",
				ScenarioVersion:   "1.0.0",
				Category:          evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION,
				PublicDescription: "Select the probe tool",
				GradingMethod:     evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
			},
		},
	}
}

func TestComputeCampaignSpecDigest_IsStableAndSensitive(t *testing.T) {
	spec := testCampaignSpec()
	first, err := ComputeCampaignSpecDigest(spec)
	require.NoError(t, err)
	second, err := ComputeCampaignSpecDigest(spec)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Len(t, first, 64)

	changed := testCampaignSpec()
	changed.ScenarioCount = 24
	changedDigest, err := ComputeCampaignSpecDigest(changed)
	require.NoError(t, err)
	assert.NotEqual(t, first, changedDigest)
}

func TestValidateCampaignSpecDigest_RejectsMismatch(t *testing.T) {
	spec := testCampaignSpec()
	digest, err := ComputeCampaignSpecDigest(spec)
	require.NoError(t, err)
	spec.CampaignDigest = digest
	require.NoError(t, ValidateCampaignSpecDigest(spec))

	spec.CampaignDigest = repeatHex('f', 64)
	assert.Error(t, ValidateCampaignSpecDigest(spec))
}

func TestComputeScenarioCatalogDigest_SortsScenariosDeterministically(t *testing.T) {
	catalog := testScenarioCatalog()
	forward, err := ComputeScenarioCatalogDigest(catalog)
	require.NoError(t, err)

	reversed := testScenarioCatalog()
	reversed.Scenarios = []*evalv1.EvaluationScenarioDefinition{
		catalog.Scenarios[1], catalog.Scenarios[0],
	}
	backward, err := ComputeScenarioCatalogDigest(reversed)
	require.NoError(t, err)
	assert.Equal(t, forward, backward)
}

func TestComputeModelVariantRegistryDigest_MatchesInferenceRegistryDigest(t *testing.T) {
	variants := []*evalv1.ModelVariant{testModelVariant()}
	evalDigest, err := ComputeModelVariantRegistryDigest("phase1a-smoke", variants)
	require.NoError(t, err)

	inferenceDigest, err := models.ComputeInferenceModelRegistryDigest("phase1a-smoke", []*operatorv1.InferenceModelVariant{{
		Model:  "qwen3:4b",
		Digest: repeatHex('b', 64),
	}})
	require.NoError(t, err)
	assert.Equal(t, inferenceDigest, evalDigest)
}

func TestComputeAssignmentDeterministicIdentity_DistinguishesHomogeneousBindings(t *testing.T) {
	base := &evalv1.EvaluationAssignment{
		CampaignId: "phase1a-smoke",
		ScenarioId: "instruction-exact-1",
		Lane:       evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		Repetition: 1,
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				CandidateVariant: testModelVariant(),
				DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
			},
		},
	}
	primary, err := ComputeAssignmentDeterministicIdentity(base)
	require.NoError(t, err)

	lite := &evalv1.EvaluationAssignment{
		CampaignId: base.CampaignId,
		ScenarioId: base.ScenarioId,
		Lane:       base.Lane,
		Repetition: base.Repetition,
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				CandidateVariant: testModelVariant(),
				DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE,
			},
		},
	}
	liteIdentity, err := ComputeAssignmentDeterministicIdentity(lite)
	require.NoError(t, err)
	assert.NotEqual(t, primary, liteIdentity)
}

func TestComputeAssignmentResultDigest_IsStableAndValidated(t *testing.T) {
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    "assign-1",
		RunId:           "run-1",
		CampaignId:      "phase1a-smoke",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	}
	digest, err := ComputeAssignmentResultDigest(result)
	require.NoError(t, err)
	result.ResultDigest = digest
	require.NoError(t, ValidateAssignmentResultDigest(result))
	result.ResultDigest = repeatHex('0', 64)
	assert.Error(t, ValidateAssignmentResultDigest(result))
}

func TestComputeAssignmentResultDigestIncludesEnrichedCaptureFields(t *testing.T) {
	base := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    "assign-enriched",
		RunId:           "run-enriched",
		CampaignId:      "phase1a-smoke",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	}
	baseline, err := ComputeAssignmentResultDigest(base)
	require.NoError(t, err)

	tests := []struct {
		name   string
		mutate func(*evalv1.EvaluationAssignmentResult)
	}{
		{
			name: "policy decision",
			mutate: func(result *evalv1.EvaluationAssignmentResult) {
				result.PolicyDecisions = []*evalv1.PolicyDecisionRecord{{
					DecisionId:   "decision-1",
					AssignmentId: result.GetAssignmentId(),
					ToolName:     "read_file",
					Outcome:      evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_ALLOW,
				}}
			},
		},
		{
			name: "tool decision capture",
			mutate: func(result *evalv1.EvaluationAssignmentResult) {
				result.ToolDecisionsCaptured = true
			},
		},
		{
			name: "tool call capture",
			mutate: func(result *evalv1.EvaluationAssignmentResult) {
				result.ToolCallsCaptured = true
			},
		},
		{
			name: "governed action capture",
			mutate: func(result *evalv1.EvaluationAssignmentResult) {
				result.GovernedActionsCaptured = true
			},
		},
		{
			name: "policy decision capture",
			mutate: func(result *evalv1.EvaluationAssignmentResult) {
				result.PolicyDecisionsCaptured = true
			},
		},
		{
			name: "scored inference span",
			mutate: func(result *evalv1.EvaluationAssignmentResult) {
				result.ScoredInferenceSpanNanos = proto.Uint64(500000000)
			},
		},
		{
			name: "model retry presence",
			mutate: func(result *evalv1.EvaluationAssignmentResult) {
				result.ModelInferences = []*evalv1.ModelInferenceRecord{{RetryCount: proto.Uint32(0)}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := proto.Clone(base).(*evalv1.EvaluationAssignmentResult)
			test.mutate(result)
			changed, err := ComputeAssignmentResultDigest(result)
			require.NoError(t, err)
			assert.NotEqual(t, baseline, changed)
		})
	}

	absent := proto.Clone(base).(*evalv1.EvaluationAssignmentResult)
	absent.ModelInferences = []*evalv1.ModelInferenceRecord{{}}
	absentDigest, err := ComputeAssignmentResultDigest(absent)
	require.NoError(t, err)
	zero := proto.Clone(base).(*evalv1.EvaluationAssignmentResult)
	zero.ModelInferences = []*evalv1.ModelInferenceRecord{{RetryCount: proto.Uint32(0)}}
	zeroDigest, err := ComputeAssignmentResultDigest(zero)
	require.NoError(t, err)
	assert.NotEqual(t, absentDigest, zeroDigest)
}
