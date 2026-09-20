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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func projectionStringValue(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value string
	require.NoError(t, json.Unmarshal(raw, &value))
	return value
}

var _ = context.Background
