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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestPlatformEnrollmentService_DisconnectDuringIssuanceStillCompletes(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)
	csr, key := generateAppCSRAndKey(t)
	requestID, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-disconnect", "dashboard.local", csr, "", "")
	proof := models.PlatformEnrollmentProofs{App: signCompletionTranscript(t, approved, key)}

	// A request whose client has already gone still reaches the lease, and the
	// issuance it starts must finish rather than roll the approval back.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	resp, err := env.enrollSvc.Complete(ctx, token, proof)
	require.NoError(t, err)
	require.NotNil(t, resp.App)
	assert.Equal(t, models.PlatformEnrollmentStateCompleted, loadStoredRequest(t, env, requestID).State)
}
