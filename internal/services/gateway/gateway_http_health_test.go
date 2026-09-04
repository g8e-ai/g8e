// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// --- handleLandingPage ---

func TestHandleLandingPage_NonRootPathReturns404(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/some/other/path", nil)
	rr := httptest.NewRecorder()

	h.healthController.handleLandingPage(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// --- handleHealth ---

func TestHandleHealth_NotReadyReturns503(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	h.healthController.isReady = func() bool { return false }

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleHealth(rr, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.JSONEq(t, `{"error":"service initializing"}`, rr.Body.String())
}

func TestHandleHealth_GovernanceReadyTrueWhenCallbackTrue(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)
	h.healthController.isGovernanceReady = func() bool { return true }

	settings := models.SettingsDocument{
		Settings:  &models.PlatformSettings{ActuatorKeyID: "test-key-id"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	settingsBytes, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionSettings),
		marshaler.DocumentID(constants.DocIDPlatformSettings),
		settingsBytes,
	))

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleHealth(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.GovernanceReady)
}

func TestHandleHealth_GovernanceReadyFalseWhenCallbackFalse(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)
	h.healthController.isGovernanceReady = func() bool { return false }

	settings := models.SettingsDocument{
		Settings:  &models.PlatformSettings{ActuatorKeyID: "test-key-id"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	settingsBytes, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionSettings),
		marshaler.DocumentID(constants.DocIDPlatformSettings),
		settingsBytes,
	))

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleHealth(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.False(t, resp.GovernanceReady)
}

func TestHandleHealth_GovernanceReadyFalseWhenCallbackNil(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)
	h.healthController.isGovernanceReady = nil

	settings := models.SettingsDocument{
		Settings:  &models.PlatformSettings{ActuatorKeyID: "test-key-id"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	settingsBytes, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionSettings),
		marshaler.DocumentID(constants.DocIDPlatformSettings),
		settingsBytes,
	))

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleHealth(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.False(t, resp.GovernanceReady)
}

func TestHandleHealth_VersionAndStateRootPopulated(t *testing.T) {
	h, cfg, infra := setupTestHTTPHandler(t)
	cfg.Version = "9.9.9-test"
	cfg.Gateway.Posture = config.PostureConsensus

	settings := models.SettingsDocument{
		Settings:  &models.PlatformSettings{ActuatorKeyID: "test-key-id"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	settingsBytes, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionSettings),
		marshaler.DocumentID(constants.DocIDPlatformSettings),
		settingsBytes,
	))

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleHealth(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "9.9.9-test", resp.Version)
	assert.Equal(t, constants.GatewayModeGateway, resp.Mode)
	assert.NotEmpty(t, resp.StateMerkleRoot, "StateMerkleRoot should be populated from state root service")
	assert.Equal(t, string(config.PostureConsensus), resp.Posture, "Posture should reflect the gateway's configured posture")
}

func TestHandleHealth_IsReadyNilProceedsToDBCheck(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)
	h.healthController.isReady = nil

	// Ensure platform_settings does not exist so we hit the 503 path
	infra.DocStore.DocDelete(
		marshaler.CollectionName(constants.CollectionSettings),
		marshaler.DocumentID(constants.DocIDPlatformSettings),
	)

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleHealth(rr, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.JSONEq(t, `{"error":"platform_settings not ready"}`, rr.Body.String())
}

// --- handleBootstrapHealth ---

func TestHandleBootstrapHealth_GovernanceReadyTrueWhenCallbackTrue(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	h.healthController.isReady = func() bool { return true }
	h.healthController.isGovernanceReady = func() bool { return true }

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleBootstrapHealth(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.GovernanceReady)
}

func TestHandleBootstrapHealth_GovernanceReadyFalseWhenCallbackFalse(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	h.healthController.isReady = func() bool { return true }
	h.healthController.isGovernanceReady = func() bool { return false }

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleBootstrapHealth(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.False(t, resp.GovernanceReady)
}

func TestHandleBootstrapHealth_GovernanceReadyFalseWhenCallbackNil(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	h.healthController.isReady = func() bool { return true }
	h.healthController.isGovernanceReady = nil

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleBootstrapHealth(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.False(t, resp.GovernanceReady)
}

func TestHandleBootstrapHealth_VersionPopulatedFromConfig(t *testing.T) {
	h, cfg, _ := setupTestHTTPHandler(t)
	h.healthController.isReady = func() bool { return true }
	cfg.Version = "3.3.3-bootstrap"
	cfg.Gateway.Posture = config.PostureDoctrine

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleBootstrapHealth(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.HealthResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "3.3.3-bootstrap", resp.Version)
	assert.Equal(t, constants.GatewayModeStatusOK, resp.Status)
	assert.Equal(t, constants.GatewayModeGateway, resp.Mode)
	assert.Empty(t, resp.StateMerkleRoot, "Bootstrap health must not include state merkle root")
	assert.Equal(t, string(config.PostureDoctrine), resp.Posture, "Posture should reflect the gateway's configured posture")
}

func TestHandleBootstrapHealth_IsReadyNilReturns200(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	h.healthController.isReady = nil

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleBootstrapHealth(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

// --- handleState ---

func TestHandleState_NotReadyReturns503(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	h.healthController.isReady = func() bool { return false }

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.State, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleState(rr, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.JSONEq(t, `{"error":"service initializing"}`, rr.Body.String())
}

func TestHandleState_ReturnsStateMerkleRoot(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	h.healthController.isReady = func() bool { return true }

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.State, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleState(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.StateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.StateMerkleRoot, "State endpoint should return a non-empty merkle root")
}

func TestHandleState_StateRootFailureReturns503(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)
	h.healthController.isReady = func() bool { return true }

	// Force state root calculation to fail by dropping a table it queries
	_, err := infra.DB.db.Exec("DROP TABLE kv_store")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.State, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleState(rr, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.JSONEq(t, `{"error":"state root calculation failed"}`, rr.Body.String())
}

func TestHandleState_IsReadyNilReturns200(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	h.healthController.isReady = nil

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.State, nil)
	rr := httptest.NewRecorder()

	h.healthController.handleState(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp models.StateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.StateMerkleRoot)
}
