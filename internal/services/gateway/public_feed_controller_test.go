// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

type testPublicFeedProjection struct {
	SchemaVersion string `json:"schema_version"`
	Kind          string `json:"kind"`
	DatasetID     string `json:"dataset_id"`
	QualityState  string `json:"quality_state"`
	ObservedAt    string `json:"observed_at"`
}

func testPublicFeedProjectionBytes(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(testPublicFeedProjection{
		SchemaVersion: "1.3.0",
		Kind:          "catalog_snapshot",
		DatasetID:     "test-dataset",
		QualityState:  "live_in_progress",
		ObservedAt:    "2026-09-21T00:00:00Z",
	})
	require.NoError(t, err)
	return body
}

func TestPublicFeedControllerHandlePublicFeedBatches_RejectsWhenSpectatorUnavailable(t *testing.T) {
	logger := testutil.NewTestLogger()
	controller := newPublicFeedController(PublicFeedControllerDeps{
		Logger:    logger,
		Responder: response.NewWriter(logger),
		Spectator: func() *PublicSpectatorRuntime { return nil },
	})

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PublicFeedBatches, bytes.NewReader([]byte("[]")))
	rr := httptest.NewRecorder()
	controller.handlePublicFeedBatches(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestPublicFeedControllerHandlePublicFeedBatches_RejectsNonPost(t *testing.T) {
	logger := testutil.NewTestLogger()
	controller := newPublicFeedController(PublicFeedControllerDeps{
		Logger:    logger,
		Responder: response.NewWriter(logger),
		Spectator: func() *PublicSpectatorRuntime { return nil },
	})

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.PublicFeedBatches, nil)
	rr := httptest.NewRecorder()
	controller.handlePublicFeedBatches(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestPublicFeedControllerHandlePublicFeedBatches_AcceptsBatch(t *testing.T) {
	privatePort := mustFreePort(t)
	publicPort := mustFreePort(t)
	runtime, err := NewPublicSpectatorRuntime(PublicSpectatorConfig{
		Enabled:              true,
		PrivateListenAddress: "127.0.0.1:" + privatePort,
		PublicListenAddress:  "127.0.0.1:" + publicPort,
		SourceID:             "test-source",
	}, newProducerFileSvc(t), testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, runtime.Start(t.Context()))

	logger := testutil.NewTestLogger()
	controller := newPublicFeedController(PublicFeedControllerDeps{
		Logger:    logger,
		Responder: response.NewWriter(logger),
		Spectator: func() *PublicSpectatorRuntime { return runtime },
	})

	recordBytes := testPublicFeedProjectionBytes(t)
	recordHash := sha256.Sum256(recordBytes)
	record := models.PublicFeedRecord{
		Sequence:    1,
		RecordType:  models.PublicFeedRecordTypeProjection,
		RecordHash:  hex.EncodeToString(recordHash[:]),
		RecordBytes: string(recordBytes),
	}
	body, err := json.Marshal([]models.PublicFeedRecord{record})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PublicFeedBatches, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	controller.handlePublicFeedBatches(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp models.PublicFeedExportBatchResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, int64(1), resp.HighWaterSequence)
	assert.NotEmpty(t, resp.FeedChainHash)

	require.NoError(t, runtime.Stop(t.Context()))
}

func TestPublicFeedControllerHandlePublicFeedBatches_RejectsUnknownEventKind(t *testing.T) {
	privatePort := mustFreePort(t)
	publicPort := mustFreePort(t)
	runtime, err := NewPublicSpectatorRuntime(PublicSpectatorConfig{
		Enabled:              true,
		PrivateListenAddress: "127.0.0.1:" + privatePort,
		PublicListenAddress:  "127.0.0.1:" + publicPort,
		SourceID:             "test-source",
	}, newProducerFileSvc(t), testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, runtime.Start(t.Context()))

	recordBytes := []byte(`{"kind":"heartbeat"}`)
	recordHash := sha256.Sum256(recordBytes)
	body, err := json.Marshal([]models.PublicFeedRecord{{
		Sequence:    1,
		RecordType:  models.PublicFeedRecordTypeEvent,
		RecordHash:  hex.EncodeToString(recordHash[:]),
		RecordBytes: string(recordBytes),
	}})
	require.NoError(t, err)

	controller := newPublicFeedController(PublicFeedControllerDeps{
		Logger:    testutil.NewTestLogger(),
		Responder: response.NewWriter(testutil.NewTestLogger()),
		Spectator: func() *PublicSpectatorRuntime { return runtime },
	})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PublicFeedBatches, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	controller.handlePublicFeedBatches(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	require.NoError(t, runtime.Stop(t.Context()))
}

func TestPublicFeedControllerHandlePublicFeedSnapshot_ReturnsHighWater(t *testing.T) {
	privatePort := mustFreePort(t)
	publicPort := mustFreePort(t)
	runtime, err := NewPublicSpectatorRuntime(PublicSpectatorConfig{
		Enabled:              true,
		PrivateListenAddress: "127.0.0.1:" + privatePort,
		PublicListenAddress:  "127.0.0.1:" + publicPort,
		SourceID:             "test-source",
	}, newProducerFileSvc(t), testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, runtime.Start(t.Context()))

	logger := testutil.NewTestLogger()
	controller := newPublicFeedController(PublicFeedControllerDeps{
		Logger:    logger,
		Responder: response.NewWriter(logger),
		Spectator: func() *PublicSpectatorRuntime { return runtime },
	})

	recordBytes := testPublicFeedProjectionBytes(t)
	recordHash := sha256.Sum256(recordBytes)
	record := models.PublicFeedRecord{
		Sequence:    1,
		RecordType:  models.PublicFeedRecordTypeProjection,
		RecordHash:  hex.EncodeToString(recordHash[:]),
		RecordBytes: string(recordBytes),
	}
	body, err := json.Marshal([]models.PublicFeedRecord{record})
	require.NoError(t, err)

	postReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.PublicFeedBatches, bytes.NewReader(body))
	postRR := httptest.NewRecorder()
	controller.handlePublicFeedBatches(postRR, postReq)
	require.Equal(t, http.StatusOK, postRR.Code)

	getReq := httptest.NewRequest(http.MethodGet, constants.APIPaths.PublicFeedSnapshot, nil)
	getRR := httptest.NewRecorder()
	controller.handlePublicFeedSnapshot(getRR, getReq)
	require.Equal(t, http.StatusOK, getRR.Code)

	var snapshot models.PublicFeedSnapshot
	require.NoError(t, json.Unmarshal(getRR.Body.Bytes(), &snapshot))
	assert.Equal(t, int64(1), snapshot.HighWaterSequence)
	assert.NotEmpty(t, snapshot.FeedChainHash)

	require.NoError(t, runtime.Stop(t.Context()))
}
