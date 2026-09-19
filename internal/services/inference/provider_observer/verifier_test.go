// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func validObservationFixtures(t *testing.T) (*evalv1.ProviderBoundaryObservationWindow, *operatorv1.InferenceProviderAttemptRecord) {
	t.Helper()
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              SchemaVersion,
		ProviderAttemptId:          "attempt-1",
		ObserverId:                 "observer-test",
		ObserverClockSource:        DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   1_000,
		WindowCompletedAtUnixNanos: 2_000,
		AttemptStartedAtUnixMs:     1_700_000_000_000,
		AttemptCompletedAtUnixMs:   1_700_000_010_000,
		ClockSkewNanos:             int64(time.Second),
		Samples: []*evalv1.ProviderBoundaryHardwareSample{
			{
				ObservedAtUnixNanos: 1_500,
				HostRamAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
				HostRamUsedBytes:    10,
				HostRamTotalBytes:   20,
			},
		},
	}
	digest, err := ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	attempt := &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: "attempt-1",
		StartedAtUnixMs:   window.GetAttemptStartedAtUnixMs(),
		CompletedAtUnixMs: window.GetAttemptCompletedAtUnixMs(),
		Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
	}
	return window, attempt
}

func TestVerifyObservationCoverage(t *testing.T) {
	t.Parallel()
	baseWindow, baseAttempt := validObservationFixtures(t)

	tests := []struct {
		name         string
		window       *evalv1.ProviderBoundaryObservationWindow
		attempt      *operatorv1.InferenceProviderAttemptRecord
		wantComplete bool
		wantFailures []string
		wantErr      bool
	}{
		{
			name:         "valid window is complete",
			window:       baseWindow,
			attempt:      baseAttempt,
			wantComplete: true,
		},
		{
			name:    "nil window",
			window:  nil,
			attempt: baseAttempt,
			wantErr: true,
		},
		{
			name:         "attempt id mismatch",
			window:       baseWindow,
			attempt:      &operatorv1.InferenceProviderAttemptRecord{ProviderAttemptId: "other"},
			wantComplete: false,
			wantFailures: []string{"provider attempt binding mismatch"},
		},
		{
			name: "digest mismatch",
			window: func() *evalv1.ProviderBoundaryObservationWindow {
				clone := *baseWindow
				clone.ObservationDigest = "bad-digest"
				return &clone
			}(),
			attempt:      baseAttempt,
			wantComplete: false,
			wantFailures: []string{"digest invalid"},
		},
		{
			name: "non-positive duration",
			window: func() *evalv1.ProviderBoundaryObservationWindow {
				clone := *baseWindow
				clone.WindowCompletedAtUnixNanos = clone.GetWindowStartedAtUnixNanos()
				digest, err := ComputeObservationDigest(&clone)
				require.NoError(t, err)
				clone.ObservationDigest = digest
				return &clone
			}(),
			attempt:      baseAttempt,
			wantComplete: false,
			wantFailures: []string{"non-positive duration"},
		},
		{
			name:   "attempt start timestamp mismatch",
			window: baseWindow,
			attempt: &operatorv1.InferenceProviderAttemptRecord{
				ProviderAttemptId: "attempt-1",
				StartedAtUnixMs:   baseAttempt.GetStartedAtUnixMs() + 1,
				CompletedAtUnixMs: baseAttempt.GetCompletedAtUnixMs(),
			},
			wantComplete: false,
			wantFailures: []string{"attempt start timestamp mismatch"},
		},
		{
			name: "clock skew exceeds tolerance",
			window: func() *evalv1.ProviderBoundaryObservationWindow {
				clone := *baseWindow
				clone.ClockSkewNanos = int64(6 * time.Second)
				digest, err := ComputeObservationDigest(&clone)
				require.NoError(t, err)
				clone.ObservationDigest = digest
				return &clone
			}(),
			attempt:      baseAttempt,
			wantComplete: false,
			wantFailures: []string{"clock skew exceeds tolerance"},
		},
		{
			name: "no samples recorded",
			window: func() *evalv1.ProviderBoundaryObservationWindow {
				clone := *baseWindow
				clone.Samples = nil
				digest, err := ComputeObservationDigest(&clone)
				require.NoError(t, err)
				clone.ObservationDigest = digest
				return &clone
			}(),
			attempt:      baseAttempt,
			wantComplete: false,
			wantFailures: []string{"no provider-boundary samples recorded"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			report, err := VerifyObservationCoverage(test.window, test.attempt)
			if test.wantErr {
				require.ErrorIs(t, err, constants.ErrMissingRequiredField)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantComplete, report.Complete)
			for _, failure := range test.wantFailures {
				assertContainsFailure(t, report.FailureReasons, failure)
			}
		})
	}
}

func assertContainsFailure(t *testing.T, failures []string, want string) {
	t.Helper()
	for _, failure := range failures {
		if strings.Contains(failure, want) {
			return
		}
	}
	t.Fatalf("expected failure containing %q in %#v", want, failures)
}

func TestAbsDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   time.Duration
		want time.Duration
	}{
		{in: -5 * time.Second, want: 5 * time.Second},
		{in: 3 * time.Second, want: 3 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.in.String(), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, absDuration(test.in))
		})
	}
}
