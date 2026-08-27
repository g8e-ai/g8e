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
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/consensus"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)

// newTestGovernanceController creates a GovernanceController with minimal
// dependencies for unit-testing the controller.
func newTestGovernanceController(t *testing.T) *GovernanceController {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	responder := response.NewWriter(logger)
	return newGovernanceController(GovernanceControllerDeps{Cfg: cfg, Logger: logger, Responder: responder, Consensus: nil})
}

func newTestRouterHandler(t *testing.T, posture config.GatewayPosture, cs *consensus.ConsensusService) *HTTPHandler {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	cfg.Gateway.Posture = posture
	logger := testutil.NewTestLogger()
	responder := response.NewWriter(logger)
	h, err := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:    cfg,
		Logger: logger,
		GovernanceControllerDeps: GovernanceControllerDeps{
			Cfg:       cfg,
			Logger:    logger,
			Responder: responder,
			Consensus: cs,
		},
		PKIControllerDeps: PKIControllerDeps{
			Cfg:       cfg,
			Logger:    logger,
			Responder: responder,
		},
	})
	require.NoError(t, err)
	return h
}

func TestHTTPRouter_ConsensusDeliberate_PostureDoctrine_Returns404(t *testing.T) {
	h := newTestRouterHandler(t, config.PostureDoctrine, nil)
	router := h.buildPublicRouter()

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ConsensusDeliberate, bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestHTTPRouter_ConsensusDeliberate_PostureConsensus_RoutesToDeliberate(t *testing.T) {
	logger := testutil.NewTestLogger()
	responder := response.NewWriter(logger)

	cs := consensus.NewConsensusService("test-cluster", nil, nil, logger, responder)
	h := newTestRouterHandler(t, config.PostureConsensus, cs)

	router := h.buildPublicRouter()

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ConsensusDeliberate, bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Deliberate with invalid empty body returns 400 Bad Request, not 404 Not Found
	require.NotEqual(t, http.StatusNotFound, w.Code)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGovernanceEnvelope_NotConfigured_Returns503_Unit(t *testing.T) {
	c := newTestGovernanceController(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	c.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), constants.ErrEnvelopeProcessorNotInit.Error())
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
