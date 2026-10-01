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
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestBuildCampaignChatRequest_RejectsRequestsItCannotSendFaithfully(t *testing.T) {
	t.Parallel()
	build := func(mutate func(req *AssignmentExecutionRequest)) error {
		req := homogeneousAssignmentExecutionRequest(t, "primary")
		mutate(&req)
		_, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{}, nil)
		return err
	}
	tests := []struct {
		name    string
		mutate  func(req *AssignmentExecutionRequest)
		wantIs  error
		wantMsg string
	}{
		{name: "no assignment", mutate: func(req *AssignmentExecutionRequest) { req.Assignment = nil }, wantIs: constants.ErrMissingRequiredField},
		{name: "no attempt id", mutate: func(req *AssignmentExecutionRequest) { req.AttemptID = "" }, wantIs: constants.ErrMissingRequiredField},
		{name: "no inference operator session", mutate: func(req *AssignmentExecutionRequest) { req.Binding.InferenceOperatorSessionID = "" }, wantIs: constants.ErrMissingRequiredField},
		{name: "no data operator", mutate: func(req *AssignmentExecutionRequest) { req.Binding.DataOperatorID = "" }, wantIs: constants.ErrMissingRequiredField},
		{name: "no data operator session", mutate: func(req *AssignmentExecutionRequest) { req.Binding.DataOperatorSessionID = "" }, wantIs: constants.ErrMissingRequiredField},
		{
			name: "a heterogeneous assignment belongs to the formation runner",
			mutate: func(req *AssignmentExecutionRequest) {
				req.Assignment.Target = &evalv1.EvaluationAssignment_Heterogeneous{Heterogeneous: &evalv1.HeterogeneousAssignmentTarget{Stack: &evalv1.HeterogeneousStackDefinition{StackId: "stack-1"}}}
			},
			wantMsg: "heterogeneous assignments execute through formation runner",
		},
		{name: "no target", mutate: func(req *AssignmentExecutionRequest) { req.Assignment.Target = nil }, wantMsg: "homogeneous target required"},
		{
			name: "no candidate variant",
			mutate: func(req *AssignmentExecutionRequest) {
				req.Assignment.Target = &evalv1.EvaluationAssignment_Homogeneous{Homogeneous: &evalv1.HomogeneousAssignmentTarget{}}
			},
			wantMsg: "homogeneous target required",
		},
		{
			name: "an unspecified designated role",
			mutate: func(req *AssignmentExecutionRequest) {
				req.Assignment.GetHomogeneous().DesignatedRole = evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_UNSPECIFIED
			},
			wantMsg: "unsupported model campaign role",
		},
		{name: "no prompt", mutate: func(req *AssignmentExecutionRequest) { req.ScenarioInput.UserPrompt = "" }, wantMsg: "missing user prompt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := build(tt.mutate)
			require.Error(t, err)
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}
			if tt.wantMsg != "" {
				assert.Contains(t, err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestBuildCampaignChatRequest_BindsTheModelRoleAndOperatorIdentity(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"primary", "assistant", "lite"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			req := homogeneousAssignmentExecutionRequest(t, role)

			chatReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{GradingMethod: req.GradingMethod}, nil)
			require.NoError(t, err)

			assert.Equal(t, "assignment-1", chatReq.AssignmentID)
			assert.Equal(t, "attempt-1", chatReq.EvaluationAttemptID)
			assert.Equal(t, "campaign-1", chatReq.CampaignID)
			assert.Equal(t, "run-1", chatReq.RunID)
			assert.Equal(t, "instruction-exact-format", chatReq.ScenarioID)
			assert.Equal(t, "model-a", chatReq.Model)
			assert.Equal(t, "a"+repeatHex('a', 63), chatReq.ModelDigest)
			assert.Equal(t, "session-1", chatReq.TargetOperatorSessionID, "scored inference is pinned to the inference operator session")
			assert.Equal(t, req.Binding.ModelRegistryDigest, chatReq.ModelRegistryDigest)
			assert.Equal(t, "model_role", chatReq.EvaluationLane)
			assert.Equal(t, role, chatReq.DesignatedModelRole)
			assert.Nil(t, chatReq.Workspace, "no workspace unless the caller supplies one")
			assert.Nil(t, chatReq.Seed)
		})
	}
}

