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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestCampaignRunVerifier_PassesCompletedAssignment(t *testing.T) {
	t.Parallel()
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	executor := &stubCampaignExecutor{}
	controller := NewCampaignController(store, executor, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	catalog := req.Catalog
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: catalog.GetSchemaVersion(),
		CatalogRef:    catalog.GetCatalogRef(),
		Scenarios:     catalog.GetScenarios()[:1],
	}
	truncatedDigest, err := ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = truncatedDigest
	req.Catalog = truncated
	_, err = controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	binding := CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              req.Inventory.ToModelRegistryFreeze().Variants,
	}
	result, executed, err := controller.ExecuteNextAssignment(context.Background(), req.RunID, binding, req.ScenarioArtifacts)
	require.NoError(t, err)
	require.True(t, executed)
	assignment, err := store.LoadAssignment(context.Background(), req.RunID, result.GetAssignmentId())
	require.NoError(t, err)
	trace := completedHomogeneousTrace("primary")
	trace["evaluation_context"] = map[string]any{
		"campaign_id":                req.CampaignID,
		"run_id":                     req.RunID,
		"assignment_id":              assignment.GetAssignmentId(),
		"evaluation_attempt_id":      "attempt-1",
		"scenario_id":                assignment.GetScenarioId(),
		"model_registry_digest":      binding.ModelRegistryDigest,
		"target_operator_session_id": binding.InferenceOperatorSessionID,
		"evaluation_lane":            "model_role",
		"designated_model_role":      "primary",
	}
	digest, err := computeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	traceBody, err := marshalSortedJSON(trace)
	require.NoError(t, err)
	require.NoError(t, store.SaveAssignmentTrace(context.Background(), req.RunID, assignment.GetAssignmentId(), traceBody))
	artifact := req.ScenarioArtifacts[assignment.GetScenarioId()]
	var scenarioInput ScenarioInputFixture
	require.NoError(t, json.Unmarshal(artifact.Input.Body, &scenarioInput))
	var scenarioGold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifact.Gold.Body, &scenarioGold))
	imported, err := ImportAssignmentResultFromTrace(AssignmentExecutionRequest{
		Assignment:    assignment,
		AttemptID:     "attempt-1",
		ScenarioInput: scenarioInput,
		ScenarioGold:  scenarioGold,
		GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		Binding:       binding,
	}, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	require.NoError(t, store.SaveAssignmentResult(context.Background(), imported))

	report, err := NewCampaignRunVerifier(func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }).VerifyRun(context.Background(), store, req.RunID, truncated, req.ScenarioArtifacts)
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, report.GetStatus())
	require.NoError(t, store.SaveCampaignVerification(context.Background(), req.RunID, report))
}
