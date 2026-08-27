// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/consensus"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/protocol"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// GovernanceController handles governance envelope submission and consensus
// deliberation endpoints. Consensus is wired directly as *consensus.ConsensusService.
// Under doctrine posture, the consensus route is not registered on the router
// (returning 404), ensuring handleConsensusDeliberate is only called when consensus
// is non-nil. EnvProc is injected at construction time; a nil value means envelope
// submission is not enabled.
type GovernanceController struct {
	cfg       *config.Config
	logger    *slog.Logger
	responder *response.Writer
	consensus *consensus.ConsensusService
	envProc   governance.EnvelopeProcessor
}

// GovernanceControllerDeps groups all dependencies for GovernanceController.
// Consensus is a *consensus.ConsensusService (nil only when the gateway posture
// is doctrine, where the deliberate route is not registered).
// EnvProc may be nil if envelope submission is not enabled.
type GovernanceControllerDeps struct {
	Cfg       *config.Config
	Logger    *slog.Logger
	Responder *response.Writer
	Consensus *consensus.ConsensusService
	EnvProc   governance.EnvelopeProcessor
}

func newGovernanceController(d GovernanceControllerDeps) *GovernanceController {
	return &GovernanceController{
		cfg:       d.Cfg,
		logger:    d.Logger,
		responder: d.Responder,
		consensus: d.Consensus,
		envProc:   d.EnvProc,
	}
}

// handleConsensusDeliberate is the HTTP handler for the consensus deliberate endpoint.
func (c *GovernanceController) handleConsensusDeliberate(w http.ResponseWriter, r *http.Request) {
	c.consensus.HandleDeliberate(w, r)
}

