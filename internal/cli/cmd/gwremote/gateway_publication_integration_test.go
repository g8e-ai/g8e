// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gwremote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestHTTPCampaignMirrorProbe_DatasetPresentFromBootstrap(t *testing.T) {
	bootstrapServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicFeedBootstrap{
			Snapshot: models.PublicFeedSnapshot{HighWaterSequence: 1},
			RecentProjections: []models.PublicFeedObject{
				models.NewPublicFeedObject(map[string]string{"kind": "catalog_snapshot", "dataset_id": "eval-run-1"}),
			},
		}))
	}))
	t.Cleanup(bootstrapServer.Close)

	originalBootstrap := PublicMirrorBootstrapURL
	originalHistory := publicMirrorHistoryURL
	PublicMirrorBootstrapURL = bootstrapServer.URL
	publicMirrorHistoryURL = bootstrapServer.URL + "/history"
	t.Cleanup(func() {
		PublicMirrorBootstrapURL = originalBootstrap
		publicMirrorHistoryURL = originalHistory
	})

	probe := NewHTTPCampaignMirrorProbe(context.Background())
	present, err := probe.DatasetPresent(context.Background(), "eval-run-1")
	require.NoError(t, err)
	assert.True(t, present)
}

func TestGatewayPublisherStatus_ReturnsBootstrapFields(t *testing.T) {
	WithGatewayHealthCheck(t, true)
	cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

	bootstrapServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicFeedBootstrap{
			Snapshot: models.PublicFeedSnapshot{
				SourceID:          "source-1",
				HighWaterSequence: 9,
				FeedChainHash:     "chain-hash",
				BatchCount:        3,
			},
		}))
	}))
	t.Cleanup(bootstrapServer.Close)

	originalBootstrap := PublicMirrorBootstrapURL
	PublicMirrorBootstrapURL = bootstrapServer.URL
	t.Cleanup(func() { PublicMirrorBootstrapURL = originalBootstrap })

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case constants.APIPaths.PublicFeedSnapshot:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"high_water_sequence":9}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(gateway.Close)
	cfg.Paths = &config.PathsConfig{Host: gateway.URL}

	status, err := GatewayPublisherStatus(context.Background(), fileSvc, cfg)
	require.NoError(t, err)
	assert.True(t, status.Enabled)
	assert.Equal(t, "source-1", status.SourceID)
	assert.Equal(t, int64(9), status.HighWaterSequence)
	assert.Equal(t, "chain-hash", status.FeedChainHash)
	assert.Equal(t, 3, status.BatchCount)
}

