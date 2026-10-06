// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package gateway contains CLI-facing helpers for Gateway inventory data.
package gateway

import (
	"encoding/json"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// FetchEnrolled returns the completed, non-revoked platform enrollments. The
// bool is false when the Gateway list could not be retrieved or decoded.
func FetchEnrolled(get func(string) ([]byte, error)) ([]models.PlatformEnrollmentEnrolledRequest, bool) {
	if get == nil {
		return nil, true
	}
	body, err := get(constants.APIPaths.AuthPlatformEnrollmentEnrolled)
	if err != nil {
		return nil, false
	}
	var resp models.PlatformEnrollmentEnrolledResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false
	}
	return CompletedEnrollments(resp.Enrollments), true
}

// CompletedEnrollments filters the enrolled response the same way Gateway
// status does: only active completed identities are included.
func CompletedEnrollments(enrollments []models.PlatformEnrollmentEnrolledRequest) []models.PlatformEnrollmentEnrolledRequest {
	completed := make([]models.PlatformEnrollmentEnrolledRequest, 0, len(enrollments))
	for _, enrollment := range enrollments {
		if enrollment.State == models.PlatformEnrollmentStateCompleted {
			completed = append(completed, enrollment)
		}
	}
	return completed
}
