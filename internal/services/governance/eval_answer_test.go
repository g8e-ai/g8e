// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package governance

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"

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
