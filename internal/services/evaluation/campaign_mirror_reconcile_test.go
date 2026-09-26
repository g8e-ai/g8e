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

func TestCampaignMirrorReconcileRunTimeout_ScalesWithAssignmentCount(t *testing.T) {
	base := 5 * time.Minute
	assert.Equal(t, base, CampaignMirrorReconcileRunTimeout(base, 0))
	assert.Equal(t, 7*time.Minute, CampaignMirrorReconcileRunTimeout(base, 60))
	assert.Equal(t, CampaignMirrorRunTimeoutMax, CampaignMirrorReconcileRunTimeout(base, 10_000))
}

func TestCampaignMirrorReconciler_ReconcileVerifiedQueuePresenceOnlyReportsMissing(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
	controller := NewCampaignController(store, &stubCampaignExecutor{}, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" }).WithPublication(coordinator)
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
	_, err = controller.ScheduleHomogeneousRun(context.Background(), run.GetRunId())
	require.NoError(t, err)
	_, _, err = controller.ExecuteNextAssignment(context.Background(), run.GetRunId(), CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
	}, req.ScenarioArtifacts)
	require.NoError(t, err)

	reconciler := NewCampaignMirrorReconciler(coordinator, store, probe)
	queue := &CampaignQueue{Models: []CampaignQueueModel{{Status: "verified", VerifiedRunID: run.GetRunId()}}}
	result, err := reconciler.ReconcileVerifiedQueue(context.Background(), queue, time.Minute, false, false, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{run.GetRunId()}, result.MissingRunIDs)
	assert.Empty(t, result.RestoredRunIDs)
}
