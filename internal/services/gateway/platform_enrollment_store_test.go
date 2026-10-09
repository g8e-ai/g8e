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
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrollmentAdmissionConcurrentCapacityAndDedup(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprintf("duplicate=%t", duplicate), func(t *testing.T) {
			store := newDocumentStoreService(t)
			_, err := store.db.Exec(gatewaySchema)
			require.NoError(t, err)
			svc := &PlatformEnrollmentService{db: store}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			const workers = 12
			results := make(chan error, workers)
			var wg sync.WaitGroup
			start := make(chan struct{})
			now := time.Now().UTC()
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					instance := fmt.Sprintf("instance-%d", i)
					tokenSeed := fmt.Sprintf("request-token-%d", i)
					if duplicate {
						instance = "same-instance"
						tokenSeed = "shared-request-token"
					}
					_, err := svc.createRequestRecord(ctx, &models.PlatformEnrollmentRequest{
						ID: fmt.Sprintf("request-%d", i), ComponentKind: models.PlatformComponentDashboard,
						InstanceID: instance, State: models.PlatformEnrollmentStatePending,
						CreatedAt: now, ExpiresAt: now.Add(time.Hour),
						TokenHash:    models.PlatformEnrollmentTokenHash(tokenSeed),
						Fingerprints: models.PlatformEnrollmentCSRFingerprints{App: "same-key"},
					})
					results <- err
				}(i)
			}
			close(start)
			wg.Wait()
			close(results)
			accepted := 0
			for err := range results {
				if err == nil {
					accepted++
					continue
				}
				require.ErrorIs(t, err, constants.ErrPlatformEnrollmentQuotaExceeded)
			}
			wantRows := constants.PlatformEnrollmentMaxLiveRequestsPerComponent
			if duplicate {
				require.Equal(t, workers, accepted)
				wantRows = 1
			} else {
				require.Equal(t, wantRows, accepted)
			}
			var count int
			require.NoError(t, store.db.QueryRow("SELECT count(*) FROM documents WHERE collection = ?", platformEnrollmentCollectionName()).Scan(&count))
			require.Equal(t, wantRows, count)
		})
	}
}

func TestEnrollmentAdmissionHistoryAndResume(t *testing.T) {
	store := newDocumentStoreService(t)
	_, err := store.db.Exec(gatewaySchema)
	require.NoError(t, err)
	svc := &PlatformEnrollmentService{db: store}
	now := time.Now().UTC()
	for i, state := range []models.PlatformEnrollmentState{
		models.PlatformEnrollmentStateCompleted, models.PlatformEnrollmentStateDenied,
		models.PlatformEnrollmentStateRevoked, models.PlatformEnrollmentStateExpired, models.PlatformEnrollmentStatePending,
	} {
		data, err := json.Marshal(models.PlatformEnrollmentRequest{
			ComponentKind: models.PlatformComponentDashboard, State: state, ExpiresAt: now.Add(-time.Minute),
			TokenHash: models.PlatformEnrollmentTokenHash(fmt.Sprintf("history-token-%d", i)),
		})
		require.NoError(t, err)
		require.NoError(t, store.DocSet(t.Context(), platformEnrollmentCollectionName(), fmt.Sprintf("history-%d", i), data))
	}
	for i := 0; i < constants.PlatformEnrollmentMaxLiveRequestsPerComponent; i++ {
		req := &models.PlatformEnrollmentRequest{
			ID: fmt.Sprintf("live-%d", i), InstanceID: fmt.Sprintf("instance-%d", i),
			ComponentKind: models.PlatformComponentDashboard, State: models.PlatformEnrollmentStatePending,
			TokenHash: models.PlatformEnrollmentTokenHash(fmt.Sprintf("live-token-%d", i)),
			CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		}
		existing, err := svc.createRequestRecord(context.Background(), req)
		require.NoError(t, err)
		require.Nil(t, existing)
		existing, err = svc.createRequestRecord(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, req.ID, existing.ID)
	}
	pending, err := svc.ListPending(context.Background())
	require.NoError(t, err)
	require.Len(t, pending.Requests, constants.PlatformEnrollmentMaxLiveRequestsPerComponent)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = svc.createRequestRecord(ctx, &models.PlatformEnrollmentRequest{})
	require.True(t, errors.Is(err, context.Canceled))
}

