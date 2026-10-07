// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"encoding/json"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// EnrollmentFingerprint is one labeled CSR key fingerprint of a pending
// platform enrollment request, as the owner compares it with the workload.
type EnrollmentFingerprint struct {
	Label string
	Value string
}

// PlatformEnrollmentFingerprints returns the non-empty CSR fingerprints of a
// pending platform enrollment request as label/value pairs for display. It is
// shared by 'g8e auth enroll' and the TUI so both show the same keys.
func PlatformEnrollmentFingerprints(fps models.PlatformEnrollmentCSRFingerprints) []EnrollmentFingerprint {
	var out []EnrollmentFingerprint
	if fps.App != "" {
		out = append(out, EnrollmentFingerprint{Label: "App key", Value: fps.App})
	}
	if fps.Operator != "" {
		out = append(out, EnrollmentFingerprint{Label: "Operator key", Value: fps.Operator})
	}
	if fps.CLI != "" {
		out = append(out, EnrollmentFingerprint{Label: "CLI key", Value: fps.CLI})
	}
	return out
}

// PlatformEnrollmentIssuedIdentity names the identity a completed platform
// enrollment issued: the Operator (or its session) for an Operator, and the
// app policy otherwise.
func PlatformEnrollmentIssuedIdentity(enrollment models.PlatformEnrollmentEnrolledRequest) string {
	switch enrollment.ComponentKind {
	case models.PlatformComponentOperator:
		if enrollment.OperatorID != "" {
			return enrollment.OperatorID
		}
		return enrollment.OperatorSessionID
	default:
		return enrollment.PolicyID
	}
}

// DecodePlatformEnrollmentDecision decodes the Gateway's response to an owner
// decision (AuthPlatformEnrollmentDecision).
func DecodePlatformEnrollmentDecision(body []byte) (*models.PlatformEnrollmentDecisionResponse, error) {
	var resp models.PlatformEnrollmentDecisionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse decision response: %w", err)
	}
	return &resp, nil
}

// DecodePlatformEnrollmentRevoke decodes the Gateway's response to a
// revocation (AuthPlatformEnrollmentRevoke).
func DecodePlatformEnrollmentRevoke(body []byte) (*models.PlatformEnrollmentRevokeResponse, error) {
	var resp models.PlatformEnrollmentRevokeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &resp, nil
}
