// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

type PlatformComponentKind string

type PlatformEnrollmentState string

type PlatformEnrollmentDecision string

const (
	PlatformComponentDashboard PlatformComponentKind = "dashboard"
	PlatformComponentEnsemble  PlatformComponentKind = "ensemble"
	PlatformComponentOperator  PlatformComponentKind = "operator"

	PlatformEnrollmentStatePending   PlatformEnrollmentState = "pending"
	PlatformEnrollmentStateApproved  PlatformEnrollmentState = "approved"
	PlatformEnrollmentStateIssuing   PlatformEnrollmentState = "issuing"
	PlatformEnrollmentStateCompleted PlatformEnrollmentState = "completed"
	PlatformEnrollmentStateDenied    PlatformEnrollmentState = "denied"
	PlatformEnrollmentStateExpired   PlatformEnrollmentState = "expired"

	PlatformEnrollmentDecisionApprove PlatformEnrollmentDecision = "approve"
	PlatformEnrollmentDecisionDeny    PlatformEnrollmentDecision = "deny"

	PlatformDashboardName = "g8ed"
	PlatformEnsembleName  = "g8ee"
	PlatformOperatorName  = "g8eo"
)

func (k PlatformComponentKind) CanonicalName() (string, error) {
	switch k {
	case PlatformComponentDashboard:
		return PlatformDashboardName, nil
	case PlatformComponentEnsemble:
		return PlatformEnsembleName, nil
	case PlatformComponentOperator:
		return PlatformOperatorName, nil
	default:
		return "", constants.ErrPlatformEnrollmentInvalidComponent
	}
}

func (s PlatformEnrollmentState) IsTerminal() bool {
	return s == PlatformEnrollmentStateCompleted || s == PlatformEnrollmentStateDenied || s == PlatformEnrollmentStateExpired
}

type PlatformEnrollmentCreateRequest struct {
	ComponentKind     PlatformComponentKind       `json:"component_kind"`
	InstanceID        string                      `json:"instance_id"`
	Hostname          string                      `json:"hostname"`
	SystemFingerprint string                      `json:"system_fingerprint,omitempty"`
	App               *PlatformAppCSRPayload      `json:"app,omitempty"`
	Operator          *PlatformOperatorCSRPayload `json:"operator,omitempty"`
}

func (r PlatformEnrollmentCreateRequest) ValidateShape() error {
	if _, err := r.ComponentKind.CanonicalName(); err != nil {
		return err
	}
	if r.InstanceID == "" {
		return constants.ErrPlatformEnrollmentInstanceIDRequired
	}
	if r.Hostname == "" {
		return constants.ErrPlatformEnrollmentHostnameRequired
	}
	if len(r.InstanceID) > constants.PlatformEnrollmentMaxInstanceIDBytes {
		return constants.ErrPlatformEnrollmentInvalidInstanceID
	}
	if len(r.Hostname) > constants.PlatformEnrollmentMaxHostnameBytes {
		return constants.ErrPlatformEnrollmentInvalidHostname
	}
	if r.ComponentKind == PlatformComponentOperator {
		if r.App != nil || r.Operator == nil || r.Operator.OperatorCSRPEM == "" || r.Operator.CLICSRPEM == "" {
			return constants.ErrPlatformEnrollmentInvalidPayload
		}
		if r.SystemFingerprint == "" {
			return constants.ErrPlatformEnrollmentFingerprintRequired
		}
		return nil
	}
	if r.App == nil || r.Operator != nil || r.App.CSRPEM == "" {
		return constants.ErrPlatformEnrollmentInvalidPayload
	}
	return nil
}

type PlatformAppCSRPayload struct {
	CSRPEM string `json:"csr_pem"`
}

type PlatformOperatorCSRPayload struct {
	OperatorCSRPEM string `json:"operator_csr_pem"`
	CLICSRPEM      string `json:"cli_csr_pem"`
}

