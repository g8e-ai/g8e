// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
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
