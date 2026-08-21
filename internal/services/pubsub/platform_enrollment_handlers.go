// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/uuid"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// PlatformEnrollmentHandler dispatches the five platform enrollment
// governance actions through the canonical L4/L5 gauntlet. Each handler
// decodes the PlatformEnrollmentGovernancePayload from the envelope
// payload, performs the typed mutation against the document store (or
// delegates to a session service), and returns a receipt summary string
// that the L5Actuator stamps into the signed final receipt.
//
// Receipt summaries contain only public identifiers and attribution
// metadata: request ID, component kind, instance ID, fingerprints,
// decision, actor user ID, and resulting document/session IDs. CSR PEM,
// token hashes, and private keys never appear in summaries or audit
// records.
type PlatformEnrollmentHandler struct {
	deps   PlatformEnrollmentDeps
	logger platformEnrollmentLogger
}

// newPlatformEnrollmentHandler constructs the handler from the gateway-side
// dependency bundle. The bundle is required in gateway mode; outbound
// (operator) mode never constructs this handler.
func newPlatformEnrollmentHandler(deps PlatformEnrollmentDeps, logger platformEnrollmentLogger) *PlatformEnrollmentHandler {
	return &PlatformEnrollmentHandler{deps: deps, logger: logger}
}

// decodePayload unmarshals the binary protobuf envelope payload into the
// typed PlatformEnrollmentGovernancePayload. The payload is binary proto
// (the L4Warden decodes the same way); protojson is only used at the
// transport boundary.
func (h *PlatformEnrollmentHandler) decodePayload(msg *PubSubCommandMessage) (*commonv1.PlatformEnrollmentGovernancePayload, error) {
	req, err := unmarshalPayload(msg.EventType, msg.Payload)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: decode payload: %w", err)
	}
	payload, ok := req.(*commonv1.PlatformEnrollmentGovernancePayload)
	if !ok {
		return nil, fmt.Errorf("platform enrollment: unexpected payload type %T: %w", req, constants.ErrTxPayloadActionMismatch)
	}
	return payload, nil
}

// protoComponentKind maps the proto enum back to the typed domain kind.
func protoComponentKind(k commonv1.PlatformComponentKind) (models.PlatformComponentKind, error) {
	switch k {
	case commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD:
		return models.PlatformComponentDashboard, nil
	case commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_ENSEMBLE:
		return models.PlatformComponentEnsemble, nil
	case commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR:
		return models.PlatformComponentOperator, nil
	default:
		return "", constants.ErrPlatformEnrollmentInvalidComponent
	}
}

// protoDecision maps the proto enum back to the typed domain decision.
func protoDecision(d commonv1.PlatformEnrollmentDecision) (models.PlatformEnrollmentDecision, error) {
	switch d {
	case commonv1.PlatformEnrollmentDecision_PLATFORM_ENROLLMENT_DECISION_APPROVE:
		return models.PlatformEnrollmentDecisionApprove, nil
	case commonv1.PlatformEnrollmentDecision_PLATFORM_ENROLLMENT_DECISION_DENY:
		return models.PlatformEnrollmentDecisionDeny, nil
	default:
		return "", constants.ErrPlatformEnrollmentInvalidDecision
	}
}

// HandleCreate is the audit-only CREATE handler. The enrollment service
// writes the pending request document directly via DocSet before
// submitting this envelope (CREATE is classified as non-mutation in
// IsMutation, so invariant 17 does not apply to that initial write).
// This handler decodes the payload, returns a receipt summary, and
// writes nothing to the doc store.
func (h *PlatformEnrollmentHandler) HandleCreate(ctx context.Context, msg *PubSubCommandMessage) (string, error) {
	_ = ctx
	payload, err := h.decodePayload(msg)
	if err != nil {
		return "", err
	}
	kind, err := protoComponentKind(payload.GetComponentKind())
	if err != nil {
		return "", err
	}
	fp := fingerprintsFromPayload(payload.GetFingerprints())
	h.logger.Info("platform enrollment create audited",
		"request_id", payload.GetRequestId(),
		"component_kind", string(kind),
		"instance_id", payload.GetInstanceId())
	return fmt.Sprintf("platform enrollment create request_id=%s component=%s instance=%s fingerprints=%s",
		payload.GetRequestId(), string(kind), payload.GetInstanceId(), fingerprintsSummary(fp)), nil
}

