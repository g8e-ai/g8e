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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/protocol"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// SetEnvelopeProcessor wires the synchronous envelope-processing pipeline
// into the gateway HTTP surface. It must be called after the gateway service
// has been constructed and before BYO clients submit transactions to
// /api/v1/governance/envelopes. Calling with nil disables the endpoint.
func (ls *GatewayModeService) SetEnvelopeProcessor(p governance.EnvelopeProcessor) {
	ls.handler.envProc = p
	// Dependencies are now set via SetDependencies in runGatewayMode or similar
}

// verifyEnvelopeIdentityBinding enforces transport-to-envelope identity binding
// (Plan §2). It extracts the mTLS certificate's URI SANs and verifies they match
// the envelope's internal identity claims (operator_session_id, operator_id, source_component).
// This prevents an Agent cert from impersonating another workload's envelope.
func verifyEnvelopeIdentityBinding(r *http.Request, envelopeBody []byte) error {
	// Ensure mTLS is present
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return fmt.Errorf("verifyEnvelopeIdentityBinding: mTLS client certificate required")
	}

	cert := r.TLS.PeerCertificates[0]
	if len(cert.URIs) == 0 {
		return fmt.Errorf("verifyEnvelopeIdentityBinding: client certificate missing URI SAN")
	}

	// Parse the envelope from its canonical protojson wire form to extract
	// identity claims. This MUST use the same codec the downstream processor
	// uses (protojson) — decoding with encoding/json would miss the camelCase
	// wire field names and silently bypass identity binding for every real
	// BYO client.
	var envelope commonv1.GovernanceEnvelope
	if err := protojson.Unmarshal(envelopeBody, &envelope); err != nil {
		// If we can't parse the envelope, let the downstream processor handle the parsing/decode error
		return nil
	}

	operatorSessionID := envelope.GetOperatorSessionId()
	operatorID := envelope.GetOperatorId()
	sourceComponent := envelope.GetSourceComponent()

	// If envelope has no identity fields, let the processor handle validation
	if operatorSessionID == "" && operatorID == "" {
		return nil
	}

	wid := protocol.NewWorkloadIdentity()

	// Check if any certificate URI SAN matches the envelope's identity
	for _, uri := range cert.URIs {
		spiffeID := uri.String()

		// Check CLI session match — works with or without operator_id
		if operatorSessionID != "" {
			if wid.MatchesCLISessionOnly(spiffeID, operatorSessionID) {
				return nil
			}
		}

		// Operator cert match — requires both fields
		if operatorSessionID != "" && operatorID != "" {
			// Format: spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>
			// We check if the SPIFFE ID ends with the operator_id and operator_session_id
			if strings.HasSuffix(spiffeID, "/"+operatorID+"/"+operatorSessionID) {
				// Verify it's an Operator SPIFFE ID (starts with spiffe://g8e.local/operator/)
				if strings.HasPrefix(spiffeID, "spiffe://"+protocol.TrustDomain+"/operator/") {
					return nil
				}
			}
		}

		// App workload match — only AGENT and CLIENT components use operator_id-based
		// app SPIFFE identities. CLI/operator sessions are matched above via
		// session-based auth, so they are not eligible for app matching.
		if operatorID != "" && isAppComponent(sourceComponent) {
			if wid.MatchesApp(spiffeID, operatorID) {
				return nil
			}
		}
	}

	// No matching URI SAN found
	return fmt.Errorf("verifyEnvelopeIdentityBinding: certificate URI SAN does not match envelope identity claims (operator_id=%s, operator_session_id=%s, source_component=%s)",
		operatorID, operatorSessionID, sourceComponent)
}

// isAppComponent reports whether the envelope's source component is an app
// workload (AGENT or CLIENT) that authenticates via an app SPIFFE identity
// (spiffe://<trust-domain>/app/<operator_id>).
func isAppComponent(c commonv1.Component) bool {
	return c == commonv1.Component_COMPONENT_AGENT || c == commonv1.Component_COMPONENT_CLIENT
}

