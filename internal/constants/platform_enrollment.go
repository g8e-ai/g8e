// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import "time"

type PlatformEnrollmentGovernanceAction string

type PlatformEnrollmentGovernanceIntent string

const (
	PlatformEnrollmentActionCreate        PlatformEnrollmentGovernanceAction = "PLATFORM_ENROLLMENT_CREATE"
	PlatformEnrollmentActionDecide        PlatformEnrollmentGovernanceAction = "PLATFORM_ENROLLMENT_DECIDE"
	PlatformEnrollmentActionIssue         PlatformEnrollmentGovernanceAction = "PLATFORM_ENROLLMENT_ISSUE"
	PlatformEnrollmentActionPersistPolicy PlatformEnrollmentGovernanceAction = "PLATFORM_ENROLLMENT_PERSIST_POLICY"
	PlatformEnrollmentActionCreateSession PlatformEnrollmentGovernanceAction = "PLATFORM_ENROLLMENT_CREATE_SESSION"

	PlatformEnrollmentIntentRequest PlatformEnrollmentGovernanceIntent = "platform_enrollment.request"
	PlatformEnrollmentIntentApprove PlatformEnrollmentGovernanceIntent = "platform_enrollment.approve"
	PlatformEnrollmentIntentDeny    PlatformEnrollmentGovernanceIntent = "platform_enrollment.deny"
	PlatformEnrollmentIntentIssue   PlatformEnrollmentGovernanceIntent = "platform_enrollment.issue"

	PlatformEnrollmentRequestTTL               = 30 * time.Minute
	PlatformEnrollmentTokenBytes               = 32
	PlatformEnrollmentProtocolVersion          = "1"
	PlatformEnrollmentMaxHostnameBytes         = 253
	PlatformEnrollmentMaxInstanceIDBytes       = 128
	PlatformEnrollmentMaxReasonBytes           = 512
	PlatformEnrollmentMaxRequestBytes    int64 = 64 * 1024
)
