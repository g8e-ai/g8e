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
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statusEntry struct {
	GoConst     string `json:"_go_const"`
	PythonConst string `json:"_python_const"`
	Value       string `json:"value"`
}

// loadStatusJSON loads protocol/constants/status.json for cross-checking Go constants.
func loadStatusJSON(t *testing.T) map[string]map[string]statusEntry {
	t.Helper()
	data, err := os.ReadFile("../../protocol/constants/status.json")
	require.NoError(t, err)
	var raw struct {
		Status map[string]map[string]statusEntry `json:"status"`
	}
	require.NoError(t, json.Unmarshal(data, &raw))
	return raw.Status
}

func TestComponentStatusConstants(t *testing.T) {
	cases := []struct {
		goConst ComponentStatus
		value   string
	}{
		{ComponentStatusActive, "active"},
		{ComponentStatusError, "error"},
		{ComponentStatusInactive, "inactive"},
		{ComponentStatusMaintenance, "maintenance"},
		{ComponentStatusDegraded, "degraded"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestWorkflowTypeConstants(t *testing.T) {
	cases := []struct {
		goConst WorkflowType
		value   string
	}{
		{WorkflowTypeG8eBound, "g8e.bound"},
		{WorkflowTypeG8eCloudBound, "g8e.cloud.bound"},
		{WorkflowTypeG8eNotBound, "g8e.not.bound"},
		{WorkflowTypeTriage, "triage"},
		{WorkflowTypeInvestigation, "investigation"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestVersionStabilityExtendedConstants(t *testing.T) {
	assert.Equal(t, "unstable", string(VersionStabilityUnstable))
	assert.Equal(t, "deprecated", string(VersionStabilityDeprecated))
}

func TestAITaskIdConstants(t *testing.T) {
	cases := []struct {
		goConst AITaskId
		value   string
	}{
		{AITaskIDAgentContinue, "ai.agent.continue"},
		{AITaskIDChat, "ai.chat"},
		{AITaskIDCommand, "ai.command"},
		{AITaskIDDirectCommand, "ai.direct.command"},
		{AITaskIDFileEdit, "ai.file.edit"},
		{AITaskIDFsList, "ai.fs.list"},
		{AITaskIDPortCheck, "ai.port.check"},
		{AITaskIDIntentGrant, "ai.intent.grant"},
		{AITaskIDIntentRevoke, "ai.intent.revoke"},
		{AITaskIDRecursiveGrep, "ai.recursive_grep"},
		{AITaskIDRestoreFile, "ai.restore.file"},
		{AITaskIDFetchFileDiff, "ai.fetch.file.diff"},
		{AITaskIDFetchFileHistory, "ai.fetch.file.history"},
		{AITaskIDFetchHistory, "ai.fetch.history"},
		{AITaskIDFetchLogs, "ai.fetch.logs"},
		{AITaskIDFsRead, "ai.fs.read"},
		{AITaskIdChat, "ai.chat"},
		{AITaskIdCase, "ai.case"},
		{AITaskIdMemory, "ai.memory"},
		{AITaskIdCommand, "ai.command"},
		{AITaskIdCommandExecution, "ai.command.execution"},
		{AITaskIdDirectCommand, "ai.direct.command"},
		{AITaskIdIntentGrant, "ai.intent.grant"},
		{AITaskIdIntentRevoke, "ai.intent.revoke"},
		{AITaskIdFileEdit, "ai.file.edit"},
		{AITaskIdFileOperation, "ai.file.operation"},
		{AITaskIdFsList, "ai.fs.list"},
		{AITaskIdRecursiveGrep, "ai.recursive.grep"},
		{AITaskIdPortCheck, "ai.port.check"},
		{AITaskIdAgentContinue, "ai.agent.continue"},
		{AITaskIdInvestigationQuery, "ai.investigation.query"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestTribunalMemberConstants(t *testing.T) {
	cases := []struct {
		goConst TribunalMember
		value   string
	}{
		{TribunalMemberAxiom, "axiom"},
		{TribunalMemberConcord, "concord"},
		{TribunalMemberVariance, "variance"},
		{TribunalMemberPragma, "pragma"},
		{TribunalMemberNemesis, "nemesis"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestTribunalAuditModeConstants(t *testing.T) {
	assert.Equal(t, "unanimous", string(TribunalAuditModeUnanimous))
	assert.Equal(t, "majority", string(TribunalAuditModeMajority))
	assert.Equal(t, "tied", string(TribunalAuditModeTied))
}

func TestTribunalAuditStatusConstants(t *testing.T) {
	assert.Equal(t, "ok", string(TribunalAuditStatusOk))
	assert.Equal(t, "revised", string(TribunalAuditStatusRevised))
	assert.Equal(t, "swap", string(TribunalAuditStatusSwap))
}

func TestAuditorReasonConstants(t *testing.T) {
	cases := []struct {
		goConst AuditorReason
		value   string
	}{
		{AuditorReasonOk, "ok"},
		{AuditorReasonRevised, "revised"},
		{AuditorReasonRevisedFromDissent, "revised_from_dissent"},
		{AuditorReasonSwappedToDissenter, "swapped_to_dissenter"},
		{AuditorReasonWhitelistViolation, "whitelist_violation"},
		{AuditorReasonNoValidRevision, "no_valid_revision"},
		{AuditorReasonAuditorError, "auditor_error"},
		{AuditorReasonEmptyResponse, "empty_response"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestTieBreakReasonConstants(t *testing.T) {
	assert.Equal(t, "shortest", string(TieBreakReasonShortest))
	assert.Equal(t, "excluded_nemesis", string(TieBreakReasonExcludedNemesis))
}

func TestReasoningAgentConstants(t *testing.T) {
	assert.Equal(t, "sage", string(ReasoningAgentSage))
	assert.Equal(t, "dash", string(ReasoningAgentDash))
}

func TestErrorCodeConstants(t *testing.T) {
	cases := []struct {
		goConst ErrorCode
		value   string
	}{
		{ErrorCodeGenericError, "G8E-1000"},
		{ErrorCodeUnexpectedError, "G8E-1001"},
		{ErrorCodeNotImplemented, "G8E-1002"},
		{ErrorCodeConfigError, "G8E-1100"},
		{ErrorCodeMissingEnvVar, "G8E-1101"},
		{ErrorCodeInvalidSettings, "G8E-1102"},
		{ErrorCodeServiceInitError, "G8E-1103"},
		{ErrorCodeAuthError, "G8E-1200"},
		{ErrorCodeTokenExpired, "G8E-1201"},
		{ErrorCodeInvalidToken, "G8E-1202"},
		{ErrorCodeInsufficientPermissions, "G8E-1203"},
		{ErrorCodeDBConnectionError, "G8E-1300"},
		{ErrorCodeDBQueryError, "G8E-1301"},
		{ErrorCodeDBDocumentNotFound, "G8E-1302"},
		{ErrorCodeDBWriteError, "G8E-1303"},
		{ErrorCodeDBTransactionError, "G8E-1304"},
		{ErrorCodePubSubConnectionError, "G8E-1400"},
		{ErrorCodePubSubPublishError, "G8E-1401"},
		{ErrorCodePubSubSubscribeError, "G8E-1402"},
		{ErrorCodePubSubTopicError, "G8E-1403"},
		{ErrorCodeStorageConnectionError, "G8E-1500"},
		{ErrorCodeStorageReadError, "G8E-1501"},
		{ErrorCodeStorageWriteError, "G8E-1502"},
		{ErrorCodeStorageDeleteError, "G8E-1503"},
		{ErrorCodeAPIConnectionError, "G8E-1600"},
		{ErrorCodeAPITimeoutError, "G8E-1601"},
		{ErrorCodeAPIResponseError, "G8E-1602"},
		{ErrorCodeAPIRequestError, "G8E-1603"},
		{ErrorCodeAPIRateLimitError, "G8E-1604"},
		{ErrorCodeGenericNotFound, "G8E-1605"},
		{ErrorCodeExternalServiceError, "G8E-1607"},
		{ErrorCodeValidationError, "G8E-1700"},
		{ErrorCodeMissingRequiredField, "G8E-1701"},
		{ErrorCodeInvalidFieldFormat, "G8E-1702"},
		{ErrorCodeInvalidFieldValue, "G8E-1703"},
		{ErrorCodeInvalidFieldType, "G8E-1704"},
		{ErrorCodeSchemaValidationFailed, "G8E-1705"},
		{ErrorCodeSchemaNotFound, "G8E-1706"},
		{ErrorCodeInvalidInput, "G8E-1707"},
		{ErrorCodeBusinessLogicError, "G8E-1800"},
		{ErrorCodeWorkflowError, "G8E-1801"},
		{ErrorCodeStateTransitionError, "G8E-1802"},
		{ErrorCodeResourceConflict, "G8E-1803"},
		{ErrorCodeTaskCreationFailed, "G8E-1804"},
		{ErrorCodeOperationFailed, "G8E-1805"},
		{ErrorCodeGovernanceRejected, "G8E-1806"},
		{ErrorCodeModelCapabilityUnsupported, "G8E-1807"},
		{ErrorCodeServiceUnavailableError, "G8E-1900"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestErrorCategoryConstants(t *testing.T) {
	cases := []struct {
		goConst ErrorCategory
		value   string
	}{
		{ErrorCategoryValidation, "validation"},
		{ErrorCategoryBusinessLogic, "business_logic"},
		{ErrorCategoryConfiguration, "configuration"},
		{ErrorCategoryAuth, "auth"},
		{ErrorCategoryPermission, "permission"},
		{ErrorCategoryResourceNotFound, "resource_not_found"},
		{ErrorCategoryConflict, "conflict"},
		{ErrorCategoryRateLimit, "rate_limit"},
		{ErrorCategoryServiceUnavailable, "service_unavailable"},
		{ErrorCategoryExternalService, "external_service"},
		{ErrorCategoryTimeout, "timeout"},
		{ErrorCategoryDatabase, "database"},
		{ErrorCategoryNetwork, "network"},
		{ErrorCategoryPubSub, "pubsub"},
		{ErrorCategoryStorage, "storage"},
		{ErrorCategoryInternal, "internal"},
		{ErrorCategoryDependency, "dependency"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestErrorSeverityConstants(t *testing.T) {
	cases := []struct {
		goConst ErrorSeverity
		value   string
	}{
		{ErrorSeverityLow, "low"},
		{ErrorSeverityMedium, "medium"},
		{ErrorSeverityHigh, "high"},
		{ErrorSeverityCritical, "critical"},
		{ErrorSeverityInfo, "info"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestStatusJSONValidity(t *testing.T) {
	data, err := os.ReadFile("../../protocol/constants/status.json")
	require.NoError(t, err)
	assert.True(t, json.Valid(data), "status.json is valid JSON")
}

func TestStatusJSONGoConstPresence(t *testing.T) {
	statusJSON := loadStatusJSON(t)
	for catName, entries := range statusJSON {
		for key, meta := range entries {
			if key == "" {
				continue
			}
			assert.NotEmpty(t, meta.GoConst,
				"category %s key %s missing _go_const", catName, key)
			assert.NotEmpty(t, meta.Value,
				"category %s key %s missing value", catName, key)
		}
	}
}

func TestStatusJSONComponentStatusMatches(t *testing.T) {
	statusJSON := loadStatusJSON(t)
	cat, ok := statusJSON["component_status"]
	require.True(t, ok, "component_status category exists")
	expected := map[string]string{
		"active":      "active",
		"error":       "error",
		"inactive":    "inactive",
		"maintenance": "maintenance",
		"degraded":    "degraded",
	}
	for key, exp := range expected {
		entry, exists := cat[key]
		require.True(t, exists, "component_status key %s exists in JSON", key)
		assert.Equal(t, exp, entry.Value)
	}
}

func TestStatusJSONTribunalMemberMatches(t *testing.T) {
	statusJSON := loadStatusJSON(t)
	cat, ok := statusJSON["tribunal_member"]
	require.True(t, ok, "tribunal_member category exists")
	expected := map[string]string{
		"axiom":    "axiom",
		"concord":  "concord",
		"variance": "variance",
		"pragma":   "pragma",
		"nemesis":  "nemesis",
	}
	for key, exp := range expected {
		entry, exists := cat[key]
		require.True(t, exists, "tribunal_member key %s exists in JSON", key)
		assert.Equal(t, exp, entry.Value)
	}
}
