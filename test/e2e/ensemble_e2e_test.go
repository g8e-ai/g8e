// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// ensembleDetailedHealthResponse is the typed model for the ensemble
// /health/details endpoint. The ensemble returns a status field and a clients
// map whose keys are service names (cache_aside_service, operator_kv,
// internal_http_client, operator_command_service, chat_pipeline) and whose
// values are "up" or "down". This replaces the prior map[string]interface{}
// assertion with a typed decode path.
type ensembleDetailedHealthResponse struct {
	Status  string            `json:"status"`
	Clients map[string]string `json:"clients"`
}

// expectedEnsembleClients are the service keys the ensemble health router
// reports when its startup phases complete without raising. Each key checks
// service-object existence on app.state via getattr; a value of "up" means
// the startup phase that creates the service object completed. If enrollment
// had failed, the lifespan exception handler would re-raise and FastAPI would
// never start serving, so /health/details would be unreachable. Reaching this
// endpoint with all services "up" is indirect proof that enrollment succeeded
// and the startup phases completed.
var expectedEnsembleClients = []string{
	"cache_aside_service",
	"operator_kv",
	"internal_http_client",
	"operator_command_service",
	"chat_pipeline",
}

// TestEnsemble_Health verifies the ensemble service is reachable and reports
// a healthy status. The ensemble runs on its own port alongside the gateway;
// its URL is derived from the gateway HTTP URL by port replacement. This
// replaces the prior container-running check with an API-visible health
// assertion.
func TestEnsemble_Health(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	body, err := e2eClient.GetEnsembleHealth(ctx, e2eCfg.ensembleURL)
	require.NoError(t, err, "ensemble /health must be reachable on an approved stack")
	require.True(t, isEnsembleHealthy(body),
		"ensemble /health must report status ok, got: %s", string(body))
	t.Logf("ensemble health: %s", string(body))
}

// TestEnsemble_DetailedHealth verifies the ensemble /health/details endpoint
// reports all expected operator clients as up. This is the typed replacement
// for the prior map[string]interface{} assertion: the response is decoded
// into ensembleDetailedHealthResponse and each expected client key is checked
// for the "up" value.
func TestEnsemble_DetailedHealth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	body, err := e2eClient.GetEnsembleDetailedHealth(ctx, e2eCfg.ensembleURL)
	require.NoError(t, err, "ensemble /health/details must be reachable on an approved stack")

	detailed, err := decodeJSON[ensembleDetailedHealthResponse](body, "ensemble detailed health")
	require.NoError(t, err, "ensemble /health/details must decode as a typed response")

	require.NotNil(t, detailed.Clients, "ensemble /health/details must include a clients map")
	for _, key := range expectedEnsembleClients {
		val, present := detailed.Clients[key]
		require.True(t, present, "clients map must include key %q: %v", key, detailed.Clients)
		assert.Equal(t, "up", val, "ensemble client %q must be up, got %q", key, val)
	}
	t.Logf("ensemble detailed health: %d clients all up", len(expectedEnsembleClients))
}

// TestDashboard_Index verifies the dashboard service is reachable and serves
// its index page over HTTP. The dashboard runs on its own port alongside the
// gateway; its URL is derived from the gateway HTTP URL by port replacement.
// This replaces the prior container-running check with an API-visible HTTP
// assertion.
func TestDashboard_Index(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	body, err := e2eClient.GetDashboardIndex(ctx, e2eCfg.dashboardURL)
	require.NoError(t, err, "dashboard index must be reachable on an approved stack")
	require.NotEmpty(t, body, "dashboard index must serve non-empty content")
	t.Logf("dashboard index served: %d bytes", len(body))
}

// TestEnsemble_GatewayStillHealthy verifies the gateway remains healthy while
// the ensemble is connected, proving the ensemble enrollment did not disrupt
// gateway operation.
func TestEnsemble_GatewayStillHealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	health, err := e2eClient.GetHealth(ctx, e2eCfg.gatewayHTTPURL)
	require.NoError(t, err, "gateway health must succeed while ensemble is connected")
	assert.Equal(t, constants.GatewayModeStatusOK, health.Status, "gateway must report ok status with ensemble connected")
}
