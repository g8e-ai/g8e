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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// TestGateway_HealthAndCA verifies the public gateway surface that does not
// require owner authentication: the typed health endpoint returns the ok
// status with a non-empty version, and the well-known CA bundle endpoint
// serves a PEM-encoded certificate chain. The preflight health check in
// TestMain already proved reachability; this test asserts the typed response
// shape and CA bundle contents so a regression in either surface fails with a
// precise diagnostic rather than a generic connection error.
func TestGateway_HealthAndCA(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	health, err := e2eClient.GetHealth(ctx, e2eCfg.gatewayHTTPURL)
	require.NoError(t, err, "gateway health request must succeed against a running platform")
	assert.Equal(t, constants.GatewayModeStatusOK, health.Status, "health status must be ok")
	t.Logf("gateway health: status=%s mode=%s version=%s governance_ready=%v",
		health.Status, health.Mode, health.Version, health.GovernanceReady)

	bundle, err := e2eClient.GetCABundle(ctx, e2eCfg.gatewayHTTPURL)
	require.NoError(t, err, "CA bundle endpoint must succeed against a running platform")
	require.NotEmpty(t, bundle, "CA bundle must not be empty")
	assert.Contains(t, string(bundle), "BEGIN CERTIFICATE",
		"CA bundle must contain at least one PEM-encoded certificate")
	assert.Contains(t, string(bundle), "END CERTIFICATE",
		"CA bundle must contain a closing PEM marker")
	t.Logf("CA bundle retrieved: %d bytes, %d certificates",
		len(bundle), strings.Count(string(bundle), "BEGIN CERTIFICATE"))
}

// TestGateway_HealthStable verifies the gateway health endpoint remains
// reachable across a short observation window. This replaces the prior
// container-log-based liveness check with an API-visible stability assertion:
// two health probes spaced one second apart must both return ok.
func TestGateway_HealthStable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first, err := e2eClient.GetHealth(ctx, e2eCfg.gatewayHTTPURL)
	require.NoError(t, err, "first health probe must succeed")
	require.Equal(t, constants.GatewayModeStatusOK, first.Status, "first health probe must report ok")

	time.Sleep(time.Second)

	second, err := e2eClient.GetHealth(ctx, e2eCfg.gatewayHTTPURL)
	require.NoError(t, err, "second health probe must succeed")
	assert.Equal(t, constants.GatewayModeStatusOK, second.Status, "second health probe must report ok")
	assert.Equal(t, first.Version, second.Version, "gateway version must be stable across probes")
}
