// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// Integration tests proving the service-level inactive-owner decision
// rejection. The router-level test
// TestPlatformEnrollmentRouter_OwnerRoutesDenyInactiveOwner proves the
// auth middleware rejects a disabled first user at the transport layer.
// These tests call PlatformEnrollmentService.Decide directly with an
// inactive first user and assert it fails closed with the typed
// authorization error, distinct from the non-owner case. This closes
// the gap between transport-layer enforcement and the service-layer
// active-owner contract.

package gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

// TestPlatformEnrollmentService_InactiveOwnerDecisionFailsClosed proves
// that calling Decide with a disabled first user fails closed with the
// typed authorization error. The service enforces the active-owner
// check independently of the controller's transport-layer
// requireActiveFirstUser, so a direct caller cannot bypass the
// active-owner requirement.
func TestPlatformEnrollmentService_InactiveOwnerDecisionFailsClosed(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	// Create a pending request.
	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-inactive-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	// Disable the owner (first user). The owner remains the first user
	// but is no longer active.
	err = env.userSvc.Disable(env.ownerID, "test: disable owner", "", "")
	require.NoError(t, err)

	// A decision from the disabled owner must fail closed.
	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidDecision,
		"disabled first user must fail closed with the typed authorization error")

	// The request must remain pending (the decision was not applied).
	stored := loadStoredRequest(t, env, createResp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStatePending, stored.State,
		"disabled owner decision must not change the request state")

	// Deny must also fail closed.
	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionDeny,
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidDecision,
		"disabled first user deny must also fail closed")
}

// TestPlatformEnrollmentService_InactiveOwnerDistinctFromNonOwner proves
// that the inactive-owner rejection is distinct from the non-owner
// rejection in effect: both return the same typed error (the error
// constant does not leak whether the user is inactive vs. non-owner),
// but the inactive owner is the first user while the non-owner is not.
// This test documents that the service does not distinguish the error
// message, only the authorization decision.
func TestPlatformEnrollmentService_InactiveOwnerDistinctFromNonOwner(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	// Create a second user (non-owner).
	secondUser, err := env.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotEqual(t, env.ownerID, secondUser.ID)

	// Create a pending request.
	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-inactive-2",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	// Disable the owner.
	err = env.userSvc.Disable(env.ownerID, "test: disable owner", "", "")
	require.NoError(t, err)

	// Both the disabled owner and the non-owner receive the same typed
	// error. The service does not leak whether the rejection is due to
	// inactivity or non-ownership.
	_, inactiveErr := env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	_, nonOwnerErr := env.enrollSvc.Decide(context.Background(), secondUser.ID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})

	assert.ErrorIs(t, inactiveErr, constants.ErrPlatformEnrollmentInvalidDecision)
	assert.ErrorIs(t, nonOwnerErr, constants.ErrPlatformEnrollmentInvalidDecision)
	assert.Equal(t, inactiveErr, nonOwnerErr,
		"inactive owner and non-owner must receive the same typed error (no leak)")

	// The request must remain pending (neither decision was applied).
	stored := loadStoredRequest(t, env, createResp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStatePending, stored.State)
}

// TestPlatformEnrollmentService_ReenabledOwnerCanDecide proves that
// after the owner is re-enabled (status set back to active), decisions
// succeed again. This verifies the active check is dynamic, not cached.
func TestPlatformEnrollmentService_ReenabledOwnerCanDecide(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	// Create a pending request.
	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-inactive-3",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	// Disable the owner.
	err = env.userSvc.Disable(env.ownerID, "test: disable owner", "", "")
	require.NoError(t, err)

	// Decision fails while disabled.
	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidDecision)

	// Re-enable the owner by directly setting the status back to active.
	updateBytes, err := json.Marshal(map[string]interface{}{
		"status": "active",
	})
	require.NoError(t, err)
	_, err = env.docStore.DocUpdate(
		marshaler.CollectionName(constants.CollectionUsers), env.ownerID, updateBytes)
	require.NoError(t, err)

	// Decision succeeds after re-enable.
	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	require.NoError(t, err, "re-enabled first user must be able to decide")

	stored := loadStoredRequest(t, env, createResp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStateApproved, stored.State)
}
