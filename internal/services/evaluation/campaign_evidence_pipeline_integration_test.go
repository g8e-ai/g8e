// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package evaluation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	pipelineModelPatternSecret = "MODEL_AUTHORED_PATTERN_SECRET"
	pipelineModelPathSecret    = "/etc/MODEL_AUTHORED_PATH_SECRET"
)

// publicContextForCatalogScenario is the public scenario context a published
// assignment carries, taken from the real built-in catalog.
func publicContextForCatalogScenario(t *testing.T, scenarioID string) *PublicScenarioContext {
	t.Helper()
	catalog, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	for _, scenario := range catalog.GetScenarios() {
		if scenario.GetScenarioId() != scenarioID {
			continue
		}
		return &PublicScenarioContext{
			ScenarioID: scenario.GetScenarioId(), ScenarioVersion: scenario.GetScenarioVersion(), Category: scenario.GetCategory(),
			PublicDescription: scenario.GetPublicDescription(), GradingMethod: scenario.GetGradingMethod(),
			AllowedTools: scenario.GetAllowedTools(), ExpectedTools: scenario.GetExpectedTools(), ForbiddenTools: scenario.GetForbiddenTools(),
			TrajectoryPolicy: scenario.GetTrajectoryPolicy(), PromptHint: clonePromptHint(scenario.GetPromptHint()),
			// The built-in catalog authors no public criteria, so a context resolved
			// from it is rejected as malformed; this one supplies the minimum a public
			// summary requires so the rest of the record can be checked.
			Criteria: []*evalv1.PublicScenarioCriterion{{
				CriterionId: "trajectory", PublicLabel: "Trajectory", PublicDescription: "The model's tool-call trajectory meets the scenario.",
				GradingMethod: scenario.GetGradingMethod(), Required: true,
			}},
			ToolScoreDimensions: cloneDimensions(scenario.GetPublicToolScoreDimensions()),
		}
	}
	t.Fatalf("scenario %q is not in the built-in catalog", scenarioID)
	return nil
}

// TestEvidencePipeline_FailedSeededAttemptPublishesOnlyBoundedFields follows one
// failed attempt of a real catalog scenario through import, the real runtime
// file service, and the public projection: the public record carries the
// trajectory fields and what the model was offered, and none of the private
// text (the model's own argument values, the attempt-scoped workspace path, or
// the validator rule).
func TestEvidencePipeline_FailedSeededAttemptPublishesOnlyBoundedFields(t *testing.T) {
	f := newSeededImportFixture(t, "recovery-error-guided-retry", func(f *seededImportFixture) {
		f.trace["tool_calls"] = []any{operatorCall(t, "c1", toolGrep, true,
			map[string]any{"pattern": pipelineModelPatternSecret, "path": pipelineModelPathSecret, "target_operators": []string{"op-1"}},
			map[string]any{"loop_turn": float64(2)})}
		f.trace["designated_role_output"] = "Searched /etc as asked."
	})
	result := f.importResult(t)
	require.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_WRONG_ARGUMENTS, result.GetTrajectoryOutcome())
	require.Contains(t, result.GetFailureReason(), "does not match", "the private sentence keeps the validator rule")

	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	store := NewStore(fileSvc)
	require.NoError(t, store.SaveAssignmentResult(context.Background(), result))
	loaded, err := store.LoadAssignmentResult(context.Background(), result.GetRunId(), result.GetAssignmentId())
	require.NoError(t, err)
	require.NoError(t, ValidateAssignmentResultDigest(loaded))

	record, err := BuildPublicAssignmentProjection(context.Background(), PublicAssignmentBuildInput{
		Assignment: f.req.Assignment, Result: loaded, ScenarioContext: publicContextForCatalogScenario(t, "recovery-error-guided-retry"),
	})
	require.NoError(t, err)
	body, err := MarshalPublicAssignmentRecord(record)
	require.NoError(t, err)

	for _, private := range []string{pipelineModelPatternSecret, pipelineModelPathSecret, f.ws.Root, "Its output began", "does not match", "AUTH_FAILURE"} {
		assert.NotContains(t, string(body), private)
	}
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &fields))
	assert.JSONEq(t, `"EVALUATION_TRAJECTORY_OUTCOME_WRONG_ARGUMENTS"`, string(fields["trajectory_outcome"]))
	var failureReason string
	require.NoError(t, json.Unmarshal(fields["failure_reason"], &failureReason))
	assert.Equal(t, loaded.GetPublicFailureReason(), failureReason)
	assert.Contains(t, failureReason, "failed validation")
	assert.Contains(t, string(fields["tools_declared"]), toolGrep, "the record says which tools the model was offered")

	envelope, err := marshalPublicAssignmentEnvelope("run-1:assignment-1:result", record)
	require.NoError(t, err)
	require.NoError(t, ValidatePublicAssignmentRecord(campaignProjectionEnvelopeEnrichedVersion, envelope))

	historical := assignmentEnvelope(t, "1.1.0", record.Projection, record.Extensions)
	assert.Error(t, ValidatePublicAssignmentRecord("", historical), "a 1.1.0 envelope that carries a trajectory field is rejected")
}