// HandleDecide loads the request, verifies it is in the pending state,
// performs a conditional pending -> approved|denied transition, and
// stamps approved_by_user_id, decided_at, decision_envelope_id, and
// decision_receipt_id (from msg.ID). The conditional update ensures a
// concurrent or repeated decision does not consume approval.
func (h *PlatformEnrollmentHandler) HandleDecide(ctx context.Context, msg *PubSubCommandMessage) (string, error) {
	_ = ctx
	payload, err := h.decodePayload(msg)
	if err != nil {
		return "", err
	}
	decision, err := protoDecision(payload.GetDecision())
	if err != nil {
		return "", err
	}
	requestID := payload.GetRequestId()
	if requestID == "" {
		return "", constants.ErrPlatformEnrollmentRequestIDRequired
	}
	actorUserID := payload.GetActorUserId()
	if actorUserID == "" {
		return "", constants.ErrPlatformEnrollmentInvalidDecision
	}

	req, err := loadPlatformEnrollmentRequest(context.Background(), h.deps, requestID)
	if err != nil {
		return "", err
	}
	if req == nil {
		return "", constants.ErrPlatformEnrollmentRequestNotFound
	}
	if req.State != models.PlatformEnrollmentStatePending {
		return "", constants.ErrPlatformEnrollmentAlreadyDecided
	}

	var targetState models.PlatformEnrollmentState
	switch decision {
	case models.PlatformEnrollmentDecisionApprove:
		targetState = models.PlatformEnrollmentStateApproved
	case models.PlatformEnrollmentDecisionDeny:
		targetState = models.PlatformEnrollmentStateDenied
	default:
		return "", constants.ErrPlatformEnrollmentInvalidDecision
	}

	now := time.Now().UTC()
	setFields := map[string]interface{}{
		"state":                string(targetState),
		"approved_by_user_id":  actorUserID,
		"decided_at":           now,
		"decision_reason":      "",
		"decision_envelope_id": msg.ID,
		"decision_receipt_id":  msg.ID,
		"last_transition_at":   now,
	}
	applied, err := h.deps.DocStore.DocConditionalUpdate(
		platformEnrollmentCollection(), requestID, setFields, "state", string(models.PlatformEnrollmentStatePending),
	)
	if err != nil {
		return "", fmt.Errorf("platform enrollment: decide %s: %w", requestID, err)
	}
	if !applied {
		return "", constants.ErrPlatformEnrollmentAlreadyDecided
	}

	h.logger.Info("platform enrollment decision recorded",
		"request_id", requestID,
		"decision", string(decision),
		"actor_user_id", actorUserID)
	return fmt.Sprintf("platform enrollment decide request_id=%s decision=%s actor=%s",
		requestID, string(decision), actorUserID), nil
}

