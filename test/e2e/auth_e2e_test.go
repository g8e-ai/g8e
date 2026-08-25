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

// TestAuth_OwnerMTLS proves the owner CLI certificate completes a strict mTLS
// handshake with the gateway and authenticates successfully. The E2EClient
// was constructed in TestMain with the owner CLI cert, CA bundle, and
// ServerName derived from the validated HTTPS URL — no InsecureSkipVerify.
// A successful authenticated request is direct proof that:
//
//  1. The owner CLI certificate chains to a CA in the trust bundle.
//  2. The gateway's certificate SANs match the ServerName from config.
//  3. The CLI session ID header is accepted by an authenticated route.
//  4. TLS 1.3 with FIPS curve preferences negotiated successfully.
//
// This replaces the prior log-based "Authentication successful" marker check
// and the operator-cert mTLS dial. The owner identity is the canonical
// requestor for E2E assertions; operator-to-gateway mTLS is observed
// indirectly through the operator registry and heartbeat tests.
func TestAuth_OwnerMTLS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	// The pending enrollments endpoint requires owner authentication. A
	// successful response proves the full mTLS + session header path works.
	// On an approved stack the pending list is empty, which is the expected
	// steady state — the assertion is that the request succeeds, not that
	// the list is populated.
	pending, err := e2eClient.GetPendingEnrollments(ctx)
	require.NoError(t, err,
		"authenticated pending enrollments request must succeed — owner mTLS or session header is broken")
	assert.Empty(t, pending.Requests,
		"on an approved stack the pending enrollment list must be empty")
	t.Logf("owner mTLS authenticated successfully: %d pending enrollments", len(pending.Requests))

	// The operators list is a second authenticated route. Reaching it
	// confirms the mTLS path is not specific to a single endpoint.
	operators, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err,
		"authenticated operators list request must succeed — owner mTLS or session header is broken")
	assert.True(t, operators.Success, "operators list response must report success")
	t.Logf("owner mTLS authenticated successfully: %d operators registered", len(operators.Operators))
}

// TestAuth_OwnerMTLSRejectsMissingSessionHeader verifies that an authenticated
// request without the CLI session header is rejected. The E2EClient always
// sets the header, so this test builds a bare mTLS request to the pending
// endpoint and asserts a non-200 response. This proves the session header is
// enforced, not just present by convention.
func TestAuth_OwnerMTLSRejectsMissingSessionHeader(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	req, err := newBareMTLSRequest(ctx, e2eCfg.gatewayHTTPSURL+constants.APIPaths.AuthPlatformEnrollmentPending)
	require.NoError(t, err)

	resp, err := e2eClient.mtlsClient.Do(req)
	require.NoError(t, err, "mTLS handshake must succeed even without the session header")
	defer resp.Body.Close()
	assert.NotEqual(t, 200, resp.StatusCode,
		"authenticated endpoint must reject a request missing the CLI session header")
	t.Logf("missing-session-header request correctly rejected with status %d", resp.StatusCode)
}
