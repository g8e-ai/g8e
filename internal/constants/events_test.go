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

func TestNewConsensusEventConstants(t *testing.T) {
	cases := []struct {
		goConst EventType
		value   string
	}{
		{EventAiConsensusSessionStarted, "g8e.v1.ai.consensus.session.started"},
		{EventAiConsensusSessionCompleted, "g8e.v1.ai.consensus.session.completed"},
		{EventAiConsensusSessionDisabled, "g8e.v1.ai.consensus.session.disabled"},
		{EventAiConsensusSessionGenerationFailed, "g8e.v1.ai.consensus.session.generation.failed"},
		{EventAiConsensusSessionModelNotConfigured, "g8e.v1.ai.consensus.session.model.not.configured"},
		{EventAiConsensusSessionProviderUnavailable, "g8e.v1.ai.consensus.session.provider.unavailable"},
		{EventAiConsensusSessionSystemError, "g8e.v1.ai.consensus.session.system.error"},
		{EventAiConsensusSessionAuditorFailed, "g8e.v1.ai.consensus.session.auditor.failed"},
		{EventAiConsensusSessionWardenBlocked, "g8e.v1.ai.consensus.session.warden.blocked"},
		{EventAiConsensusVotingPassCompleted, "g8e.v1.ai.consensus.voting.pass.completed"},
		{EventAiConsensusVotingConsensusReached, "g8e.v1.ai.consensus.voting.consensus.reached"},
		{EventAiConsensusVotingConsensusNotReached, "g8e.v1.ai.consensus.voting.consensus.not.reached"},
		{EventAiConsensusVotingConsensusFailed, "g8e.v1.ai.consensus.voting.consensus.failed"},
		{EventAiConsensusVotingRoundStarted, "g8e.v1.ai.consensus.voting.round.started"},
		{EventAiConsensusVotingRoundCompleted, "g8e.v1.ai.consensus.voting.round.completed"},
		{EventAiConsensusVotingRound2Started, "g8e.v1.ai.consensus.voting.round.2.started"},
		{EventAiConsensusVotingRound2ConsensusReached, "g8e.v1.ai.consensus.voting.round.2.consensus.reached"},
		{EventAiConsensusVotingRound2ConsensusFailed, "g8e.v1.ai.consensus.voting.round.2.consensus.failed"},
		{EventAiConsensusVotingDissentRecorded, "g8e.v1.ai.consensus.voting.dissent.recorded"},
		{EventAiConsensusVotingAuditStarted, "g8e.v1.ai.consensus.voting.audit.started"},
		{EventAiConsensusVotingAuditCompleted, "g8e.v1.ai.consensus.voting.audit.completed"},
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
