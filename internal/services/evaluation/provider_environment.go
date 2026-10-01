// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ProviderEnvironmentSourceObserved marks an environment derived from
// provider-boundary observation windows rather than written down by a person.
const ProviderEnvironmentSourceObserved = "observed"

// ProviderEnvironment is the hardware capacity the provider-boundary observer
// reported for a run. The JSON field names are the explorer
// catalog_snapshot.provider_environment wire shape, so the value projects
// without remapping. The observer reports GPU memory and system RAM capacity
// only; it reports no processor and no model names, so none are claimed here.
type ProviderEnvironment struct {
	Source   string `json:"source"`
	Memory   string `json:"memory,omitempty"`
	Graphics string `json:"graphics,omitempty"`
}

// ObservedProviderEnvironment derives a run's provider environment from the
// observation windows of its scored inferences. It reads every window it can
// load and skips attempts whose window was never captured. It returns nil when
// no window reported a capacity, and also when the windows disagree about a
// capacity: a run that spans different hardware makes no environment claim, so
// it can never be matched against another run's.
func (r *CampaignProviderObservationReader) ObservedProviderEnvironment(ctx context.Context, results map[string]*evalv1.EvaluationAssignmentResult) (*ProviderEnvironment, error) {
	if r == nil || r.windows == nil {
		return nil, fmt.Errorf("evaluation: observed provider environment: %w", constants.ErrMissingRequiredField)
	}
	vramTotals := map[uint64]struct{}{}
	hostRAMTotals := map[uint64]struct{}{}
	for _, result := range results {
		for _, inferenceRecord := range scoredModelInferences(result) {
			attemptID := inferenceRecord.GetProviderAttemptId()
			if attemptID == "" {
				continue
			}
			window, err := r.windows.Load(ctx, attemptID)
			if err != nil {
				if isProviderEvidenceNotFound(err) {
					continue
				}
				return nil, fmt.Errorf("evaluation: observed provider environment: %w", err)
			}
			for _, sample := range window.GetSamples() {
				if sample.GetVramBytesAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED && sample.GetVramTotalBytes() > 0 {
					vramTotals[sample.GetVramTotalBytes()] = struct{}{}
				}
				if sample.GetHostRamAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED && sample.GetHostRamTotalBytes() > 0 {
					hostRAMTotals[sample.GetHostRamTotalBytes()] = struct{}{}
				}
			}
		}
	}
	return observedProviderEnvironment(vramTotals, hostRAMTotals), nil
}

func observedProviderEnvironment(vramTotals, hostRAMTotals map[uint64]struct{}) *ProviderEnvironment {
	if len(vramTotals) > 1 || len(hostRAMTotals) > 1 {
		return nil
	}
	environment := &ProviderEnvironment{Source: ProviderEnvironmentSourceObserved}
	for total := range hostRAMTotals {
		environment.Memory = formatCapacity(total)
	}
	for total := range vramTotals {
		environment.Graphics = formatCapacity(total) + " VRAM"
	}
	if environment.Memory == "" && environment.Graphics == "" {
		return nil
	}
	return environment
}

// formatCapacity renders a byte count in whole GiB, so a capacity that differs
// only by the platform's reserved memory does not read as different hardware.
func formatCapacity(totalBytes uint64) string {
	if gib := (totalBytes + 1<<29) >> 30; gib > 0 {
		return fmt.Sprintf("%d GiB", gib)
	}
	return fmt.Sprintf("%d MiB", (totalBytes+1<<19)>>20)
}
