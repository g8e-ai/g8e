// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/models"
)

// TestPlatformEnrollment_Denial_E2E verifies that denying a platform enrollment
// request prevents the component from becoming healthy. The operator submits a
// platform enrollment request on startup; the owner denies it; the operator
// container must never reach the healthy state (operator.crt is never written).
// This is a per-test fixture (not the shared fixture) because the denial
// scenario requires a fresh stack where no approvals have been issued.
func TestPlatformEnrollment_Denial_E2E(t *testing.T) {
	f := NewDockerE2EFixtureUpToBootstrap(t, "docker-compose.yml")

	t.Run("pending operator request appears", func(t *testing.T) {
		req, err := f.waitForPendingRequestByKind(models.PlatformComponentOperator, 60*time.Second)
		require.NoError(t, err, "Operator platform enrollment request should appear in pending list")
		assert.Equal(t, models.PlatformEnrollmentStatePending, req.State)
		assert.NotEmpty(t, req.RequestID, "Request ID must not be empty")
		t.Logf("Found pending operator request: %s (instance: %s, hostname: %s)", req.RequestID, req.InstanceID, req.Hostname)
	})

	t.Run("pending list does not expose tokens", func(t *testing.T) {
		// The typed pending model does not expose tokens, but verify the raw
		// JSON does not contain token-like strings either (defense-in-depth).
		raw := f.fetchPendingRaw(t)
		assert.NotContains(t, raw, "token_hash", "pending list must not expose token_hash")
		assert.NotContains(t, raw, "csr_pem", "pending list must not expose csr_pem")
		assert.NotContains(t, raw, "BEGIN CERTIFICATE", "pending list must not expose certificate PEM")
	})

	t.Run("deny operator enrollment", func(t *testing.T) {
		req, err := f.waitForPendingRequestByKind(models.PlatformComponentOperator, 30*time.Second)
		require.NoError(t, err)
		require.NoError(t, f.denyEnrollment(req.RequestID), "Denying operator enrollment should succeed")
		t.Logf("Denied operator enrollment request %s", req.RequestID)
	})

	t.Run("denied operator never becomes healthy", func(t *testing.T) {
		opContainer := f.ContainerPrefix + "-operator"
		// The operator healthcheck checks operator.crt existence. A denied
		// enrollment never writes operator.crt, so the healthcheck must
		// transition to unhealthy after the start_period. Wait long enough
		// for the denial to propagate and the healthcheck to exhaust retries.
		err := f.waitForContainerUnhealthy(opContainer, 120*time.Second)
		require.NoError(t, err, "Denied operator container must become unhealthy (operator.crt never written)")
		t.Logf("Operator container is unhealthy as expected after denial")
	})

	t.Run("denied request state is terminal", func(t *testing.T) {
		// After denial, the request should no longer appear in the pending
		// list (it is in the denied terminal state, not pending).
		require.Eventually(t, func() bool {
			_, ok := f.pendingRequestByKind(models.PlatformComponentOperator)
			return !ok
		}, 30*time.Second, 2*time.Second, "Denied operator request should leave the pending list")
		t.Logf("Denied operator request is no longer in pending list")
	})
}

// TestPlatformEnrollment_HeadlessGatewayOnly_E2E verifies that starting only
// the gateway service (headless mode) via explicit service selection produces
// a healthy gateway without any workloads. No platform enrollment requests
// are submitted because no workloads are running. This is the headless
// deployment mode: docker compose up -d --no-deps g8e-gateway.
func TestPlatformEnrollment_HeadlessGatewayOnly_E2E(t *testing.T) {
	f := NewDockerE2EFixtureGatewayOnly(t, "docker-compose.yml")

	t.Run("gateway is healthy in headless mode", func(t *testing.T) {
		health := f.GetHealth(t)
		assert.Equal(t, "ok", health["status"], "Gateway must be healthy in headless (gateway-only) mode")
		t.Logf("Gateway is healthy in headless mode")
	})

	t.Run("no workloads are running", func(t *testing.T) {
		// In headless mode, only the gateway container should be running.
		// The operator, dashboard, and ensemble containers should not exist.
		for _, suffix := range []string{"-operator", "-dashboard", "-ensemble"} {
			container := f.ContainerPrefix + suffix
			cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", container)
			output, err := cmd.CombinedOutput()
			if err != nil {
				// Container does not exist — expected in headless mode.
				t.Logf("Container %s does not exist (expected in headless mode)", container)
				continue
			}
			status := strings.TrimSpace(string(output))
			t.Logf("Container %s status: %s", container, status)
			t.Errorf("Container %s should not be running in headless mode (status: %s)", container, status)
		}
	})

	t.Run("no pending platform enrollment requests", func(t *testing.T) {
		// No workloads are running, so no platform enrollment requests should
		// be submitted. The pending list should be empty (or the endpoint
		// may return 401 if no owner has been bootstrapped — both are valid
		// headless states). We do not bootstrap a user here because headless
		// mode is gateway-only.
		pending := f.fetchPendingEnrollments()
		assert.Empty(t, pending.Requests, "No platform enrollment requests should exist in headless mode")
		t.Logf("No pending platform enrollment requests in headless mode")
	})

	t.Run("CA bundle is available", func(t *testing.T) {
		bundle := f.GetCABundle(t)
		assert.NotEmpty(t, bundle, "CA bundle must be available in headless mode")
		assert.Contains(t, bundle, "BEGIN CERTIFICATE", "CA bundle must contain PEM certificates")
		t.Logf("CA bundle is available in headless mode")
	})
}

