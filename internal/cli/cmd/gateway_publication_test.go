// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

type stubGatewayPublicationClient struct {
	mu        sync.Mutex
	highWater int64
	postCalls int
}

func (c *stubGatewayPublicationClient) Get(path string) ([]byte, error) {
	if path != constants.APIPaths.PublicFeedSnapshot {
		return nil, fmt.Errorf("unexpected get path: %s", path)
	}
	c.mu.Lock()
	highWater := c.highWater
	c.mu.Unlock()
	if highWater == 0 {
		return nil, fmt.Errorf("%w: status 404: {\"error\":\"public-feed: snapshot not found\"}", constants.ErrHTTPStatusError)
	}
	snapshot := models.PublicFeedSnapshot{HighWaterSequence: highWater}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (c *stubGatewayPublicationClient) Post(path string, body interface{}) ([]byte, error) {
	if path != constants.APIPaths.PublicFeedBatches {
		return nil, fmt.Errorf("unexpected post path: %s", path)
	}
	records, ok := body.([]models.PublicFeedRecord)
	if !ok {
		return nil, fmt.Errorf("unexpected post body type: %T", body)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.postCalls++
	expected := c.highWater + 1
	for index, record := range records {
		if record.Sequence != expected+int64(index) {
			return nil, fmt.Errorf("%w: status 400: {\"error\":\"public-feed: batch sequence is out of order\"}", constants.ErrHTTPStatusError)
		}
	}
	c.highWater = records[len(records)-1].Sequence
	resp := models.PublicFeedExportBatchResponse{HighWaterSequence: c.highWater}
	payload, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *stubGatewayPublicationClient) Put(string, interface{}) ([]byte, error) {
	return nil, fmt.Errorf("unexpected put")
}

func (c *stubGatewayPublicationClient) Delete(string) ([]byte, error) {
	return nil, fmt.Errorf("unexpected delete")
}

func TestRemoteGatewayCampaignFeedExporterUsesGatewaySnapshot(t *testing.T) {
	client := &stubGatewayPublicationClient{highWater: 41}
	exporter := &remoteGatewayCampaignFeedExporter{client: client}

	highWater, err := exporter.HighWaterSequence(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(41), highWater)
}

func TestRemoteGatewayCampaignFeedExporterEmptySnapshotReturnsZero(t *testing.T) {
	client := &stubGatewayPublicationClient{}
	exporter := &remoteGatewayCampaignFeedExporter{client: client}

	highWater, err := exporter.HighWaterSequence(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), highWater)
}

func TestRemoteGatewayCampaignFeedExporterRetriesSequenceOutOfOrder(t *testing.T) {
	client := &stubGatewayPublicationClient{highWater: 10}
	exporter := &remoteGatewayCampaignFeedExporter{client: client}
	records := []evaluation.CampaignPublicFeedRecord{{
		Sequence:    6,
		RecordHash:  "hash",
		RecordBytes: "{}",
	}}
	require.NoError(t, exporter.ExportBatch(context.Background(), records))
	assert.Equal(t, int64(11), records[0].Sequence)
	assert.Equal(t, 2, client.postCalls)
}

func TestShouldPublishViaGatewayWithoutHostExportConfig(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	original := gatewayHealthCheck
	gatewayHealthCheck = func() bool { return true }
	t.Cleanup(func() { gatewayHealthCheck = original })

	assert.False(t, isHostPublicFeedConfigured(context.Background(), fileSvc))
	assert.True(t, shouldPublishViaGateway(context.Background(), fileSvc))
}

func TestShouldPublishViaGatewayReturnsFalseWhenGatewayUnhealthy(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	original := gatewayHealthCheck
	gatewayHealthCheck = func() bool { return false }
	t.Cleanup(func() { gatewayHealthCheck = original })

	assert.False(t, isHostPublicFeedConfigured(context.Background(), fileSvc))
	assert.False(t, shouldPublishViaGateway(context.Background(), fileSvc))
}

func TestMirrorCatalogDatasetPresent(t *testing.T) {
	assert.True(t, mirrorCatalogDatasetPresent(map[string]any{
		"kind":       "catalog_snapshot",
		"dataset_id": "eval-run-1",
	}, "eval-run-1"))
	assert.False(t, mirrorCatalogDatasetPresent(map[string]any{
		"kind":       "catalog_snapshot",
		"dataset_id": "other-run",
	}, "eval-run-1"))
	assert.True(t, mirrorCatalogDatasetPresent(map[string]any{
		"record": map[string]any{
			"kind":       "catalog_snapshot",
			"dataset_id": "nested-run",
		},
	}, "nested-run"))
}

func TestMirrorProjectionHasDataset(t *testing.T) {
	projections := []map[string]any{
		{"kind": "other", "dataset_id": "missing"},
		{"kind": "catalog_snapshot", "dataset_id": "eval-run-1"},
	}
	assert.True(t, mirrorProjectionHasDataset(projections, "eval-run-1"))
	assert.False(t, mirrorProjectionHasDataset(projections, "eval-run-2"))
}

func TestHTTPCampaignMirrorProbe_DatasetPresentFromBootstrap(t *testing.T) {
	bootstrapServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicFeedBootstrap{
			Snapshot: models.PublicFeedSnapshot{HighWaterSequence: 1},
			RecentProjections: []map[string]any{
				{"kind": "catalog_snapshot", "dataset_id": "eval-run-1"},
			},
		}))
	}))
	t.Cleanup(bootstrapServer.Close)

	originalBootstrap := publicMirrorBootstrapURL
	originalHistory := publicMirrorHistoryURL
	publicMirrorBootstrapURL = bootstrapServer.URL
	publicMirrorHistoryURL = bootstrapServer.URL + "/history"
	t.Cleanup(func() {
		publicMirrorBootstrapURL = originalBootstrap
		publicMirrorHistoryURL = originalHistory
	})

	probe := newHTTPCampaignMirrorProbe(context.Background())
	present, err := probe.DatasetPresent(context.Background(), "eval-run-1")
	require.NoError(t, err)
	assert.True(t, present)
}

func TestIsPublicFeedSequenceOutOfOrder(t *testing.T) {
	assert.True(t, isPublicFeedSequenceOutOfOrder(fmt.Errorf("gateway export batch: %w", constants.ErrPublicFeedSequenceOutOfOrder)))
	assert.False(t, isPublicFeedSequenceOutOfOrder(nil))
	assert.False(t, isPublicFeedSequenceOutOfOrder(fmt.Errorf("other error")))
}

func TestIsPublicFeedSnapshotNotFound(t *testing.T) {
	assert.True(t, isPublicFeedSnapshotNotFound(fmt.Errorf("snapshot lookup: %w", constants.ErrPublicFeedSnapshotNotFound)))
	assert.False(t, isPublicFeedSnapshotNotFound(nil))
}
