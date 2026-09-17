// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestRepairAssignmentTraceDigests_RecomputesLegacyDigest(t *testing.T) {
	t.Parallel()
	store := NewStore(newCampaignMemoryFileService())
	now := func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	controller := NewCampaignController(store, nil, now, func(prefix string) string { return prefix + "-id" })

	runID := "run-trace-digest"
	assignmentID := "assignment-trace-digest"
	trace := map[string]any{
		"schema_version":         "1",
		"chat_execution_id":        "exec-1",
		"status":                   "completed",
		"completed_at":               "2026-09-17T03:36:01.016030+00:00",
		"designated_role_output":   "Proposed Action & Safeguard",
		"evaluation_context": map[string]any{
			"campaign_id":                "eval-smoke-mini",
			"run_id":                     runID,
			"assignment_id":              assignmentID,
			"evaluation_attempt_id":      "attempt-1",
			"scenario_id":                "tech-error-diagnosis",
			"model_registry_digest":      "c" + repeatHex('c', 63),
			"target_operator_session_id": "session-1",
			"evaluation_lane":            "model_role",
			"grading_method":             "deterministic",
		},
		"model_calls": []any{
			map[string]any{
				"agent_role":              "sage",
				"provider":                "G8EProvider",
				"model":                   "gemma3:4b",
				"succeeded":               true,
				"governed_transaction_id": "tx-1",
				"governed_result_digest":  "a" + repeatHex('a', 63),
				"provider_attempt_id":     "attempt-1",
			},
		},
		"governed_actions": []any{},
		"grader_calls":     []any{},
		"policy_decisions": []any{},
		"semantic_grades":  []any{},
		"tool_calls":       []any{},
		"tool_decisions":   []any{},
		"finish_reason":    "STOP",
		"role_outcome":     "invoked",
		"trace_digest":     "legacy-digest-not-authoritative",
	}
	body, err := json.Marshal(trace)
	require.NoError(t, err)
	require.NoError(t, store.SaveAssignmentTrace(context.Background(), runID, assignmentID, body))
	assignment := homogeneousAssignment(assignmentID, "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY)
	assignment.SchemaVersion = CampaignSchemaVersion
	assignment.RunId = runID
	assignment.CampaignId = "eval-smoke-mini"
	assignment.ScenarioId = "tech-error-diagnosis"
	assignment.LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	require.NoError(t, store.SaveAssignment(context.Background(), assignment))

	repaired, err := controller.RepairAssignmentTraceDigests(context.Background(), runID)
	require.NoError(t, err)
	require.Equal(t, 1, repaired)

	loaded, err := store.LoadAssignmentTrace(context.Background(), runID, assignmentID)
	require.NoError(t, err)
	require.NoError(t, validateTraceDigest(loaded))
}