type PlatformEnrollmentCSRFingerprints struct {
	App      string `json:"app,omitempty"`
	Operator string `json:"operator,omitempty"`
	CLI      string `json:"cli,omitempty"`
}

type PlatformEnrollmentCreateResponse struct {
	RequestID     string                            `json:"request_id"`
	Token         string                            `json:"token"`
	ComponentKind PlatformComponentKind             `json:"component_kind"`
	ComponentName string                            `json:"component_name"`
	Fingerprints  PlatformEnrollmentCSRFingerprints `json:"fingerprints"`
	ApprovalURL   string                            `json:"approval_url"`
	ExpiresAt     time.Time                         `json:"expires_at"`
}

type PlatformEnrollmentStatusResponse struct {
	RequestID     string                  `json:"request_id"`
	ComponentKind PlatformComponentKind   `json:"component_kind"`
	State         PlatformEnrollmentState `json:"state"`
	ExpiresAt     time.Time               `json:"expires_at"`
	RetryAfter    int                     `json:"retry_after_seconds,omitempty"`
	FailureReason string                  `json:"failure_reason,omitempty"`
}

type PlatformEnrollmentPendingRequest struct {
	RequestID         string                            `json:"request_id"`
	ComponentKind     PlatformComponentKind             `json:"component_kind"`
	ComponentName     string                            `json:"component_name"`
	InstanceID        string                            `json:"instance_id"`
	Hostname          string                            `json:"hostname"`
	SystemFingerprint string                            `json:"system_fingerprint,omitempty"`
	Fingerprints      PlatformEnrollmentCSRFingerprints `json:"fingerprints"`
	State             PlatformEnrollmentState           `json:"state"`
	CreatedAt         time.Time                         `json:"created_at"`
	ExpiresAt         time.Time                         `json:"expires_at"`
}

type PlatformEnrollmentPendingResponse struct {
	Requests []PlatformEnrollmentPendingRequest `json:"requests"`
}

type PlatformEnrollmentDecisionRequest struct {
	RequestID string                     `json:"request_id"`
	Decision  PlatformEnrollmentDecision `json:"decision"`
	Reason    string                     `json:"reason,omitempty"`
}

func (r PlatformEnrollmentDecisionRequest) Validate() error {
	if r.RequestID == "" {
		return constants.ErrPlatformEnrollmentRequestIDRequired
	}
	if r.Decision != PlatformEnrollmentDecisionApprove && r.Decision != PlatformEnrollmentDecisionDeny {
		return constants.ErrPlatformEnrollmentInvalidDecision
	}
	if len(r.Reason) > constants.PlatformEnrollmentMaxReasonBytes {
		return constants.ErrPlatformEnrollmentReasonTooLong
	}
	return nil
}

type PlatformEnrollmentDecisionResponse struct {
	RequestID string                  `json:"request_id"`
	State     PlatformEnrollmentState `json:"state"`
}

type PlatformEnrollmentProofs struct {
	App      string `json:"app,omitempty"`
	Operator string `json:"operator,omitempty"`
	CLI      string `json:"cli,omitempty"`
}

type PlatformEnrollmentCompleteRequest struct {
	Token  string                   `json:"token"`
	Proofs PlatformEnrollmentProofs `json:"proofs"`
}

type PlatformEnrollmentAppCredentials struct {
	AppID       string    `json:"app_id"`
	AppCert     string    `json:"app_cert"`
	CertChain   string    `json:"cert_chain"`
	TrustBundle string    `json:"trust_bundle"`
	ExpiresAt   time.Time `json:"expires_at"`
	PolicyID    string    `json:"policy_id"`
}

type PlatformEnrollmentOperatorCredentials struct {
	OperatorCert      string `json:"operator_cert"`
	OperatorCertChain string `json:"operator_cert_chain"`
	HubTrustBundle    string `json:"hub_trust_bundle"`
	OperatorSessionID string `json:"operator_session_id"`
	OperatorID        string `json:"operator_id"`
	CLISessionID      string `json:"cli_session_id"`
	CLICert           string `json:"cli_cert"`
	CLICertChain      string `json:"cli_cert_chain"`
	Posture           string `json:"posture,omitempty"`
	ActuatorKeyID     string `json:"actuator_key_id,omitempty"`
	ActuatorPubKey    string `json:"actuator_pub_key,omitempty"`
}

