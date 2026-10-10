// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration && !race

package gateway

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// platformEnrollmentBurstClientDeadline is the service-level budget each burst
// call runs under. The Operator client itself sets no per-request timeout; its
// approval wait is bounded by the request expiry.
const platformEnrollmentBurstClientDeadline = 10 * time.Second

const (
	burstOutcomeOK       = "ok"
	burstOutcomeDeadline = "deadline"
)

// TestPlatformEnrollmentBurst is the service-level enrollment burst fixture:
// N concurrent CreateRequest calls, one DecideBatch over the whole cohort, then
// N synchronized Complete calls (the end-of-staging approval release). Each
// call runs under the client's 10 s deadline.
//
// Every call must succeed on its first attempt before the deadline, and the
// cohort must end with exactly N distinct issued identities. Each phase logs
// its wall time, throughput and latency percentiles. Timings are only
// meaningful from a build without race or coverage instrumentation. This
// deadline contract is excluded from race builds and skips coverage runs;
// make test-enrollment-burst runs it separately. Correctness tests retain race
// detection.
func TestPlatformEnrollmentBurst(t *testing.T) {
	if testing.CoverMode() != "" {
		t.Skip("timing contract requires an uninstrumented build; run make test-enrollment-burst")
	}
	for _, n := range []int{100, 1000} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			runPlatformEnrollmentBurst(t, n, burstScenario{})
		})
	}
}

// A live Operator authenticates as soon as completion returns, while the rest
// of the cohort is still issuing. Include that work in the release burst under
// the same correctness budget as the deployment-events scenario.
func TestPlatformEnrollmentBurstWithSessionValidation(t *testing.T) {
	runPlatformEnrollmentBurst(t, 1000, burstScenario{validateSession: true})
}

// Include the deploying owner's live stream, each issued worker CLI session,
// command readiness announcements and initial heartbeat writes in the burst.
// An enrollment-only fixture misses the fan-out work performed by a real deploy.
func TestPlatformEnrollmentBurstWithDeploymentEvents(t *testing.T) {
	runPlatformEnrollmentBurst(t, 1000, burstScenario{validateSession: true, deploymentEvents: true})
}

type burstScenario struct {
	validateSession  bool
	deploymentEvents bool
}

type burstWorker struct {
	create      models.PlatformEnrollmentCreateRequest
	token       string
	operatorKey *ecdsa.PrivateKey
	cliKey      *ecdsa.PrivateKey
	created     *models.PlatformEnrollmentCreateResponse
	proofs      models.PlatformEnrollmentProofs
	completed   *models.PlatformEnrollmentCompleteResponse
}

