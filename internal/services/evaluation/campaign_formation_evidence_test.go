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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type stubFormationRunStore struct {
	body []byte
}

func (s *stubFormationRunStore) SaveAssignmentFormationRun(_ context.Context, _, _ string, body []byte) error {
	if s != nil {
		s.body = append([]byte(nil), body...)
	}
	return nil
}

func TestBuildFormationRunEvidence_RoundTripsAndValidatesDigest(t *testing.T) {
	req := heterogeneousAssignmentExecutionRequest(t, mustHeterogeneousStack(t), testHeterogeneousVariants())
	runContext := FormationRunContext{
		CampaignID:          req.Assignment.GetCampaignId(),
		RunID:               req.Assignment.GetRunId(),
		AssignmentID:        req.Assignment.GetAssignmentId(),
		EvaluationAttemptID: req.AttemptID,
		ScenarioID:          req.Assignment.GetScenarioId(),
		ModelRegistryDigest: req.Binding.ModelRegistryDigest,
		InferenceSessionID:  req.Binding.InferenceOperatorSessionID,
		DataSessionID:       req.Binding.DataOperatorSessionID,
	}
	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)
	formation, err := BindHeterogeneousStack(FormationBindingRequest{Stack: req.Assignment.GetHeterogeneous().GetStack(), Variants: testHeterogeneousVariants()})
	require.NoError(t, err)
	initialState, err := BuildFormationInitialState(req.ScenarioInput)
	require.NoError(t, err)
	formationResult, err := harness.RunBoundFormation(context.Background(), formation, initialState)
	require.NoError(t, err)
	for index := range formationResult.Roles {
		formationResult.Roles[index].UsageAvailability = evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED
		formationResult.Roles[index].PromptTokens = uint32(index + 21)
	}

	body, evidence, err := BuildFormationRunEvidence(req, runContext, formationResult)
	require.NoError(t, err)
	require.NotEmpty(t, body)
	require.NoError(t, ValidateFormationRunEvidenceDigest(evidence))

	files := newCampaignMemoryFileService()
	store := NewStore(files)
	require.NoError(t, store.SaveAssignmentFormationRun(context.Background(), req.Assignment.GetRunId(), req.Assignment.GetAssignmentId(), body))
	loaded, err := store.LoadAssignmentFormationRun(context.Background(), req.Assignment.GetRunId(), req.Assignment.GetAssignmentId())
	require.NoError(t, err)
	assert.Equal(t, evidence.EvidenceDigest, loaded.EvidenceDigest)
	assert.Equal(t, formationResult.FormationID, loaded.Result.FormationID)
	assert.Len(t, loaded.Result.Roles, 3)
	restored, err := FormationRunResultFromEvidence(loaded)
	require.NoError(t, err)
	require.Len(t, restored.Roles, 3)
	for index, role := range restored.Roles {
		assert.Equal(t, evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED, role.UsageAvailability)
		assert.Equal(t, uint32(index+21), role.PromptTokens)
	}

	assignmentResult, err := ImportAssignmentResultFromFormationRun(req, formationResult, time.Unix(1_700_000_001, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	assignmentResult.ModelInferences[0].PromptTokens++
	assert.Error(t, VerifyFormationRunEvidenceMatchesResult(req.Assignment, loaded, assignmentResult))
}

func TestCampaignFormationExecutor_PersistsFormationRunEvidence(t *testing.T) {
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	store := stubFormationRunStore{}
	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)
	executor := NewCampaignFormationExecutor(variants, &harnessCampaignFormationRunner{harness: harness}, &store, nil, func(prefix string) string { return prefix + "-1" })
	req := heterogeneousAssignmentExecutionRequest(t, stack, variants)

	result, err := executor.ExecuteAssignment(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, store.body)
	evidence := &FormationRunEvidence{}
	require.NoError(t, json.Unmarshal(store.body, evidence))
	require.NoError(t, ValidateFormationRunEvidenceDigest(evidence))
}

func TestBuildFailureAssignmentResult_RecoversFromPersistedFormationRun(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := heterogeneousAssignmentExecutionRequest(t, mustHeterogeneousStack(t), testHeterogeneousVariants())
	req.Assignment.SchemaVersion = CampaignSchemaVersion
	runContext := FormationRunContext{
		CampaignID:          req.Assignment.GetCampaignId(),
		RunID:               req.Assignment.GetRunId(),
		AssignmentID:        req.Assignment.GetAssignmentId(),
		EvaluationAttemptID: req.AttemptID,
		ScenarioID:          req.Assignment.GetScenarioId(),
		ModelRegistryDigest: req.Binding.ModelRegistryDigest,
		InferenceSessionID:  req.Binding.InferenceOperatorSessionID,
		DataSessionID:       req.Binding.DataOperatorSessionID,
	}
	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)
	formation, err := BindHeterogeneousStack(FormationBindingRequest{Stack: req.Assignment.GetHeterogeneous().GetStack(), Variants: testHeterogeneousVariants()})
	require.NoError(t, err)
	initialState, err := BuildFormationInitialState(req.ScenarioInput)
	require.NoError(t, err)
	formationResult, err := harness.RunBoundFormation(context.Background(), formation, initialState)
	require.NoError(t, err)
	body, _, err := BuildFormationRunEvidence(req, runContext, formationResult)
	require.NoError(t, err)
	require.NoError(t, store.SaveAssignmentFormationRun(context.Background(), req.Assignment.GetRunId(), req.Assignment.GetAssignmentId(), body))

	recovered, err := controller.buildFailureAssignmentResult(context.Background(), req, assert.AnError)
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, recovered.GetLifecycleStatus())
	assert.Len(t, recovered.GetModelInferences(), 3)
	for _, inference := range recovered.GetModelInferences() {
		assert.Equal(t, evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED, inference.GetUsageAvailability())
		assert.NotZero(t, inference.GetPromptTokens())
	}
}
