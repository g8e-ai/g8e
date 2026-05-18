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

package listen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
)

// fakeEnvelopeProcessor is a test double that returns predetermined results
// for ProcessEnvelope. It captures the payload it was called with so tests
// can assert the handler forwarded the body unchanged.
type fakeEnvelopeProcessor struct {
	receipt    *operatorv1.ActionReceipt
	err        error
	gotPayload []byte
	calls      int
}

func (f *fakeEnvelopeProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	f.calls++
	f.gotPayload = append([]byte(nil), payload...)
	return f.receipt, f.err
}

func newGovernanceEnvelopeHandler(t *testing.T, proc EnvelopeProcessor) *HTTPHandler {
	t.Helper()
	h, _ := setupTestHTTPHandler(t)
	h.envProc = proc
	return h
}

func TestGovernanceEnvelope_NotConfigured_Returns503(t *testing.T) {
	h := newGovernanceEnvelopeHandler(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/governance/envelope", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestGovernanceEnvelope_NonPostMethod_Returns405(t *testing.T) {
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
	proc := &fakeEnvelopeProcessor{}
	h := newGovernanceEnvelopeHandler(t, proc)

	req := httptest.NewRequest(http.MethodPost, "/api/governance/envelope", bytes.NewReader(nil))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Zero(t, proc.calls, "envelope processor must not be called for empty body")
}

func TestGovernanceEnvelope_VerificationErrors_Return403(t *testing.T) {
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
		{"l3 verifier not configured", governance.ErrL3VerifierNotConfigured},
		{"state root mismatch", governance.ErrStateRootMismatch},
		{"state root missing", governance.ErrStateRootMissing},
		{"state root required", governance.ErrStateRootRequired},
		{"l1 validation failed", fmt.Errorf("%w: violation", governance.ErrL1ValidationFailed)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proc := &fakeEnvelopeProcessor{err: tc.err}
			h := newGovernanceEnvelopeHandler(t, proc)

			req := httptest.NewRequest(http.MethodPost, "/api/governance/envelope", bytes.NewReader([]byte(`{"id":"x"}`)))
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
	proc := &fakeEnvelopeProcessor{err: errors.New("invalid UAP JSON envelope: unexpected token")}
	h := newGovernanceEnvelopeHandler(t, proc)

	req := httptest.NewRequest(http.MethodPost, "/api/governance/envelope", bytes.NewReader([]byte(`not-json`)))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGovernanceEnvelope_OversizedPayload_Returns400(t *testing.T) {
	proc := &fakeEnvelopeProcessor{err: errors.New("payload exceeds 1048576 byte limit")}
	h := newGovernanceEnvelopeHandler(t, proc)

	req := httptest.NewRequest(http.MethodPost, "/api/governance/envelope", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGovernanceEnvelope_Success_Returns200WithSignedReceipt(t *testing.T) {
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    "tx-abc",
		TransactionHash:  "abc123",
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:    "completed",
		StateRootBefore:  "root-before",
		StateRootAfter:   "root-after",
		ExecutedAtUnixMs: 1234567890,
		SignerKeyId:      "warden-key-id",
		Signature:        "deadbeef",
	}
	proc := &fakeEnvelopeProcessor{receipt: receipt}
	h := newGovernanceEnvelopeHandler(t, proc)

	body := []byte(`{"id":"tx-abc"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/governance/envelope", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, proc.calls)
	require.Equal(t, body, proc.gotPayload, "handler must forward the body unchanged to the processor")

	// Receipt should be returned as JSON. Field names follow protojson.
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, "tx-abc", got["transaction_id"])
	require.Equal(t, "abc123", got["transaction_hash"])
	require.Equal(t, "deadbeef", got["signature"])
	require.Equal(t, "warden-key-id", got["signer_key_id"])
}

func TestGovernanceEnvelope_FailedExecution_StillReturns200(t *testing.T) {
	// A signed FAILED receipt is still cryptographic evidence and must be
	// returned to the caller with HTTP 200, not surfaced as a server error.
	receipt := &operatorv1.ActionReceipt{
		TransactionId: "tx-fail",
		Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
		ResultSummary: "failed: handler error",
		Signature:     "cafef00d",
		SignerKeyId:   "warden-key-id",
	}
	// Warden returns (receipt, execErr) when a handler fails. ProcessEnvelope
	// propagates that pair. The HTTP layer treats a non-nil receipt as
	// authoritative — execErr alone is not 5xx territory.
	proc := &fakeEnvelopeProcessor{receipt: receipt, err: nil}
	h := newGovernanceEnvelopeHandler(t, proc)

	req := httptest.NewRequest(http.MethodPost, "/api/governance/envelope", bytes.NewReader([]byte(`{"id":"tx-fail"}`)))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestGovernanceEnvelope_NilReceiptNilError_Returns500(t *testing.T) {
	// Defensive: a regression in the processor that returns (nil, nil) must
	// not be silently masked as success.
	proc := &fakeEnvelopeProcessor{receipt: nil, err: nil}
	h := newGovernanceEnvelopeHandler(t, proc)

	req := httptest.NewRequest(http.MethodPost, "/api/governance/envelope", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	h.handleGovernanceEnvelope(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}