// HandleIssue loads the request, verifies it is in the issuing state
// (the enrollment service acquires the issuance lease by transitioning
// approved -> issuing before submitting this envelope), reads the CSR
// PEM from the stored document, calls the PKI signer, stores the issued
// cert material and generated IDs, and performs the issuing -> completed
// transition. The envelope/receipt ID (msg.ID) is stamped as
// issuance_envelope_id and issuance_receipt_id. Operator issuance signs
// both the operator and CLI CSRs and persists the operator document;
// app issuance uses SignPlatformAppCSR for the dual-SAN app certificate.
func (h *PlatformEnrollmentHandler) HandleIssue(ctx context.Context, msg *PubSubCommandMessage) (string, error) {
	_ = ctx
	payload, err := h.decodePayload(msg)
	if err != nil {
		return "", err
	}
	kind, err := protoComponentKind(payload.GetComponentKind())
	if err != nil {
		return "", err
	}
	requestID := payload.GetRequestId()
	if requestID == "" {
		return "", constants.ErrPlatformEnrollmentRequestIDRequired
	}
	actorUserID := payload.GetActorUserId()
	if actorUserID == "" {
		return "", constants.ErrPlatformEnrollmentInvalidDecision
	}

	req, err := loadPlatformEnrollmentRequest(context.Background(), h.deps, requestID)
	if err != nil {
		return "", err
	}
	if req == nil {
		return "", constants.ErrPlatformEnrollmentRequestNotFound
	}
	// The enrollment service acquires the issuance lease (approved ->
	// issuing) before submitting the ISSUE envelope. The handler starts
	// from the issuing state and performs the signing + issuing ->
	// completed transition. A request still in the approved state means
	// the lease was never acquired; a request in any other state is a
	// concurrent or stale call.
	if req.State != models.PlatformEnrollmentStateIssuing {
		return "", constants.ErrPlatformEnrollmentIssuanceInProgress
	}

	issued, operatorID, operatorSessionID, cliSessionID, certSerial, certFingerprint, err := h.signComponent(req, actorUserID)
	if err != nil {
		// Roll back issuing -> approved so a retry can re-acquire. A
		// failed signing must not permanently consume approval.
		_, rollbackErr := h.deps.DocStore.DocConditionalUpdate(
			platformEnrollmentCollection(), requestID,
			map[string]interface{}{
				"state":              string(models.PlatformEnrollmentStateApproved),
				"last_transition_at": time.Now().UTC(),
			},
			"state", string(models.PlatformEnrollmentStateIssuing),
		)
		if rollbackErr != nil {
			h.logger.Error("platform enrollment: rollback issuing failed",
				"request_id", requestID, "error", rollbackErr)
		}
		return "", fmt.Errorf("platform enrollment: issue %s: %w", requestID, err)
	}

	// Persist the issued response, generated IDs, cert metadata, and
	// transition issuing -> completed in a single conditional update.
	completionFields := map[string]interface{}{
		"state":                   string(models.PlatformEnrollmentStateCompleted),
		"issued":                  issued,
		"completed_at":            time.Now().UTC(),
		"last_transition_at":      time.Now().UTC(),
		"issuance_envelope_id":    msg.ID,
		"issuance_receipt_id":     msg.ID,
		"certificate_serial":      certSerial,
		"certificate_fingerprint": certFingerprint,
	}
	if operatorID != "" {
		completionFields["operator_id"] = operatorID
	}
	if operatorSessionID != "" {
		completionFields["operator_session_id"] = operatorSessionID
	}
	if cliSessionID != "" {
		completionFields["cli_session_id"] = cliSessionID
	}

	applied, err := h.deps.DocStore.DocConditionalUpdate(
		platformEnrollmentCollection(), requestID, completionFields,
		"state", string(models.PlatformEnrollmentStateIssuing),
	)
	if err != nil {
		return "", fmt.Errorf("platform enrollment: complete issuance %s: %w", requestID, err)
	}
	if !applied {
		return "", constants.ErrPlatformEnrollmentIssuanceInProgress
	}

	h.logger.Info("platform enrollment issued",
		"request_id", requestID,
		"component_kind", string(kind),
		"operator_id", operatorID,
		"cert_fingerprint", certFingerprint)
	summary := fmt.Sprintf("platform enrollment issue request_id=%s component=%s cert_fingerprint=%s",
		requestID, string(kind), certFingerprint)
	if operatorID != "" {
		summary += fmt.Sprintf(" operator_id=%s", operatorID)
	}
	return summary, nil
}

