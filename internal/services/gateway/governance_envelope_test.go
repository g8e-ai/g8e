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

//go:build integration

package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// marshalEnvelope encodes a GovernanceEnvelope in its canonical protojson wire
// form — the exact bytes a BYO client POSTs and the format the downstream
// processor decodes. Identity-binding tests must use this rather than
// hand-written snake_case JSON, which is not the wire format.
func marshalEnvelope(t *testing.T, env *commonv1.GovernanceEnvelope) []byte {
	t.Helper()
	b, err := protojson.Marshal(env)
	require.NoError(t, err)
	return b
}

// identityEnvelope builds a wire envelope carrying only operator/session
// identity, for CLI- and operator-cert binding cases that have no source
// component.
func identityEnvelope(t *testing.T, operatorID, operatorSessionID string) []byte {
	t.Helper()
	return marshalEnvelope(t, &commonv1.GovernanceEnvelope{
		OperatorId:        operatorID,
		OperatorSessionId: operatorSessionID,
	})
}

// appEnvelope builds a wire envelope for an app workload (AGENT/CLIENT) that
// authenticates via an app SPIFFE identity.
func appEnvelope(t *testing.T, operatorID string, source commonv1.Component) []byte {
	t.Helper()
	return marshalEnvelope(t, &commonv1.GovernanceEnvelope{
		OperatorId:      operatorID,
		SourceComponent: source,
	})
}

// fakeEnvelopeProcessor is a test double that returns predetermined results
// for ProcessEnvelope. It captures the payload it was called with so tests
// can assert the handler forwarded the body unchanged.
type fakeEnvelopeProcessor struct {
	receipt    *operatorv1.ActionReceipt
	err        error
	gotPayload []byte
	calls      int
}

// actionReceiptJSON represents the JSON response format for ActionReceipt
// using protojson field naming conventions.
type actionReceiptJSON struct {
	TransactionID   string `json:"transaction_id"`
	TransactionHash string `json:"transaction_hash"`
	Signature       string `json:"signature"`
	SignerKeyID     string `json:"signer_key_id"`
}

func (f *fakeEnvelopeProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	f.calls++
	f.gotPayload = append([]byte(nil), payload...)
	return f.receipt, f.err
}

func newGovernanceEnvelopeHandler(t *testing.T, proc governance.EnvelopeProcessor) *HTTPHandler {
	t.Helper()
	h, _ := setupTestHTTPHandler(t)
	h.envProc = proc
	return h
}

func TestGovernanceEnvelope_NotConfigured_Returns503(t *testing.T) {
	t.Parallel()
	h := newGovernanceEnvelopeHandler(t, nil)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestGovernanceEnvelope_NonPostMethod_Returns405(t *testing.T) {
	t.Parallel()
	proc := &fakeEnvelopeProcessor{}
	h := newGovernanceEnvelopeHandler(t, proc)

	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(m, "/api/governance/envelope", nil)
		w := httptest.NewRecorder()
		h.handleGovernanceEnvelope(w, req)
		require.Equal(t, http.StatusMethodNotAllowed, w.Code, "method=%s", m)
	}
	require.Zero(t, proc.calls, "envelope processor must not be called for non-POST methods")
}

func TestGovernanceEnvelope_EmptyBody_Returns400(t *testing.T) {
	t.Parallel()
	proc := &fakeEnvelopeProcessor{}
	h := newGovernanceEnvelopeHandler(t, proc)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader(nil))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Zero(t, proc.calls, "envelope processor must not be called for empty body")
}

