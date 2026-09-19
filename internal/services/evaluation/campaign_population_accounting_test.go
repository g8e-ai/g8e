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

func TestCampaignPopulationAccountant_PartialRunReportsCoverageGap(t *testing.T) {
	t.Parallel()
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	_, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	count, err := controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	require.Greater(t, count, 1)

	assignments, err := store.ListAssignments(context.Background(), req.RunID)
	require.NoError(t, err)
	first := assignments[0]
	first.LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	require.NoError(t, store.SaveAssignment(context.Background(), first))

	report, err := NewCampaignPopulationAccountant(func() time.Time { return time.Unix(1_700_000_100, 0).UTC() }).AccountRun(context.Background(), store, req.RunID, req.Catalog)
	require.NoError(t, err)
	assert.False(t, report.Complete)
	assert.Equal(t, uint32(count), report.ScheduledAssignments)
	assert.Equal(t, uint32(1), report.TerminalCount)
	assert.Equal(t, uint32(count-1), report.QueuedCount)
	assert.Len(t, report.MissingCells, 0)
	assert.NotEmpty(t, report.TerminalWithoutResult)
}

func TestCampaignPopulationAccountant_CompleteRunPasses(t *testing.T) {
	t.Parallel()
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: req.Catalog.GetSchemaVersion(),
		CatalogRef:    req.Catalog.GetCatalogRef(),
		Scenarios:     req.Catalog.GetScenarios()[:1],
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
	for _, assignment := range assignments {
		assignment.LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
		require.NoError(t, store.SaveAssignment(context.Background(), assignment))
		result := &evalv1.EvaluationAssignmentResult{
			SchemaVersion:   CampaignSchemaVersion,
			CampaignId:      req.CampaignID,
			RunId:           req.RunID,
			AssignmentId:    assignment.GetAssignmentId(),
			LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
			Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		}
		digest, err := ComputeAssignmentResultDigest(result)
		require.NoError(t, err)
		result.ResultDigest = digest
		require.NoError(t, store.SaveAssignmentResult(context.Background(), result))
	}

	report, err := NewCampaignPopulationAccountant(func() time.Time { return time.Unix(1_700_000_200, 0).UTC() }).AccountRun(context.Background(), store, req.RunID, truncated)
	require.NoError(t, err)
	assert.True(t, report.Complete)
	assert.Empty(t, report.FailureReasons)
	assert.Equal(t, report.ScheduledAssignments, uint32(report.ExpectedCells))
	assert.Equal(t, report.TerminalCount, uint32(report.ExpectedCells))
}
