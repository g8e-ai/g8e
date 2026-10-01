// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestValidateChatProbeTrace_RequiresTerminalCompletedTrace(t *testing.T) {
	t.Parallel()
	req := ChatProbeRequest{
		AssignmentID:            "assignment-1",
		EvaluationAttemptID:     "attempt-1",
		CampaignID:              "campaign-1",
		RunID:                   "run-1",
		ScenarioID:              "scenario-1",
		TargetOperatorSessionID: "session-1",
		ModelRegistryDigest:     "d" + repeatHex('d', 63),
	}
	trace := EvaluationTrace{
		"status": "running",
	}
	err := ValidateChatProbeTrace(req, trace)
	require.Error(t, err)
}

func TestValidateChatProbeTrace_AcceptsGovernedCompletedTrace(t *testing.T) {
	t.Parallel()
	req := ChatProbeRequest{
		AssignmentID:            "assignment-1",
		EvaluationAttemptID:     "attempt-1",
		CampaignID:              "campaign-1",
		RunID:                   "run-1",
		ScenarioID:              "scenario-1",
		TargetOperatorSessionID: "session-1",
		ModelRegistryDigest:     "d" + repeatHex('d', 63),
		EvaluationLane:          "system",
	}
	trace := EvaluationTrace{
		"schema_version":    "1",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      "2026-09-15T00:00:00+00:00",
		"evaluation_context": EvaluationTrace{
			"campaign_id":                "campaign-1",
			"run_id":                     "run-1",
			"assignment_id":              "assignment-1",
			"evaluation_attempt_id":      "attempt-1",
			"scenario_id":                "scenario-1",
			"model_registry_digest":      "d" + repeatHex('d', 63),
			"target_operator_session_id": "session-1",
			"evaluation_lane":            "system",
		},
		"model_calls": []any{
			EvaluationTrace{
				"agent_role":              "sage",
				"provider":                "G8EProvider",
				"governed_transaction_id": "tx-1",
				"governed_result_digest":  "a" + repeatHex('a', 63),
				"provider_attempt_id":     "attempt-1",
			},
		},
	}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	require.NoError(t, ValidateChatProbeTrace(req, trace))
}

func TestValidateChatProbeTrace_IgnoresFailedGovernedCalls(t *testing.T) {
	t.Parallel()
	req := ChatProbeRequest{
		AssignmentID:            "assignment-1",
		EvaluationAttemptID:     "attempt-1",
		CampaignID:              "campaign-1",
		RunID:                   "run-1",
		ScenarioID:              "scenario-1",
		TargetOperatorSessionID: "session-1",
		ModelRegistryDigest:     "d" + repeatHex('d', 63),
		EvaluationLane:          "system",
	}
	trace := EvaluationTrace{
		"schema_version":    "1",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      "2026-09-15T00:00:00+00:00",
		"evaluation_context": EvaluationTrace{
			"campaign_id":                "campaign-1",
			"run_id":                     "run-1",
			"assignment_id":              "assignment-1",
			"evaluation_attempt_id":      "attempt-1",
			"scenario_id":                "scenario-1",
			"model_registry_digest":      "d" + repeatHex('d', 63),
			"target_operator_session_id": "session-1",
			"evaluation_lane":            "system",
		},
		"model_calls": []any{
			EvaluationTrace{
				"agent_role": "triage",
				"provider":   "G8EProvider",
				"succeeded":  false,
			},
			EvaluationTrace{
				"agent_role":              "sage",
				"provider":                "G8EProvider",
				"succeeded":               true,
				"governed_transaction_id": "tx-1",
				"governed_result_digest":  "a" + repeatHex('a', 63),
				"provider_attempt_id":     "attempt-1",
			},
		},
	}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	require.NoError(t, ValidateChatProbeTrace(req, trace))
}

// seededProbeFixture returns a request carrying a seed and a completed trace
// whose evaluation_context echoes echoedSeed, so a test varies only the echo.
func seededProbeFixture(t *testing.T, echoedSeed any) (ChatProbeRequest, EvaluationTrace) {
	t.Helper()
	req := ChatProbeRequest{
		AssignmentID:            "assignment-1",
		EvaluationAttemptID:     "attempt-1",
		CampaignID:              "campaign-1",
		RunID:                   "run-1",
		ScenarioID:              "scenario-1",
		TargetOperatorSessionID: "session-1",
		ModelRegistryDigest:     "d" + repeatHex('d', 63),
		EvaluationLane:          "system",
		Seed: &harnessclient.EnsembleInvestigationSeed{
			CaseTitle: "Checkout payment timeouts",
			Turns:     []harnessclient.EnsembleSeedTurn{{Sender: "user", Content: "Checkout is failing."}},
			HistoryEvents: []harnessclient.EnsembleSeedHistoryEvent{{
				EventType: "g8e.v1.operator.filesystem.grep.failed",
				Actor:     "system",
				Summary:   "grep failed",
				ToolName:  "recursive_grep_search",
				Error:     "path is required",
			}},
		},
	}
	trace := EvaluationTrace{
		"schema_version":    "6",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      "2026-09-15T00:00:00+00:00",
		"evaluation_context": EvaluationTrace{
			"campaign_id":                "campaign-1",
			"run_id":                     "run-1",
			"assignment_id":              "assignment-1",
			"evaluation_attempt_id":      "attempt-1",
			"scenario_id":                "scenario-1",
			"model_registry_digest":      "d" + repeatHex('d', 63),
			"target_operator_session_id": "session-1",
			"evaluation_lane":            "system",
			"seed":                       echoedSeed,
		},
		"model_calls": []any{
			EvaluationTrace{
				"agent_role":              "sage",
				"provider":                "G8EProvider",
				"governed_transaction_id": "tx-1",
				"governed_result_digest":  "a" + repeatHex('a', 63),
				"provider_attempt_id":     "attempt-1",
			},
		},
	}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	return req, trace
}

