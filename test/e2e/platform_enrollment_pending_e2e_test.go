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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/models"
)

// TestPlatformEnrollment_PendingDiscovery verifies the authenticated pending
// platform enrollment list against a stack where the owner is bootstrapped but
// no enrollments have been approved. The user starts the full stack
// (docker compose up) without approving any enrollment requests, then runs:
//
//	./g8e test e2e --run TestPlatformEnrollment_PendingDiscovery
//
// The test asserts that all three component kinds (operator, dashboard,
// ensemble) appear in the pending list, that request IDs are non-empty and
// unique, and that the raw JSON response excludes requester tokens, token
// hashes, CSR PEM, certificate PEM, and private key material. The pending
// list is the owner's discovery surface for approval decisions; leaking
// secret material there would allow an attacker to complete enrollment
// without owner approval.
//
// This replaces the prior TestPlatformEnrollment_PendingDiscoveryNoTokens_E2E
// which used a per-test Docker fixture. This version connects to a
// user-managed platform and asserts the same invariants through the typed
// E2E client.
func TestPlatformEnrollment_PendingDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Wait for all three component kinds to appear. The operator, dashboard,
	// and ensemble submit enrollment requests on startup; they may not all
	// be present immediately. A 120-second window accommodates startup
	// jitter without being so generous that a missing component could pass.
	var pending models.PlatformEnrollmentPendingResponse
	require.Eventually(t, func() bool {
		p, err := e2eClient.GetPendingEnrollments(ctx)
		if err != nil {
			t.Logf("pending list attempt error: %v", err)
			return false
		}
		pending = p
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
	}, 120*time.Second, 3*time.Second,
		"all three component kinds (operator, dashboard, ensemble) must appear in the pending list")

	require.NotEmpty(t, pending.Requests, "pending list must contain requests on an unapproved stack")

	// Assert all three component kinds are present.
	kinds := make(map[models.PlatformComponentKind]bool, len(pending.Requests))
	for _, r := range pending.Requests {
		kinds[r.ComponentKind] = true
	}
	assert.True(t, kinds[models.PlatformComponentOperator], "pending list must include an operator request")
	assert.True(t, kinds[models.PlatformComponentDashboard], "pending list must include a dashboard request")
	assert.True(t, kinds[models.PlatformComponentEnsemble], "pending list must include an ensemble request")
	t.Logf("pending list contains %d requests covering %d component kinds",
		len(pending.Requests), len(kinds))

	// Assert request IDs are non-empty and unique.
	seen := make(map[string]bool, len(pending.Requests))
	for _, r := range pending.Requests {
		require.NotEmpty(t, r.RequestID, "each pending request must have a non-empty request ID")
		require.False(t, seen[r.RequestID], "request ID %s is duplicated in the pending list", r.RequestID)
		seen[r.RequestID] = true
		assert.Equal(t, models.PlatformEnrollmentStatePending, r.State,
			"pending list request %s must be in the pending state", r.RequestID)
	}
	t.Logf("all %d request IDs are non-empty and unique", len(pending.Requests))

	// Assert the raw JSON response excludes secret material. The typed
	// PlatformEnrollmentPendingRequest model omits token hashes, CSR PEM,
	// and certificates by construction, but the raw wire payload is checked
	// directly as defense-in-depth: a future field addition that leaks
	// secret material would not be caught by the typed decode alone.
	raw, err := e2eClient.GetPendingRaw(ctx)
	require.NoError(t, err, "raw pending list fetch must succeed for secret-leak assertion")
	for _, forbidden := range []string{
		"token_hash",
		"csr_pem",
		"operator_csr_pem",
		"cli_csr_pem",
		"BEGIN CERTIFICATE",
		"END CERTIFICATE",
		"BEGIN CERTIFICATE REQUEST",
		"END CERTIFICATE REQUEST",
		"BEGIN PRIVATE KEY",
		"END PRIVATE KEY",
		"BEGIN EC PRIVATE KEY",
		"END EC PRIVATE KEY",
		"BEGIN RSA PRIVATE KEY",
		"END RSA PRIVATE KEY",
		"requester_token",
	} {
		assert.NotContains(t, raw, forbidden,
			"pending list raw JSON must not expose %q", forbidden)
	}
	t.Logf("pending list raw JSON contains no tokens, hashes, CSR PEM, certificates, or private keys")
}
