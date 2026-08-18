// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/consensus"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// newTestGovernanceController creates a GovernanceController with minimal
// dependencies for unit-testing the 503 guard logic. No consensus or envelope
// processor is wired — simulates a posture where these features are not
// configured. The Consensus dep is a non-nil *atomic.Pointer holding nil (the
// "not configured" state); production always passes &ls.consensusSvc, so the
// controller contract is a non-nil pointer-to-atomic whose Load() may be nil.
func newTestGovernanceController(t *testing.T) *GovernanceController {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	responder := response.NewWriter(logger)
	consensusPtr := &atomic.Pointer[consensus.ConsensusService]{}
	return newGovernanceController(GovernanceControllerDeps{Cfg: cfg, Logger: logger, Responder: responder, Consensus: consensusPtr})
}

func TestConsensusDeliberate_NotConfigured_Returns503(t *testing.T) {
	c := newTestGovernanceController(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ConsensusDeliberate, bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	c.handleConsensusDeliberate(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), constants.ErrConsensusNotConfigured.Error())
}

func TestGovernanceEnvelope_NotConfigured_Returns503_Unit(t *testing.T) {
	c := newTestGovernanceController(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	c.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), constants.ErrEnvelopeProcessorNotInit.Error())
}

func TestGovernanceController_NoPanicWhenNotConfigured(t *testing.T) {
	c := newTestGovernanceController(t)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	results := make([]int, goroutines*2) // status codes

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ConsensusDeliberate, bytes.NewReader([]byte(`{}`)))
			w := httptest.NewRecorder()
			c.handleConsensusDeliberate(w, req)
			results[idx] = w.Code
		}(i)

		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
			w := httptest.NewRecorder()
			c.handleGovernanceEnvelope(w, req)
			results[goroutines+idx] = w.Code
		}(i)
	}

	wg.Wait()

	for i, code := range results {
		require.Equal(t, http.StatusServiceUnavailable, code, "goroutine %d: expected 503, got %d", i, code)
	}
}

// TestGovernanceController_SetConsensusService_RaceWithDeliberateRequest
// guards against a data race between SetConsensusService (which stores into
// ls.consensusSvc) and handleConsensusDeliberate (which loads from
// c.consensus, where c.consensus == &ls.consensusSvc). The consensus cell is
// backed by atomic.Pointer: the writer calls Store and the reader calls Load,
// which provides the required happens-before edge. Under `go test -race` this
// test passes with the atomic.Pointer backing and fails if the cell regresses
// to a raw **consensus.ConsensusService (which carries no memory ordering
// semantics).
func TestGovernanceController_SetConsensusService_RaceWithDeliberateRequest(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	responder := response.NewWriter(logger)

	// Minimal GatewayModeService carrying only the consensusSvc cell — the
	// aliased memory that SetConsensusService stores and GovernanceController
	// loads. A zero-value GatewayModeService is sufficient because
	// SetConsensusService and the controller only touch this field.
	ls := &GatewayModeService{}

	svc := consensus.NewConsensusService("race-test", nil, nil, logger, responder)

	c := newGovernanceController(GovernanceControllerDeps{
		Cfg:       cfg,
		Logger:    logger,
		Responder: responder,
		Consensus: &ls.consensusSvc,
	})

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: mimics SetConsensusService being called (e.g. during boot or
	// re-wiring), alternating between a real service and nil.
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			ls.SetConsensusService(svc)
			ls.SetConsensusService(nil)
		}
	}()

	// Reader: mimics the request path loading from c.consensus.
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ConsensusDeliberate, bytes.NewReader([]byte(`{}`)))
			w := httptest.NewRecorder()
			c.handleConsensusDeliberate(w, req)
		}
	}()

	wg.Wait()
}
