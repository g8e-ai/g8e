// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type failingCampaignExecutor struct {
	message string
}

func (f *failingCampaignExecutor) ExecuteAssignment(_ context.Context, _ AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error) {
	return nil, assignmentExecutionError(f.message, fmt.Errorf("injected executor failure"))
}

func TestBuildExecutorFailureAssignmentResult_ClassifiesProviderFailures(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	result, err := BuildExecutorFailureAssignmentResult(req, fmt.Errorf("evaluation: execute assignment: wait for trace: deadline exceeded"), time.Unix(1_700_000_000, 0).UTC())
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED, result.GetLifecycleStatus())
	assert.NotEmpty(t, result.GetResultDigest())
}

func TestBuildExecutorFailureAssignmentResult_ClassifiesValidationFailures(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	result, err := BuildExecutorFailureAssignmentResult(req, fmt.Errorf(`evaluation: execute assignment: submit chat: ensemble chat: status 422: {"detail":[{"loc":["body","evaluation_context","gold_summary","expected_tools"]}]}`), time.Unix(1_700_000_000, 0).UTC())
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED, result.GetLifecycleStatus())
}

func TestIsRecoverableAssignmentExecutionError_UsesTypedSentinel(t *testing.T) {
	t.Parallel()
	assert.True(t, isRecoverableAssignmentExecutionError(assignmentExecutionError("evaluation: execute heterogeneous assignment", fmt.Errorf("status 503"))))
	assert.False(t, isRecoverableAssignmentExecutionError(fmt.Errorf("evaluation: execute heterogeneous assignment: status 503")))
	assert.ErrorIs(t, assignmentExecutionError("evaluation: execute assignment: submit chat", fmt.Errorf("connection refused")), constants.ErrEvaluationAssignmentExecutionFailed)
}

func TestExecuteNextAssignment_PersistsResultOnRecoverableExecutorFailure(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	executor := &failingCampaignExecutor{message: "evaluation: execute assignment: submit chat: connection refused"}
	controller := NewCampaignController(store, executor, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	catalog := req.Catalog
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: catalog.GetSchemaVersion(),
		CatalogRef:    catalog.GetCatalogRef(),
		Scenarios:     catalog.GetScenarios()[:1],
	}
	truncatedDigest, err := ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = truncatedDigest
	req.Catalog = truncated
	_, err = controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)

	result, executed, err := controller.ExecuteNextAssignment(context.Background(), req.RunID, CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              req.Inventory.ToModelRegistryFreeze().Variants,
	}, req.ScenarioArtifacts)
	require.NoError(t, err)
	require.True(t, executed)
	require.NotNil(t, result)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED, result.GetLifecycleStatus())

	exists, err := store.AssignmentResultExists(context.Background(), req.RunID, result.GetAssignmentId())
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestExecuteNextAssignment_PersistsResultOnRecoverableHeterogeneousExecutorFailure(t *testing.T) {
	variants := testHeterogeneousVariants()
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	executor := &failingCampaignExecutor{message: "evaluation: execute heterogeneous assignment"}
	controller := NewCampaignController(store, executor, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	req.Lane = evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM
	var err error
	req.Inventory, err = MaterializeModelRegistry(req.CampaignID, variants)
	require.NoError(t, err)
	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.GenerateHeterogeneousStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId(), 11)
	require.NoError(t, err)

	_, err = controller.ScheduleHeterogeneousRun(context.Background(), run.GetRunId())
	require.NoError(t, err)

	result, executed, err := controller.ExecuteNextAssignment(context.Background(), run.GetRunId(), CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              InferenceVariantsFromEvalRegistry(variants),
	}, req.ScenarioArtifacts)
	require.NoError(t, err)
	require.True(t, executed)
	require.NotNil(t, result)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED, result.GetLifecycleStatus())

	exists, err := store.AssignmentResultExists(context.Background(), run.GetRunId(), result.GetAssignmentId())
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRepairAssignmentsWithoutResults_BackfillsTerminalLifecycle(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	catalog := req.Catalog
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: catalog.GetSchemaVersion(),
		CatalogRef:    catalog.GetCatalogRef(),
		Scenarios:     catalog.GetScenarios()[:1],
	}
	truncatedDigest, err := ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = truncatedDigest
	req.Catalog = truncated
	_, err = controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)

	assignments, err := store.ListAssignments(context.Background(), req.RunID)
	require.NoError(t, err)
	assignment := assignments[0]
	assignment.LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED
	assignment.CompletedAt = timestamppb.New(time.Unix(1_700_000_100, 0).UTC())
	require.NoError(t, store.SaveAssignment(context.Background(), assignment))

	repaired, err := controller.RepairAssignmentsWithoutResults(context.Background(), req.RunID, req.ScenarioArtifacts)
	require.NoError(t, err)
	assert.Equal(t, 1, repaired)

	exists, err := store.AssignmentResultExists(context.Background(), req.RunID, assignment.GetAssignmentId())
	require.NoError(t, err)
	assert.True(t, exists)
}