// TestPlatformEnrollment_RestartDuringPending_E2E verifies that restarting a
// component during pending enrollment resumes the same request and key
// material rather than generating a new request. The operator submits a
// platform enrollment request and waits for approval. While it is pending,
// the operator container is restarted. After restart, the operator should
// resume polling the same request ID (not submit a new request). The test
// then approves the original request and confirms the operator becomes
// healthy. This is a per-test fixture because the shared fixture has already
// approved all enrollments.
func TestPlatformEnrollment_RestartDuringPending_E2E(t *testing.T) {
	f := NewDockerE2EFixtureUpToBootstrap(t, "docker-compose.yml")

	// Wait for the operator's pending request to appear.
	req, err := f.waitForPendingRequestByKind(models.PlatformComponentOperator, 60*time.Second)
	require.NoError(t, err, "Operator platform enrollment request should appear")
	originalRequestID := req.RequestID
	t.Logf("Original operator request: %s", originalRequestID)

	// Restart the operator while it is pending. The operator's pending state
	// (private keys, requester token, request ID, CSR fingerprints) is
	// persisted to pki/pending-enrollment/g8eo.json in the operator volume,
	// so after restart it should resume the same request.
	opContainer := f.ContainerPrefix + "-operator"
	_, err = f.restartContainer(opContainer)
	require.NoError(t, err, "Restarting operator during pending enrollment should succeed")
	t.Logf("Restarted operator while pending, waiting for it to resume polling")

	// After restart, the operator should resume the same request. Verify
	// no new request is created by checking that the original request ID is
	// still the only pending operator request. Give the operator time to
	// restart and re-submit its polling.
	time.Sleep(10 * time.Second)

	resumedReq, ok := f.pendingRequestByKind(models.PlatformComponentOperator)
	require.True(t, ok, "Operator request should still be pending after restart")
	assert.Equal(t, originalRequestID, resumedReq.RequestID,
		"Operator must resume the same request ID after restart, not create a new one")
	t.Logf("Operator resumed same request %s after restart", resumedReq.RequestID)

	// Now approve the original request. The operator should complete
	// enrollment and become healthy.
	require.NoError(t, f.approveEnrollment(originalRequestID), "Approving operator enrollment should succeed")
	t.Logf("Approved operator enrollment request %s", originalRequestID)

	require.NoError(t, f.waitForContainerHealth(opContainer, 180*time.Second),
		"Operator must become healthy after approval and restart")
	t.Logf("Operator is healthy after restart-during-pending and approval")
}

// TestPlatformEnrollment_PendingDiscoveryNoTokens_E2E verifies that the
// authenticated pending list endpoint returns request IDs and metadata but
// never exposes requester tokens, token hashes, CSR PEM, or certificates.
// This is a per-test fixture so we can inspect the pending list before any
// approvals occur.
func TestPlatformEnrollment_PendingDiscoveryNoTokens_E2E(t *testing.T) {
	f := NewDockerE2EFixtureUpToBootstrap(t, "docker-compose.yml")

	t.Run("pending requests appear for all components", func(t *testing.T) {
		// Wait for at least one request to appear, then verify all three
		// component kinds are present.
		_, err := f.waitForPendingRequestByKind(models.PlatformComponentOperator, 60*time.Second)
		require.NoError(t, err, "At least one pending request should appear")

		// Give the dashboard and ensemble time to submit their requests.
		require.Eventually(t, func() bool {
			pending := f.fetchPendingEnrollments()
			hasOp, hasDash, hasEns := false, false, false
			for _, r := range pending.Requests {
				switch r.ComponentKind {
				case models.PlatformComponentOperator:
					hasOp = true
				case models.PlatformComponentDashboard:
					hasDash = true
				case models.PlatformComponentEnsemble:
					hasEns = true
				}
			}
			return hasOp && hasDash && hasEns
		}, 60*time.Second, 2*time.Second, "All three component kinds should have pending requests")
		t.Logf("All three component kinds have pending requests")
	})

	t.Run("pending list output never contains tokens or secrets", func(t *testing.T) {
		raw := f.fetchPendingRaw(t)
		// The pending list must never expose requester tokens, token hashes,
		// CSR PEM, or certificates. These are secrets that could be used to
		// complete enrollment without owner approval.
		for _, forbidden := range []string{"token_hash", "csr_pem", "BEGIN CERTIFICATE", "BEGIN CERTIFICATE REQUEST", "requester_token"} {
			assert.NotContains(t, raw, forbidden, "Pending list must not expose %q", forbidden)
		}
		t.Logf("Pending list contains no tokens, hashes, CSR PEM, or certificates")
	})

	t.Run("request IDs are non-empty and unique", func(t *testing.T) {
		pending := f.fetchPendingEnrollments()
		require.NotEmpty(t, pending.Requests, "Pending list should have requests")
		seen := make(map[string]bool)
		for _, r := range pending.Requests {
			require.NotEmpty(t, r.RequestID, "Request ID must not be empty")
			require.False(t, seen[r.RequestID], "Request ID %s is duplicated", r.RequestID)
			seen[r.RequestID] = true
		}
		t.Logf("All %d request IDs are non-empty and unique", len(pending.Requests))
	})
}