func TestEnrollmentQueriesUseIndexes(t *testing.T) {
	store := newDocumentStoreService(t)
	_, err := store.db.Exec(gatewaySchema)
	require.NoError(t, err)
	for _, tc := range []struct {
		query, index string
		args         []any
	}{
		{`SELECT count(*) FROM documents WHERE collection = ? AND json_extract(data, '$.component_kind') = ? AND ` + enrollmentLivePredicate,
			"idx_enrollment_capacity", []any{platformEnrollmentCollectionName(), models.PlatformComponentOperator, time.Now().UTC().Format(time.RFC3339Nano)}},
		{`SELECT data FROM documents WHERE collection = ? AND json_extract(data, '$.token_hash') = ?`, "idx_enrollment_token", []any{platformEnrollmentCollectionName(), "token"}},
		{`SELECT data FROM documents WHERE collection = ? AND json_extract(data, '$.state') = ? AND julianday(json_extract(data, '$.expires_at')) >= julianday(?)`,
			"idx_enrollment_pending", []any{platformEnrollmentCollectionName(), models.PlatformEnrollmentStatePending, time.Now().UTC().Format(time.RFC3339Nano)}},
		{operatorLeaseIdentityQuery, "idx_operator_lease_identity", []any{
			marshaler.CollectionName(constants.CollectionOperators), "owner", "fingerprint", constants.OperatorTypeRemote, constants.OperatorStatusTerminated}},
		{operatorSessionIdentityQuery, "idx_operator_session_identity", []any{
			marshaler.CollectionName(constants.CollectionOperators), "session"}},
	} {
		t.Run(tc.index, func(t *testing.T) {
			rows, err := store.db.Query("EXPLAIN QUERY PLAN "+tc.query, tc.args...)
			require.NoError(t, err)
			defer rows.Close()
			var details []string
			for rows.Next() {
				var id, parent, unused int
				var detail string
				require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
				details = append(details, detail)
			}
			require.NoError(t, rows.Err())
			require.Contains(t, strings.Join(details, "\n"), tc.index)
		})
	}
}

func TestFindOperatorLeases_IdentityScopeAndReadOnly(t *testing.T) {
	store := newDocumentStoreService(t)
	_, err := store.db.Exec(gatewaySchema)
	require.NoError(t, err)
	for _, identity := range []struct {
		id, owner, fingerprint string
		status                 constants.OperatorStatus
		typeOf                 constants.OperatorType
	}{
		{"active", "owner", "fingerprint", constants.OperatorStatusActive, constants.OperatorTypeRemote},
		{"stale", "owner", "fingerprint", constants.OperatorStatusStale, constants.OperatorTypeRemote},
		{"stopped", "owner", "fingerprint", constants.OperatorStatusStopped, constants.OperatorTypeRemote},
		{"offline", "owner", "fingerprint", constants.OperatorStatusOffline, constants.OperatorTypeRemote},
		{"terminated", "owner", "fingerprint", constants.OperatorStatusTerminated, constants.OperatorTypeRemote},
		{"embedded", "owner", "fingerprint", constants.OperatorStatusActive, constants.OperatorTypeEmbedded},
		{"other-owner", "other", "fingerprint", constants.OperatorStatusActive, constants.OperatorTypeRemote},
		{"other-fingerprint", "owner", "other", constants.OperatorStatusActive, constants.OperatorTypeRemote},
	} {
		op := remoteOperator(identity.status)
		op.OperatorType = string(identity.typeOf)
		op.UserId = identity.owner
		op.SystemFingerprint = identity.fingerprint
		op.LastHeartbeatAt = timeAgo(constants.OperatorHeartbeatStaleAfter * 2)
		putOperator(t, store, identity.id, op, time.Hour)
	}
	// An unrelated malformed runtime configuration must not be decoded by
	// lease replacement or cause an unrelated identity's enrollment to fail.
	require.NoError(t, store.DocSet(t.Context(), operatorsCollection, "malformed-other", json.RawMessage(
		`{"user_id":"other","system_fingerprint":"other","status":"active","operator_type":"remote","runtime_config":"invalid"}`)))
	rootSvc := NewStateRootService(store.db, store.logger)
	before, err := rootSvc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	docs, err := store.FindOperatorLeases(t.Context(), "owner", "fingerprint")
	require.NoError(t, err)
	var ids []string
	for _, doc := range docs {
		ids = append(ids, doc.ID)
	}
	assert.ElementsMatch(t, []string{"active", "stale", "stopped", "offline"}, ids)
	after, err := rootSvc.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before, after, "lease lookup must neither reconcile heartbeat state nor write")

	_, err = store.DocQuery(t.Context(), operatorsCollection, nil, "", 0)
	require.Error(t, err, "the fixture must exercise a malformed fleet record that reconciliation would decode")
}

