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
	"crypto/ecdsa"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// platformEnrollmentBurstClientDeadline mirrors the Operator's enrollment HTTP
// timeout (operatorEnrollHTTPTimeout in internal/cli/serve). Every call in the
// burst runs under it.
const platformEnrollmentBurstClientDeadline = 10 * time.Second

const (
	burstOutcomeOK          = "ok"
	burstOutcomeRateLimited = "rate_limited"
	burstOutcomeDeadline    = "deadline"
)

// TestPlatformEnrollmentBurst is the service-level enrollment burst fixture:
// N concurrent CreateRequest calls, one DecideBatch over the whole cohort, then
// N synchronized Complete calls (the end-of-staging approval release). Each
// call runs under the client's 10 s deadline.
//
// It asserts the gate contract, which must hold at any size: every call either
// succeeds or receives the typed rate-limited error before the client deadline,
// a rate-limited completion leaves its approval intact and succeeds on retry,
// and the cohort ends with exactly N distinct issued identities. Latency and
// gate counters are logged as the phase report; they are not asserted, because
// Tier 2 runs with the race detector and its absolute timings cannot size the
// gates for production.
func TestPlatformEnrollmentBurst(t *testing.T) {
	for _, n := range []int{100} {
		for _, limit := range []int{1, 2} {
			t.Run(fmt.Sprintf("n=%d/limit=%d", n, limit), func(t *testing.T) {
				runPlatformEnrollmentBurst(t, n, limit)
			})
		}
	}
}

type burstWorker struct {
	create      models.PlatformEnrollmentCreateRequest
	operatorKey *ecdsa.PrivateKey
	cliKey      *ecdsa.PrivateKey
	created     *models.PlatformEnrollmentCreateResponse
	proofs      models.PlatformEnrollmentProofs
	completed   *models.PlatformEnrollmentCompleteResponse

	firstCompleteErr error
}

func runPlatformEnrollmentBurst(t *testing.T, n, limit int) {
	env := setupPlatformEnrollmentEnv(t, true)
	svc := env.enrollSvc
	svc.intake = newPlatformEnrollmentGate(limit, constants.PlatformEnrollmentMaxAdmissionWait)
	svc.issuance = newPlatformEnrollmentGate(limit, constants.PlatformEnrollmentMaxAdmissionWait)

	workers := make([]*burstWorker, n)
	for i := range workers {
		operatorCSR, operatorKey, cliCSR, cliKey := generateOperatorCSRsAndKeys(t)
		workers[i] = &burstWorker{
			create: models.PlatformEnrollmentCreateRequest{
				ComponentKind:     models.PlatformComponentOperator,
				InstanceID:        fmt.Sprintf("burst-%04d", i),
				Hostname:          "burst.local",
				SystemFingerprint: fmt.Sprintf("burst-host-%04d", i),
				Operator:          &models.PlatformOperatorCSRPayload{OperatorCSRPEM: operatorCSR, CLICSRPEM: cliCSR},
			},
			operatorKey: operatorKey,
			cliKey:      cliKey,
		}
	}

	// Phase 1: synchronized intake.
	create := runBurstPhase("create", workers, func(ctx context.Context, w *burstWorker) error {
		resp, err := svc.CreateRequest(ctx, w.create, "https://gateway.local/console")
		if err == nil {
			w.created = resp
		}
		return err
	})
	t.Logf("enrollment burst n=%d (Tier 2, race detector on; relative numbers only)", n)
	t.Log(create.report())
	t.Logf("  intake gate after first attempts: %s", formatGateStats(svc.intake.stats()))
	assertGateContract(t, create)
	createRetries := retryRateLimited(t, workers, func(w *burstWorker) bool { return w.created == nil },
		func(ctx context.Context, w *burstWorker) error {
			resp, err := svc.CreateRequest(ctx, w.create, "https://gateway.local/console")
			if err == nil {
				w.created = resp
			}
			return err
		})
	intake := svc.intake.stats()

	// Phase 2: one owner decision over the fixed staged cohort.
	batch := models.PlatformEnrollmentBatchDecisionRequest{Decision: models.PlatformEnrollmentDecisionApprove}
	for _, w := range workers {
		batch.Requests = append(batch.Requests, models.PlatformEnrollmentDecisionTarget{
			RequestID: w.created.RequestID, Fingerprints: w.created.Fingerprints,
		})
	}
	decideStart := time.Now()
	decided, err := svc.DecideBatch(context.Background(), env.ownerID, batch)
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

	// Phase 3: synchronized approval release.
	complete := runBurstPhase("complete", workers, func(ctx context.Context, w *burstWorker) error {
		resp, err := svc.Complete(ctx, w.created.Token, w.proofs)
		if err == nil {
			w.completed = resp
		}
		w.firstCompleteErr = err
		return err
	})
	t.Logf("  batch decision: %d members in %s", n, decideWall.Round(time.Millisecond))
	t.Log(complete.report())
	t.Logf("  issuance gate after first attempts: %s", formatGateStats(svc.issuance.stats()))
	assertGateContract(t, complete)
	for _, w := range workers {
		if classifyBurstOutcome(w.firstCompleteErr) == burstOutcomeRateLimited {
			assert.Equal(t, models.PlatformEnrollmentStateApproved, loadStoredRequest(t, env, w.created.RequestID).State,
				"a rate-limited completion must leave its approval intact")
		}
	}
	completeRetries := retryRateLimited(t, workers, func(w *burstWorker) bool { return w.completed == nil },
		func(ctx context.Context, w *burstWorker) error {
			resp, err := svc.Complete(ctx, w.created.Token, w.proofs)
			if err == nil {
				w.completed = resp
			}
			return err
		})
	issuance := svc.issuance.stats()

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

	t.Logf("  create retries after 429: %d; intake gate: %s", createRetries, formatGateStats(intake))
	t.Logf("  complete retries after 429: %d; issuance gate: %s", completeRetries, formatGateStats(issuance))
	t.Error("TEMP: print report")
}