type PlatformEnrollmentCompleteResponse struct {
	RequestID     string                                 `json:"request_id"`
	ComponentKind PlatformComponentKind                  `json:"component_kind"`
	App           *PlatformEnrollmentAppCredentials      `json:"app,omitempty"`
	Operator      *PlatformEnrollmentOperatorCredentials `json:"operator,omitempty"`
}

type PlatformEnrollmentRequest struct {
	ID                     string                              `json:"request_id"`
	TokenHash              string                              `json:"token_hash"`
	ComponentKind          PlatformComponentKind               `json:"component_kind"`
	ComponentName          string                              `json:"component_name"`
	InstanceID             string                              `json:"instance_id"`
	Hostname               string                              `json:"hostname"`
	SystemFingerprint      string                              `json:"system_fingerprint,omitempty"`
	App                    *PlatformAppCSRPayload              `json:"app,omitempty"`
	Operator               *PlatformOperatorCSRPayload         `json:"operator,omitempty"`
	Fingerprints           PlatformEnrollmentCSRFingerprints   `json:"fingerprints"`
	State                  PlatformEnrollmentState             `json:"state"`
	AttemptCount           int                                 `json:"attempt_count"`
	CreatedAt              time.Time                           `json:"created_at"`
	ExpiresAt              time.Time                           `json:"expires_at"`
	LastTransitionAt       time.Time                           `json:"last_transition_at"`
	ApprovedByUserID       string                              `json:"approved_by_user_id,omitempty"`
	DecidedAt              *time.Time                          `json:"decided_at,omitempty"`
	DecisionReason         string                              `json:"decision_reason,omitempty"`
	DecisionEnvelopeID     string                              `json:"decision_envelope_id,omitempty"`
	DecisionReceiptID      string                              `json:"decision_receipt_id,omitempty"`
	IssuanceEnvelopeID     string                              `json:"issuance_envelope_id,omitempty"`
	IssuanceReceiptID      string                              `json:"issuance_receipt_id,omitempty"`
	IssuanceLeaseOwner     string                              `json:"issuance_lease_owner,omitempty"`
	IssuanceLeaseExpiresAt *time.Time                          `json:"issuance_lease_expires_at,omitempty"`
	OperatorID             string                              `json:"operator_id,omitempty"`
	OperatorSessionID      string                              `json:"operator_session_id,omitempty"`
	CLISessionID           string                              `json:"cli_session_id,omitempty"`
	PolicyID               string                              `json:"policy_id,omitempty"`
	Issued                 *PlatformEnrollmentCompleteResponse `json:"issued,omitempty"`
	CompletedAt            *time.Time                          `json:"completed_at,omitempty"`
	FailureReason          string                              `json:"failure_reason,omitempty"`
}

func (r PlatformEnrollmentRequest) PendingMetadata() PlatformEnrollmentPendingRequest {
	return PlatformEnrollmentPendingRequest{
		RequestID:         r.ID,
		ComponentKind:     r.ComponentKind,
		ComponentName:     r.ComponentName,
		InstanceID:        r.InstanceID,
		Hostname:          r.Hostname,
		SystemFingerprint: r.SystemFingerprint,
		Fingerprints:      r.Fingerprints,
		State:             r.State,
		CreatedAt:         r.CreatedAt,
		ExpiresAt:         r.ExpiresAt,
	}
}

func (r PlatformEnrollmentRequest) ValidateStoredIdentity() error {
	name, err := r.ComponentKind.CanonicalName()
	if err != nil {
		return err
	}
	if r.ComponentName != name {
		return fmt.Errorf("%w: component name", constants.ErrPlatformEnrollmentStoredRequestInvalid)
	}
	return nil
}
