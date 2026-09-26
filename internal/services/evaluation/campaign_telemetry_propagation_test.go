// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestTelemetryOptionality_EndToEndFromTraceV2(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	trace["schema_version"] = "2"
	call := trace["model_calls"].([]any)[0].(EvaluationTrace)
	call["usage_reported"] = true
	call["input_tokens"] = float64(10)
	call["output_tokens"] = float64(5)
	call["cache_tokens"] = float64(3)
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest

	req := homogeneousAssignmentExecutionRequest(t, "primary")
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)

	inference := result.GetModelInferences()[0]
	assert.Nil(t, inference.ThinkingTokens)
	require.NotNil(t, inference.CacheTokens)
	assert.Equal(t, uint32(3), inference.GetCacheTokens())

	summary, err := BuildPublicResourceSummary(result)
	require.NoError(t, err)
	require.NotNil(t, summary.InputTokens.Value)
	assert.Equal(t, float64(10), *summary.InputTokens.Value)
	require.NotNil(t, summary.OutputTokens.Value)
	assert.Equal(t, float64(5), *summary.OutputTokens.Value)
	assert.Nil(t, summary.ThinkingTokens.Value)
	assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE, summary.ThinkingTokens.UnavailableReason)
	require.NotNil(t, summary.CacheTokens.Value)
	assert.Equal(t, float64(3), *summary.CacheTokens.Value)

	activity, _, err := BuildPublicAssignmentEvidence(PublicAssignmentEvidenceInput{Result: result})
	require.NoError(t, err)
	record := activity.Summary.ModelActivity.Records[0]
	assert.Nil(t, record.ThinkingTokens)
	require.NotNil(t, record.CacheTokens)
	assert.Equal(t, uint64(3), record.GetCacheTokens())
}
