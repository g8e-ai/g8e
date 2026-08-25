// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

func TestValidatePlatformEnrollmentTransitionAllowsOnlyDefinedStateMachineEdges(t *testing.T) {
	tests := []struct {
		name string
		from models.PlatformEnrollmentState
		to   models.PlatformEnrollmentState
		err  error
	}{
		{name: "pending to approved", from: models.PlatformEnrollmentStatePending, to: models.PlatformEnrollmentStateApproved},
		{name: "pending to denied", from: models.PlatformEnrollmentStatePending, to: models.PlatformEnrollmentStateDenied},
		{name: "pending to expired", from: models.PlatformEnrollmentStatePending, to: models.PlatformEnrollmentStateExpired},
		{name: "approved to issuing", from: models.PlatformEnrollmentStateApproved, to: models.PlatformEnrollmentStateIssuing},
		{name: "approved to expired", from: models.PlatformEnrollmentStateApproved, to: models.PlatformEnrollmentStateExpired},
		{name: "issuing to completed", from: models.PlatformEnrollmentStateIssuing, to: models.PlatformEnrollmentStateCompleted},
		{name: "issuing lease recovery", from: models.PlatformEnrollmentStateIssuing, to: models.PlatformEnrollmentStateApproved},
		{name: "issuing to expired", from: models.PlatformEnrollmentStateIssuing, to: models.PlatformEnrollmentStateExpired},
		{name: "completed is terminal", from: models.PlatformEnrollmentStateCompleted, to: models.PlatformEnrollmentStateApproved, err: constants.ErrPlatformEnrollmentInvalidState},
		{name: "denied is terminal", from: models.PlatformEnrollmentStateDenied, to: models.PlatformEnrollmentStatePending, err: constants.ErrPlatformEnrollmentInvalidState},
		{name: "approved cannot deny", from: models.PlatformEnrollmentStateApproved, to: models.PlatformEnrollmentStateDenied, err: constants.ErrPlatformEnrollmentInvalidState},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, validatePlatformEnrollmentTransition(tt.from, tt.to), tt.err)
		})
	}
}
