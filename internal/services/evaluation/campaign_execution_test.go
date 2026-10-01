// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildCampaignChatRequest_MessageIncludesInlineContext guards against the
// homogeneous-lane message builder silently dropping InlineContext (as it did
// when the field was named Attachments but was never read): a scenario whose
// prompt says content is provided "below" must actually carry that content in
// the message sent to the model.
func TestBuildCampaignChatRequest_MessageIncludesInlineContext(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	req.ScenarioInput.UserPrompt = "Identify the failing service named in the synthetic log excerpt below."
	req.ScenarioInput.InlineContext = []ScenarioInlineContent{
		syntheticInlineContent("log", "synthetic-service-log", "2026-09-16T08:05:11Z ERROR service=checkout-api upstream=payments.internal.example reason=timeout"),
	}

	chatReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{GradingMethod: req.GradingMethod}, nil)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(chatReq.Message, req.ScenarioInput.UserPrompt))
	assert.Contains(t, chatReq.Message, "service=checkout-api")
	assert.Contains(t, chatReq.Message, "synthetic-service-log")
}

func TestBuildCampaignChatRequest_MessageIsBarePromptWithoutInlineContext(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")

	chatReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{GradingMethod: req.GradingMethod}, nil)
	require.NoError(t, err)

	assert.Equal(t, req.ScenarioInput.UserPrompt, chatReq.Message)
}

// TestBuildCampaignChatRequest_RendersWorkspaceIntoMessageAndSeed guards the
// attempt-scoped workspace contract: the token in the prompt, inline content,
// and every seed field renders to the workspace root, and the request echoes
// the workspace so the trace can prove which one the model saw.
func TestBuildCampaignChatRequest_RendersWorkspaceIntoMessageAndSeed(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	ws, err := NewScenarioWorkspace(req.Binding.DataOperatorWorkingDirectory, req.Assignment.GetRunId(), req.AttemptID)
	require.NoError(t, err)
	req.ScenarioInput.UserPrompt = "Read " + ScenarioWorkspaceToken + "/config/retry-config.env and report retry_limit."
	req.ScenarioInput.InlineContext = []ScenarioInlineContent{
		syntheticInlineContent("log", "synthetic-log", "see "+ScenarioWorkspaceToken+"/logs/app.log"),
	}
	req.ScenarioInput.Seed = InvestigationSeed{
		CaseTitle: "Retry tuning for checkout",
		Turns:     []InvestigationSeedTurn{{Sender: "user", Content: "Look in " + ScenarioWorkspaceToken + "/config first."}},
		HistoryEvents: []InvestigationSeedHistoryEvent{{
			EventType: "g8e.v1.operator.filesystem.grep.failed",
			Actor:     "system",
			Summary:   "grep failed under " + ScenarioWorkspaceToken,
			Command:   "grep -r retry " + ScenarioWorkspaceToken,
		}},
	}

	chatReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{GradingMethod: req.GradingMethod}, &ws)
	require.NoError(t, err)

	assert.NotContains(t, chatReq.Message, ScenarioWorkspaceToken)
	assert.Contains(t, chatReq.Message, "Read "+ws.Root+"/config/retry-config.env")
	assert.Contains(t, chatReq.Message, "see "+ws.Root+"/logs/app.log")
	require.Same(t, &ws, chatReq.Workspace)
	seed := chatReq.Seed
	require.NotNil(t, seed)
	assert.Equal(t, "Look in "+ws.Root+"/config first.", seed.Turns[0].Content)
	assert.Equal(t, "grep failed under "+ws.Root, seed.HistoryEvents[0].Summary)
	assert.Equal(t, "grep -r retry "+ws.Root, seed.HistoryEvents[0].Command)
}

// TestBuildCampaignChatRequest_SendsNoSeedForFixtureWithoutOne keeps catalog
// 1.0.0 fixtures, which predate seeds, verifiable: the request carries no seed,
// so a trace is not required to echo one.
func TestBuildCampaignChatRequest_SendsNoSeedForFixtureWithoutOne(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")

	chatReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{GradingMethod: req.GradingMethod}, nil)
	require.NoError(t, err)

	assert.Nil(t, chatReq.Seed)
}