// signComponent performs the component-specific signing and returns the
// issued completion response plus the generated IDs, cert serial, and
// cert fingerprint. Operator issuance signs both CSRs and persists the
// operator document; app issuance uses SignPlatformAppCSR for the
// dual-SAN certificate.
func (h *PlatformEnrollmentHandler) signComponent(req *models.PlatformEnrollmentRequest, actorUserID string) (*models.PlatformEnrollmentCompleteResponse, string, string, string, string, string, error) {
	trustBundle, err := h.deps.PKI.GatewayTrustBundle()
	if err != nil {
		return nil, "", "", "", "", "", fmt.Errorf("trust bundle: %w", err)
	}

	switch req.ComponentKind {
	case models.PlatformComponentDashboard, models.PlatformComponentEnsemble:
		return h.signAppComponent(req, actorUserID, trustBundle)
	case models.PlatformComponentOperator:
		return h.signOperatorComponent(req, actorUserID, trustBundle)
	default:
		return nil, "", "", "", "", "", constants.ErrPlatformEnrollmentInvalidComponent
	}
}

// signAppComponent signs the dashboard/ensemble app CSR with the
// dual-SAN SignPlatformAppCSR signer and builds the app credentials
// response. The app certificate carries the canonical app SPIFFE URI
// plus the approving user's SPIFFE URI.
func (h *PlatformEnrollmentHandler) signAppComponent(req *models.PlatformEnrollmentRequest, actorUserID string, trustBundle []byte) (*models.PlatformEnrollmentCompleteResponse, string, string, string, string, string, error) {
	if req.App == nil {
		return nil, "", "", "", "", "", constants.ErrPlatformEnrollmentInvalidPayload
	}
	certPEM, chainPEM, err := h.deps.PKI.SignPlatformAppCSR(req.App.CSRPEM, req.ComponentName, actorUserID)
	if err != nil {
		return nil, "", "", "", "", "", fmt.Errorf("sign app csr: %w", err)
	}
	certFingerprint := fingerprintFromPEM(certPEM)
	certSerial := serialFromPEM(certPEM)
	expiresAt := expiryFromPEM(certPEM)

	creds := &models.PlatformEnrollmentAppCredentials{
		AppID:       req.ComponentName,
		AppCert:     certPEM,
		CertChain:   chainPEM,
		TrustBundle: string(trustBundle),
		ExpiresAt:   expiresAt,
	}
	resp := &models.PlatformEnrollmentCompleteResponse{
		RequestID:     req.ID,
		ComponentKind: req.ComponentKind,
		App:           creds,
	}
	return resp, "", "", "", certSerial, certFingerprint, nil
}

// signOperatorComponent signs both the operator and CLI CSRs, persists
// the operator document stamped with the approving owner's user_id, and
// builds the operator credentials response. The operator document is
// user-owned (user_id = actorUserID) so the approving owner can discover
// and manage it through ListUserOperators. is_slot remains false:
// platform-enrolled operators are not user-created slots, but they are
// user-owned.
func (h *PlatformEnrollmentHandler) signOperatorComponent(req *models.PlatformEnrollmentRequest, actorUserID string, trustBundle []byte) (*models.PlatformEnrollmentCompleteResponse, string, string, string, string, string, error) {
	if req.Operator == nil {
		return nil, "", "", "", "", "", constants.ErrPlatformEnrollmentInvalidPayload
	}
	operatorID := uuid.NewString()
	operatorSessionID := uuid.NewString()
	cliSessionID := uuid.NewString()
	now := time.Now().UTC()

	operatorCertPEM, operatorChainPEM, err := h.deps.PKI.SignCSR(
		req.Operator.OperatorCSRPEM, constants.LeafTypeOperator, "", operatorID, "", operatorSessionID, "",
	)
	if err != nil {
		return nil, "", "", "", "", "", fmt.Errorf("sign operator csr: %w", err)
	}
	cliCertPEM, cliCertChainPEM, err := h.deps.PKI.SignCSR(
		req.Operator.CLICSRPEM, constants.LeafTypeCLI, "", "", "", cliSessionID, "",
	)
	if err != nil {
		return nil, "", "", "", "", "", fmt.Errorf("sign cli csr: %w", err)
	}
	cliCertFingerprint := fingerprintFromPEM(cliCertPEM)
	cliCertSerial := serialFromPEM(cliCertPEM)

	// Persist the operator document idempotently. The operator identity
	// is certificate-based; user_id is the approving owner so the owner
	// can discover and manage the platform-enrolled operator.
	operatorDoc := &models.OperatorDocumentGo{
		ID:                operatorID,
		UserID:            actorUserID,
		Component:         constants.ComponentNameG8EO,
		Name:              req.Hostname,
		Status:            constants.OperatorStatusActive,
		OperatorSessionID: operatorSessionID,
		OperatorType:      constants.OperatorTypeSystem,
		SystemFingerprint: req.SystemFingerprint,
		Claimed:           true,
		ClaimedAt:         &now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	opBytes, err := json.Marshal(operatorDoc)
	if err != nil {
		return nil, "", "", "", "", "", fmt.Errorf("marshal operator doc: %w", err)
	}
	if err := h.deps.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes,
	); err != nil {
		return nil, "", "", "", "", "", fmt.Errorf("persist operator doc: %w", err)
	}

	creds := &models.PlatformEnrollmentOperatorCredentials{
		OperatorCert:      operatorCertPEM,
		OperatorCertChain: operatorChainPEM,
		HubTrustBundle:    string(trustBundle),
		OperatorSessionID: operatorSessionID,
		OperatorID:        operatorID,
		CLISessionID:      cliSessionID,
		CLICert:           cliCertPEM,
		CLICertChain:      cliCertChainPEM,
		Posture:           h.deps.Posture,
	}
	resp := &models.PlatformEnrollmentCompleteResponse{
		RequestID:     req.ID,
		ComponentKind: req.ComponentKind,
		Operator:      creds,
	}
	return resp, operatorID, operatorSessionID, cliSessionID, cliCertSerial, cliCertFingerprint, nil
}