func echoedSeed(mutate func(seed map[string]any)) map[string]any {
	// The shape g8ee records: the sent seed plus the model defaults it filled
	// in (an empty case_description and unset optional fields omitted).
	seed := map[string]any{
		"case_title":       "Checkout payment timeouts",
		"case_description": "",
		"turns":            []any{map[string]any{"sender": "user", "content": "Checkout is failing."}},
		"history_events": []any{map[string]any{
			"event_type": "g8e.v1.operator.filesystem.grep.failed",
			"actor":      "system",
			"summary":    "grep failed",
			"tool_name":  "recursive_grep_search",
			"error":      "path is required",
		}},
	}
	if mutate != nil {
		mutate(seed)
	}
	return seed
}

// TestValidateChatProbeTrace_SeedEchoIsComparedAfterTypedDecoding guards the
// seed echo check against g8ee's model defaults: an honest echo that adds
// defaults the harness never sent must verify, and any real difference in what
// the seed carried must not.
func TestValidateChatProbeTrace_SeedEchoIsComparedAfterTypedDecoding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(seed map[string]any)
		wantErr bool
	}{
		{name: "echo with model defaults added", mutate: nil},
		{name: "a turn whose content differs", mutate: func(seed map[string]any) {
			seed["turns"] = []any{map[string]any{"sender": "user", "content": "Something else."}}
		}, wantErr: true},
		{name: "a missing turn", mutate: func(seed map[string]any) { seed["turns"] = []any{} }, wantErr: true},
		{name: "a history event field dropped", mutate: func(seed map[string]any) {
			event := seed["history_events"].([]any)[0].(map[string]any)
			delete(event, "error")
		}, wantErr: true},
		{name: "a different case title", mutate: func(seed map[string]any) { seed["case_title"] = "Another case" }, wantErr: true},
		{name: "an injected case memory", mutate: func(seed map[string]any) {
			seed["case_memory"] = map[string]any{"investigation_summary": "planted"}
		}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req, trace := seededProbeFixture(t, echoedSeed(tt.mutate))

			err := ValidateChatProbeTrace(req, trace)

			if tt.wantErr {
				require.ErrorContains(t, err, "evaluation_context.seed mismatch")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateChatProbeTrace_RequiresTheEchoedSeedWhenOneWasSent(t *testing.T) {
	t.Parallel()
	req, trace := seededProbeFixture(t, nil)

	err := ValidateChatProbeTrace(req, trace)

	require.ErrorContains(t, err, "evaluation_context.seed is required")
}

func TestBuildChatProbeRequest_MarshalsEmptyGoldToolListsAsArrays(t *testing.T) {
	t.Parallel()
	req := ChatProbeRequest{
		AssignmentID:            "assignment-1",
		EvaluationAttemptID:     "attempt-1",
		CampaignID:              "campaign-1",
		RunID:                   "run-1",
		ScenarioID:              "semantic-judge-scenario",
		Model:                   "qwen3:4b",
		ModelDigest:             "a" + repeatHex('a', 63),
		TargetOperatorSessionID: "session-1",
		ModelRegistryDigest:     "d" + repeatHex('d', 63),
		ModelRegistry: []*operatorv1.InferenceModelVariant{{
			Model:  "qwen3:4b",
			Digest: "a" + repeatHex('a', 63),
		}},
		EvaluationLane:      "model_role",
		DesignatedModelRole: "primary",
		Message:             "hello",
		GradingMethod:       evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE,
		GoldSummary: &ChatProbeGoldSummary{
			UserPrompt:       "hello",
			ExpectedBehavior: "be helpful",
		},
	}
	chatReq, err := BuildChatProbeRequest(req, "data-op", "data-session")
	require.NoError(t, err)
	body, err := json.Marshal(chatReq)
	require.NoError(t, err)
	payload := string(body)
	require.NotContains(t, payload, `"expected_tools":null`)
	require.NotContains(t, payload, `"forbidden_tools":null`)
	require.Contains(t, payload, `"expected_tools":[]`)
	require.Contains(t, payload, `"forbidden_tools":[]`)
}
