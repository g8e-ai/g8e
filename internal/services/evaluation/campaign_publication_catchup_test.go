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

type batchCountingCampaignFeedExporter struct {
	maxBatchSize int
	records      []CampaignPublicFeedRecord
}

func (e *batchCountingCampaignFeedExporter) HighWaterSequence(context.Context) (int64, error) {
	if e == nil || len(e.records) == 0 {
		return 0, nil
	}
	return e.records[len(e.records)-1].Sequence, nil
}

func (e *batchCountingCampaignFeedExporter) ExportBatch(_ context.Context, records []CampaignPublicFeedRecord) error {
	if len(records) > e.maxBatchSize {
		e.maxBatchSize = len(records)
	}
	e.records = append(e.records, records...)
	return nil
}

func TestPublishRunCatchUpExportsAssignmentsInOneBatch(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &batchCountingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
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
	for i := 0; i < 3; i++ {
		_, _, err = controller.ExecuteNextAssignment(context.Background(), run.GetRunId(), CampaignExecutionBinding{
			InferenceOperatorSessionID: "inf-session",
			DataOperatorID:             "data-op",
			DataOperatorSessionID:      "data-session",
			ModelRegistryDigest:        req.Inventory.RegistryDigest,
			ModelRegistry:              InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
		}, req.ScenarioArtifacts)
		require.NoError(t, err)
	}

	published, err := coordinator.PublishRunCatchUp(context.Background(), run.GetRunId())
	require.NoError(t, err)
	assert.Greater(t, published, 0)
	assert.GreaterOrEqual(t, exporter.maxBatchSize, 4)
}