func TestBuildCampaignChatRequest_GoldSummaryIsPrivateToSemanticJudgeScenarios(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	ws := mustWorkspace(t, "/home/operator", "run-1", "attempt-1")
	req.ScenarioInput.UserPrompt = "Read " + ScenarioWorkspaceToken + "/a.txt and explain."
	grading := CampaignChatGradingContext{
		GradingMethod:    evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE,
		ScenarioGold:     ScenarioGoldCriteria{ExpectedBehavior: "Explain the missing file."},
		ScenarioTools:    ScenarioToolExpectations{ExpectedTools: []string{toolRead}, ForbiddenTools: []string{toolRun}},
		RequiredConcepts: []string{"missing", "file"},
	}

	judged, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, grading, &ws)
	require.NoError(t, err)
	require.NotNil(t, judged.GoldSummary)
	assert.Equal(t, "Read "+ws.Root+"/a.txt and explain.", judged.GoldSummary.UserPrompt, "the judge sees the prompt exactly as the model did")
	assert.Equal(t, "Explain the missing file.", judged.GoldSummary.ExpectedBehavior)
	assert.Equal(t, []string{"missing", "file"}, judged.GoldSummary.RequiredConcepts)
	assert.Equal(t, []string{toolRead}, judged.GoldSummary.ExpectedTools)
	assert.Equal(t, []string{toolRun}, judged.GoldSummary.ForbiddenTools)

	grading.GradingMethod = evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC
	deterministic, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, grading, &ws)
	require.NoError(t, err)
	assert.Nil(t, deterministic.GoldSummary, "a deterministic scenario never sends gold to g8ee")

	t.Run("empty gold lists are non-nil so they marshal as arrays", func(t *testing.T) {
		t.Parallel()
		summary := buildChatProbeGoldSummary("m", CampaignChatGradingContext{GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE})
		require.NotNil(t, summary)
		assert.NotNil(t, summary.RequiredConcepts)
		assert.NotNil(t, summary.ExpectedTools)
		assert.NotNil(t, summary.ForbiddenTools)
		assert.Empty(t, summary.ExpectedTools)
	})
}

