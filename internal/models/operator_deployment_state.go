// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import "time"

// OperatorDeploymentPhase is the externally observable enrollment/startup
// progress of one Operator working directory.
type OperatorDeploymentPhase string

const (
	// OperatorDeploymentPhasePendingApproval: the enrollment request exists and
	// waits for an owner decision. RequestID is set.
	OperatorDeploymentPhasePendingApproval OperatorDeploymentPhase = "pending_approval"
	// OperatorDeploymentPhaseEnrolled: credentials and the Operator session are
	// issued. OperatorSessionID is set.
	OperatorDeploymentPhaseEnrolled OperatorDeploymentPhase = "enrolled"
	// OperatorDeploymentPhaseReady: this process established its command
	// subscription. OperatorSessionID is set.
	OperatorDeploymentPhaseReady OperatorDeploymentPhase = "ready"
	// OperatorDeploymentPhaseFailed: enrollment or startup failed. Error is set.
	OperatorDeploymentPhaseFailed OperatorDeploymentPhase = "failed"
)

// OperatorDeploymentState is the non-secret deployment state an Operator
// persists for the CLI that launched it. It never carries the requester token,
// private keys, or fingerprints. LaunchID binds progress to the deploying
// invocation without depending on synchronized clocks between hosts.
type OperatorDeploymentState struct {
	LaunchID          string                  `json:"launch_id,omitempty"`
	Phase             OperatorDeploymentPhase `json:"phase"`
	RequestID         string                  `json:"request_id,omitempty"`
	OperatorSessionID string                  `json:"operator_session_id,omitempty"`
	Error             string                  `json:"error,omitempty"`
	UpdatedAt         time.Time               `json:"updated_at"`
}