// verifyEnvelopeIdentityBinding enforces transport-to-envelope identity binding
// (Plan §2). It extracts the mTLS certificate's URI SANs and verifies they match
// the envelope's internal identity claims (operator_session_id, operator_id, source_component).
// This prevents an Agent cert from impersonating another workload's envelope.
func verifyEnvelopeIdentityBinding(r *http.Request, envelopeBody []byte) error {
	// Ensure mTLS is present
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return constants.ErrMTLSRequired
	}

	cert := r.TLS.PeerCertificates[0]
	if len(cert.URIs) == 0 {
		return constants.ErrCertURISANMissing
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
	actionType := constants.ActionType(envelope.GetActionType())

	// Fail closed for governed mutations: an envelope that performs a
	// mutation MUST bind to a transport identity. An empty operator_id AND
	// operator_session_id on a mutation means the envelope is unbound and
	// must be rejected at the transport boundary rather than deferred to
	// the downstream processor, which would otherwise admit it as a
	// fail-open path. Non-mutation reads keep the pass-through behavior so
	// the downstream processor validates them.
	if operatorSessionID == "" && operatorID == "" {
		if actionType.IsMutation() {
			return fmt.Errorf("%w: mutation action %q requires operator_id or operator_session_id binding",
				constants.ErrIdentityBindingFailed, actionType)
		}
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
	return fmt.Errorf("%w (operator_id=%s, operator_session_id=%s, source_component=%s)",
		constants.ErrURISANMismatch, operatorID, operatorSessionID, sourceComponent)
}

// isAppComponent reports whether the envelope's source component is an app
// workload (AGENT or CLIENT) that authenticates via an app SPIFFE identity
// (spiffe://<trust-domain>/app/<operator_id>).
func isAppComponent(c commonv1.Component) bool {
	return c == commonv1.Component_COMPONENT_AGENT || c == commonv1.Component_COMPONENT_CLIENT
}

// injectEnvelopePosture sets the gateway's governance posture on an incoming
// envelope that was built by a client (ensemble, CLI, BYO app). Clients do not
// know the gateway's posture — the gateway is the posture authority. Posture
// is not part of the transaction hash, so injecting it here does not
// invalidate the client's signature or hash. If the envelope already carries a
// non-empty posture, it is left unchanged (gateway-internal construction paths
// set posture at build time).
func injectEnvelopePosture(body []byte, posture string) ([]byte, error) {
	var envelope commonv1.GovernanceEnvelope
	if err := protojson.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	if envelope.Posture != "" {
		return body, nil
	}
	envelope.Posture = posture
	reMarshaled, err := protojson.Marshal(&envelope)
	if err != nil {
		return nil, fmt.Errorf("re-marshal envelope: %w", err)
	}
	return reMarshaled, nil
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
//   - 503 Service Unavailable: envelope processor not configured.
//   - 405 Method Not Allowed: non-POST methods.
//
// @Summary		Submit governance envelope
// @Description	Accepts a GovernanceEnvelope, verifies it through the 5-layer gauntlet (L1–L3 pre-flight,
// @Description	L4 warden, L5 actuator), and returns the signed ActionReceipt. Transport-to-envelope
// @Description	identity binding is enforced via mTLS certificate URI SAN matching.
// @Tags			governance
// @Accept			json
// @Produce		json
// @Param			envelope	body		[]byte		true	"GovernanceEnvelope (protojson)"
// @Success		200			{object}	json.RawMessage	"ActionReceipt"
// @Failure		400			{string}	string			"Bad Request — malformed envelope"
// @Failure		403			{string}	string			"Forbidden — governance verification failed"
// @Failure		405			{string}	string			"Method Not Allowed"
// @Failure		503			{string}	string			"Service Unavailable — processor not configured"
// @Router			/api/v1/governance/envelopes [post]
func (c *GovernanceController) handleGovernanceEnvelope(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}
	if c.envProc == nil {
		c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrEnvelopeProcessorNotInit.Error())
		return
	}
	proc := c.envProc

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("handleGovernanceEnvelope: failed to read request body: %w", err).Error())
		return
	}
	if len(body) == 0 {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	// Transport-to-envelope identity binding (Plan §2)
	// Extract mTLS certificate URI SANs and verify they match the envelope's
	// internal identity claims. This prevents an Agent cert from impersonating
	// another workload's envelope.
	// Skip identity binding if no TLS is present (test mode)
	if r.TLS != nil {
		if err := verifyEnvelopeIdentityBinding(r, body); err != nil {
			c.responder.Error(w, http.StatusForbidden, fmt.Errorf("handleGovernanceEnvelope: identity binding: %w", err).Error())
			return
		}
	}

	// Inject the gateway's governance posture into the envelope. The gateway
	// is the posture authority; clients (ensemble, CLI, BYO apps) do not know
	// the gateway's posture and must not set it. Posture is not part of the
	// transaction hash, so injecting it here does not invalidate the client's
	// signature. If the envelope already carries a posture (e.g. from a
	// gateway-internal construction path), leave it unchanged.
	body, err = injectEnvelopePosture(body, string(c.cfg.Gateway.Posture))
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, fmt.Errorf("handleGovernanceEnvelope: inject posture: %w", err).Error())
		return
	}

	receipt, procErr := proc.ProcessEnvelope(r.Context(), body)
	if procErr != nil {
		status := classifyEnvelopeError(procErr)
		c.responder.Error(w, status, procErr.Error())
		return
	}
	if receipt == nil {
		// Defensive: a nil receipt with nil error should never happen, but if
		// the processor regresses, do not mask the failure.
		c.responder.Error(w, http.StatusInternalServerError, constants.ErrNilReceipt.Error())
		return
	}

	// Receipt is returned as JSON. Execution-failure receipts (status=FAILED)
	// are still HTTP 200 because they represent a verified, audited outcome
	// - the caller has cryptographic evidence of the attempt.
	c.responder.JSON(w, http.StatusOK, receipt)
}

// classifyEnvelopeError maps a governance verification error to an HTTP status.
// Caller-side bad input (decode failures, empty/oversized payloads) maps to 400;
// all governance sentinel errors are 403 (caller-side proof failures);
// anything else is 500.
func classifyEnvelopeError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	switch {
	case errors.Is(err, constants.ErrPubSubEmptyPayload),
		errors.Is(err, constants.ErrPayloadExceedsLimit),
		errors.Is(err, constants.ErrTxInvalidEnvelope):
		return http.StatusBadRequest
	case errors.Is(err, constants.ErrTxTransactionIDMissing),
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
		errors.Is(err, constants.ErrTxL2ConsensusNotConfigured),
		errors.Is(err, constants.ErrTxL2QuorumNotMet),
		errors.Is(err, constants.ErrTxL2DuplicateSigner),
		errors.Is(err, constants.ErrTxL3ProofMissing),
		errors.Is(err, constants.ErrTxL3ProofInvalid),
		errors.Is(err, constants.ErrTxL3NotaryNotConfigured),
		errors.Is(err, constants.ErrTxInFlight):
		return http.StatusForbidden
	}
	return http.StatusInternalServerError
}
