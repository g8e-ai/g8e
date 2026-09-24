//go:build integration

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestMirrorWithdrawCampaignDatasetHidesHistoryAndBootstrap(t *testing.T) {
	env := newMirrorTestEnv(t)
	runID := "eval-init-glm-5-3-air-1790258676"
	datasetID := campaignMirrorDatasetID(runID)
	batch := env.buildBatch([]models.PublicFeedRecord{
		env.makeRecord(1, models.NewPublicFeedObject(map[string]string{
			"kind":           "evaluation_summary",
			"dataset_id":     datasetID,
			"run_id":         runID,
			"schema_version": "1.5.0",
		})),
	}, constants.PublicFeedZeroHashHex)
	_, response := env.sendIngest(batch)
	require.True(t, response.Accepted)

	require.NoError(t, env.mirror.WithdrawCampaignDataset(context.Background(), runID))
	assert.True(t, env.mirror.CampaignDatasetWithdrawn(runID))

	bootstrapReq := httptest.NewRequest(http.MethodGet, "/bootstrap", nil)
	bootstrapRec := httptest.NewRecorder()
	env.mirror.handleBootstrap(bootstrapRec, bootstrapReq)
	require.Equal(t, http.StatusOK, bootstrapRec.Code)
	var bootstrap models.PublicFeedBootstrap
	require.NoError(t, json.Unmarshal(bootstrapRec.Body.Bytes(), &bootstrap))
	assert.Empty(t, bootstrap.RecentProjections)

	historyReq := httptest.NewRequest(http.MethodGet, "/history?limit=10", nil)
	historyRec := httptest.NewRecorder()
	env.mirror.handleHistory(historyRec, historyReq)
	require.Equal(t, http.StatusOK, historyRec.Code)
	var history models.PublicFeedCursorPage
	require.NoError(t, json.Unmarshal(historyRec.Body.Bytes(), &history))
	assert.Empty(t, history.Items)
}

func TestMirrorProjectionMatchesCampaignRun(t *testing.T) {
	runID := "eval-init-qwen3-4b-1789000001"
	item := models.NewPublicFeedObject(map[string]string{
		"idempotency_key": runID + ":aggregate:catalog:s75:t0",
	})
	assert.True(t, mirrorProjectionMatchesCampaignRun(item, runID))
	assert.False(t, mirrorProjectionMatchesCampaignRun(item, "other-run"))
}
