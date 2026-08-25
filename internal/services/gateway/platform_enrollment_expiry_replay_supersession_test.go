// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// Integration tests covering the full set of terminal and recoverable
// state transitions beyond lease recovery: a request that expires while
// pending, a request that expires while approved, a replayed completion
// after the request has expired, a supersession attempt (a second live
// request for the same component instance and key set while the first
// is non-terminal), and the issuing -> approved rollback when the
// issuance lease expires before artifacts are committed.

package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// ============================================================================
// Expiry state transition tests
// ============================================================================

// TestPlatformEnrollmentExpiry_PendingRequestExpires proves that a
// request in the pending state transitions to expired when its TTL
// elapses. A subsequent status query returns the expired error, and
// completion is rejected.
func TestPlatformEnrollmentExpiry_PendingRequestExpires(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-exp-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	// Backdate the request past its TTL.
	backdateExpiry(t, env, createResp.RequestID, time.Now().UTC().Add(-time.Hour))

	// Status query must detect the expiry and return the expired error.
	_, err = env.enrollSvc.GetStatus(context.Background(), createResp.Token)
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequestExpired)

	// The request must be in the expired state.
	stored := loadStoredRequest(t, env, createResp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStateExpired, stored.State)

	// Completion is rejected.
	_, err = env.enrollSvc.Complete(context.Background(), createResp.Token, models.PlatformEnrollmentProofs{})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequestExpired)
}

// TestPlatformEnrollmentExpiry_ApprovedRequestExpires proves that a
// request in the approved state transitions to expired when its TTL
// elapses. An approved-but-expired request cannot be completed.
func TestPlatformEnrollmentExpiry_ApprovedRequestExpires(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-exp-2",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	// Approve the request.
	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	require.NoError(t, err)

	// Backdate the approved request past its TTL.
	backdateExpiry(t, env, createResp.RequestID, time.Now().UTC().Add(-time.Hour))

	// Completion must detect the expiry and return the expired error.
	_, err = env.enrollSvc.Complete(context.Background(), createResp.Token, models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, loadStoredRequest(t, env, createResp.RequestID), key),
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequestExpired)

	// The request must be in the expired state.
	stored := loadStoredRequest(t, env, createResp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStateExpired, stored.State)
}

// TestPlatformEnrollmentExpiry_ReplayedCompletionAfterExpiry proves
// that a completion attempt on an expired request is rejected, even
// with valid proofs. The expiry is terminal and cannot be bypassed by
// replaying a completion.
func TestPlatformEnrollmentExpiry_ReplayedCompletionAfterExpiry(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-exp-3",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	// Approve the request.
	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	require.NoError(t, err)

	// Backdate past TTL.
	backdateExpiry(t, env, createResp.RequestID, time.Now().UTC().Add(-time.Hour))

	// Attempt completion with valid proofs — must be rejected.
	stored := loadStoredRequest(t, env, createResp.RequestID)
	proof := models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, stored, key),
	}
	_, err = env.enrollSvc.Complete(context.Background(), createResp.Token, proof)
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequestExpired)

	// A second replay attempt is also rejected.
	_, err = env.enrollSvc.Complete(context.Background(), createResp.Token, proof)
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequestExpired)
}

// ============================================================================
// Supersession tests
// ============================================================================

// TestPlatformEnrollmentSupersession_SecondLiveRequestForSameInstanceDeduped
// proves that a second live request for the same component kind,
// instance ID, and fingerprint set is deduplicated rather than creating
// a supersession. The existing request is returned without a new token.
func TestPlatformEnrollmentSupersession_SecondLiveRequestForSameInstanceDeduped(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	req := models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-sup-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}

	first, err := env.enrollSvc.CreateRequest(context.Background(), req, "https://gateway.local/console")
	require.NoError(t, err)
	require.NotEmpty(t, first.Token)

	// A second request with the same component kind, instance ID, and
	// fingerprint set is deduplicated (returns the same request ID,
	// no new token).
	second, err := env.enrollSvc.CreateRequest(context.Background(), req, "https://gateway.local/console")
	require.NoError(t, err)
	assert.Equal(t, first.RequestID, second.RequestID)
	assert.Empty(t, second.Token, "deduplicated response must not return a new token")
}

// TestPlatformEnrollmentSupersession_SecondLiveRequestWithDifferentKeysCreatesNewRequest
// proves that a second live request for the same component kind and
// instance ID but with a different key set creates a new request. This
// is not a supersession — both requests coexist as live pending
// requests. The quota bounds how many can coexist.
func TestPlatformEnrollmentSupersession_SecondLiveRequestWithDifferentKeysCreatesNewRequest(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr1, _ := generateAppCSRAndKey(t)
	first, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-sup-2",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr1},
	}, "https://gateway.local/console")
	require.NoError(t, err)
	require.NotEmpty(t, first.Token)

	// A second request with a different CSR (different key) creates a
	// new request rather than deduplicating.
	csr2, _ := generateAppCSRAndKey(t)
	second, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-sup-2",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr2},
	}, "https://gateway.local/console")
	require.NoError(t, err)
	assert.NotEqual(t, first.RequestID, second.RequestID,
		"different key set must create a new request, not deduplicate")
	assert.NotEmpty(t, second.Token, "new request must return a new token")

	// Both requests are live and pending.
	list, err := env.enrollSvc.ListPending(context.Background())
	require.NoError(t, err)
	assert.Len(t, list.Requests, 2, "both live requests must appear in the pending list")
}