func TestBuildHarnessInvestigationSeed(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/home/operator", "run-1", "attempt-1")
	token := ScenarioWorkspaceToken
	full := InvestigationSeed{
		CaseTitle:       "Checkout payment timeouts",
		CaseDescription: "Logs live in " + token + "/logs.",
		Turns: []InvestigationSeedTurn{
			{Sender: "user", Content: "Look in " + token},
			{Sender: "primary", Content: "No tokens here."},
		},
		HistoryEvents: []InvestigationSeedHistoryEvent{{
			EventType:     "g8e.v1.operator.filesystem.grep.failed",
			Actor:         "system",
			Summary:       "grep under " + token,
			ToolName:      toolGrep,
			ExecutionID:   "exec-1",
			ArgumentsJSON: `{"path":"` + token + `"}`,
			Command:       "grep -r x " + token,
			Error:         "no such directory " + token,
			ErrorType:     "execution.error",
		}},
		CaseMemory: &InvestigationSeedMemory{
			InvestigationSummary:     "Summary " + token,
			CommunicationPreferences: "Prefers " + token,
			TechnicalBackground:      "Background " + token,
			ResponseStyle:            "Style " + token,
			ProblemSolvingApproach:   "Approach " + token,
			InteractionStyle:         "Interaction " + token,
		},
	}

	t.Run("a nil seed sends none", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, buildHarnessInvestigationSeed(nil, &ws))
	})
	t.Run("a fixture with no case title sends none, whatever else it holds", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, buildHarnessInvestigationSeed(&InvestigationSeed{Turns: full.Turns}, &ws))
	})
	t.Run("every string renders through the workspace", func(t *testing.T) {
		t.Parallel()
		seed := buildHarnessInvestigationSeed(&full, &ws)
		require.NotNil(t, seed)
		assert.Equal(t, "Checkout payment timeouts", seed.CaseTitle)
		assert.Equal(t, "Logs live in "+ws.Root+"/logs.", seed.CaseDescription)
		require.Len(t, seed.Turns, 2)
		assert.Equal(t, harnessclient.EnsembleSeedTurn{Sender: "user", Content: "Look in " + ws.Root}, seed.Turns[0])
		assert.Equal(t, "No tokens here.", seed.Turns[1].Content)
		require.Len(t, seed.HistoryEvents, 1)
		assert.Equal(t, harnessclient.EnsembleSeedHistoryEvent{
			EventType:     "g8e.v1.operator.filesystem.grep.failed",
			Actor:         "system",
			Summary:       "grep under " + ws.Root,
			ToolName:      toolGrep,
			ExecutionID:   "exec-1",
			ArgumentsJSON: `{"path":"` + ws.Root + `"}`,
			Command:       "grep -r x " + ws.Root,
			Error:         "no such directory " + ws.Root,
			ErrorType:     "execution.error",
		}, seed.HistoryEvents[0])
		require.NotNil(t, seed.CaseMemory)
		assert.Equal(t, harnessclient.EnsembleSeedMemory{
			InvestigationSummary:     "Summary " + ws.Root,
			CommunicationPreferences: "Prefers " + ws.Root,
			TechnicalBackground:      "Background " + ws.Root,
			ResponseStyle:            "Style " + ws.Root,
			ProblemSolvingApproach:   "Approach " + ws.Root,
			InteractionStyle:         "Interaction " + ws.Root,
		}, *seed.CaseMemory)
	})
	t.Run("without a workspace the text is copied verbatim", func(t *testing.T) {
		t.Parallel()
		seed := buildHarnessInvestigationSeed(&full, nil)
		require.NotNil(t, seed)
		assert.Equal(t, "Look in "+token, seed.Turns[0].Content)
		assert.Equal(t, "Summary "+token, seed.CaseMemory.InvestigationSummary)
		assert.Equal(t, "no such directory "+token, seed.HistoryEvents[0].Error)
	})
	t.Run("a seed with only a title has no turns, events, or memory", func(t *testing.T) {
		t.Parallel()
		seed := buildHarnessInvestigationSeed(&InvestigationSeed{CaseTitle: "Release readiness check"}, &ws)
		require.NotNil(t, seed)
		assert.Empty(t, seed.Turns)
		assert.Empty(t, seed.HistoryEvents)
		assert.Nil(t, seed.CaseMemory)
	})
	t.Run("the request seed does not alias the frozen fixture", func(t *testing.T) {
		t.Parallel()
		fixture := full
		seed := buildHarnessInvestigationSeed(&fixture, &ws)
		seed.Turns[0].Content = "mutated"
		seed.CaseMemory.ResponseStyle = "mutated"
		assert.Equal(t, "Look in "+token, fixture.Turns[0].Content)
		assert.Equal(t, "Style "+token, fixture.CaseMemory.ResponseStyle)
	})
}

func TestRenderScenarioMessage(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/home/operator", "run-1", "attempt-1")
	tests := []struct {
		name    string
		prompt  string
		inline  []ScenarioInlineContent
		ws      *ScenarioWorkspace
		want    string
		wantNot []string
	}{
		{name: "bare prompt is trimmed", prompt: "  Reply with exactly: READY \n", want: "Reply with exactly: READY"},
		{
			name: "a labelled block with a kind", prompt: "Classify.",
			inline: []ScenarioInlineContent{{Kind: "log", Label: "synthetic-app-log", Content: "ERROR x"}},
			want:   "Classify.\n\n[synthetic-app-log (log)]\nERROR x",
		},
		{
			name: "a block with only a label", prompt: "p",
			inline: []ScenarioInlineContent{{Label: "notes", Content: "body"}},
			want:   "p\n\n[notes]\nbody",
		},
		{
			name: "a block with only a kind", prompt: "p",
			inline: []ScenarioInlineContent{{Kind: "log", Content: "body"}},
			want:   "p\n\n[log]\nbody",
		},
		{
			name: "a block with neither is appended bare", prompt: "p",
			inline: []ScenarioInlineContent{{Content: "body"}},
			want:   "p\n\nbody",
		},
		{
			name: "several blocks keep their order", prompt: "Compare.",
			inline: []ScenarioInlineContent{{Kind: "config", Label: "service-a", Content: "t=30"}, {Kind: "config", Label: "service-b", Content: "t=5"}},
			want:   "Compare.\n\n[service-a (config)]\nt=30\n\n[service-b (config)]\nt=5",
		},
		{
			name: "the workspace renders in the prompt and in blocks", prompt: "Read " + ScenarioWorkspaceToken + "/a.",
			inline: []ScenarioInlineContent{{Kind: "log", Label: "l", Content: "at " + ScenarioWorkspaceToken}},
			ws:     &ws,
			want:   "Read " + ws.Root + "/a.\n\n[l (log)]\nat " + ws.Root,
		},
		{
			name: "a header never says Below or evaluation", prompt: "Summarize the log.",
			inline:  []ScenarioInlineContent{{Kind: "log", Label: "synthetic-log", Content: "x"}},
			want:    "Summarize the log.\n\n[synthetic-log (log)]\nx",
			wantNot: []string{"Below:", "evaluation"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := renderScenarioMessage(tt.prompt, tt.inline, tt.ws)
			assert.Equal(t, tt.want, got)
			for _, fragment := range tt.wantNot {
				assert.NotContains(t, got, fragment)
			}
		})
	}
}

