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
	"google.golang.org/protobuf/types/known/timestamppb"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type stubCampaignExecutor struct {
	calls int
}

func (s *stubCampaignExecutor) ExecuteAssignment(_ context.Context, req AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error) {
	s.calls++
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    req.Assignment.GetAssignmentId(),
		RunId:           req.Assignment.GetRunId(),
		CampaignId:      req.Assignment.GetCampaignId(),
		Lane:            req.Assignment.GetLane(),
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		CompletedAt:     timestamppb.Now(),
	}
	digest, err := ComputeAssignmentResultDigest(result)
	if err != nil {
		return nil, err
	}
	result.ResultDigest = digest
	return result, nil
}

func testCampaignInitRequest(t *testing.T) CampaignInitRequest {
	t.Helper()
	catalog, artifacts, err := LoadScenarioCatalog()
	require.NoError(t, err)
	inventory, err := MaterializeModelRegistry("north-star-smoke", []*evalv1.ModelVariant{testModelVariant()})
	require.NoError(t, err)
	return CampaignInitRequest{
		CampaignID:                 "north-star-smoke",
		RunID:                      "run-smoke-1",
		Catalog:                    catalog,
		Inventory:                  inventory,
		ScenarioArtifacts:          artifacts,
		RepetitionCount:            1,
		InferenceOperatorSessionID: "inf-session",
		DataOperatorSessionID:      "data-session",
	}
}

func TestCampaignControllerInitializeScheduleAndResume(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	executor := &stubCampaignExecutor{}
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

	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "run-smoke-1", run.GetRunId())

	count, err := controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	first, ok, err := controller.ResumeNextAssignment(context.Background(), req.RunID)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, first)

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
	assert.Equal(t, 1, executor.calls)

	exists, err := store.AssignmentResultExists(context.Background(), req.RunID, first.GetAssignmentId())
	require.NoError(t, err)
	assert.True(t, exists)

	restarted := NewCampaignController(store, executor, func() time.Time { return time.Unix(1_700_000_100, 0).UTC() }, func(prefix string) string { return prefix + "-2" })
	resumed, ok, err := restarted.ResumeNextAssignment(context.Background(), req.RunID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.NotEqual(t, first.GetAssignmentId(), resumed.GetAssignmentId())
}

func TestCampaignControllerRunSummaryCountsAssignments(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	_, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	count, err := controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	require.Greater(t, count, 0)

	summary, err := controller.RunSummary(context.Background(), req.RunID)
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Equal(t, req.RunID, summary.Run.GetRunId())
	assert.Greater(t, summary.ExpectedAssignment, uint64(0))
	assert.Equal(t, uint32(count), summary.QueuedCount)
}
