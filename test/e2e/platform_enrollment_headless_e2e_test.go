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
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// TestPlatformEnrollment_Headless verifies the gateway-only deployment mode
// (headless): the gateway is running and bootstrapped, but no operator,
// dashboard, or ensemble workloads are running. The user starts only the
// gateway service (docker compose up -d --no-deps g8e-gateway or the
// equivalent), bootstraps the owner identity, then runs:
//
//	./g8e test e2e --run TestPlatformEnrollment_Headless
//
// The test asserts that the gateway health and CA bundle endpoints succeed,
// that the authenticated pending enrollment list is empty (no workloads are
// running to submit requests), that the operator list is empty (no operators
// are registered), and that the ensemble and dashboard endpoints are absent
// (their services are not running). This is the headless deployment mode: a
// usable gateway with no pending workloads or registered operators.
//
// This replaces the prior TestPlatformEnrollment_HeadlessGatewayOnly_E2E
// which used a per-test Docker fixture and inspected container state with
// docker inspect. This version asserts the same invariants through typed
// gateway APIs and endpoint reachability checks.
func TestPlatformEnrollment_Headless(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	// Gateway health must succeed in headless mode. The gateway is the only
	// running service and must be reachable on its HTTP port.
	health, err := e2eClient.GetHealth(ctx, e2eCfg.gatewayHTTPURL)
	require.NoError(t, err, "gateway health must succeed in headless mode")
	assert.Equal(t, constants.GatewayModeStatusOK, health.Status,
		"gateway must report ok status in headless mode")
	t.Logf("gateway healthy in headless mode: status=%s version=%s", health.Status, health.Version)

	// CA bundle must be available in headless mode. The gateway serves the
	// trust bundle from the well-known PKI endpoint regardless of workload
	// state.
	bundle, err := e2eClient.GetCABundle(ctx, e2eCfg.gatewayHTTPURL)
	require.NoError(t, err, "CA bundle endpoint must succeed in headless mode")
	require.NotEmpty(t, bundle, "CA bundle must not be empty in headless mode")
	assert.Contains(t, string(bundle), "BEGIN CERTIFICATE",
		"CA bundle must contain PEM certificates in headless mode")
	t.Logf("CA bundle available in headless mode: %d bytes", len(bundle))

	// Authenticated pending list must be empty. No workloads are running,
	// so no platform enrollment requests have been submitted. The owner
	// authentication (mTLS + session header) must still succeed — headless
	// mode does not disable owner authentication.
	pending, err := e2eClient.GetPendingEnrollments(ctx)
	require.NoError(t, err,
		"authenticated pending list must succeed in headless mode — owner mTLS must work without workloads")
	assert.Empty(t, pending.Requests,
		"pending enrollment list must be empty in headless mode — no workloads are running to submit requests")
	t.Logf("pending enrollment list is empty in headless mode")

	// Operator list must be empty. No operators are registered because no
	// operator workload is running and no enrollment has been approved.
	operators, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err,
		"authenticated operator list must succeed in headless mode — owner mTLS must work without workloads")
	assert.True(t, operators.Success, "operator list response must report success")
	assert.Empty(t, operators.Operators,
		"operator list must be empty in headless mode — no operators are registered")
	t.Logf("operator list is empty in headless mode")

	// Ensemble and dashboard endpoints must be absent. Their services are
	// not running in headless mode, so connecting to their ports must fail.
	// A short timeout ensures the test fails fast rather than hanging on
	// a connection attempt to a non-listening port.
	endpointCtx, endpointCancel := context.WithTimeout(ctx, 5*time.Second)
	defer endpointCancel()

	ensembleReq, err := http.NewRequestWithContext(endpointCtx, http.MethodGet, e2eCfg.ensembleURL+"/health", nil)
	require.NoError(t, err, "build ensemble health request must succeed")
	ensembleClient := &http.Client{Timeout: 5 * time.Second}
	_, ensembleErr := ensembleClient.Do(ensembleReq)
	assert.Error(t, ensembleErr,
		"ensemble endpoint must be unreachable in headless mode — the ensemble service is not running")
	t.Logf("ensemble endpoint correctly absent in headless mode: %v", ensembleErr)

	dashboardReq, err := http.NewRequestWithContext(endpointCtx, http.MethodGet, e2eCfg.dashboardURL+"/", nil)
	require.NoError(t, err, "build dashboard index request must succeed")
	dashboardClient := &http.Client{Timeout: 5 * time.Second}
	_, dashboardErr := dashboardClient.Do(dashboardReq)
	assert.Error(t, dashboardErr,
		"dashboard endpoint must be unreachable in headless mode — the dashboard service is not running")
	t.Logf("dashboard endpoint correctly absent in headless mode: %v", dashboardErr)
}
