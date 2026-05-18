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
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
)

// EnvelopeProcessor verifies and executes UAP JSON envelopes synchronously,
// returning a signed ActionReceipt or a governance verification error. It is
// implemented by *pubsub.PubSubCommandService and injected into the listen
// HTTP surface after construction so the substrate's fail-closed mutation
// boundary is reachable via POST /api/governance/envelope.
type EnvelopeProcessor interface {
	ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error)
}

// SetEnvelopeProcessor wires the synchronous envelope-processing pipeline
// into the listen HTTP surface. It must be called after the listen service
// has been constructed and before BYO clients submit transactions to
// /api/governance/envelope. Calling with nil disables the endpoint.
func (ls *ListenService) SetEnvelopeProcessor(p EnvelopeProcessor) {
	ls.handler.envProc = p
}

// handleGovernanceEnvelope is the canonical synchronous mutation entry point
// for BYO substrate clients. It accepts a UAP JSON envelope, verifies it
// through the substrate's fail-closed gate (id, hash, expiry, nonce, state
// root, L2/L3 governance), executes it through the Warden, and returns the
// signed ActionReceipt.
//
// Status semantics:
//   - 200 OK: envelope verified and executed (receipt body); receipt.status
//     reflects whether the underlying handler succeeded or failed.
//   - 400 Bad Request: malformed envelope (decode failure, empty body,
//     payload too large) — no governance state mutated.
//   - 403 Forbidden: governance verification failed before execution
//     (expired, replayed, hash mismatch, missing/invalid L2/L3, unknown
//     action type) — no state mutated, no receipt produced.
//   - 503 Service Unavailable: envelope processor not yet initialized.
//   - 405 Method Not Allowed: non-POST methods.
func (h *HTTPHandler) handleGovernanceEnvelope(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.envProc == nil {
		jsonError(w, http.StatusServiceUnavailable, "envelope processor not initialized")
		return
	}

	body, err := readBody(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	if len(body) == 0 {
		jsonError(w, http.StatusBadRequest, "empty request body")
		return
	}

	receipt, procErr := h.envProc.ProcessEnvelope(r.Context(), body)
	if procErr != nil {
		status := classifyEnvelopeError(procErr)
		jsonError(w, status, procErr.Error())
		return
	}
	if receipt == nil {
		// Defensive: a nil receipt with nil error should never happen, but if
		// the processor regresses, do not mask the failure.
		jsonError(w, http.StatusInternalServerError, "envelope processor returned nil receipt without error")
		return
	}

	// Receipt is returned as JSON. Execution-failure receipts (status=FAILED)
	// are still HTTP 200 because they represent a verified, audited outcome
	// — the caller has cryptographic evidence of the attempt.
	jsonResponse(w, http.StatusOK, receipt)
}

// classifyEnvelopeError maps a governance verification error to an HTTP status.
// Decode failures are 400 (malformed input); all governance sentinel errors
// are 403 (caller-side proof failures); anything else is 500.
func classifyEnvelopeError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	// Decode failures are caller-side bad input.
	var jsonErr *json.SyntaxError
	if errors.As(err, &jsonErr) {
		return http.StatusBadRequest
	}
	msg := err.Error()
	switch {
	case errors.Is(err, governance.ErrInvalidEnvelope),
		errors.Is(err, governance.ErrTransactionIDMissing),
		errors.Is(err, governance.ErrUnknownActionType),
		errors.Is(err, governance.ErrPayloadMissing),
		errors.Is(err, governance.ErrPayloadDecodeFailed),
		errors.Is(err, governance.ErrL1ValidationFailed),
		errors.Is(err, governance.ErrTransactionHashMissing),
		errors.Is(err, governance.ErrTransactionHashMismatch),
		errors.Is(err, governance.ErrTransactionExpired),
		errors.Is(err, governance.ErrNonceMissing),
		errors.Is(err, governance.ErrTransactionReplay),
		errors.Is(err, governance.ErrStateRootMissing),
		errors.Is(err, governance.ErrStateRootRequired),
		errors.Is(err, governance.ErrStateRootMismatch),
		errors.Is(err, governance.ErrL2SignatureMissing),
		errors.Is(err, governance.ErrL2SignatureInvalid),
		errors.Is(err, governance.ErrL2KeyNotConfigured),
		errors.Is(err, governance.ErrL3ProofMissing),
		errors.Is(err, governance.ErrL3ProofInvalid),
		errors.Is(err, governance.ErrL3VerifierNotConfigured):
		return http.StatusForbidden
	}
	// Wrapped invalid-envelope decode error from ProcessEnvelope.
	if len(msg) > 0 && (containsAny(msg, "invalid UAP JSON envelope", "empty payload", "payload exceeds")) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) == 0 {
			continue
		}
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

// indexOf is a tiny dependency-free substring index to avoid pulling strings
// just for two predicates inside a status classifier.
func indexOf(s, sub string) int {
	if len(sub) == 0 || len(s) < len(sub) {
		return -1
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