func runPlatformEnrollmentBurst(t *testing.T, n int, scenario burstScenario) {
	env := setupPlatformEnrollmentEnv(t, true)
	svc := env.enrollSvc
	deadline := platformEnrollmentBurstClientDeadline
	if scenario.validateSession || scenario.deploymentEvents {
		// This scenario checks scale correctness on shared CI hosts and reports
		// timing for enrollment plus post-issuance work. The enrollment-only
		// performance budget stays at 10s.
		deadline = 90 * time.Second
	}
	var eventsMu sync.Mutex
	var published [][]byte
	if scenario.deploymentEvents {
		putCLISession(t, env.docStore, "cli-burst-owner", env.ownerID)
		t.Cleanup(env.svc.GetGatewayWebSocketHandler().RegisterHandler(sseCLIChannelPrefix+"cli-burst-owner", func(_ string, data []byte) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			published = append(published, append([]byte(nil), data...))
		}))
	}

	workers := make([]*burstWorker, n)
	for i := range workers {
		operatorCSR, operatorKey, cliCSR, cliKey := generateOperatorCSRsAndKeys(t)
		token, tokenHash := newPlatformEnrollmentTestToken(t)
		workers[i] = &burstWorker{
			token: token,
			create: models.PlatformEnrollmentCreateRequest{
				TokenHash:         tokenHash,
				ComponentKind:     models.PlatformComponentOperator,
				InstanceID:        fmt.Sprintf("burst-%04d", i),
				Hostname:          "burst.local",
				SystemFingerprint: fmt.Sprintf("burst-host-%04d", i),
				Operator:          &models.PlatformOperatorCSRPayload{OperatorCSRPEM: operatorCSR, CLICSRPEM: cliCSR},
			},
			operatorKey: operatorKey,
			cliKey:      cliKey,
		}
		if scenario.deploymentEvents {
			workers[i].create.DeploymentID = fmt.Sprintf("10000000-0000-4000-8000-%012d", i)
		}
	}

	// Phase 1: synchronized intake.
	create := runBurstPhase(t, "create", workers, deadline, func(ctx context.Context, w *burstWorker) error {
		resp, err := svc.CreateRequest(ctx, w.create, "https://gateway.local/console")
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		w.created = resp
		return nil
	})
	t.Logf("enrollment burst n=%d", n)
	t.Log(create.report())
	requireBurstContract(t, create)

	// Phase 2: one owner decision over the fixed staged cohort.
	batch := models.PlatformEnrollmentBatchDecisionRequest{Decision: models.PlatformEnrollmentDecisionApprove}
	for _, w := range workers {
		batch.Requests = append(batch.Requests, models.PlatformEnrollmentDecisionTarget{
			RequestID: w.created.RequestID, Fingerprints: w.created.Fingerprints,
		})
	}
	decideStart := time.Now()
	decided, err := svc.DecideBatch(t.Context(), env.ownerID, batch)
	decideWall := time.Since(decideStart)
	require.NoError(t, err)
	require.Len(t, decided.Requests, n)
	for _, w := range workers {
		stored := loadStoredRequest(t, env, w.created.RequestID)
		require.Equal(t, models.PlatformEnrollmentStateApproved, stored.State)
		w.proofs = models.PlatformEnrollmentProofs{
			Operator: signCompletionTranscript(t, stored, w.operatorKey),
			CLI:      signCompletionTranscript(t, stored, w.cliKey),
		}
	}
	t.Logf("  batch decision: %d members in %s", n, decideWall.Round(time.Millisecond))

	// Phase 3: synchronized approval release.
	// Each step wraps its error so a failure names the step it came from, and the
	// outcome buckets separate completion failures from login (session
	// validation) failures.
	complete := runBurstPhase(t, "complete", workers, deadline, func(ctx context.Context, w *burstWorker) error {
		resp, err := svc.Complete(ctx, w.token, w.proofs)
		if err != nil {
			return fmt.Errorf("complete: %w", err)
		}
		w.completed = resp
		if scenario.validateSession {
			if _, err := env.svc.auth.ValidateOperatorSession(ctx, resp.Operator.OperatorSessionID); err != nil {
				return fmt.Errorf("validate operator session: %w", err)
			}
		}
		if scenario.deploymentEvents {
			if err := env.docStore.OperatorCommandSubscribed(ctx, resp.Operator.OperatorID, resp.Operator.OperatorSessionID, w.create.DeploymentID); err != nil {
				return fmt.Errorf("command subscribed: %w", err)
			}
			if err := env.docStore.RecordOperatorHeartbeat(ctx, resp.Operator.OperatorID, heartbeatUpdate{LastHeartbeatAt: time.Now().UTC()}); err != nil {
				return fmt.Errorf("record heartbeat: %w", err)
			}
		}
		return nil
	})
	t.Log(complete.report())
	requireBurstContract(t, complete)

	operatorIDs := make(map[string]struct{}, n)
	sessionIDs := make(map[string]struct{}, 2*n)
	for _, w := range workers {
		require.NotNil(t, w.completed.Operator)
		operatorIDs[w.completed.Operator.OperatorID] = struct{}{}
		sessionIDs[w.completed.Operator.OperatorSessionID] = struct{}{}
		sessionIDs[w.completed.Operator.CLISessionID] = struct{}{}
		assert.Equal(t, models.PlatformEnrollmentStateCompleted, loadStoredRequest(t, env, w.created.RequestID).State)
	}
	assert.Len(t, operatorIDs, n, "every worker must receive a distinct Operator identity")
	assert.Len(t, sessionIDs, 2*n, "every worker must receive distinct Operator and CLI sessions")
	if scenario.deploymentEvents {
		eventsMu.Lock()
		observed := append([][]byte(nil), published...)
		eventsMu.Unlock()
		staged, ready := map[string]string{}, map[string]models.OperatorStatusUpdatedPayload{}
		for _, raw := range observed {
			var event models.SSEPublishedEvent
			require.NoError(t, json.Unmarshal(raw, &event))
			var push models.SSEPushPayload
			require.NoError(t, json.Unmarshal(event.Payload, &push))
			var envelope sseEventEnvelope
			require.NoError(t, json.Unmarshal(push.Event, &envelope))
			switch envelope.Type {
			case string(constants.EventPlatformApprovalsChanged):
				var data models.ApprovalsChangedPayload
				require.NoError(t, json.Unmarshal(envelope.Data, &data))
				if data.DeploymentID != "" {
					assert.Zero(t, event.ID, "staging is live-only")
					assert.NotContains(t, staged, data.DeploymentID, "one staging announcement per launch")
					staged[data.DeploymentID] = data.RequestID
				}
			case string(constants.EventOperatorStatusUpdatedActive):
				var data models.OperatorStatusUpdatedPayload
				require.NoError(t, json.Unmarshal(envelope.Data, &data))
				if data.DeploymentID != "" {
					assert.NotContains(t, ready, data.DeploymentID, "one command readiness announcement per launch")
					ready[data.DeploymentID] = data
				}
			}
		}
		require.Len(t, staged, n)
		require.Len(t, ready, n)
		assert.LessOrEqual(t, len(observed), 4*n+4, "live fan-out must grow linearly with the cohort")
		for _, w := range workers {
			assert.Equal(t, w.created.RequestID, staged[w.create.DeploymentID])
			ack := ready[w.create.DeploymentID]
			assert.Equal(t, w.completed.Operator.OperatorID, ack.OperatorID)
			assert.Equal(t, w.completed.Operator.OperatorSessionID, ack.OperatorSessionID)
			assert.Equal(t, constants.OperatorStatusActive, ack.Status)
			rows, err := env.svc.sseStore.SSEEventsListSince(SSERoute{UserID: env.ownerID, CLISessionID: w.completed.Operator.CLISessionID}, 0, 1)
			require.NoError(t, err)
			require.Empty(t, rows, "worker CLI sessions without a stream must receive zero event rows")
		}
		rows, err := env.svc.sseStore.SSEEventsListSince(SSERoute{UserID: env.ownerID, CLISessionID: "cli-burst-owner"}, 0, 4*n)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(rows), 3*n, "durable fan-out must grow linearly with the cohort")
		t.Logf("  deployment events: staged=%d ready=%d live_events=%d durable_owner_rows=%d worker_rows=0", len(staged), len(ready), len(observed), len(rows))
	}
}

