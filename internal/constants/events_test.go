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

func TestNewTribunalEventConstants(t *testing.T) {
	cases := []struct {
		goConst EventType
		value   string
	}{
		{EventAiTribunalSessionStarted, "g8e.v1.ai.tribunal.session.started"},
		{EventAiTribunalSessionCompleted, "g8e.v1.ai.tribunal.session.completed"},
		{EventAiTribunalSessionDisabled, "g8e.v1.ai.tribunal.session.disabled"},
		{EventAiTribunalSessionGenerationFailed, "g8e.v1.ai.tribunal.session.generation.failed"},
		{EventAiTribunalSessionModelNotConfigured, "g8e.v1.ai.tribunal.session.model.not.configured"},
		{EventAiTribunalSessionProviderUnavailable, "g8e.v1.ai.tribunal.session.provider.unavailable"},
		{EventAiTribunalSessionSystemError, "g8e.v1.ai.tribunal.session.system.error"},
		{EventAiTribunalSessionAuditorFailed, "g8e.v1.ai.tribunal.session.auditor.failed"},
		{EventAiTribunalSessionWardenBlocked, "g8e.v1.ai.tribunal.session.warden.blocked"},
		{EventAiTribunalVotingPassCompleted, "g8e.v1.ai.tribunal.voting.pass.completed"},
		{EventAiTribunalVotingConsensusReached, "g8e.v1.ai.tribunal.voting.consensus.reached"},
		{EventAiTribunalVotingConsensusNotReached, "g8e.v1.ai.tribunal.voting.consensus.not.reached"},
		{EventAiTribunalVotingConsensusFailed, "g8e.v1.ai.tribunal.voting.consensus.failed"},
		{EventAiTribunalVotingRoundStarted, "g8e.v1.ai.tribunal.voting.round.started"},
		{EventAiTribunalVotingRoundCompleted, "g8e.v1.ai.tribunal.voting.round.completed"},
		{EventAiTribunalVotingRound2Started, "g8e.v1.ai.tribunal.voting.round.2.started"},
		{EventAiTribunalVotingRound2ConsensusReached, "g8e.v1.ai.tribunal.voting.round.2.consensus.reached"},
		{EventAiTribunalVotingRound2ConsensusFailed, "g8e.v1.ai.tribunal.voting.round.2.consensus.failed"},
		{EventAiTribunalVotingDissentRecorded, "g8e.v1.ai.tribunal.voting.dissent.recorded"},
		{EventAiTribunalVotingAuditStarted, "g8e.v1.ai.tribunal.voting.audit.started"},
		{EventAiTribunalVotingAuditCompleted, "g8e.v1.ai.tribunal.voting.audit.completed"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestOtherNewEventConstants(t *testing.T) {
	cases := []struct {
		goConst EventType
		value   string
	}{
		{EventAiLLMChatIterationThinkingStopped, "g8e.v1.ai.llm.chat.iteration.thinking.stopped"},
		{EventAiReputationStateUpdated, "g8e.v1.ai.reputation.state.updated"},
		{EventAppCaseDeleted, "g8e.v1.app.case.deleted"},
		{EventAppInvestigationDeleted, "g8e.v1.app.investigation.deleted"},
		{EventAppMemoryCreated, "g8e.v1.app.memory.created"},
		{EventAppMemoryUpdated, "g8e.v1.app.memory.updated"},
		{EventOperatorPortCheckRequested, "g8e.v1.operator.port.check.requested"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestOperatorIntentEventConstants(t *testing.T) {
	cases := []struct {
		goConst EventType
		value   string
	}{
		{EventOperatorIntentApprovalGranted, "g8e.v1.operator.intent.approval.granted"},
		{EventOperatorIntentApprovalRejected, "g8e.v1.operator.intent.approval.rejected"},
		{EventOperatorIntentApprovalRequested, "g8e.v1.operator.intent.approval.requested"},
		{EventOperatorIntentDenied, "g8e.v1.operator.intent.denied"},
		{EventOperatorIntentGranted, "g8e.v1.operator.intent.granted"},
		{EventOperatorIntentRequested, "g8e.v1.operator.intent.requested"},
		{EventOperatorIntentRevokeRequested, "g8e.v1.operator.intent.revoke.requested"},
		{EventOperatorIntentRevoked, "g8e.v1.operator.intent.revoked"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.value, string(tc.goConst))
	}
}

func TestEventsJSONValidity(t *testing.T) {
	data, err := os.ReadFile("../../protocol/constants/events.json")
	require.NoError(t, err)
	assert.True(t, json.Valid(data), "events.json is valid JSON")
}

func TestEventsJSONGoConstPresence(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile("../../protocol/constants/events.json")
	require.NoError(t, err)
	var raw struct {
		Events map[string]struct {
			GoConst     string `json:"_go_const"`
			PythonConst string `json:"_python_const"`
			Value       string `json:"value"`
		} `json:"events"`
	}
	require.NoError(t, json.Unmarshal(data, &raw))
	for key, meta := range raw.Events {
		assert.NotEmpty(t, meta.GoConst, "event %s missing _go_const", key)
		assert.NotEmpty(t, meta.Value, "event %s missing value", key)
	}
}