func TestPublishJSONLViaGateway_ExportsRecords(t *testing.T) {
	WithGatewayHealthCheck(t, true)
	cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

	recordsPath := filepath.Join(t.TempDir(), "records.jsonl")
	recordLine, err := json.Marshal(models.PublicFeedRecordInput{
		RecordType:  models.PublicFeedRecordTypeProjection,
		RecordBytes: `{"schema_version":"1.3.0","kind":"catalog_snapshot","dataset_id":"campaign-a","quality_state":"live_in_progress","observed_at":"2026-09-21T00:00:00Z"}`,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recordsPath, append(recordLine, '\n'), 0o600))

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case constants.APIPaths.PublicFeedSnapshot:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"high_water_sequence":0}`))
		case constants.APIPaths.PublicFeedBatches:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"high_water_sequence":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(gateway.Close)
	config.SetEndpointOverride(gateway.URL)
	t.Cleanup(func() { config.SetEndpointOverride("") })

	published, err := PublishJSONLViaGateway(context.Background(), fileSvc, cfg, recordsPath)
	require.NoError(t, err)
	assert.Equal(t, 1, published)
}

func TestFetchPublicMirrorHistory_ReturnsCursorPage(t *testing.T) {
	historyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "source-1", r.URL.Query().Get("source"))
		assert.Equal(t, "cursor-1", r.URL.Query().Get("cursor"))
		assert.Equal(t, "25", r.URL.Query().Get("limit"))
		assert.Equal(t, "catalog_snapshot", r.URL.Query().Get("kind"))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicFeedCursorPage{
			Items: []models.PublicFeedObject{
				models.NewPublicFeedObject(map[string]string{"kind": "catalog_snapshot", "dataset_id": "eval-run-1"}),
			},
			Cursor:  "cursor-2",
			HasMore: true,
		}))
	}))
	t.Cleanup(historyServer.Close)

	originalHistory := publicMirrorHistoryURL
	publicMirrorHistoryURL = historyServer.URL
	t.Cleanup(func() { publicMirrorHistoryURL = originalHistory })

	page, err := fetchPublicMirrorHistory(context.Background(), nil, "source-1", "cursor-1", 25)
	require.NoError(t, err)
	assert.True(t, page.HasMore)
	assert.Equal(t, "cursor-2", page.Cursor)
	assert.Len(t, page.Items, 1)
}

func TestHTTPCampaignMirrorProbe_DatasetPresentFromHistory(t *testing.T) {
	bootstrapServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicFeedBootstrap{
			Snapshot: models.PublicFeedSnapshot{HighWaterSequence: 1},
		}))
	}))
	historyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicFeedCursorPage{
			Items: []models.PublicFeedObject{
				models.NewPublicFeedObject(map[string]string{"kind": "catalog_snapshot", "dataset_id": "eval-run-history"}),
			},
		}))
	}))
	t.Cleanup(bootstrapServer.Close)
	t.Cleanup(historyServer.Close)

	originalBootstrap := PublicMirrorBootstrapURL
	originalHistory := publicMirrorHistoryURL
	PublicMirrorBootstrapURL = bootstrapServer.URL
	publicMirrorHistoryURL = historyServer.URL
	t.Cleanup(func() {
		PublicMirrorBootstrapURL = originalBootstrap
		publicMirrorHistoryURL = originalHistory
	})

	probe := NewHTTPCampaignMirrorProbe(context.Background())
	present, err := probe.DatasetPresent(context.Background(), "eval-run-history")
	require.NoError(t, err)
	assert.True(t, present)
}

func TestHTTPCampaignMirrorProbe_IndexesMirrorOnceForMultipleDatasets(t *testing.T) {
	var bootstrapRequests atomic.Int32
	var historyRequests atomic.Int32
	bootstrapServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bootstrapRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicFeedBootstrap{
			Snapshot: models.PublicFeedSnapshot{HighWaterSequence: 2},
			RecentProjections: []models.PublicFeedObject{
				models.NewPublicFeedObject(map[string]string{"kind": "catalog_snapshot", "dataset_id": "eval-run-recent"}),
			},
		}))
	}))
	historyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		historyRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicFeedCursorPage{
			Items: []models.PublicFeedObject{
				models.NewPublicFeedObject(map[string]string{"kind": "catalog_snapshot", "dataset_id": "eval-run-history"}),
			},
		}))
	}))
	t.Cleanup(bootstrapServer.Close)
	t.Cleanup(historyServer.Close)

	originalBootstrap := PublicMirrorBootstrapURL
	originalHistory := publicMirrorHistoryURL
	PublicMirrorBootstrapURL = bootstrapServer.URL
	publicMirrorHistoryURL = historyServer.URL
	t.Cleanup(func() {
		PublicMirrorBootstrapURL = originalBootstrap
		publicMirrorHistoryURL = originalHistory
	})

	probe := NewHTTPCampaignMirrorProbe(context.Background())
	for datasetID, expected := range map[string]bool{
		"eval-run-recent":  true,
		"eval-run-history": true,
		"eval-run-missing": false,
	} {
		present, err := probe.DatasetPresent(context.Background(), datasetID)
		require.NoError(t, err)
		assert.Equal(t, expected, present)
	}
	assert.Equal(t, int32(1), bootstrapRequests.Load())
	assert.Equal(t, int32(1), historyRequests.Load())
}

func TestFetchPublicMirrorBootstrap_RetriesRateLimit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, constants.ErrPublicFeedRateLimited.Error(), http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.PublicFeedBootstrap{}))
	}))
	t.Cleanup(server.Close)

	originalBootstrap := PublicMirrorBootstrapURL
	PublicMirrorBootstrapURL = server.URL
	t.Cleanup(func() { PublicMirrorBootstrapURL = originalBootstrap })

	_, err := FetchPublicMirrorBootstrap(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(2), requests.Load())
}
