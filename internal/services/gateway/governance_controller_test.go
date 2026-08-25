// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/consensus"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
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

// identityBindingRequest builds an httptest.Request carrying an mTLS peer
// certificate with the supplied SPIFFE URI SANs. The request body is unused
// by verifyEnvelopeIdentityBinding (it reads envelopeBody separately).
func identityBindingRequest(t *testing.T, spiffeIDs ...string) *http.Request {
	t.Helper()
	uris := make([]*url.URL, 0, len(spiffeIDs))
	for _, s := range spiffeIDs {
		u, err := url.Parse(s)
		require.NoError(t, err)
		uris = append(uris, u)
	}
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{URIs: uris}},
	}
	return req
}

// mutationEnvelopeBytes builds a wire GovernanceEnvelope with the given action
// type and identity claims, encoded in canonical protojson (the wire form
// verifyEnvelopeIdentityBinding decodes).
func mutationEnvelopeBytes(t *testing.T, actionType constants.ActionType, operatorID, operatorSessionID string, source commonv1.Component) []byte {
	t.Helper()
	b, err := protojson.Marshal(&commonv1.GovernanceEnvelope{
		ActionType:        string(actionType),
		OperatorId:        operatorID,
		OperatorSessionId: operatorSessionID,
		SourceComponent:   source,
	})
	require.NoError(t, err)
	return b
}

// TestVerifyEnvelopeIdentityBinding_DocumentUpdateEmptyIdentity_FailsClosed
// asserts that a DOCUMENT_UPDATE mutation envelope with empty operator_id and
// operator_session_id is rejected at the transport boundary with
// ErrIdentityBindingFailed rather than passing through to the downstream
// processor (the previous fail-open behavior).
func TestVerifyEnvelopeIdentityBinding_DocumentUpdateEmptyIdentity_FailsClosed(t *testing.T) {
	req := identityBindingRequest(t, "spiffe://g8e.local/operator/org-1/op-1/sess-1")
	env := mutationEnvelopeBytes(t, constants.ActionTypeDocumentUpdate, "", "", commonv1.Component_COMPONENT_G8EO)
	err := verifyEnvelopeIdentityBinding(req, env)
	require.Error(t, err)
	require.True(t, errors.Is(err, constants.ErrIdentityBindingFailed), "expected ErrIdentityBindingFailed, got %v", err)
}

// TestVerifyEnvelopeIdentityBinding_DocumentDeleteEmptyIdentity_FailsClosed
// asserts the same fail-closed guarantee for DOCUMENT_DELETE.
func TestVerifyEnvelopeIdentityBinding_DocumentDeleteEmptyIdentity_FailsClosed(t *testing.T) {
	req := identityBindingRequest(t, "spiffe://g8e.local/operator/org-1/op-1/sess-1")
	env := mutationEnvelopeBytes(t, constants.ActionTypeDocumentDelete, "", "", commonv1.Component_COMPONENT_G8EO)
	err := verifyEnvelopeIdentityBinding(req, env)
	require.Error(t, err)
	require.True(t, errors.Is(err, constants.ErrIdentityBindingFailed), "expected ErrIdentityBindingFailed, got %v", err)
}

// TestVerifyEnvelopeIdentityBinding_FileEditEmptyIdentity_FailsClosed asserts
// the same fail-closed guarantee for FILE_EDIT.
func TestVerifyEnvelopeIdentityBinding_FileEditEmptyIdentity_FailsClosed(t *testing.T) {
	req := identityBindingRequest(t, "spiffe://g8e.local/operator/org-1/op-1/sess-1")
	env := mutationEnvelopeBytes(t, constants.ActionTypeFileEdit, "", "", commonv1.Component_COMPONENT_G8EO)
	err := verifyEnvelopeIdentityBinding(req, env)
	require.Error(t, err)
	require.True(t, errors.Is(err, constants.ErrIdentityBindingFailed), "expected ErrIdentityBindingFailed, got %v", err)
}

// TestVerifyEnvelopeIdentityBinding_MutationWithMatchingOperatorCert_Admitted
// is the positive counterpart: a DOCUMENT_UPDATE envelope carrying both
// operator_id and operator_session_id, presented via a matching operator
// SPIFFE cert, is admitted (returns nil).
func TestVerifyEnvelopeIdentityBinding_MutationWithMatchingOperatorCert_Admitted(t *testing.T) {
	req := identityBindingRequest(t, "spiffe://g8e.local/operator/org-1/op-1/sess-1")
	env := mutationEnvelopeBytes(t, constants.ActionTypeDocumentUpdate, "op-1", "sess-1", commonv1.Component_COMPONENT_G8EO)
	err := verifyEnvelopeIdentityBinding(req, env)
	require.NoError(t, err)
}

// TestVerifyEnvelopeIdentityBinding_NonMutationReadEmptyIdentity_PassesThrough
// asserts the fail-closed path applies only to mutations. A non-mutation read
// (FS_READ) with empty identity fields still returns nil so the downstream
// processor validates it — preserving the prior behavior for reads.
func TestVerifyEnvelopeIdentityBinding_NonMutationReadEmptyIdentity_PassesThrough(t *testing.T) {
	req := identityBindingRequest(t, "spiffe://g8e.local/operator/org-1/op-1/sess-1")
	env := mutationEnvelopeBytes(t, constants.ActionTypeFsRead, "", "", commonv1.Component_COMPONENT_G8EO)
	err := verifyEnvelopeIdentityBinding(req, env)
	require.NoError(t, err, "non-mutation read with empty identity should pass through to processor")
}
