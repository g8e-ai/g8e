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

func TestCampaignController_GenerateHeterogeneousStackSet_PersistsStacks(t *testing.T) {
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
	req.Inventory, err = MaterializeModelRegistry(req.CampaignID, testHeterogeneousVariants())
	require.NoError(t, err)

	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)

	stackSet, err := controller.GenerateHeterogeneousStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId(), 17)
	require.NoError(t, err)
	require.NotNil(t, stackSet)
	assert.NotEmpty(t, stackSet.Stacks)

	loaded, err := store.LoadHeterogeneousStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)
	assert.Equal(t, stackSet.SetDigest, loaded.SetDigest)
}

func TestCampaignController_ScheduleAndExecuteHeterogeneousAssignment(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)
	variants := testHeterogeneousVariants()
	formationExecutor := NewCampaignFormationExecutor(
		variants,
		&harnessCampaignFormationRunner{harness: harness},
		store,
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-1" },
	)
	router := NewCampaignAssignmentRouter(&stubCampaignExecutor{}, formationExecutor)
	controller := NewCampaignController(store, router, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })

	req := testCampaignInitRequest(t)
	req.Lane = evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM
	req.Inventory, err = MaterializeModelRegistry(req.CampaignID, variants)
	require.NoError(t, err)

	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.GenerateHeterogeneousStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId(), 17)
	require.NoError(t, err)
	count, err := controller.ScheduleHeterogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	assert.Greater(t, count, 0)

	binding := CampaignExecutionBinding{
		InferenceOperatorSessionID: req.InferenceOperatorSessionID,
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      req.DataOperatorSessionID,
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              InferenceVariantsFromEvalRegistry(variants),
	}
	result, ok, err := controller.ExecuteNextAssignment(context.Background(), req.RunID, binding, req.ScenarioArtifacts)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, result)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, result.GetLifecycleStatus())
	assert.Len(t, result.GetModelInferences(), 3)

	nextAssignment, ok, err := controller.ResumeNextAssignment(context.Background(), req.RunID)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, nextAssignment)
	assert.NotEqual(t, result.GetAssignmentId(), nextAssignment.GetAssignmentId())

	evidence, err := store.LoadAssignmentFormationRun(context.Background(), req.RunID, result.GetAssignmentId())
	require.NoError(t, err)
	require.NotNil(t, evidence)

	report, err := NewCampaignRunVerifier(func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }).VerifyRun(context.Background(), store, req.RunID, req.Catalog, req.ScenarioArtifacts)
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, report.GetStatus())
}

func TestCampaignController_ScheduleAndExecuteFormationCatalogAssignment(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)
	variants := formationCatalogTestVariants()
	formationExecutor := NewCampaignFormationExecutor(
		variants,
		&harnessCampaignFormationRunner{harness: harness},
		store,
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-1" },
	)
	router := NewCampaignAssignmentRouter(&stubCampaignExecutor{}, formationExecutor)
	controller := NewCampaignController(store, router, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })

	req := testCampaignInitRequest(t)
	req.Lane = evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM
	req.Inventory, err = MaterializeModelRegistry(req.CampaignID, variants)
	require.NoError(t, err)

	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	stackSet, err := controller.GenerateFormationCatalogStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId(), 17)
	require.NoError(t, err)
	assert.Equal(t, FormationCatalogStackGenerationRule, stackSet.GenerationRule)
	assert.Len(t, stackSet.Stacks, 4)

	count, err := controller.ScheduleHeterogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	assert.Equal(t, 4*StandardScenarioCount, count)

	binding := CampaignExecutionBinding{
		InferenceOperatorSessionID: req.InferenceOperatorSessionID,
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      req.DataOperatorSessionID,
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              InferenceVariantsFromEvalRegistry(variants),
	}
	result, ok, err := controller.ExecuteNextAssignment(context.Background(), req.RunID, binding, req.ScenarioArtifacts)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, result)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, result.GetLifecycleStatus())
	assert.Len(t, result.GetModelInferences(), 3)
}
