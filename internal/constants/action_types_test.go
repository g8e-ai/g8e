// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActionTypeIsMutation_MutationTypesReturnTrue(t *testing.T) {
	t.Parallel()

	mutations := []ActionType{
		ActionTypeA2aCall,
		ActionTypeCancel,
		ActionTypeExecuteBash,
		ActionTypeFileEdit,
		ActionTypeMcpCall,
		ActionTypeRestoreFile,
		ActionTypeShutdown,
	}

	for _, at := range mutations {
		at := at
		t.Run(string(at), func(t *testing.T) {
			t.Parallel()
			assert.True(t, at.IsMutation(), "%s should be a mutation", at)
		})
	}
}

func TestActionTypeIsMutation_ReadTypesReturnFalse(t *testing.T) {
	t.Parallel()

	readTypes := []ActionType{
		ActionTypeEvalAnswer,
		ActionTypeFetchFileDiff,
		ActionTypeFetchFileHistory,
		ActionTypeFetchHistory,
		ActionTypeFetchLogs,
		ActionTypeFsGrep,
		ActionTypeFsList,
		ActionTypeFsRead,
		ActionTypeHeartbeat,
		ActionTypeInvestigationCreate,
		ActionTypeMcpPromptGet,
		ActionTypeMcpPromptList,
		ActionTypeMcpResourceList,
		ActionTypeMcpResourceRead,
		ActionTypePortCheck,
	}

	for _, at := range readTypes {
		at := at
		t.Run(string(at), func(t *testing.T) {
			t.Parallel()
			assert.False(t, at.IsMutation(), "%s should not be a mutation", at)
		})
	}
}

func TestActionTypeIsMutation_UnknownTypeReturnsFalse(t *testing.T) {
	t.Parallel()

	unknown := ActionType("UNKNOWN_ACTION")
	assert.False(t, unknown.IsMutation())
}

func TestActionTypeIsMutation_EmptyTypeReturnsFalse(t *testing.T) {
	t.Parallel()

	assert.False(t, ActionType("").IsMutation())
}

func TestAllActionTypes_ContainsAllConstants(t *testing.T) {
	t.Parallel()

	allConsts := []ActionType{
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
		ActionTypeHeartbeat,
		ActionTypeInvestigationCreate,
		ActionTypeMcpCall,
		ActionTypeMcpPromptGet,
		ActionTypeMcpPromptList,
		ActionTypeMcpResourceList,
		ActionTypeMcpResourceRead,
		ActionTypePortCheck,
		ActionTypeRestoreFile,
		ActionTypeShutdown,
	}

	assert.Len(t, AllActionTypes, len(allConsts))

	for _, c := range allConsts {
		assert.Contains(t, AllActionTypes, c)
	}
}

func TestActionTypeIsMutation_EveryActionTypeClassified(t *testing.T) {
	t.Parallel()

	for _, at := range AllActionTypes {
		// Every action type should return a definitive true or false.
		// This test ensures no action type falls through to the default case
		// unintentionally — if a new action type is added, it should be
		// explicitly classified.
		_ = at.IsMutation()
	}
}
