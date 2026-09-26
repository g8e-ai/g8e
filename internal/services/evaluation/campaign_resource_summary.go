// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	maxPublicNumber     = uint64(1<<53 - 1)
	maxPublicDurationMS = uint64(604800000)
	maxPublicRetryCount = uint64(1000)
)

func (metric PublicResourceMetric) MarshalJSON() ([]byte, error) {
	value := struct {
		Value             *float64 `json:"value,omitempty"`
		UnavailableReason string   `json:"unavailable_reason,omitempty"`
	}{Value: metric.Value}
	if metric.UnavailableReason != evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED {
		value.UnavailableReason = strings.ToLower(strings.TrimPrefix(metric.UnavailableReason.String(), "PUBLIC_UNAVAILABLE_REASON_"))
	}
	return json.Marshal(value)
}

func (summary PublicResourceSummary) MarshalJSON() ([]byte, error) {
	value := struct {
		LatencyMS      *PublicResourceMetric `json:"latency_ms,omitempty"`
		InputTokens    *PublicResourceMetric `json:"input_tokens,omitempty"`
		OutputTokens   *PublicResourceMetric `json:"output_tokens,omitempty"`
		ThinkingTokens *PublicResourceMetric `json:"thinking_tokens,omitempty"`
		CacheTokens    *PublicResourceMetric `json:"cache_tokens,omitempty"`
		Retries        *PublicResourceMetric `json:"retries,omitempty"`
	}{
		LatencyMS: metricOrNil(summary.LatencyMS), InputTokens: metricOrNil(summary.InputTokens), OutputTokens: metricOrNil(summary.OutputTokens),
		ThinkingTokens: metricOrNil(summary.ThinkingTokens), CacheTokens: metricOrNil(summary.CacheTokens), Retries: metricOrNil(summary.Retries),
	}
	return json.Marshal(value)
}

func metricOrNil(metric PublicResourceMetric) *PublicResourceMetric {
	if metric.Value == nil && metric.UnavailableReason == evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED {
		return nil
	}
	return &metric
}

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

	input, inputReason, err := aggregateRequiredUsageTokens(calls, func(call *evalv1.ModelInferenceRecord) uint32 {
		return call.GetPromptTokens()
	}, "input tokens")
	if err != nil {
		return nil, err
	}
	output, outputReason, err := aggregateRequiredUsageTokens(calls, func(call *evalv1.ModelInferenceRecord) uint32 {
		return call.GetCompletionTokens()
	}, "output tokens")
	if err != nil {
		return nil, err
	}
	thinking, thinkingReason, err := aggregateOptionalUsageTokens(calls, func(call *evalv1.ModelInferenceRecord) (uint32, bool) {
		if call.ThinkingTokens == nil {
			return 0, false
		}
		return *call.ThinkingTokens, true
	}, "thinking tokens")
	if err != nil {
		return nil, err
	}
	cache, cacheReason, err := aggregateOptionalUsageTokens(calls, func(call *evalv1.ModelInferenceRecord) (uint32, bool) {
		if call.CacheTokens == nil {
			return 0, false
		}
		return *call.CacheTokens, true
	}, "cache tokens")
	if err != nil {
		return nil, err
	}

	var retries uint64
	allRetriesPresent := true
	for _, call := range calls {
		if call == nil {
			return nil, fmt.Errorf("evaluation: build public resource summary: nil inference record: %w", constants.ErrEvidenceArtifactMalformed)
		}
		if call.RetryCount == nil {
			allRetriesPresent = false
			continue
		}
		retries, err = addResourceValue(retries, uint64(call.GetRetryCount()), "retries")
		if err != nil {
			return nil, err
		}
	}

	resources := &PublicResourceSummary{}
	applyResourceMetric(&resources.InputTokens, input, inputReason)
	applyResourceMetric(&resources.OutputTokens, output, outputReason)
	applyResourceMetric(&resources.ThinkingTokens, thinking, thinkingReason)
	applyResourceMetric(&resources.CacheTokens, cache, cacheReason)
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

func aggregateRequiredUsageTokens(calls []*evalv1.ModelInferenceRecord, getter func(*evalv1.ModelInferenceRecord) uint32, name string) (uint64, evalv1.PublicUnavailableReason, error) {
	for _, call := range calls {
		if call == nil {
			return 0, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED, fmt.Errorf("evaluation: build public resource summary: nil inference record: %w", constants.ErrEvidenceArtifactMalformed)
		}
		if call.GetUsageAvailability() != evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED {
			return 0, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE, nil
		}
	}
	var sum uint64
	var err error
	for _, call := range calls {
		sum, err = addResourceValue(sum, uint64(getter(call)), name)
		if err != nil {
			return 0, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED, err
		}
	}
	return sum, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED, nil
}

func aggregateOptionalUsageTokens(calls []*evalv1.ModelInferenceRecord, present func(*evalv1.ModelInferenceRecord) (uint32, bool), name string) (uint64, evalv1.PublicUnavailableReason, error) {
	presentCount := 0
	var sum uint64
	for _, call := range calls {
		if call == nil {
			return 0, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED, fmt.Errorf("evaluation: build public resource summary: nil inference record: %w", constants.ErrEvidenceArtifactMalformed)
		}
		value, ok := present(call)
		if !ok {
			continue
		}
		presentCount++
		var err error
		sum, err = addResourceValue(sum, uint64(value), name)
		if err != nil {
			return 0, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED, err
		}
	}
	if presentCount == 0 {
		return 0, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE, nil
	}
	if presentCount < len(calls) {
		return 0, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE, nil
	}
	return sum, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED, nil
}

func applyResourceMetric(metric *PublicResourceMetric, sum uint64, reason evalv1.PublicUnavailableReason) {
	if reason == evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED {
		metric.Value = resourceFloat(sum)
		return
	}
	metric.UnavailableReason = reason
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
