// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestPlatformEnrollmentCreateRequestValidateShape(t *testing.T) {
	tests := []struct {
		name    string
		request PlatformEnrollmentCreateRequest
		wantErr error
	}{
		{
			name: "dashboard accepts app payload",
			request: PlatformEnrollmentCreateRequest{
				ComponentKind: PlatformComponentDashboard,
				InstanceID:    "dashboard-1",
				Hostname:      "dashboard-host",
				App:           &PlatformAppCSRPayload{CSRPEM: "csr"},
			},
		},
		{
			name: "ensemble accepts app payload",
			request: PlatformEnrollmentCreateRequest{
				ComponentKind: PlatformComponentEnsemble,
				InstanceID:    "ensemble-1",
				Hostname:      "ensemble-host",
				App:           &PlatformAppCSRPayload{CSRPEM: "csr"},
			},
		},
		{
			name: "operator accepts operator payload and metadata",
			request: PlatformEnrollmentCreateRequest{
				ComponentKind:     PlatformComponentOperator,
				InstanceID:        "operator-1",
				Hostname:          "operator-host",
				SystemFingerprint: "fingerprint",
				Operator: &PlatformOperatorCSRPayload{
					OperatorCSRPEM: "operator-csr",
					CLICSRPEM:      "cli-csr",
				},
			},
		},
		{
			name: "rejects unknown component",
			request: PlatformEnrollmentCreateRequest{
				ComponentKind: "unknown",
				InstanceID:    "unknown-1",
				Hostname:      "unknown-host",
				App:           &PlatformAppCSRPayload{CSRPEM: "csr"},
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidComponent,
		},
		{
			name: "rejects missing instance ID",
			request: PlatformEnrollmentCreateRequest{
				ComponentKind: PlatformComponentDashboard,
				Hostname:      "dashboard-host",
				App:           &PlatformAppCSRPayload{CSRPEM: "csr"},
			},
			wantErr: constants.ErrPlatformEnrollmentInstanceIDRequired,
		},
		{
			name: "rejects mismatched app payload",
			request: PlatformEnrollmentCreateRequest{
				ComponentKind: PlatformComponentOperator,
				InstanceID:    "operator-1",
				Hostname:      "operator-host",
				App:           &PlatformAppCSRPayload{CSRPEM: "csr"},
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidPayload,
		},
		{
			name: "rejects both payloads",
			request: PlatformEnrollmentCreateRequest{
				ComponentKind: PlatformComponentDashboard,
				InstanceID:    "dashboard-1",
				Hostname:      "dashboard-host",
				App:           &PlatformAppCSRPayload{CSRPEM: "csr"},
				Operator:      &PlatformOperatorCSRPayload{OperatorCSRPEM: "operator-csr", CLICSRPEM: "cli-csr"},
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidPayload,
		},
		{
			name: "rejects operator without fingerprint",
			request: PlatformEnrollmentCreateRequest{
				ComponentKind: PlatformComponentOperator,
				InstanceID:    "operator-1",
				Hostname:      "operator-host",
				Operator:      &PlatformOperatorCSRPayload{OperatorCSRPEM: "operator-csr", CLICSRPEM: "cli-csr"},
			},
			wantErr: constants.ErrPlatformEnrollmentFingerprintRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.ValidateShape()
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestPlatformEnrollmentComponentCanonicalName(t *testing.T) {
	tests := []struct {
		kind PlatformComponentKind
		name string
		err  error
	}{
		{kind: PlatformComponentDashboard, name: PlatformDashboardName},
		{kind: PlatformComponentEnsemble, name: PlatformEnsembleName},
		{kind: PlatformComponentOperator, name: PlatformOperatorName},
		{kind: "unknown", err: constants.ErrPlatformEnrollmentInvalidComponent},
	}

	for _, tt := range tests {
		name, err := tt.kind.CanonicalName()
		assert.Equal(t, tt.name, name)
		assert.ErrorIs(t, err, tt.err)
	}
}
