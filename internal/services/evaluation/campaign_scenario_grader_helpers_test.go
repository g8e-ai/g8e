// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestExtractJSONObject_ParsesEmbeddedObject(t *testing.T) {
	t.Parallel()
	payload := extractJSONObject(`prefix {"answer":"ok","count":2} suffix`)
	require.NotNil(t, payload)
	assert.Equal(t, "ok", payload["answer"])
	assert.EqualValues(t, 2, payload["count"])
}

func TestExtractJSONObject_RejectsMalformedPayload(t *testing.T) {
	t.Parallel()
	assert.Nil(t, extractJSONObject("not json"))
	assert.Nil(t, extractJSONObject(`{"broken":`))
}

func TestNumericValue_AcceptsSupportedTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  any
		want int64
		ok   bool
	}{
		{name: "float64", raw: float64(12), want: 12, ok: true},
		{name: "int", raw: int(7), want: 7, ok: true},
		{name: "int64", raw: int64(99), want: 99, ok: true},
		{name: "json number", raw: json.Number("15"), want: 15, ok: true},
		{name: "string", raw: "nope", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, ok := numericValue(test.raw)
			assert.Equal(t, test.ok, ok)
			if test.ok {
				assert.Equal(t, test.want, got)
			}
		})
	}
}

func TestCountWordsAndSentences(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 3, countWords("one two three"))
	assert.Equal(t, 2, countSentences("Hello world. How are you?"))
	assert.Equal(t, 1, countSentences("Only one sentence"))
	assert.Equal(t, 0, countSentences(""))
}

func TestCleanContentLabel(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "error", cleanContentLabel("**ERROR**."))
	assert.Equal(t, "noise", cleanContentLabel("`noise`!"))
	assert.Equal(t, "ready", cleanContentLabel("\"ready\""))
}

func TestGradesEquivalent_ComparesNormalizedFields(t *testing.T) {
	t.Parallel()
	grade := &evalv1.DeterministicGrade{
		GradeId:     "assignment-1:criterion",
		CriterionId: "criterion",
		Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		Score:       1,
		Detail:      "ok",
	}
	assert.True(t, gradesEquivalent([]*evalv1.DeterministicGrade{grade}, []*evalv1.DeterministicGrade{grade}))
	assert.False(t, gradesEquivalent([]*evalv1.DeterministicGrade{grade}, []*evalv1.DeterministicGrade{{
		GradeId:     "assignment-1:criterion",
		CriterionId: "criterion",
		Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		Score:       1,
		Detail:      "ok",
	}}))
}

func TestRoleInvokedGradeStatus(t *testing.T) {
	t.Parallel()
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		roleInvokedGradeStatus(true, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED))
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		roleInvokedGradeStatus(false, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED))
}

func findDeterministicGrade(grades []*evalv1.DeterministicGrade, criterionID string) *evalv1.DeterministicGrade {
	for _, grade := range grades {
		if grade != nil && grade.GetCriterionId() == criterionID {
			return grade
		}
	}
	return nil
}