func TestNonNullStringSlice(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{}, nonNullStringSlice(nil))
	assert.NotNil(t, nonNullStringSlice(nil))
	assert.Equal(t, []string{}, nonNullStringSlice([]string{}))
	source := []string{"a", "b"}
	copied := nonNullStringSlice(source)
	assert.Equal(t, source, copied)
	copied[0] = "changed"
	assert.Equal(t, "a", source[0], "the copy never aliases its source")
}

func probeBuildBase() ChatProbeRequest {
	return ChatProbeRequest{
		AssignmentID:            "assignment-1",
		EvaluationAttemptID:     "attempt-1",
		CampaignID:              "campaign-1",
		RunID:                   "run-1",
		ScenarioID:              "scenario-1",
		Model:                   "qwen3:4b",
		ModelDigest:             "a" + repeatHex('a', 63),
		TargetOperatorSessionID: "session-1",
		ModelRegistryDigest:     "d" + repeatHex('d', 63),
		ModelRegistry:           []*operatorv1.InferenceModelVariant{{Model: "qwen3:4b", Digest: "a" + repeatHex('a', 63)}, nil},
	}
}

func TestBuildChatProbeRequest_RejectsMissingBindingsAndIdentity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		mutate       func(req *ChatProbeRequest)
		dataOperator string
		dataSession  string
		wantIs       error
		wantMsg      string
	}{
		{name: "no data operator", dataOperator: "", dataSession: "s", mutate: func(*ChatProbeRequest) {}, wantMsg: "data operator binding is required"},
		{name: "no data operator session", dataOperator: "o", dataSession: "", mutate: func(*ChatProbeRequest) {}, wantMsg: "data operator binding is required"},
		{name: "no assignment id", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.AssignmentID = "" }, wantIs: constants.ErrMissingRequiredField},
		{name: "no attempt id", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.EvaluationAttemptID = "" }, wantIs: constants.ErrMissingRequiredField},
		{name: "no model", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.Model = "" }, wantIs: constants.ErrMissingRequiredField},
		{name: "no target session", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.TargetOperatorSessionID = "" }, wantIs: constants.ErrMissingRequiredField},
		{name: "no campaign", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.CampaignID = "" }, wantIs: constants.ErrInferenceModelRegistryInvalid},
		{name: "no run", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.RunID = "" }, wantIs: constants.ErrInferenceModelRegistryInvalid},
		{name: "no scenario", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.ScenarioID = "" }, wantIs: constants.ErrInferenceModelRegistryInvalid},
		{name: "no registry digest", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.ModelRegistryDigest = "" }, wantIs: constants.ErrInferenceModelRegistryInvalid},
		{name: "no registry", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.ModelRegistry = nil }, wantIs: constants.ErrInferenceModelRegistryInvalid},
		{name: "no model digest", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.ModelDigest = "" }, wantIs: constants.ErrInferenceModelRegistryInvalid},
		{name: "model_role lane needs a designated role", dataOperator: "o", dataSession: "s", mutate: func(req *ChatProbeRequest) { req.EvaluationLane = "model_role" }, wantMsg: "designated model role required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := probeBuildBase()
			tt.mutate(&req)
			_, err := BuildChatProbeRequest(req, tt.dataOperator, tt.dataSession)
			require.Error(t, err)
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}
			if tt.wantMsg != "" {
				assert.Contains(t, err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestBuildChatProbeRequest_ProducesTheGovernedScoredRequestShape(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/home/operator", "run-1", "attempt-1")
	seed := &harnessclient.EnsembleInvestigationSeed{CaseTitle: "Checkout payment timeouts", Turns: []harnessclient.EnsembleSeedTurn{{Sender: "user", Content: "hi"}}}
	req := probeBuildBase()
	req.EvaluationLane = "model_role"
	req.DesignatedModelRole = "assistant"
	req.Message = "  scored prompt  "
	req.Seed = seed
	req.Workspace = &ws
	req.GradingMethod = evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE
	req.GoldSummary = &ChatProbeGoldSummary{UserPrompt: "p", ExpectedBehavior: "b", ExpectedTools: []string{toolRead}}

	chatReq, err := BuildChatProbeRequest(req, "data-op", "data-session")
	require.NoError(t, err)

	assert.Equal(t, "scored prompt", chatReq.Message)
	assert.True(t, chatReq.SentinelMode)
	assert.Equal(t, "CLIENT", chatReq.Context.SourceComponent)
	assert.Equal(t, "data-op", chatReq.Context.OperatorID)
	assert.Equal(t, "data-session", chatReq.Context.OperatorSessionID)
	require.Len(t, chatReq.Context.BoundOperators, 1, "exactly one bound operator, so g8ee injects the single target")
	assert.Equal(t, harnessclient.EnsembleBoundOperator{OperatorID: "data-op", OperatorSessionID: "data-session", Status: "BOUND"}, chatReq.Context.BoundOperators[0])

	require.NotNil(t, chatReq.ResourceCreation)
	assert.True(t, chatReq.ResourceCreation.CreateCase, "a seed is only accepted with inline case creation (R3)")
	assert.Equal(t, "Checkout payment timeouts", chatReq.ResourceCreation.CaseTitle)

	for _, provider := range []string{chatReq.LLMPrimaryProvider, chatReq.LLMAssistantProvider, chatReq.LLMLiteProvider} {
		assert.Equal(t, "g8e", provider)
	}
	for _, model := range []string{chatReq.LLMPrimaryModel, chatReq.LLMAssistantModel, chatReq.LLMLiteModel} {
		assert.Equal(t, "qwen3:4b", model, "every tier slot is the candidate model, so every sub-call is part of the measured system")
	}

	evalCtx := chatReq.EvaluationContext
	require.NotNil(t, evalCtx)
	assert.Equal(t, "model_role", evalCtx.EvaluationLane)
	assert.Equal(t, "assistant", evalCtx.DesignatedModelRole)
	assert.Equal(t, "semantic_judge", evalCtx.GradingMethod)
	assert.Same(t, seed, evalCtx.Seed)
	require.NotNil(t, evalCtx.Workspace)
	assert.Equal(t, ws.Root, evalCtx.Workspace.Root)
	assert.Equal(t, ws.OperatorWorkingDirectory, evalCtx.Workspace.OperatorWorkingDirectory)
	require.Len(t, evalCtx.ModelRegistry, 1, "a nil registry entry is skipped")
	assert.Equal(t, "qwen3:4b", evalCtx.ModelRegistry[0].Model)
	require.NotNil(t, evalCtx.GoldSummary)
	assert.Equal(t, []string{toolRead}, evalCtx.GoldSummary.ExpectedTools)
	assert.NotNil(t, evalCtx.GoldSummary.ForbiddenTools)
}

func TestBuildChatProbeRequest_DefaultsAreForTheSystemLane(t *testing.T) {
	t.Parallel()
	chatReq, err := BuildChatProbeRequest(probeBuildBase(), "data-op", "data-session")
	require.NoError(t, err)

	assert.Equal(t, "Reply with exactly: chat-system-ok", chatReq.Message)
	assert.Equal(t, "system", chatReq.EvaluationContext.EvaluationLane)
	assert.Empty(t, chatReq.EvaluationContext.DesignatedModelRole)
	assert.Equal(t, "deterministic", chatReq.EvaluationContext.GradingMethod)
	assert.Nil(t, chatReq.EvaluationContext.GoldSummary)
	assert.Nil(t, chatReq.EvaluationContext.Seed)
	assert.Nil(t, chatReq.EvaluationContext.Workspace)
	assert.Empty(t, chatReq.ResourceCreation.CaseTitle)
}
