// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildPublicAssignmentProjectionComposesApprovedSections(t *testing.T) {
	t.Parallel()
	assignment := &evalv1.EvaluationAssignment{AssignmentId: "assignment-1", RunId: "run-1", ScenarioId: "scenario-1", LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED}
	result := &evalv1.EvaluationAssignmentResult{AssignmentId: "assignment-1", RunId: "run-1", LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, DeterministicGrades: []*evalv1.DeterministicGrade{{CriterionId: "criterion-1", Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, Detail: "private detail"}}}
	scenarioContext := &PublicScenarioContext{
		ScenarioID: "scenario-1", ScenarioVersion: "1.0.0", PublicDescription: "Public scenario",
		Criteria: []*evalv1.PublicScenarioCriterion{{CriterionId: "criterion-1", PublicLabel: "Criterion", PublicDescription: "Public criterion", Required: true}},
	}

	record, err := BuildPublicAssignmentProjection(context.Background(), PublicAssignmentBuildInput{
		Assignment: assignment, Result: result, ScenarioContext: scenarioContext, VerificationStatus: "unverified",
	})
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "scenario-1", record.Projection.GetScenarioSummary().GetScenarioId())
	assert.Equal(t, "pass", record.Extensions.BenchmarkObservations.GradeSummaries[0].Status)
	assert.NotNil(t, record.Projection.GetActivitySummary())
	assert.NotNil(t, record.Extensions.ResourceSummary)
	assert.NotContains(t, record.Extensions.BenchmarkObservations.GradeSummaries[0].Detail, "private")
}

func TestMarshalAssignmentResultProjectionEnvelopeUsesCanonicalExtensions(t *testing.T) {
	t.Parallel()
	record := &PublicAssignmentRecord{Projection: &evalv1.PublicAssignmentResultProjection{AssignmentId: "assignment-1", RunId: "run-1", ScenarioId: "scenario-1"}, Extensions: PublicAssignmentRecordExtensions{ResourceSummary: &PublicResourceSummary{InputTokens: PublicResourceMetric{Value: resourceFloat(0)}}}}

	body, err := MarshalAssignmentResultProjectionEnvelope("run-1:assignment-1:result", record)
	require.NoError(t, err)
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, campaignProjectionEnvelopeEnrichedVersion, projectionStringValue(t, envelope["schema_version"]))
	var projected map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope["record"], &projected))
	assert.Contains(t, projected, "resource_summary")
	require.NoError(t, ValidatePublicAssignmentRecord(campaignProjectionEnvelopeEnrichedVersion, body))
}

func TestBuildPublicAssignmentProjectionRejectsInvalidPublicBinding(t *testing.T) {
	t.Parallel()
	_, err := BuildPublicAssignmentProjection(context.Background(), PublicAssignmentBuildInput{
		Assignment:       &evalv1.EvaluationAssignment{AssignmentId: "assignment-1", RunId: "run-1", ScenarioId: "scenario-1"},
		Result:           &evalv1.EvaluationAssignmentResult{AssignmentId: "assignment-1", RunId: "run-1"},
		ScenarioContext:  &PublicScenarioContext{ScenarioID: "scenario-1", ScenarioVersion: "1.0.0", PublicDescription: "Public", Criteria: []*evalv1.PublicScenarioCriterion{{CriterionId: "criterion-1"}}},
		EvidenceBindings: []*evalv1.PublicEvidenceBinding{{Sha256: "private-id", SchemaRef: "private", Kind: "private"}},
	})
	assert.Error(t, err)
}

func TestMarshalPublicAssignmentRecordRejectsRestrictedBenchmarkDetail(t *testing.T) {
	t.Parallel()
	_, err := MarshalPublicAssignmentRecord(&PublicAssignmentRecord{
		Projection: &evalv1.PublicAssignmentResultProjection{AssignmentId: "assignment-1", RunId: "run-1", ScenarioId: "scenario-1"},
		Extensions: PublicAssignmentRecordExtensions{BenchmarkObservations: &PublicBenchmarkObservations{GradeSummaries: []PublicGradeSummary{{CriterionID: "criterion-1", Status: "fail", Detail: "private detail"}}}},
	})
	assert.Error(t, err)
}

func TestBuildPublicAssignmentProjectionPreservesCanonicalZeroAndUint64Values(t *testing.T) {
	t.Parallel()
	assignment := &evalv1.EvaluationAssignment{AssignmentId: "assignment-1", RunId: "run-1", ScenarioId: "scenario-1"}
	result := &evalv1.EvaluationAssignmentResult{
		AssignmentId: "assignment-1", RunId: "run-1",
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId: "inference-1", ModelRole: evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
			AgentPersona: "primary", ModelVariant: &evalv1.ModelVariant{VariantId: "variant-1"},
			UsageAvailability: evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED,
			PromptTokens:      7, CompletionTokens: 11, ThinkingTokens: projectionUint32Ptr(13), CacheTokens: projectionUint32Ptr(17),
			FinishReason: "stop", LoadState: evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_COLD,
			RetryCount: projectionUint32Ptr(0),
		}},
		ScoredInferenceSpanNanos: projectionUint64Ptr(0),
	}
	contextValue := &PublicScenarioContext{
		ScenarioID: "scenario-1", ScenarioVersion: "1.0.0",
		Category:          evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION,
		PublicDescription: "Public", GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		Criteria: []*evalv1.PublicScenarioCriterion{{CriterionId: "criterion-1", PublicLabel: "Criterion", PublicDescription: "Public criterion", GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC}},
	}
	record, err := BuildPublicAssignmentProjection(context.Background(), PublicAssignmentBuildInput{Assignment: assignment, Result: result, ScenarioContext: contextValue})
	require.NoError(t, err)
	first, err := MarshalPublicAssignmentRecord(record)
	require.NoError(t, err)
	second, err := MarshalPublicAssignmentRecord(record)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Contains(t, string(first), `"input_tokens":"7"`)
	assert.Contains(t, string(first), `"output_tokens":"11"`)
	assert.Contains(t, string(first), `"retry_count":0`)
	assert.Contains(t, string(first), `"retries":{"value":0}`)
	assert.Contains(t, string(first), `"latency_ms":{"value":0}`)
}

func TestBuildPublicAssignmentProjectionRejectsScopeMismatch(t *testing.T) {
	t.Parallel()
	_, err := BuildPublicAssignmentProjection(context.Background(), PublicAssignmentBuildInput{
		Assignment:      &evalv1.EvaluationAssignment{AssignmentId: "assignment-1", RunId: "run-1", ScenarioId: "scenario-1"},
		Result:          &evalv1.EvaluationAssignmentResult{AssignmentId: "assignment-2", RunId: "run-1"},
		ScenarioContext: &PublicScenarioContext{ScenarioID: "scenario-1"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceScopeMismatch))
}

func projectionUint32Ptr(value uint32) *uint32 {
	return &value
}

func projectionUint64Ptr(value uint64) *uint64 {
	return &value
}

func projectionStringValue(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value string
	require.NoError(t, json.Unmarshal(raw, &value))
	return value
}

var _ = context.Background
