// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func validatePlatformEnrollmentTransition(from, to models.PlatformEnrollmentState) error {
	valid := false
	switch from {
	case models.PlatformEnrollmentStatePending:
		valid = to == models.PlatformEnrollmentStateApproved || to == models.PlatformEnrollmentStateDenied || to == models.PlatformEnrollmentStateExpired
	case models.PlatformEnrollmentStateApproved:
		valid = to == models.PlatformEnrollmentStateIssuing || to == models.PlatformEnrollmentStateExpired
	case models.PlatformEnrollmentStateIssuing:
		valid = to == models.PlatformEnrollmentStateCompleted || to == models.PlatformEnrollmentStateApproved || to == models.PlatformEnrollmentStateExpired
	}
	if !valid {
		return constants.ErrPlatformEnrollmentInvalidState
	}
	return nil
}
