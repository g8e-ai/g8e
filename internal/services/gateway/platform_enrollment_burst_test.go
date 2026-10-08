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
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
// meaningful from a build without the race detector, so this deadline contract
// is excluded from race builds. Correctness tests retain race detection.
func TestPlatformEnrollmentBurst(t *testing.T) {
	for _, n := range []int{100, 1000} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			runPlatformEnrollmentBurst(t, n, false)
		})
	}
}

// A live Operator authenticates as soon as completion returns, while the rest
// of the cohort is still issuing. Include that work in the release burst.
func TestPlatformEnrollmentBurstWithSessionValidation(t *testing.T) {
	runPlatformEnrollmentBurst(t, 1000, true)
}

type burstWorker struct {
	create      models.PlatformEnrollmentCreateRequest
	operatorKey *ecdsa.PrivateKey
	cliKey      *ecdsa.PrivateKey
	created     *models.PlatformEnrollmentCreateResponse
	proofs      models.PlatformEnrollmentProofs
	completed   *models.PlatformEnrollmentCompleteResponse
}

func runPlatformEnrollmentBurst(t *testing.T, n int, validateSession bool) {
	env := setupPlatformEnrollmentEnv(t, true)
	svc := env.enrollSvc

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
	t.Logf("  batch decision: %d members in %s", n, decideWall.Round(time.Millisecond))

	// Phase 3: synchronized approval release.
	complete := runBurstPhase("complete", workers, func(ctx context.Context, w *burstWorker) error {
		resp, err := svc.Complete(ctx, w.created.Token, w.proofs)
		if err == nil {
			w.completed = resp
			if validateSession {
				_, err = env.svc.auth.ValidateOperatorSession(resp.Operator.OperatorSessionID)
			}
		}
		return err
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
}

// burstPhase records one synchronized phase: every worker's single attempt.
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
	require.Less(t, slices.Max(phase.latencies), platformEnrollmentBurstClientDeadline,
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