// burstPhase records one synchronized phase: every worker's single attempt.
type burstPhase struct {
	name      string
	wall      time.Duration
	deadline  time.Duration
	latencies []time.Duration
	outcomes  map[string]int
}

// burstLiveFailureLimit caps how many failures a phase logs as they happen. The
// rest are still counted in the outcome buckets, so a mass failure cannot flood
// the log.
const burstLiveFailureLimit = 10

// runBurstPhase starts one goroutine per worker, releases them together, and
// runs op once per worker under the client deadline. Calls are bound to the
// test's context, so a failed or timed-out test cancels in-flight workers. Each
// failure is logged the moment it happens, up to burstLiveFailureLimit.
func runBurstPhase(t *testing.T, name string, workers []*burstWorker, deadline time.Duration, op func(context.Context, *burstWorker) error) burstPhase {
	t.Helper()
	phase := burstPhase{name: name, deadline: deadline, latencies: make([]time.Duration, len(workers)), outcomes: map[string]int{}}
	outcomes := make([]string, len(workers))
	var failures atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, w := range workers {
		wg.Add(1)
		go func(i int, w *burstWorker) {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(t.Context(), deadline)
			defer cancel()
			began := time.Now()
			err := op(ctx, w)
			phase.latencies[i] = time.Since(began)
			outcomes[i] = classifyBurstOutcome(err)
			if err != nil {
				if seen := failures.Add(1); seen <= burstLiveFailureLimit {
					t.Logf("  %s FAILED worker=%s after %s: %v", name, w.create.InstanceID, phase.latencies[i].Round(time.Millisecond), err)
				}
			}
		}(i, w)
	}
	began := time.Now()
	close(start)
	wg.Wait()
	phase.wall = time.Since(began)
	for _, outcome := range outcomes {
		phase.outcomes[outcome]++
	}
	if seen := int(failures.Load()); seen > burstLiveFailureLimit {
		t.Logf("  %s: %d more failures not logged individually; see outcome buckets", name, seen-burstLiveFailureLimit)
	}
	return phase
}

func classifyBurstOutcome(err error) string {
	switch {
	case err == nil:
		return burstOutcomeOK
	case errors.Is(err, context.DeadlineExceeded):
		return burstOutcomeDeadline
	default:
		return err.Error()
	}
}

// requireBurstContract stops the burst unless every call in the phase
// succeeded before the client deadline. The report is logged first.
func requireBurstContract(t *testing.T, phase burstPhase) {
	t.Helper()
	require.Equal(t, map[string]int{burstOutcomeOK: len(phase.latencies)}, phase.outcomes,
		"%s: every first attempt must succeed", phase.name)
	require.Less(t, slices.Max(phase.latencies), phase.deadline,
		"%s: every call must finish before the client deadline", phase.name)
}

func (p burstPhase) report() string {
	sorted := slices.Clone(p.latencies)
	slices.Sort(sorted)
	outcomes := make([]string, 0, len(p.outcomes))
	for outcome, count := range p.outcomes {
		outcomes = append(outcomes, fmt.Sprintf("%s=%d", outcome, count))
	}
	slices.Sort(outcomes)
	throughput := float64(p.outcomes[burstOutcomeOK]) / p.wall.Seconds()
	return fmt.Sprintf("  %s: wall %s, %.1f ok/s, p50 %s p95 %s p99 %s max %s, outcomes [%s]",
		p.name, p.wall.Round(time.Millisecond), throughput,
		percentile(sorted, 50), percentile(sorted, 95), percentile(sorted, 99), sorted[len(sorted)-1].Round(time.Millisecond),
		strings.Join(outcomes, " "))
}

// percentile returns the nearest-rank percentile of sorted latencies.
func percentile(sorted []time.Duration, p int) time.Duration {
	rank := (p*len(sorted) + 99) / 100
	return sorted[max(rank-1, 0)].Round(time.Millisecond)
}
