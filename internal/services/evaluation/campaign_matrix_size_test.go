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

// truncatedCatalog stands in for a custom or smoke suite smaller than the
// default suite.
func truncatedCatalog(t *testing.T, catalog *evalv1.EvaluationScenarioCatalog, scenarios int) *evalv1.EvaluationScenarioCatalog {
	t.Helper()
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: catalog.GetSchemaVersion(),
		CatalogRef:    catalog.GetCatalogRef(),
		Scenarios:     catalog.GetScenarios()[:scenarios],
	}
	digest, err := ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = digest
	return truncated
}

func TestCatalogMatrixSizesFollowTheCatalogNotTheBuiltInSuite(t *testing.T) {
	catalog, _, err := LoadScenarioCatalog()
	require.NoError(t, err)
	small := truncatedCatalog(t, catalog, 3)

	assert.Equal(t, ComputeHomogeneousMatrixSize(1), ModelRoleMatrixSize(catalog, 1, 1), "the default suite sizes as it always did")
	assert.Equal(t, uint64(len(catalog.GetScenarios())), uint64(StandardScenarioCount))

	var wantCells uint64
	for _, scenario := range small.GetScenarios() {
		wantCells += uint64(len(scenario.GetEligibleRoles()))
	}
	assert.Equal(t, wantCells, CatalogModelRoleCells(small))
	assert.Equal(t, wantCells*2*4, ModelRoleMatrixSize(small, 2, 4))
	assert.Equal(t, wantCells, ModelRoleMatrixSize(small, 1, 0), "zero repetitions count as one")
	assert.Equal(t, uint64(5*3), FormationMatrixSize(small, 5))
	assert.Equal(t, uint64(5*StandardScenarioCount), FormationMatrixSize(catalog, 5))
}

func TestCampaignControllerRunSummaryExpectsTheFrozenCatalogMatrix(t *testing.T) {
	store := NewStore(newCampaignMemoryFileService())
	controller := NewCampaignController(store, &stubCampaignExecutor{}, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	req.Catalog = truncatedCatalog(t, req.Catalog, 3)

	_, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	scheduled, err := controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)

	summary, err := controller.RunSummary(context.Background(), req.RunID)
	require.NoError(t, err)
	assert.Equal(t, uint64(scheduled), summary.ExpectedAssignment, "expected must equal what the frozen catalog schedules")
	assert.NotEqual(t, ComputeHomogeneousMatrixSize(1), summary.ExpectedAssignment, "a three-scenario suite must not expect the default suite's matrix")
}

func TestCampaignControllerSchedulesASystemRunOverAnyFrozenCatalog(t *testing.T) {
	store := NewStore(newCampaignMemoryFileService())
	controller := NewCampaignController(store, &stubCampaignExecutor{}, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	req.Catalog = truncatedCatalog(t, req.Catalog, 3)
	req.Lane = evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM
	var err error
	req.Inventory, err = MaterializeModelRegistry(req.CampaignID, testHeterogeneousVariants())
	require.NoError(t, err)

	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	stackSet, err := controller.GenerateHeterogeneousStackSet(context.Background(), run.GetCampaignBinding().GetCampaignId(), 17)
	require.NoError(t, err)

	scheduled, err := controller.ScheduleHeterogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err, "a system run over a three-scenario suite must schedule")
	assert.Equal(t, len(stackSet.Stacks)*3, scheduled)

	summary, err := controller.RunSummary(context.Background(), req.RunID)
	require.NoError(t, err)
	assert.Equal(t, uint64(scheduled), summary.ExpectedAssignment)
}
