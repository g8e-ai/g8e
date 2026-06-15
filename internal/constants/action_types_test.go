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

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllActionTypes(t *testing.T) {
	t.Run("returns non-nil slice", func(t *testing.T) {
		result := AllActionTypes()
		assert.NotNil(t, result)
	})

	t.Run("contains all expected action types", func(t *testing.T) {
		result := AllActionTypes()

		expectedTypes := []ActionType{
			ActionTypeA2aCall,
			ActionTypeCancel,
			ActionTypeEvalAnswer,
			ActionTypeExecuteBash,
			ActionTypeFetchFileDiff,
			ActionTypeFetchFileHistory,
			ActionTypeFetchHistory,
			ActionTypeFetchLogs,
			ActionTypeFileEdit,
			ActionTypeFsGrep,
			ActionTypeFsList,
			ActionTypeFsRead,
			ActionTypeGrantIntent,
			ActionTypeHeartbeat,
			ActionTypeInvestigationCreate,
			ActionTypeMcpCall,
			ActionTypeMcpPromptGet,
			ActionTypeMcpPromptList,
			ActionTypeMcpResourceList,
			ActionTypeMcpResourceRead,
			ActionTypePortCheck,
			ActionTypeRestoreFile,
			ActionTypeRevokeIntent,
			ActionTypeShutdown,
		}

		for _, expected := range expectedTypes {
			assert.Contains(t, result, expected, "AllActionTypes should contain %s", expected)
		}
	})

	t.Run("has correct length", func(t *testing.T) {
		result := AllActionTypes()
		assert.Len(t, result, 24, "AllActionTypes should return 24 action types")
	})

	t.Run("all values are unique", func(t *testing.T) {
		result := AllActionTypes()
		seen := make(map[ActionType]bool)
		for _, actionType := range result {
			assert.False(t, seen[actionType], "ActionType %s appears multiple times", actionType)
			seen[actionType] = true
		}
	})
}

func TestIsMutation(t *testing.T) {
	t.Run("returns true for mutation action types", func(t *testing.T) {
		mutationTypes := []ActionType{
			ActionTypeA2aCall,
			ActionTypeCancel,
			ActionTypeExecuteBash,
			ActionTypeFileEdit,
			ActionTypeMcpCall,
			ActionTypeRestoreFile,
			ActionTypeShutdown,
		}

		for _, actionType := range mutationTypes {
			assert.True(t, IsMutation(actionType), "%s should be a mutation", actionType)
		}
	})

	t.Run("returns false for non-mutation action types", func(t *testing.T) {
		nonMutationTypes := []ActionType{
			ActionTypeEvalAnswer,
			ActionTypeFetchFileDiff,
			ActionTypeFetchFileHistory,
			ActionTypeFetchHistory,
			ActionTypeFetchLogs,
			ActionTypeFsGrep,
			ActionTypeFsList,
			ActionTypeFsRead,
			ActionTypeGrantIntent,
			ActionTypeHeartbeat,
			ActionTypeInvestigationCreate,
			ActionTypeMcpPromptGet,
			ActionTypeMcpPromptList,
			ActionTypeMcpResourceList,
			ActionTypeMcpResourceRead,
			ActionTypePortCheck,
			ActionTypeRevokeIntent,
		}

		for _, actionType := range nonMutationTypes {
			assert.False(t, IsMutation(actionType), "%s should not be a mutation", actionType)
		}
	})

	t.Run("handles unknown action type", func(t *testing.T) {
		unknownType := ActionType("UNKNOWN_ACTION")
		assert.False(t, IsMutation(unknownType), "unknown action type should not be a mutation")
	})
}

func TestActionTypeConstants(t *testing.T) {
	t.Run("action type constants have correct string values", func(t *testing.T) {
		assert.Equal(t, "A2A_CALL", string(ActionTypeA2aCall))
		assert.Equal(t, "EVAL_ANSWER", string(ActionTypeEvalAnswer))
		assert.Equal(t, "EXECUTE_BASH", string(ActionTypeExecuteBash))
		assert.Equal(t, "FILE_EDIT", string(ActionTypeFileEdit))
		assert.Equal(t, "HEARTBEAT", string(ActionTypeHeartbeat))
	})

	t.Run("all action type constants are distinct", func(t *testing.T) {
		types := []ActionType{
			ActionTypeA2aCall,
			ActionTypeCancel,
			ActionTypeEvalAnswer,
			ActionTypeExecuteBash,
			ActionTypeFetchFileDiff,
			ActionTypeFetchFileHistory,
			ActionTypeFetchHistory,
			ActionTypeFetchLogs,
			ActionTypeFileEdit,
			ActionTypeFsGrep,
			ActionTypeFsList,
			ActionTypeFsRead,
			ActionTypeGrantIntent,
			ActionTypeHeartbeat,
			ActionTypeInvestigationCreate,
			ActionTypeMcpCall,
			ActionTypeMcpPromptGet,
			ActionTypeMcpPromptList,
			ActionTypeMcpResourceList,
			ActionTypeMcpResourceRead,
			ActionTypePortCheck,
			ActionTypeRestoreFile,
			ActionTypeRevokeIntent,
			ActionTypeShutdown,
		}

		seen := make(map[ActionType]bool)
		for _, actionType := range types {
			assert.False(t, seen[actionType], "ActionType %s is duplicated", actionType)
			seen[actionType] = true
		}
	})
}
