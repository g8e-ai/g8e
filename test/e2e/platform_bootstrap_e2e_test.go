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

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// TestPlatform_FullBootstrap verifies that a fully bootstrapped live stack
// (gateway, operator, ensemble, dashboard) is healthy and properly connected.
// It assumes the stack is running and enrolled; it does not start or stop
// containers.
func TestPlatform_FullBootstrap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	t.Run("GatewayHealth", func(t *testing.T) {
		health, err := e2eClient.GetHealth(ctx, e2eCfg.gatewayHTTPURL)
		require.NoError(t, err, "gateway health check must succeed")
		assert.Equal(t, constants.GatewayModeStatusOK, health.Status, "gateway must report status ok")
	})

	t.Run("OperatorRegistered", func(t *testing.T) {
		operators, err := e2eClient.ListOperators(ctx)
		require.NoError(t, err, "listing operators must succeed")
		require.True(t, operators.Success, "operator list must report success")
		require.NotEmpty(t, operators.Operators, "at least one operator must be registered")
		assert.NotEmpty(t, operators.Operators[0].OperatorSessionID, "operator must have an active session ID")
	})

	t.Run("EnsembleHealth", func(t *testing.T) {
		body, err := e2eClient.GetEnsembleHealth(ctx, e2eCfg.ensembleURL)
		require.NoError(t, err, "ensemble health check must succeed")
		require.True(t, isEnsembleHealthy(body), "ensemble /health must report ok: %s", string(body))
	})

	t.Run("EnsembleDetailedHealth", func(t *testing.T) {
		body, err := e2eClient.GetEnsembleDetailedHealth(ctx, e2eCfg.ensembleURL)
		require.NoError(t, err, "ensemble /health/details must be reachable")

		detailed, err := decodeJSON[ensembleDetailedHealthResponse](body, "ensemble detailed health")
		require.NoError(t, err, "ensemble detailed health must decode")
		require.NotNil(t, detailed.Clients, "ensemble /health/details must include clients map")

		for _, clientName := range expectedEnsembleClients {
			val, present := detailed.Clients[clientName]
			require.True(t, present, "client %q must be in detailed health: %v", clientName, detailed.Clients)
			assert.Equal(t, "up", val, "client %q must be up", clientName)
		}
	})

	t.Run("DashboardIndex", func(t *testing.T) {
		body, err := e2eClient.GetDashboardIndex(ctx, e2eCfg.dashboardURL)
		require.NoError(t, err, "dashboard index must be reachable")
		assert.NotEmpty(t, body, "dashboard index must serve non-empty body")
	})
}
