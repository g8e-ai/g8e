// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestSplitCSVModelTags(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: nil},
		{name: "single tag", raw: "qwen3:4b", want: []string{"qwen3:4b"}},
		{name: "comma separated", raw: "qwen3:4b,gemma3:4b", want: []string{"qwen3:4b", "gemma3:4b"}},
		{name: "trims spaces", raw: " qwen3:4b , gemma3:4b ", want: []string{"qwen3:4b", "gemma3:4b"}},
		{name: "skips empty segments", raw: "qwen3:4b,,gemma3:4b", want: []string{"qwen3:4b", "gemma3:4b"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, splitCSVModelTags(test.raw))
		})
	}
}

func TestVerificationReportHelpers(t *testing.T) {
	report := &evalv1.EvaluationVerificationReport{
		Status:         evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		FailureCount:   2,
		FailureReasons: []string{"missing witness", "digest mismatch"},
	}

	assert.Equal(t, report.GetStatus().String(), verificationStatusString(report))
	assert.Equal(t, 2, verificationFailureCount(report))
	assert.Equal(t, []string{"missing witness", "digest mismatch"}, verificationFailureReasons(report))

	assert.Empty(t, verificationStatusString(nil))
	assert.Zero(t, verificationFailureCount(nil))
	assert.Nil(t, verificationFailureReasons(nil))
}
