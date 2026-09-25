// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildPublicResourceSummary(t *testing.T) {
	reported := evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED
	zero := uint32(0)
	tests := []struct {
		name       string
		result     *evalv1.EvaluationAssignmentResult
		assertions func(t *testing.T, summary *PublicResourceSummary)
	}{
		{
			name:   "no scored calls is explicitly unavailable",
			result: &evalv1.EvaluationAssignmentResult{},
			assertions: func(t *testing.T, summary *PublicResourceSummary) {
				assert.Nil(t, summary.InputTokens.Value)
				assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS, summary.InputTokens.UnavailableReason)
			},
		},
		{
			name: "reported prompt output zeros remain observed while optional counters stay unavailable",
			result: &evalv1.EvaluationAssignmentResult{
				ModelInferences:          []*evalv1.ModelInferenceRecord{{UsageAvailability: reported, RetryCount: &zero}},
				ScoredInferenceSpanNanos: proto.Uint64(2_000_000),
			},
			assertions: func(t *testing.T, summary *PublicResourceSummary) {
				require.NotNil(t, summary.InputTokens.Value)
				assert.Zero(t, *summary.InputTokens.Value)
				require.NotNil(t, summary.OutputTokens.Value)
				assert.Zero(t, *summary.OutputTokens.Value)
				assert.Nil(t, summary.ThinkingTokens.Value)
				assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE, summary.ThinkingTokens.UnavailableReason)
				assert.Nil(t, summary.CacheTokens.Value)
				assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE, summary.CacheTokens.UnavailableReason)
				require.NotNil(t, summary.Retries.Value)
				assert.Zero(t, *summary.Retries.Value)
				require.NotNil(t, summary.LatencyMS.Value)
				assert.Equal(t, float64(2), *summary.LatencyMS.Value)
			},
		},
		{
			name: "explicit zero optional counters remain observed",
			result: &evalv1.EvaluationAssignmentResult{
				ModelInferences: []*evalv1.ModelInferenceRecord{{
					UsageAvailability: reported,
					ThinkingTokens:    &zero,
					CacheTokens:       &zero,
				}},
			},
			assertions: func(t *testing.T, summary *PublicResourceSummary) {
				require.NotNil(t, summary.ThinkingTokens.Value)
				assert.Zero(t, *summary.ThinkingTokens.Value)
				require.NotNil(t, summary.CacheTokens.Value)
				assert.Zero(t, *summary.CacheTokens.Value)
			},
		},
		{
			name: "cache absent on one call leaves input output observed and cache unavailable",
			result: &evalv1.EvaluationAssignmentResult{
				ModelInferences: []*evalv1.ModelInferenceRecord{
					{UsageAvailability: reported, PromptTokens: 3, CompletionTokens: 4, CacheTokens: &zero},
					{UsageAvailability: reported, PromptTokens: 1, CompletionTokens: 2},
				},
			},
			assertions: func(t *testing.T, summary *PublicResourceSummary) {
				require.NotNil(t, summary.InputTokens.Value)
				assert.Equal(t, float64(4), *summary.InputTokens.Value)
				require.NotNil(t, summary.OutputTokens.Value)
				assert.Equal(t, float64(6), *summary.OutputTokens.Value)
				assert.Nil(t, summary.CacheTokens.Value)
				assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE, summary.CacheTokens.UnavailableReason)
			},
		},
		{
			name: "mixed thinking presence is incomplete without affecting complete cache",
			result: &evalv1.EvaluationAssignmentResult{
				ModelInferences: []*evalv1.ModelInferenceRecord{
					{UsageAvailability: reported, ThinkingTokens: proto.Uint32(2), CacheTokens: &zero},
					{UsageAvailability: reported, CacheTokens: &zero},
				},
			},
			assertions: func(t *testing.T, summary *PublicResourceSummary) {
				assert.Nil(t, summary.ThinkingTokens.Value)
				assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE, summary.ThinkingTokens.UnavailableReason)
				require.NotNil(t, summary.CacheTokens.Value)
				assert.Zero(t, *summary.CacheTokens.Value)
			},
		},
		{
			name: "missing one contributor does not become zero",
			result: &evalv1.EvaluationAssignmentResult{
				ModelInferences: []*evalv1.ModelInferenceRecord{{UsageAvailability: reported}, {UsageAvailability: evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE}},
			},
			assertions: func(t *testing.T, summary *PublicResourceSummary) {
				assert.Nil(t, summary.InputTokens.Value)
				assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE, summary.InputTokens.UnavailableReason)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, err := BuildPublicResourceSummary(tt.result)
			require.NoError(t, err)
			tt.assertions(t, summary)
		})
	}

	_, err := BuildPublicResourceSummary(&evalv1.EvaluationAssignmentResult{ModelInferences: []*evalv1.ModelInferenceRecord{{UsageAvailability: reported}}, ScoredInferenceSpanNanos: proto.Uint64(math.MaxUint64)})
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}

func TestBuildPublicResourceSummary_NilResult(t *testing.T) {
	_, err := BuildPublicResourceSummary(nil)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
	assert.False(t, errors.Is(err, constants.ErrEvidenceArtifactMalformed))
}