func TestGovernanceEnvelope_VerificationErrors_Return403(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
	}{
		{"expired", governance.ErrTransactionExpired},
		{"replay", governance.ErrTransactionReplay},
		{"hash mismatch", governance.ErrTransactionHashMismatch},
		{"unknown action", governance.ErrUnknownActionType},
		{"missing l2", governance.ErrL2SignatureMissing},
		{"invalid l2 signature", governance.ErrL2SignatureInvalid},
		{"unknown l2 signer", governance.ErrL2KeyNotConfigured},
		{"missing l3 proof", governance.ErrL3ProofMissing},
		{"invalid l3 proof", governance.ErrL3ProofInvalid},
		{"l3 notary not configured", governance.ErrL3NotaryNotConfigured},
		{"state root mismatch", governance.ErrStateRootMismatch},
		{"state root missing", governance.ErrStateRootMissing},
		{"state root required", governance.ErrStateRootRequired},
		{"l1 validation failed", fmt.Errorf("%w: violation", governance.ErrL1ValidationFailed)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			proc := &fakeEnvelopeProcessor{err: tc.err}
			h := newGovernanceEnvelopeHandler(t, proc)

			req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{"id":"x"}`)))
			w := httptest.NewRecorder()

			h.handleGovernanceEnvelope(w, req)

			require.Equal(t, http.StatusForbidden, w.Code, "error %v should map to 403", tc.err)

			var body struct {
				Error string `json:"error"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.NotEmpty(t, body.Error)
		})
	}
}

func TestGovernanceEnvelope_DecodeFailure_Returns400(t *testing.T) {
	t.Parallel()
	proc := &fakeEnvelopeProcessor{err: errors.New("invalid GovernanceEnvelope: unexpected token")}
	h := newGovernanceEnvelopeHandler(t, proc)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`not-json`)))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGovernanceEnvelope_OversizedPayload_Returns400(t *testing.T) {
	t.Parallel()
	proc := &fakeEnvelopeProcessor{err: errors.New("payload exceeds 1048576 byte limit")}
	h := newGovernanceEnvelopeHandler(t, proc)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGovernanceEnvelope_Success_Returns200WithSignedReceipt(t *testing.T) {
	t.Parallel()
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    "tx-abc",
		TransactionHash:  "abc123",
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:    "completed",
		StateRootBefore:  "root-before",
		StateRootAfter:   "root-after",
		ExecutedAtUnixMs: 1234567890,
		SignerKeyId:      "Actuator-key-id",
		Signature:        "deadbeef",
	}
	proc := &fakeEnvelopeProcessor{receipt: receipt}
	h := newGovernanceEnvelopeHandler(t, proc)

	body := []byte(`{"id":"tx-abc"}`)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, proc.calls)
	require.Equal(t, body, proc.gotPayload, "handler must forward the body unchanged to the processor")

	// Receipt should be returned as JSON. Field names follow protojson.
	var got actionReceiptJSON
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, "tx-abc", got.TransactionID)
	require.Equal(t, "abc123", got.TransactionHash)
	require.Equal(t, "deadbeef", got.Signature)
	require.Equal(t, "Actuator-key-id", got.SignerKeyID)
}