func TestEnrollmentDecisionAtomicAuthorityAndRollback(t *testing.T) {
	for _, scenario := range []string{"approve", "deny", "stale", "fingerprints", "expired", "disabled-owner", "non-owner", "duplicate", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			store := newDocumentStoreService(t)
			_, err := store.db.Exec(gatewaySchema)
			require.NoError(t, err)
			owner := models.User{Status: constants.UserStatusActive}
			if scenario == "disabled-owner" {
				owner.Status = constants.UserStatusDisabled
			}
			data, err := json.Marshal(owner)
			require.NoError(t, err)
			require.NoError(t, store.DocSet(t.Context(), marshaler.CollectionName(constants.CollectionUsers), "owner", data))
			req := models.PlatformEnrollmentBatchDecisionRequest{Decision: models.PlatformEnrollmentDecisionApprove, Reason: "reviewed cohort"}
			if scenario == "deny" {
				req.Decision = models.PlatformEnrollmentDecisionDeny
			}
			for i := 0; i < 2; i++ {
				record := models.PlatformEnrollmentRequest{ID: fmt.Sprintf("request-%d", i), State: models.PlatformEnrollmentStatePending,
					Fingerprints: models.PlatformEnrollmentCSRFingerprints{App: "key"}, ExpiresAt: time.Now().Add(time.Hour)}
				if i == 1 {
					switch scenario {
					case "stale":
						record.State = models.PlatformEnrollmentStateDenied
					case "expired":
						record.ExpiresAt = time.Now().Add(-time.Second)
					}
				}
				data, err := json.Marshal(record)
				require.NoError(t, err)
				require.NoError(t, store.DocSet(t.Context(), platformEnrollmentCollectionName(), record.ID, data))
				req.Requests = append(req.Requests, models.PlatformEnrollmentDecisionTarget{RequestID: record.ID, Fingerprints: record.Fingerprints})
			}
			if scenario == "fingerprints" {
				req.Requests[1].Fingerprints.App = "changed-key"
			}
			if scenario == "duplicate" {
				req.Requests[1] = req.Requests[0]
			}
			actor := "owner"
			if scenario == "non-owner" {
				actor = "outsider"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			err = store.DecidePlatformEnrollments(ctx, actor, req, "receipt-1")
			want := models.PlatformEnrollmentStatePending
			switch scenario {
			case "approve":
				require.NoError(t, err)
				want = models.PlatformEnrollmentStateApproved
			case "deny":
				require.NoError(t, err)
				want = models.PlatformEnrollmentStateDenied
			case "stale":
				require.ErrorIs(t, err, constants.ErrPlatformEnrollmentAlreadyDecided)
			case "expired":
				require.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequestExpired)
			case "canceled":
				require.ErrorIs(t, err, context.Canceled)
			default:
				require.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidDecision)
			}
			doc, err := store.DocGet(t.Context(), platformEnrollmentCollectionName(), "request-0")
			require.NoError(t, err)
			record, err := decodePlatformEnrollmentRequest(doc)
			require.NoError(t, err)
			require.Equal(t, want, record.State, "a rejected later member must roll back earlier writes")
			if want != models.PlatformEnrollmentStatePending {
				require.Equal(t, "receipt-1", record.DecisionReceiptID)
				require.Equal(t, "reviewed cohort", record.DecisionReason)
				require.Equal(t, "owner", record.ApprovedByUserID)
			}
		})
	}
}