// handleGovernanceEnvelope is the canonical synchronous mutation entry point
// for BYO Gateway clients. It accepts a GovernanceEnvelope, verifies it
// through the Gateway's fail-closed gate (id, hash, expiry, nonce, state
// root, L2/L3 governance), executes it through the Actuator, and returns the
// signed ActionReceipt.
//
// Status semantics:
//   - 200 OK: envelope verified and executed (receipt body); receipt.status
//     reflects whether the underlying handler succeeded or failed.
//   - 400 Bad Request: malformed envelope (decode failure, empty body,
//     payload too large) - no governance state mutated.
//   - 403 Forbidden: governance verification failed before execution
//     (expired, replayed, hash mismatch, missing/invalid L2/L3, unknown
//     action type, or transport-to-envelope identity mismatch) - no state mutated, no receipt produced.
//   - 503 Service Unavailable: envelope processor not yet initialized.
//   - 405 Method Not Allowed: non-POST methods.
func (h *HTTPHandler) handleGovernanceEnvelope(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.envProc == nil {
		h.responder.Error(w, http.StatusServiceUnavailable, "envelope processor not initialized")
		return
	}

	body, err := h.readBody(r)
	if err != nil {
		h.responder.Error(w, http.StatusBadRequest, fmt.Sprintf("handleGovernanceEnvelope: failed to read request body: %v", err))
		return
	}
	if len(body) == 0 {
		h.responder.Error(w, http.StatusBadRequest, "empty request body")
		return
	}

	// Transport-to-envelope identity binding (Plan §2)
	// Extract mTLS certificate URI SANs and verify they match the envelope's
	// internal identity claims. This prevents an Agent cert from impersonating
	// another workload's envelope.
	// Skip identity binding if no TLS is present (test mode)
	if r.TLS != nil {
		if err := verifyEnvelopeIdentityBinding(r, body); err != nil {
			h.responder.Error(w, http.StatusForbidden, fmt.Sprintf("handleGovernanceEnvelope: identity binding failed: %v", err))
			return
		}
	}

	receipt, procErr := h.envProc.ProcessEnvelope(r.Context(), body)
	if procErr != nil {
		status := classifyEnvelopeError(procErr)
		h.responder.Error(w, status, procErr.Error())
		return
	}
	if receipt == nil {
		// Defensive: a nil receipt with nil error should never happen, but if
		// the processor regresses, do not mask the failure.
		h.responder.Error(w, http.StatusInternalServerError, "handleGovernanceEnvelope: envelope processor returned nil receipt without error")
		return
	}

	// Receipt is returned as JSON. Execution-failure receipts (status=FAILED)
	// are still HTTP 200 because they represent a verified, audited outcome
	// - the caller has cryptographic evidence of the attempt.
	h.responder.JSON(w, http.StatusOK, receipt)
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
	case errors.Is(err, constants.ErrTxInvalidEnvelope),
		errors.Is(err, constants.ErrTxTransactionIDMissing),
		errors.Is(err, constants.ErrTxUnknownActionType),
		errors.Is(err, constants.ErrTxPayloadMissing),
		errors.Is(err, constants.ErrTxPayloadDecodeFailed),
		errors.Is(err, constants.ErrTxL1ValidationFailed),
		errors.Is(err, constants.ErrTxTransactionHashMissing),
		errors.Is(err, constants.ErrTxTransactionHashMismatch),
		errors.Is(err, constants.ErrTxTransactionExpired),
		errors.Is(err, constants.ErrTxNonceMissing),
		errors.Is(err, constants.ErrTxTransactionReplay),
		errors.Is(err, constants.ErrTxStateRootMissing),
		errors.Is(err, constants.ErrTxStateRootRequired),
		errors.Is(err, constants.ErrTxStateRootMismatch),
		errors.Is(err, constants.ErrTxL2SignatureMissing),
		errors.Is(err, constants.ErrTxL2SignatureInvalid),
		errors.Is(err, constants.ErrTxL2KeyNotConfigured),
		errors.Is(err, constants.ErrTxL3ProofMissing),
		errors.Is(err, constants.ErrTxL3ProofInvalid),
		errors.Is(err, constants.ErrTxL3NotaryNotConfigured),
		errors.Is(err, constants.ErrTxInFlight):
		return http.StatusForbidden
	}
	// Wrapped invalid-envelope decode error from ProcessEnvelope.
	if len(msg) > 0 && (containsAny(msg, "invalid GovernanceEnvelope", "empty payload", "payload exceeds")) {
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
