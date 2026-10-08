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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestPlatformEnrollmentStatusWait_DecisionAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		decision models.PlatformEnrollmentDecision
		batch    bool
	}{
		{name: "approve", decision: models.PlatformEnrollmentDecisionApprove},
		{name: "deny", decision: models.PlatformEnrollmentDecisionDeny},
		{name: "batch_approve", decision: models.PlatformEnrollmentDecisionApprove, batch: true},
		{name: "cancel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := setupPlatformEnrollmentRouterEnv(t)
			csr, _ := generateAppCSRAndKey(t)
			created, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
				ComponentKind: models.PlatformComponentDashboard, InstanceID: "status-wait", Hostname: "status-wait.local",
				App: &models.PlatformAppCSRPayload{CSRPEM: csr},
			}, "https://gateway.local/console")
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentStatus+"?wait=true&token="+url.QueryEscape(created.Token), nil).WithContext(ctx)
			rr := httptest.NewRecorder()
			done := make(chan struct{})
			defer func() {
				cancel()
				<-done
			}()
			go func() {
				env.httpRouter.ServeHTTP(rr, req)
				close(done)
			}()
			select {
			case <-done:
				t.Fatal("pending enrollment returned before its decision")
			case <-time.After(25 * time.Millisecond):
			}
			if tc.decision == "" {
				cancel()
			} else if tc.batch {
				_, err = env.enrollSvc.DecideBatch(context.Background(), env.ownerID, models.PlatformEnrollmentBatchDecisionRequest{
					Requests: []models.PlatformEnrollmentDecisionTarget{{RequestID: created.RequestID, Fingerprints: created.Fingerprints}}, Decision: tc.decision,
				})
				require.NoError(t, err)
			} else {
				_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{RequestID: created.RequestID, Decision: tc.decision})
				require.NoError(t, err)
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("status request did not finish after decision or cancellation")
			}
			if tc.decision == "" {
				return
			}
			require.Equal(t, http.StatusOK, rr.Code)
			var status models.PlatformEnrollmentStatusResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &status))
			want := models.PlatformEnrollmentStateApproved
			if tc.decision == models.PlatformEnrollmentDecisionDeny {
				want = models.PlatformEnrollmentStateDenied
			}
			require.Equal(t, want, status.State)
			require.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
		})
	}
}