// HandlePersistPolicy writes the app policy document via DocSet at the
// target_collection/target_document_id carried in the payload. The
// ownership and cert metadata come from the payload (populated by the
// enrollment service from the ISSUE handler outputs). This handler is
// only invoked for dashboard and ensemble components.
func (h *PlatformEnrollmentHandler) HandlePersistPolicy(ctx context.Context, msg *PubSubCommandMessage) (string, error) {
	_ = ctx
	payload, err := h.decodePayload(msg)
	if err != nil {
		return "", err
	}
	requestID := payload.GetRequestId()
	if requestID == "" {
		return "", constants.ErrPlatformEnrollmentRequestIDRequired
	}
	targetCollection := payload.GetTargetCollection()
	targetDocumentID := payload.GetTargetDocumentId()
	if targetCollection == "" || targetDocumentID == "" {
		return "", constants.ErrPlatformEnrollmentInvalidPayload
	}
	ownerUserID := payload.GetOwnerUserId()
	if ownerUserID == "" {
		return "", constants.ErrPlatformEnrollmentInvalidPayload
	}

	now := time.Now().UTC()
	policy := models.AppPolicy{
		AppID:                  targetDocumentID,
		OwnerUserID:            ownerUserID,
		ApprovedByUserID:       payload.GetActorUserId(),
		EnrollmentRequestID:    requestID,
		CertificateSerial:      payload.GetCertificateSerial(),
		CertificateFingerprint: payload.GetCertificateFingerprint(),
		AllowedCollections:     nil,
		RateLimitRPS:           0,
		MaxPayloadBytes:        0,
		RequireL3Approval:      false,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("platform enrollment: marshal policy: %w", err)
	}
	if err := h.deps.DocStore.DocSet(targetCollection, targetDocumentID, data); err != nil {
		return "", fmt.Errorf("platform enrollment: persist policy: %w", err)
	}

	policyID := payload.GetPolicyId()
	h.logger.Info("platform enrollment policy persisted",
		"request_id", requestID,
		"policy_id", policyID,
		"target_document_id", targetDocumentID)
	return fmt.Sprintf("platform enrollment persist_policy request_id=%s policy_id=%s document=%s",
		requestID, policyID, targetDocumentID), nil
}

