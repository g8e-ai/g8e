// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	// RepeatabilityRepetitionCount is the preregistered repetition
	// count for Phase 11 unbiased repeatability campaigns.
	RepeatabilityRepetitionCount = 5
)

// ValidateRepeatabilitySpec verifies the campaign spec matches the
// preregistered repeatability repetition count.
func ValidateRepeatabilitySpec(spec *evalv1.EvaluationCampaignSpec) error {
	if spec == nil {
		return fmt.Errorf("evaluation: validate repeatability spec: %w", constants.ErrMissingRequiredField)
	}
	if spec.GetRepetitionCount() != RepeatabilityRepetitionCount {
		return fmt.Errorf("evaluation: validate repeatability spec: expected repetition_count %d, got %d", RepeatabilityRepetitionCount, spec.GetRepetitionCount())
	}
	return nil
}

// ComputeRepeatabilityMatrixSize returns model variants × roles ×
// scenarios × repeatability repetitions.
func ComputeRepeatabilityMatrixSize(variantCount uint64) uint64 {
	return ComputeHomogeneousMatrixSize(variantCount) * uint64(RepeatabilityRepetitionCount)
}