func TestEnrollmentBatchDecisionGovernedReceipt(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)
	req := models.PlatformEnrollmentBatchDecisionRequest{Decision: models.PlatformEnrollmentDecisionApprove, Reason: "fixed cohort"}
	for i := 0; i < 2; i++ {
		csr, _ := generateAppCSRAndKey(t)
		created, err := createPlatformEnrollmentRequest(t, env.enrollSvc, t.Context(), models.PlatformEnrollmentCreateRequest{
			ComponentKind: models.PlatformComponentDashboard, InstanceID: fmt.Sprintf("batch-%d", i), Hostname: "batch.local",
			App: &models.PlatformAppCSRPayload{CSRPEM: csr},
		}, "https://gateway.local")
		require.NoError(t, err)
		req.Requests = append(req.Requests, models.PlatformEnrollmentDecisionTarget{RequestID: created.RequestID, Fingerprints: created.Fingerprints})
	}
	result, err := env.enrollSvc.DecideBatch(t.Context(), env.ownerID, req)
	require.NoError(t, err)
	require.Len(t, result.Requests, 2)
	require.NotEmpty(t, result.ReceiptID)
	for _, target := range req.Requests {
		stored, err := env.enrollSvc.loadByID(t.Context(), target.RequestID)
		require.NoError(t, err)
		require.Equal(t, models.PlatformEnrollmentStateApproved, stored.State)
		require.Equal(t, result.ReceiptID, stored.DecisionReceiptID)
		require.Equal(t, req.Reason, stored.DecisionReason)
	}
}

func TestEnrollmentBatchDecisionPreservesPosturePolicy(t *testing.T) {
	for _, posture := range []config.GatewayPosture{config.PostureConsensus, config.PostureRatify, config.PostureNotary} {
		t.Run(string(posture), func(t *testing.T) {
			gateway := newTestGatewayService(t, testGatewayOpts{posture: posture})
			owner, err := gateway.GetUserService().CreateUser(t.Context())
			require.NoError(t, err)
			record := models.PlatformEnrollmentRequest{State: models.PlatformEnrollmentStatePending, ExpiresAt: time.Now().Add(time.Hour), Fingerprints: models.PlatformEnrollmentCSRFingerprints{App: "key"}}
			data, err := json.Marshal(record)
			require.NoError(t, err)
			require.NoError(t, gateway.GetDocStore().DocSet(t.Context(), platformEnrollmentCollectionName(), "request", data))
			_, err = gateway.GetPlatformEnrollmentService().DecideBatch(t.Context(), owner.ID, models.PlatformEnrollmentBatchDecisionRequest{
				Decision: models.PlatformEnrollmentDecisionApprove, Requests: []models.PlatformEnrollmentDecisionTarget{{RequestID: "request", Fingerprints: record.Fingerprints}},
			})
			want := models.PlatformEnrollmentStatePending
			if posture == config.PostureConsensus {
				// Existing enrollment bootstrap policy exempts DECIDE from L2.
				require.NoError(t, err)
				want = models.PlatformEnrollmentStateApproved
			} else {
				require.ErrorIs(t, err, constants.ErrPlatformEnrollmentGovernanceRejected)
			}
			doc, err := gateway.GetDocStore().DocGet(t.Context(), platformEnrollmentCollectionName(), "request")
			require.NoError(t, err)
			stored, err := decodePlatformEnrollmentRequest(doc)
			require.NoError(t, err)
			require.Equal(t, want, stored.State)
		})
	}
}
