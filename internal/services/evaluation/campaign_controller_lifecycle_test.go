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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func secondTestModelVariant() *evalv1.ModelVariant {
	return &evalv1.ModelVariant{
		VariantId:      "gemma3-4b",
		ProviderClass:  "ollama",
		ServedModelTag: "gemma3:4b",
		ModelDigest:    repeatHex('c', 64),
		ModelFamily:    "gemma3",
	}
}

func TestCampaignControllerCreateCampaignIsIdempotentForTheSameFrozenSpec(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	create := CampaignCreateRequest{
		CampaignID:        f.req.CampaignID,
		Catalog:           f.req.Catalog,
		Inventory:         f.req.Inventory,
		ScenarioArtifacts: f.req.ScenarioArtifacts,
		RepetitionCount:   f.req.RepetitionCount,
	}

	first, err := f.controller.CreateCampaign(ctx, create)
	require.NoError(t, err)
	second, err := f.controller.CreateCampaign(ctx, create)
	require.NoError(t, err)
	assert.Equal(t, first.GetCampaignDigest(), second.GetCampaignDigest())
}

func TestCampaignControllerCreateCampaignRejectsADifferentSpecUnderTheSameID(t *testing.T) {
	f := newArchiveFixture(t)
	other, err := MaterializeModelRegistry(f.req.CampaignID, []*evalv1.ModelVariant{testModelVariant(), secondTestModelVariant()})
	require.NoError(t, err)

	_, err = f.controller.CreateCampaign(context.Background(), CampaignCreateRequest{
		CampaignID:        f.req.CampaignID,
		Catalog:           f.req.Catalog,
		Inventory:         other,
		ScenarioArtifacts: f.req.ScenarioArtifacts,
	})
	require.ErrorIs(t, err, constants.ErrEvaluationCampaignConflict)
}

func TestCampaignControllerCreateCampaignRequiresInputs(t *testing.T) {
	f := newArchiveFixture(t)
	_, err := f.controller.CreateCampaign(context.Background(), CampaignCreateRequest{CampaignID: f.req.CampaignID})
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestCampaignControllerStartRunBindsANewRunToAnExistingCampaign(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()

	run, err := f.controller.StartRun(ctx, RunStartRequest{
		CampaignID:                 f.req.CampaignID,
		RunID:                      "run-c",
		InferenceOperatorSessionID: "inf-2",
		DataOperatorSessionID:      "data-2",
	})
	require.NoError(t, err)
	assert.Equal(t, "run-c", run.GetRunId())
	assert.Equal(t, evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE, run.GetLane())
	assert.Equal(t, "inf-2", run.GetCampaignBinding().GetInferenceOperatorSessionId())
	assert.Equal(t, f.req.Inventory.RegistryDigest, run.GetCampaignBinding().GetModelRegistryDigest())

	runIDs, err := f.store.ListCampaignRunIDs(ctx, f.req.CampaignID)
	require.NoError(t, err)
	assert.Equal(t, []string{"run-a", "run-b", "run-c"}, runIDs)
}

func TestCampaignControllerStartRunRejectsExistingRunAndMissingCampaign(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()

	_, err := f.controller.StartRun(ctx, RunStartRequest{CampaignID: f.req.CampaignID, RunID: "run-a"})
	require.ErrorContains(t, err, "run already exists")

	_, err = f.controller.StartRun(ctx, RunStartRequest{CampaignID: "no-such-campaign", RunID: "run-x"})
	require.Error(t, err)

	_, err = f.controller.StartRun(ctx, RunStartRequest{CampaignID: f.req.CampaignID})
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestCampaignControllerCancelRunStopsEveryAssignmentWithoutAResult(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	binding := CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        f.req.Inventory.RegistryDigest,
		ModelRegistry:              f.req.Inventory.ToModelRegistryFreeze().Variants,
	}
	_, executed, err := f.controller.ExecuteNextAssignment(ctx, "run-a", binding, f.req.ScenarioArtifacts)
	require.NoError(t, err)
	require.True(t, executed)

	stopped, err := f.controller.CancelRun(ctx, "run-a")
	require.NoError(t, err)
	assert.Equal(t, 2, stopped)

	summary, err := f.controller.RunSummary(ctx, "run-a")
	require.NoError(t, err)
	assert.Zero(t, summary.QueuedCount)
	assert.Zero(t, summary.RunningCount)
	assert.Equal(t, uint32(3), summary.TerminalCount)
	assert.Equal(t, uint32(2), summary.StoppedCount)
	assert.Equal(t, "cancelled", CampaignRunStatus(summary, nil, false))

	next, ok, err := f.controller.ResumeNextAssignment(ctx, "run-a")
	require.NoError(t, err)
	assert.False(t, ok, "a cancelled run has nothing left to resume")
	assert.Nil(t, next)

	stoppedAgain, err := f.controller.CancelRun(ctx, "run-a")
	require.NoError(t, err)
	assert.Zero(t, stoppedAgain, "cancel is idempotent")

	other, err := f.controller.RunSummary(ctx, "run-b")
	require.NoError(t, err)
	assert.Equal(t, uint32(3), other.QueuedCount, "cancel is scoped to one run")
}

func TestCampaignControllerCancelRunStopsAnUnresolvedRunningAssignment(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	assignments, err := f.store.ListAssignments(ctx, "run-a")
	require.NoError(t, err)
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING
	require.NoError(t, f.store.SaveAssignment(ctx, assignments[0]))

	summary, err := f.controller.RunSummary(ctx, "run-a")
	require.NoError(t, err)
	assert.Equal(t, "interrupted", CampaignRunStatus(summary, nil, false), "a running assignment with no live holder is interrupted")
	assert.Equal(t, "running", CampaignRunStatus(summary, nil, true))

	stopped, err := f.controller.CancelRun(ctx, "run-a")
	require.NoError(t, err)
	assert.Equal(t, 3, stopped)

	summary, err = f.controller.RunSummary(ctx, "run-a")
	require.NoError(t, err)
	assert.Equal(t, "cancelled", CampaignRunStatus(summary, nil, false))
}

func TestCampaignControllerCancelRunRequiresAStoreAndKnownRun(t *testing.T) {
	_, err := (*CampaignController)(nil).CancelRun(context.Background(), "run-a")
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
}