// HandleCreateSession delegates to CLISessionService and
// OperatorSessionService using the session IDs carried in the payload.
// The handler writes nothing to the doc store directly; the session
// services own their persistence. This handler is only invoked for the
// operator component (dashboard and ensemble have no sessions). Both
// sessions are bound to the approving owner's user_id (the actor) so the
// owner can discover and manage the platform-enrolled operator.
func (h *PlatformEnrollmentHandler) HandleCreateSession(ctx context.Context, msg *PubSubCommandMessage) (string, error) {
	_ = ctx
	payload, err := h.decodePayload(msg)
	if err != nil {
		return "", err
	}
	requestID := payload.GetRequestId()
	if requestID == "" {
		return "", constants.ErrPlatformEnrollmentRequestIDRequired
	}
	operatorID := payload.GetOperatorId()
	operatorSessionID := payload.GetOperatorSessionId()
	cliSessionID := payload.GetCliSessionId()
	if operatorID == "" || operatorSessionID == "" || cliSessionID == "" {
		return "", constants.ErrPlatformEnrollmentInvalidPayload
	}
	actorUserID := payload.GetActorUserId()
	if actorUserID == "" {
		return "", constants.ErrPlatformEnrollmentInvalidDecision
	}

	// Persist the CLI session bound to the approving owner's user_id.
	// The cert fingerprint/serial come from the payload (populated by
	// the enrollment service from the ISSUE handler outputs).
	if err := h.deps.CLISessions.PersistCLISession(
		cliSessionID, operatorSessionID, actorUserID,
		"", payload.GetCertificateFingerprint(), payload.GetCertificateSerial(),
		string(constants.HeartbeatTypeBootstrap),
	); err != nil {
		return "", fmt.Errorf("platform enrollment: persist cli session: %w", err)
	}
	if err := h.deps.OperatorSessions.PersistOperatorSession(
		operatorSessionID, actorUserID, "", operatorID,
		string(constants.HeartbeatTypeBootstrap),
	); err != nil {
		return "", fmt.Errorf("platform enrollment: persist operator session: %w", err)
	}

	h.logger.Info("platform enrollment sessions created",
		"request_id", requestID,
		"operator_id", operatorID,
		"operator_session_id", operatorSessionID,
		"cli_session_id", cliSessionID,
		"actor_user_id", actorUserID)
	return fmt.Sprintf("platform enrollment create_session request_id=%s operator_id=%s operator_session=%s cli_session=%s actor=%s",
		requestID, operatorID, operatorSessionID, cliSessionID, actorUserID), nil
}

// fingerprintsFromPayload converts the proto fingerprints message into
// the typed domain struct.
func fingerprintsFromPayload(fp *commonv1.PlatformEnrollmentFingerprints) models.PlatformEnrollmentCSRFingerprints {
	if fp == nil {
		return models.PlatformEnrollmentCSRFingerprints{}
	}
	return models.PlatformEnrollmentCSRFingerprints{
		App:      fp.GetApp(),
		Operator: fp.GetOperator(),
		CLI:      fp.GetCli(),
	}
}

func fingerprintsSummary(f models.PlatformEnrollmentCSRFingerprints) string {
	parts := []string{}
	if f.App != "" {
		parts = append(parts, "app="+f.App)
	}
	if f.Operator != "" {
		parts = append(parts, "operator="+f.Operator)
	}
	if f.CLI != "" {
		parts = append(parts, "cli="+f.CLI)
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ","))
}

// fingerprintFromPEM computes the SHA-256 fingerprint of the DER-encoded
// certificate inside the PEM block. Returns an empty string on decode
// failure; callers treat an empty fingerprint as a non-fatal warning.
func fingerprintFromPEM(certPEM string) string {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// serialFromPEM extracts the certificate serial number as a decimal
// string. Returns an empty string on decode failure.
func serialFromPEM(certPEM string) string {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	return cert.SerialNumber.String()
}

// expiryFromPEM extracts the NotAfter timestamp from the PEM-encoded
// certificate. Returns the zero time on decode failure.
func expiryFromPEM(certPEM string) time.Time {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return time.Time{}
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}
	}
	return cert.NotAfter
}
