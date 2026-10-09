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
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// Cancel at the real CREATE audit boundary to model a disconnected requester
// after admission commits. All governance and persistence still execute.
type cancelAfterCreateProcessor struct {
	inner       governanceEnvelopeProcessor
	afterCreate func()
}

func (p *cancelAfterCreateProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	receipt, err := p.inner.ProcessEnvelope(ctx, payload)
	if err == nil && decodeActionTypeFromWire(payload) == string(constants.PlatformEnrollmentActionCreate) {
		p.afterCreate()
	}
	return receipt, err
}

type cancelDuringIssueProcessor struct {
	inner  governanceEnvelopeProcessor
	cancel context.CancelFunc
}

func (p *cancelDuringIssueProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	receipt, err := p.inner.ProcessEnvelope(ctx, payload)
	if err == nil && decodeActionTypeFromWire(payload) == string(constants.PlatformEnrollmentActionIssue) {
		p.cancel()
	}
	return receipt, err
}

func TestPlatformEnrollmentService_CanceledCreateReleasesOnlyPendingReservation(t *testing.T) {
	for _, approved := range []bool{false, true} {
		name := "pending"
		if approved {
			name = "already approved"
		}
		t.Run(name, func(t *testing.T) {
			env := setupPlatformEnrollmentEnv(t, true)
			csr, _ := generateAppCSRAndKey(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			original := env.enrollSvc.envProc
			env.enrollSvc.envProc = &cancelAfterCreateProcessor{inner: original, afterCreate: func() {
				pending, err := env.enrollSvc.ListPending(t.Context())
				require.NoError(t, err)
				require.Len(t, pending.Requests, 1)
				if approved {
					_, err := env.enrollSvc.Decide(t.Context(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
						RequestID: pending.Requests[0].RequestID,
						Decision:  models.PlatformEnrollmentDecisionApprove,
					})
					require.NoError(t, err)
				}
				cancel()
			}}
			t.Cleanup(func() { env.enrollSvc.envProc = original })
			req := models.PlatformEnrollmentCreateRequest{
				ComponentKind: models.PlatformComponentDashboard, InstanceID: "dashboard-canceled-create", Hostname: "dashboard.local",
				App: &models.PlatformAppCSRPayload{CSRPEM: csr},
			}
			resp, err := env.enrollSvc.CreateRequest(ctx, req, "https://localhost")
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, resp)
			var count int
			require.NoError(t, env.enrollSvc.db.db.QueryRow("SELECT count(*) FROM documents WHERE collection = ?", platformEnrollmentCollectionName()).Scan(&count))
			if approved {
				require.Equal(t, 1, count, "disconnect must preserve owner authorization")
			} else {
				require.Zero(t, count, "undelivered pending request must not consume quota")
				env.enrollSvc.envProc = original
				resp, err = env.enrollSvc.CreateRequest(t.Context(), req, "https://localhost")
				require.NoError(t, err)
				require.NotEmpty(t, resp.Token, "retry must receive a usable token")
			}
		})
	}
}

func TestPlatformEnrollmentService_DisconnectDuringIssuanceStillCompletes(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)
	csr, key := generateAppCSRAndKey(t)
	requestID, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-disconnect", "dashboard.local", csr, "", "")
	proof := models.PlatformEnrollmentProofs{App: signCompletionTranscript(t, approved, key)}

	// Disconnect after ISSUE starts. Once Complete acquires its issuance lease,
	// it must finish the saga with its detached lease context.
	ctx, cancel := context.WithCancel(t.Context())
	original := env.enrollSvc.envProc
	env.enrollSvc.envProc = &cancelDuringIssueProcessor{inner: original, cancel: cancel}
	t.Cleanup(func() { env.enrollSvc.envProc = original })
	resp, err := env.enrollSvc.Complete(ctx, token, proof)
	require.NoError(t, err)
	require.NotNil(t, resp.App)
	assert.Equal(t, models.PlatformEnrollmentStateCompleted, loadStoredRequest(t, env, requestID).State)
}

// Occupy the only database connection after fixture setup so creation blocks
// inside its bootstrap read, rather than in the context-aware reservation.
func TestPlatformEnrollmentService_CreateCancellationInterruptsBootstrapRead(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)
	csr, _ := generateAppCSRAndKey(t)
	db := env.enrollSvc.db.db
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := env.enrollSvc.CreateRequest(ctx, models.PlatformEnrollmentCreateRequest{
			ComponentKind: models.PlatformComponentDashboard,
			InstanceID:    "dashboard-blocked-bootstrap",
			Hostname:      "dashboard.local",
			App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
		}, "https://localhost")
		done <- err
	}()
	// Always release the connection and join the requester before fixture cleanup,
	// including when the regression demonstrates the uncancellable read.
	defer func() {
		cancel()
		require.NoError(t, conn.Close())
		<-done
	}()
	select {
	case err := <-done:
		done <- err
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(3 * time.Second):
		t.Fatal("bootstrap read remained blocked after request cancellation")
	}
	require.Positive(t, db.Stats().WaitCount, "request must have waited for the occupied connection")
}