// burstPhase records one synchronized phase: every worker's first attempt.
type burstPhase struct {
	name      string
	wall      time.Duration
	latencies []time.Duration
	outcomes  map[string]int
}

// runBurstPhase starts one goroutine per worker, releases them together, and
// runs op once per worker under the client deadline.
func runBurstPhase(name string, workers []*burstWorker, op func(context.Context, *burstWorker) error) burstPhase {
	phase := burstPhase{name: name, latencies: make([]time.Duration, len(workers)), outcomes: map[string]int{}}
	outcomes := make([]string, len(workers))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, w := range workers {
		wg.Add(1)
		go func(i int, w *burstWorker) {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), platformEnrollmentBurstClientDeadline)
			defer cancel()
			began := time.Now()
			err := op(ctx, w)
			phase.latencies[i] = time.Since(began)
			outcomes[i] = classifyBurstOutcome(err)
		}(i, w)
	}
	began := time.Now()
	close(start)
	wg.Wait()
	phase.wall = time.Since(began)
	for _, outcome := range outcomes {
		phase.outcomes[outcome]++
	}
	return phase
}

func classifyBurstOutcome(err error) string {
	switch {
	case err == nil:
		return burstOutcomeOK
	case errors.Is(err, constants.ErrPlatformEnrollmentRateLimited):
		return burstOutcomeRateLimited
	case errors.Is(err, context.DeadlineExceeded):
		return burstOutcomeDeadline
	default:
		return err.Error()
	}
}

// assertGateContract checks the gate contract for a phase: success or the typed
// rate-limited error, never a client deadline or another failure. Violations
// are recorded without stopping the burst, so later phases still report.
func assertGateContract(t *testing.T, phase burstPhase) {
	t.Helper()
	for outcome, count := range phase.outcomes {
		assert.Contains(t, []string{burstOutcomeOK, burstOutcomeRateLimited}, outcome,
			"%s: %d calls failed outside the gate contract", phase.name, count)
	}
	assert.Less(t, slices.Max(phase.latencies), platformEnrollmentBurstClientDeadline,
		"%s: a call must finish before the client deadline", phase.name)
}

// retryRateLimited retries pending workers until all succeed, as the client does
// after a 429 or a timeout, and returns the number of retries. Any other error
// stops the burst.
func retryRateLimited(t *testing.T, workers []*burstWorker, pending func(*burstWorker) bool, op func(context.Context, *burstWorker) error) int {
	t.Helper()
	retries := 0
	for _, w := range workers {
		for pending(w) {
			retries++
			require.Less(t, retries, 10*len(workers), "rate-limited retries did not converge")
			ctx, cancel := context.WithTimeout(context.Background(), platformEnrollmentBurstClientDeadline)
			err := op(ctx, w)
			cancel()
			if err != nil {
				require.Contains(t, []string{burstOutcomeRateLimited, burstOutcomeDeadline}, classifyBurstOutcome(err))
			}
		}
	}
	return retries
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

func formatGateStats(s platformEnrollmentGateStats) string {
	return fmt.Sprintf("admitted %d, queued %d, 429 %d, cancelled %d, peak in-flight %d, max wait %s, total wait %s",
		s.Admitted, s.Queued, s.RateLimited, s.Cancelled, s.PeakInFlight,
		s.MaxWait.Round(time.Millisecond), s.TotalWait.Round(time.Millisecond))
}
