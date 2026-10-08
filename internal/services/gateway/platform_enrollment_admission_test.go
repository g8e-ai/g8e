// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func saturate(t *testing.T, gate *platformEnrollmentGate) {
	t.Helper()
	for range cap(gate.slots) {
		_, _, err := gate.acquire(t.Context())
		require.NoError(t, err)
	}
}

func TestPlatformEnrollmentService_CreateUnderSaturatedIntakeStoresNothing(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)
	env.enrollSvc.intake = newPlatformEnrollmentGate(1, 50*time.Millisecond)
	saturate(t, env.enrollSvc.intake)

	csr, _ := generateAppCSRAndKey(t)
	create := models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-saturated",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}

	_, err := env.enrollSvc.CreateRequest(t.Context(), create, "https://gateway.local/console")
	require.ErrorIs(t, err, constants.ErrPlatformEnrollmentRateLimited)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = env.enrollSvc.CreateRequest(ctx, create, "https://gateway.local/console")
	require.ErrorIs(t, err, context.Canceled)

	pending, err := env.enrollSvc.ListPending(t.Context())
	require.NoError(t, err)
	assert.Empty(t, pending.Requests, "a request refused at admission must not reserve pending capacity")
}

func TestPlatformEnrollmentService_CompleteUnderSaturatedIssuanceKeepsApproval(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)
	csr, key := generateAppCSRAndKey(t)
	requestID, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-queued", "dashboard.local", csr, "", "")
	proof := models.PlatformEnrollmentProofs{App: signCompletionTranscript(t, approved, key)}

	env.enrollSvc.issuance = newPlatformEnrollmentGate(1, 50*time.Millisecond)
	saturate(t, env.enrollSvc.issuance)

	_, err := env.enrollSvc.Complete(t.Context(), token, proof)
	require.ErrorIs(t, err, constants.ErrPlatformEnrollmentRateLimited)
	assert.Equal(t, models.PlatformEnrollmentStateApproved, loadStoredRequest(t, env, requestID).State,
		"a queue refusal must leave the approval and lease untouched")

	env.enrollSvc.issuance = newPlatformEnrollmentGate(1, time.Second)
	resp, err := env.enrollSvc.Complete(t.Context(), token, proof)
	require.NoError(t, err, "the same approval completes once capacity frees")
	require.NotNil(t, resp.App)
}

func TestPlatformEnrollmentService_DisconnectDuringIssuanceStillCompletes(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)
	csr, key := generateAppCSRAndKey(t)
	requestID, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-disconnect", "dashboard.local", csr, "", "")
	proof := models.PlatformEnrollmentProofs{App: signCompletionTranscript(t, approved, key)}

	// A free slot is granted without consulting the context, so a request whose
	// client has already gone reaches the lease and must still finish issuing.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	resp, err := env.enrollSvc.Complete(ctx, token, proof)
	require.NoError(t, err)
	require.NotNil(t, resp.App)
	assert.Equal(t, models.PlatformEnrollmentStateCompleted, loadStoredRequest(t, env, requestID).State)
}
