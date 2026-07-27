// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// newTestGovernanceController creates a GovernanceController with minimal
// dependencies for unit-testing the 503 guard logic. No consensus or envelope
// processor is wired — the controller is in the partially-constructed state
// that exists during the boot sequence before SetConsensus / SetEnvelopeProcessor
// are called.
func newTestGovernanceController(t *testing.T) *GovernanceController {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	responder := response.NewWriter(logger)
	return newGovernanceController(cfg, logger, responder, nil)
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

func TestGovernanceController_NoPanicDuringPartialConstruction(t *testing.T) {
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
