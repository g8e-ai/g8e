// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"

	"github.com/stretchr/testify/assert"
)

// TestEvalAnswerIsNotMutation verifies that EVAL_ANSWER is not treated as a mutation
// and does not require L3 verification.
func TestEvalAnswerIsNotMutation(t *testing.T) {
	t.Parallel()
	verifier := &L4Warden{}

	assert.False(t, verifier.isMutation(constants.ActionTypeEvalAnswer), "EVAL_ANSWER should not be treated as a mutation")

	// Verify that actual mutations are still detected
	assert.True(t, verifier.isMutation(constants.ActionTypeExecuteBash), "EXECUTE_BASH should be treated as a mutation")
	assert.True(t, verifier.isMutation(constants.ActionTypeFileEdit), "FILE_EDIT should be treated as a mutation")
}