func TestGovernanceEnvelope_FailedExecution_StillReturns200(t *testing.T) {
	t.Parallel()
	// A signed FAILED receipt is still cryptographic evidence and must be
	// returned to the caller with HTTP 200, not surfaced as a server error.
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "tx-fail",
		Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
		ResultSummary: "failed: handler error",
		Signature:     "cafef00d",
		SignerKeyId:   "Actuator-key-id",
	}
	// Actuator returns (receipt, execErr) when a handler fails. ProcessEnvelope
	// propagates that pair. The HTTP layer treats a non-nil receipt as
	// authoritative - execErr alone is not 5xx territory.
	proc := &fakeEnvelopeProcessor{receipt: receipt, err: nil}
	h := newGovernanceEnvelopeHandler(t, proc)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{"id":"tx-fail"}`)))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestGovernanceEnvelope_NilReceiptNilError_Returns500(t *testing.T) {
	t.Parallel()
	// Defensive: a regression in the processor that returns (nil, nil) must
	// not be silently masked as success.
	proc := &fakeEnvelopeProcessor{receipt: nil, err: nil}
	h := newGovernanceEnvelopeHandler(t, proc)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestVerifyEnvelopeIdentityBinding_NoMTLS_ReturnsError(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	err := verifyEnvelopeIdentityBinding(req, identityEnvelope(t, "op-1", "sess-1"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "mTLS client certificate required")
}

func TestVerifyEnvelopeIdentityBinding_NoURISAN_ReturnsError(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{}},
	}
	err := verifyEnvelopeIdentityBinding(req, identityEnvelope(t, "op-1", "sess-1"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "client certificate missing URI SAN")
}

func TestVerifyEnvelopeIdentityBinding_MatchingOperatorSPIFFEID_ReturnsNil(t *testing.T) {
	t.Parallel()
	spiffeURL, parseErr := url.Parse("spiffe://g8e.local/operator/org-1/op-1/sess-1")
	require.NoError(t, parseErr)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			URIs: []*url.URL{spiffeURL},
		}},
	}
	envelope := identityEnvelope(t, "op-1", "sess-1")
	err := verifyEnvelopeIdentityBinding(req, envelope)
	require.NoError(t, err)
}

func TestVerifyEnvelopeIdentityBinding_MismatchedOperatorID_ReturnsError(t *testing.T) {
	t.Parallel()
	spiffeURL, parseErr := url.Parse("spiffe://g8e.local/operator/org-1/op-2/sess-1")
	require.NoError(t, parseErr)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			URIs: []*url.URL{spiffeURL},
		}},
	}
	envelope := identityEnvelope(t, "op-1", "sess-1")
	err := verifyEnvelopeIdentityBinding(req, envelope)
	require.Error(t, err)
	require.Contains(t, err.Error(), "certificate URI SAN does not match envelope identity claims")
}

func TestVerifyEnvelopeIdentityBinding_MatchingAppSPIFFEID_ReturnsNil(t *testing.T) {
	t.Parallel()
	spiffeURL, parseErr := url.Parse("spiffe://g8e.local/app/op-1")
	require.NoError(t, parseErr)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			URIs: []*url.URL{spiffeURL},
		}},
	}
	envelope := appEnvelope(t, "op-1", commonv1.Component_COMPONENT_AGENT)
	err := verifyEnvelopeIdentityBinding(req, envelope)
	require.NoError(t, err)
}

func TestVerifyEnvelopeIdentityBinding_InvalidJSON_ReturnsNil(t *testing.T) {
	t.Parallel()
	spiffeURL, parseErr := url.Parse("spiffe://g8e.local/operator/org-1/op-1/sess-1")
	require.NoError(t, parseErr)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			URIs: []*url.URL{spiffeURL},
		}},
	}
	envelope := []byte(`not-json`)
	err := verifyEnvelopeIdentityBinding(req, envelope)
	require.NoError(t, err, "invalid JSON should pass through to processor for decode error")
}

func TestVerifyEnvelopeIdentityBinding_NoIdentityFields_ReturnsNil(t *testing.T) {
	t.Parallel()
	spiffeURL, parseErr := url.Parse("spiffe://g8e.local/operator/org-1/op-1/sess-1")
	require.NoError(t, parseErr)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader([]byte(`{}`)))
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			URIs: []*url.URL{spiffeURL},
		}},
	}
	envelope, marshalErr := protojson.Marshal(&commonv1.GovernanceEnvelope{EventType: "test"})
	require.NoError(t, marshalErr)
	err := verifyEnvelopeIdentityBinding(req, envelope)
	require.NoError(t, err, "envelope without identity fields should pass through to processor")
}

func TestGatewayModeService_SetEnvelopeProcessor(t *testing.T) {
	t.Parallel()
	ls, _ := setupTestGatewayService(t)

	// Initially envProc should be nil
	require.Nil(t, ls.handler.envProc, "envProc should be nil initially")

	// Create a fake processor
	fakeProc := &fakeEnvelopeProcessor{}

	// Set the processor
	ls.SetEnvelopeProcessor(fakeProc)

	// Verify it was set
	require.Equal(t, fakeProc, ls.handler.envProc, "envProc should be set to the provided processor")

	// Set to nil to disable
	ls.SetEnvelopeProcessor(nil)
	require.Nil(t, ls.handler.envProc, "envProc should be nil after setting to nil")
}
