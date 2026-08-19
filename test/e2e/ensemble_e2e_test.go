// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnsembleGatewayOperator_E2E exercises the ensemble -> gateway -> operator
// path end to end against the unified docker-compose stack. The shared fixture
// (TestMain in main_test.go) brings up all 4 services (gateway, operator,
// ensemble, dashboard) and `docker compose up --wait` blocks until every
// service with a healthcheck reaches `healthy`. With the ensemble and dashboard
// healthchecks added in WS3, the test body can assume a healthy stack without
// require.Eventually retry logic.
//
// This is a Tier 3 (Docker E2E) test. It does NOT exercise a full
// chat -> consensus -> command -> execution roundtrip, because that requires an
// LLM provider (API keys) which the E2E environment does not have. That
// roundtrip is a Tier 4 (External) test, not Tier 3. The v2.0.0 success
// criterion is that the unified stack comes up healthy with all 4 services
// connected — the ensemble enrolled and its operator clients up, the dashboard
// serving — not that a full agentic loop runs.
func TestEnsembleGatewayOperator_E2E(t *testing.T) {
	if sharedFixture == nil {
		t.Skip("Docker E2E fixture not available")
	}
	f := sharedFixture

	t.Run("ensemble container is running", func(t *testing.T) {
		f.CheckEnsembleContainer(t)
		t.Log("Ensemble container is running and enrollment completed (log marker present)")
	})

	t.Run("ensemble health endpoint returns ok", func(t *testing.T) {
		health := f.GetEnsembleHealth(t)
		require.Equal(t, "ok", health["status"], "ensemble /health status != ok")
		t.Logf("Ensemble health: %v", health)
	})

	t.Run("ensemble operator clients are connected", func(t *testing.T) {
		detailed := f.GetEnsembleDetailedHealth(t)
		clients, ok := detailed["clients"].(map[string]interface{})
		require.True(t, ok, "ensemble /health/details response missing 'clients' map: %v", detailed)

		// The clients map keys are defined in
		// ensemble/app/routers/health_router.py: cache_aside_service,
		// operator_kv (which checks state.pubsub_client, not the KV client —
		// mislabeled in the health router), internal_http_client,
		// operator_command_service, and chat_pipeline. Each key checks
		// service-object existence on app.state via getattr, not live mTLS
		// connection state. A value of "up" means the startup phase that
		// creates the service object completed without raising. If enrollment
		// had failed, the lifespan exception handler would re-raise and FastAPI
		// would never start serving, so /health/details would be unreachable.
		// Reaching this endpoint with all services "up" is therefore indirect
		// proof that enrollment succeeded and the startup phases completed. A
		// future workstream could add a live mTLS connectivity probe to the
		// health endpoint for a direct connection check.
		expectedClients := []string{
			"cache_aside_service",
			"operator_kv",
			"internal_http_client",
			"operator_command_service",
			"chat_pipeline",
		}
		for _, key := range expectedClients {
			val, present := clients[key]
			require.True(t, present, "clients map missing key %q: %v", key, clients)
			require.Equal(t, "up", val, "client %q is not up: %v", key, clients)
		}
		t.Logf("Ensemble detailed health clients: %v", clients)
	})

	t.Run("dashboard container is running", func(t *testing.T) {
		f.CheckDashboardContainer(t)
		t.Log("Dashboard container is running and serving its index page")
	})

	t.Run("gateway still healthy with ensemble connected", func(t *testing.T) {
		health := f.GetHealth(t)
		require.Equal(t, "ok", health["status"], "gateway health check failed after ensemble connected")
		t.Logf("Gateway health with ensemble connected: %v", health)
	})
}
