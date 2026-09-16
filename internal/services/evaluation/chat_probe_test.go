// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
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
	trace := map[string]any{
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
	trace := map[string]any{
		"schema_version":    "1",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      "2026-09-15T00:00:00+00:00",
		"evaluation_context": map[string]any{
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
			map[string]any{
				"agent_role":              "sage",
				"provider":                "G8EProvider",
				"governed_transaction_id": "tx-1",
				"governed_result_digest":  "a" + repeatHex('a', 63),
				"provider_attempt_id":     "attempt-1",
			},
		},
	}
	digest, err := computeTraceDigest(trace)
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
	trace := map[string]any{
		"schema_version":    "1",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      "2026-09-15T00:00:00+00:00",
		"evaluation_context": map[string]any{
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
			map[string]any{
				"agent_role": "triage",
				"provider":   "G8EProvider",
				"succeeded":  false,
			},
			map[string]any{
				"agent_role":              "sage",
				"provider":                "G8EProvider",
				"succeeded":               true,
				"governed_transaction_id": "tx-1",
				"governed_result_digest":  "a" + repeatHex('a', 63),
				"provider_attempt_id":     "attempt-1",
			},
		},
	}
	digest, err := computeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	require.NoError(t, ValidateChatProbeTrace(req, trace))
}
