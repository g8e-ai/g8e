// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"math"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	maxPublicNumber     = uint64(1<<53 - 1)
	maxPublicDurationMS = uint64(604800000)
	maxPublicRetryCount = uint64(1000)
)

// BuildPublicResourceSummary projects complete resource observations from scored
// inference calls. Grader calls are not part of the assignment result's model
// inference list and are never included in these totals.
func BuildPublicResourceSummary(result *evalv1.EvaluationAssignmentResult) (*PublicResourceSummary, error) {
	if result == nil {
		return nil, fmt.Errorf("evaluation: build public resource summary: %w", constants.ErrMissingRequiredField)
	}
	calls := result.GetModelInferences()
	if len(calls) == 0 {
		reason := evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS
		return &PublicResourceSummary{
			LatencyMS:      PublicResourceMetric{UnavailableReason: reason},
			InputTokens:    PublicResourceMetric{UnavailableReason: reason},
			OutputTokens:   PublicResourceMetric{UnavailableReason: reason},
			ThinkingTokens: PublicResourceMetric{UnavailableReason: reason},
			CacheTokens:    PublicResourceMetric{UnavailableReason: reason},
			Retries:        PublicResourceMetric{UnavailableReason: reason},
		}, nil
	}

	var input, output, thinking, cache, retries uint64
	allUsageReported := true
	allRetriesPresent := true
	for _, call := range calls {
		if call == nil {
			return nil, fmt.Errorf("evaluation: build public resource summary: nil inference record: %w", constants.ErrEvidenceArtifactMalformed)
		}
		if call.GetUsageAvailability() != evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED {
			allUsageReported = false
		}
		var err error
		input, err = addResourceValue(input, uint64(call.GetPromptTokens()), "input tokens")
		if err != nil {
			return nil, err
		}
		output, err = addResourceValue(output, uint64(call.GetCompletionTokens()), "output tokens")
		if err != nil {
			return nil, err
		}
		thinking, err = addResourceValue(thinking, uint64(call.GetThinkingTokens()), "thinking tokens")
		if err != nil {
			return nil, err
		}
		cache, err = addResourceValue(cache, uint64(call.GetCacheTokens()), "cache tokens")
		if err != nil {
			return nil, err
		}
		if call.RetryCount == nil {
			allRetriesPresent = false
		} else {
			retries, err = addResourceValue(retries, uint64(call.GetRetryCount()), "retries")
			if err != nil {
				return nil, err
			}
		}
	}

	metricReason := evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE
	if !allUsageReported {
		metricReason = evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE
	}
	resources := &PublicResourceSummary{}
	if allUsageReported {
		resources.InputTokens.Value = resourceFloat(input)
		resources.OutputTokens.Value = resourceFloat(output)
		resources.ThinkingTokens.Value = resourceFloat(thinking)
		resources.CacheTokens.Value = resourceFloat(cache)
	} else {
		resources.InputTokens.UnavailableReason = metricReason
		resources.OutputTokens.UnavailableReason = metricReason
		resources.ThinkingTokens.UnavailableReason = metricReason
		resources.CacheTokens.UnavailableReason = metricReason
	}
	if !allRetriesPresent {
		resources.Retries.UnavailableReason = evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE
	} else if retries > maxPublicRetryCount {
		return nil, fmt.Errorf("evaluation: build public resource summary: retries exceed public bound: %w", constants.ErrEvidenceArtifactMalformed)
	} else {
		resources.Retries.Value = resourceFloat(retries)
	}

	if result.ScoredInferenceSpanNanos == nil {
		resources.LatencyMS.UnavailableReason = evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE
	} else {
		span := result.GetScoredInferenceSpanNanos()
		if span/1_000_000 > maxPublicDurationMS {
			return nil, fmt.Errorf("evaluation: build public resource summary: scored inference span exceeds public bound: %w", constants.ErrEvidenceArtifactMalformed)
		}
		resources.LatencyMS.Value = resourceFloat(span / 1_000_000)
	}
	return resources, nil
}

func addResourceValue(current, value uint64, name string) (uint64, error) {
	if value > math.MaxUint64-current || current+value > maxPublicNumber {
		return 0, fmt.Errorf("evaluation: build public resource summary: %s overflow: %w", name, constants.ErrEvidenceArtifactMalformed)
	}
	return current + value, nil
}

func resourceFloat(value uint64) *float64 {
	converted := float64(value)
	return &converted
}
