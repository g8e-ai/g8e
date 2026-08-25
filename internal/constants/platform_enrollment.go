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

	// PlatformEnrollmentIssuanceLeaseTTL bounds how long an issuance handler
	// may hold the issuing state before reconciliation recovers it back to
	// approved. Signing is sub-second; this is a crash-recovery bound.
	PlatformEnrollmentIssuanceLeaseTTL = 30 * time.Second

	// PlatformEnrollmentMaxLiveRequestsPerComponent bounds the number of
	// concurrent non-terminal requests for the same component kind. This
	// prevents unbounded pending request creation from a misbehaving or
	// malicious requester.
	PlatformEnrollmentMaxLiveRequestsPerComponent = 3

	// PlatformEnrollmentCleanupInterval is how often the managed cleanup
	// goroutine runs reconciliation of expired leases and removal of
	// terminal requests past the retention window.
	PlatformEnrollmentCleanupInterval = 5 * time.Minute

	// PlatformEnrollmentCleanupRetention is how long terminal request
	// records are kept after the terminal transition before cleanup removes
	// them. Issued artifacts stored on completed requests are never removed
	// by cleanup (they are the sole copy for idempotent retry).
	PlatformEnrollmentCleanupRetention = 24 * time.Hour

	// PlatformEnrollmentRetryAfterSeconds is the Retry-After value returned
	// to a client that polls a request in the issuing state with a live
	// lease.
	PlatformEnrollmentRetryAfterSeconds = 5
)
