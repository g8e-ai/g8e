// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestAppendUniqueStrings(t *testing.T) {
	values := appendUniqueStrings([]string{"a"}, "b")
	assert.Equal(t, []string{"a", "b"}, values)
	assert.Equal(t, []string{"a", "b"}, appendUniqueStrings(values, "b"))
}

func TestMaxUint32Ptr(t *testing.T) {
	first := maxUint32Ptr(nil, 5)
	requireUint32(t, 5, first)
	second := maxUint32Ptr(first, 3)
	requireUint32(t, 5, second)
	third := maxUint32Ptr(second, 9)
	requireUint32(t, 9, third)
}

func TestPublicVerdictStatus(t *testing.T) {
	assert.Equal(t, "pass", publicVerdictStatus(evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS))
	assert.Equal(t, "fail", publicVerdictStatus(evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL))
	assert.Equal(t, "unspecified", publicVerdictStatus(evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSPECIFIED))
}

func requireUint32(t *testing.T, want uint32, got *uint32) {
	t.Helper()
	if got == nil {
		t.Fatalf("expected %d, got nil", want)
	}
	assert.Equal(t, want, *got)
}
