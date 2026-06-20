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

// ActionType constants are manually maintained Go constants that mirror the
// canonical values in protocol/constants/status.json (SSOT).
//
// These constants are verified by contract tests in internal/contracts/protocol_constants_test.go
// to ensure they remain in sync with the JSON source of truth.
//
// Adding new action types:
// 1. Add to protocol/constants/status.json under status.action_type
// 2. Add corresponding constant here
// 3. Run contract tests to verify alignment

// ActionType is a typed string for action types.
type ActionType string

const (
	ActionTypeA2aCall             ActionType = "A2A_CALL"
	ActionTypeCancel              ActionType = "CANCEL"
	ActionTypeEvalAnswer          ActionType = "EVAL_ANSWER"
	ActionTypeExecuteBash         ActionType = "EXECUTE_BASH"
	ActionTypeFetchFileDiff       ActionType = "FETCH_FILE_DIFF"
	ActionTypeFetchFileHistory    ActionType = "FETCH_FILE_HISTORY"
	ActionTypeFetchHistory        ActionType = "FETCH_HISTORY"
	ActionTypeFetchLogs           ActionType = "FETCH_LOGS"
	ActionTypeFileEdit            ActionType = "FILE_EDIT"
	ActionTypeFsGrep              ActionType = "FS_GREP"
	ActionTypeFsList              ActionType = "FS_LIST"
	ActionTypeFsRead              ActionType = "FS_READ"
	ActionTypeGrantIntent         ActionType = "GRANT_INTENT"
	ActionTypeHeartbeat           ActionType = "HEARTBEAT"
	ActionTypeInvestigationCreate ActionType = "INVESTIGATION_CREATE"
	ActionTypeMcpCall             ActionType = "MCP_CALL"
	ActionTypeMcpPromptGet        ActionType = "MCP_PROMPT_GET"
	ActionTypeMcpPromptList       ActionType = "MCP_PROMPT_LIST"
	ActionTypeMcpResourceList     ActionType = "MCP_RESOURCE_LIST"
	ActionTypeMcpResourceRead     ActionType = "MCP_RESOURCE_READ"
	ActionTypeMigrationTransfer   ActionType = "MIGRATION_TRANSFER"
	ActionTypePortCheck           ActionType = "PORT_CHECK"
	ActionTypeRestoreFile         ActionType = "RESTORE_FILE"
	ActionTypeRevokeIntent        ActionType = "REVOKE_INTENT"
	ActionTypeShutdown            ActionType = "SHUTDOWN"
)

// AllActionTypes is the canonical slice of all valid action types.
// Verified by contract tests against protocol/constants/status.json.
var AllActionTypes = []ActionType{
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
	ActionTypeMigrationTransfer,
	ActionTypePortCheck,
	ActionTypeRestoreFile,
	ActionTypeRevokeIntent,
	ActionTypeShutdown,
}

// IsMutation returns true if the action type modifies system state.
// Must match the "_mutation": true flag in protocol/constants/status.json.
func (a ActionType) IsMutation() bool {
	switch a {
	case ActionTypeA2aCall,
		ActionTypeCancel,
		ActionTypeExecuteBash,
		ActionTypeFileEdit,
		ActionTypeMcpCall,
		ActionTypeMigrationTransfer,
		ActionTypeRestoreFile,
		ActionTypeShutdown:
		return true
	default:
		return false
	}
}
