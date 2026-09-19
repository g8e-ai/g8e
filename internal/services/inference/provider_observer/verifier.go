// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

import (
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const (
	DefaultClockSkewTolerance = 5 * time.Second
)

// CoverageReport records provider-boundary observation coverage for one attempt.
type CoverageReport struct {
	ProviderAttemptID string
	SampleCount       int
	GPUReported       bool
	HostRAMReported   bool
	ClockSkewNanos    int64
	Complete          bool
	FailureReasons    []string
}

// VerifyObservationCoverage independently verifies one observation window
// against the durable provider attempt record.
func VerifyObservationCoverage(window *evalv1.ProviderBoundaryObservationWindow, attempt *operatorv1.InferenceProviderAttemptRecord) (*CoverageReport, error) {
	if window == nil || attempt == nil {
		return nil, fmt.Errorf("provider observer: verify observation coverage: %w", constants.ErrMissingRequiredField)
	}
	report := &CoverageReport{
		ProviderAttemptID: attempt.GetProviderAttemptId(),
		ClockSkewNanos:    window.GetClockSkewNanos(),
	}
	failures := make([]string, 0)
	if window.GetProviderAttemptId() != attempt.GetProviderAttemptId() {
		failures = append(failures, "provider attempt binding mismatch")
	}
	if err := ValidateObservationWindow(window); err != nil {
		failures = append(failures, "observation window digest invalid: "+err.Error())
	}
	if window.GetWindowCompletedAtUnixNanos() <= window.GetWindowStartedAtUnixNanos() {
		failures = append(failures, "observation window has non-positive duration")
	}
	if attempt.GetStartedAtUnixMs() > 0 && window.GetAttemptStartedAtUnixMs() != attempt.GetStartedAtUnixMs() {
		failures = append(failures, "attempt start timestamp mismatch")
	}
	if attempt.GetCompletedAtUnixMs() > 0 && window.GetAttemptCompletedAtUnixMs() != attempt.GetCompletedAtUnixMs() {
		failures = append(failures, "attempt completion timestamp mismatch")
	}
	if absDuration(time.Duration(window.GetClockSkewNanos())) > DefaultClockSkewTolerance {
		failures = append(failures, "clock skew exceeds tolerance")
	}
	report.SampleCount = len(window.GetSamples())
	if report.SampleCount == 0 {
		failures = append(failures, "no provider-boundary samples recorded")
	} else {
		for _, sample := range window.GetSamples() {
			if sample.GetGpuUtilizationAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED ||
				sample.GetVramBytesAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
				report.GPUReported = true
			}
			if sample.GetHostRamAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
				report.HostRAMReported = true
			}
		}
	}
	report.FailureReasons = failures
	report.Complete = len(failures) == 0
	return report, nil
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