// TestPlatformEnrollmentSupersession_TerminalRequestAllowsNewRequestForSameInstance
// proves that after a request reaches a terminal state (denied), a new
// request for the same component kind, instance ID, and key set can be
// created. The terminal request does not block a new enrollment attempt.
func TestPlatformEnrollmentSupersession_TerminalRequestAllowsNewRequestForSameInstance(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	req := models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-sup-3",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}

	first, err := env.enrollSvc.CreateRequest(context.Background(), req, "https://gateway.local/console")
	require.NoError(t, err)

	// Deny the first request.
	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: first.RequestID,
		Decision:  models.PlatformEnrollmentDecisionDeny,
	})
	require.NoError(t, err)

	// A new request with the same parameters can be created (the
	// denied request is terminal and does not block).
	second, err := env.enrollSvc.CreateRequest(context.Background(), req, "https://gateway.local/console")
	require.NoError(t, err)
	assert.NotEqual(t, first.RequestID, second.RequestID,
		"new request after terminal denial must have a different request ID")
	assert.NotEmpty(t, second.Token, "new request must return a new token")
}

// ============================================================================
// Issuing -> approved rollback tests
// ============================================================================

// TestPlatformEnrollmentIssuingRollback_LeaseExpiresBeforeArtifactsCommitted
// proves that when the issuance lease expires before the ISSUE handler
// commits the artifacts, reconciliation rolls the request back to
// approved so a new completion attempt can re-acquire the lease and
// succeed. This is the recoverable saga boundary.
func TestPlatformEnrollmentIssuingRollback_LeaseExpiresBeforeArtifactsCommitted(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	_, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-rb-1", "dashboard.local",
		csr, "", "")

	// Manually acquire the issuance lease with an already-expired expiry,
	// simulating a crash that left the request in the issuing state with
	// an expired lease and no committed artifacts.
	leaseExpiry := time.Now().UTC().Add(-time.Second)
	applied, err := env.docStore.DocConditionalUpdate(
		platformEnrollmentCollectionName(), approved.ID,
		map[string]interface{}{
			"state":                     string(models.PlatformEnrollmentStateIssuing),
			"issuance_lease_owner":      "crashed-owner",
			"issuance_lease_expires_at": leaseExpiry,
			"last_transition_at":        time.Now().UTC(),
		},
		"state", string(models.PlatformEnrollmentStateApproved),
	)
	require.NoError(t, err)
	require.True(t, applied)

	// Verify the request is in the issuing state with no issued material.
	stored := loadStoredRequest(t, env, approved.ID)
	assert.Equal(t, models.PlatformEnrollmentStateIssuing, stored.State)
	assert.Nil(t, stored.Issued, "no artifacts must be committed before lease expiry")

	// Reconciliation must recover the expired lease.
	err = env.enrollSvc.ReconcileExpiredLeases()
	require.NoError(t, err)

	stored = loadStoredRequest(t, env, approved.ID)
	assert.Equal(t, models.PlatformEnrollmentStateApproved, stored.State,
		"expired lease must be recovered back to approved")

	// A new completion attempt must succeed and issue the certificate.
	proof := models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, stored, key),
	}
	resp, err := env.enrollSvc.Complete(context.Background(), token, proof)
	require.NoError(t, err, "completion must succeed after lease recovery")
	require.NotNil(t, resp.App)
	assert.NotEmpty(t, resp.App.AppCert)

	// The request is now completed.
	stored = loadStoredRequest(t, env, approved.ID)
	assert.Equal(t, models.PlatformEnrollmentStateCompleted, stored.State)
}

// TestPlatformEnrollmentIssuingRollback_ExpiredLeaseWithExpiredRequest
// proves that when both the issuance lease and the request TTL have
// expired, reconciliation transitions the request to expired (not back
// to approved). An expired request cannot be recovered.
func TestPlatformEnrollmentIssuingRollback_ExpiredLeaseWithExpiredRequest(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	_, _, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-rb-2", "dashboard.local",
		csr, "", "")

	// Acquire the lease with an expired expiry AND backdate the request
	// past its TTL.
	pastTime := time.Now().UTC().Add(-time.Hour)
	applied, err := env.docStore.DocConditionalUpdate(
		platformEnrollmentCollectionName(), approved.ID,
		map[string]interface{}{
			"state":                     string(models.PlatformEnrollmentStateIssuing),
			"issuance_lease_owner":      "crashed-owner",
			"issuance_lease_expires_at": pastTime,
			"expires_at":                pastTime,
			"last_transition_at":        time.Now().UTC(),
		},
		"state", string(models.PlatformEnrollmentStateApproved),
	)
	require.NoError(t, err)
	require.True(t, applied)

	// Reconciliation must transition to expired (not back to approved).
	err = env.enrollSvc.ReconcileExpiredLeases()
	require.NoError(t, err)

	stored := loadStoredRequest(t, env, approved.ID)
	assert.Equal(t, models.PlatformEnrollmentStateExpired, stored.State,
		"expired request with expired lease must transition to expired, not approved")
}

// ============================================================================
// Helper functions
// ============================================================================

// backdateExpiry sets the request's expires_at field to the given past
// time, simulating TTL elapse. This directly manipulates the stored
// document rather than waiting for the real TTL. Uses DocUpdate
// (unconditional) because the request ID is the document key; the
// condition field "id" is stripped from the stored data by DocSet.
func backdateExpiry(t *testing.T, env *platformEnrollmentTestEnv, requestID string, pastTime time.Time) {
	t.Helper()
	updateBytes, err := json.Marshal(map[string]interface{}{
		"expires_at": pastTime,
	})
	require.NoError(t, err)
	_, err = env.docStore.DocUpdate(platformEnrollmentCollectionName(), requestID, updateBytes)
	require.NoError(t, err)
}
