// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

var importNow = time.Unix(1_700_000_000, 0).UTC()

func importIDs(prefix string) string { return prefix + "-id" }

func sealTrace(t *testing.T, trace EvaluationTrace) {
	t.Helper()
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
}

// seededImportFixture is a request and honest trace for one real catalog
// scenario: the frozen input and gold, the scenario's tool expectations, and a
// trace that echoes the seed and workspace the request would have sent.
type seededImportFixture struct {
	req   AssignmentExecutionRequest
	trace EvaluationTrace
	ws    ScenarioWorkspace
}

func newSeededImportFixture(t *testing.T, scenarioID string, mutate func(f *seededImportFixture)) seededImportFixture {
	t.Helper()
	catalog, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var scenario *evalv1.EvaluationScenarioDefinition
	for _, candidate := range catalog.GetScenarios() {
		if candidate.GetScenarioId() == scenarioID {
			scenario = candidate
		}
	}
	require.NotNil(t, scenario)

	req := homogeneousAssignmentExecutionRequest(t, "primary")
	req.Assignment.ScenarioId = scenarioID
	// Decode into fresh values: unmarshalling over the helper's
	// instruction-exact-format fixture would merge its content check in.
	req.ScenarioInput = ScenarioInputFixture{}
	req.ScenarioGold = ScenarioGoldCriteria{}
	require.NoError(t, json.Unmarshal(artifacts[scenarioID].Input.Body, &req.ScenarioInput))
	require.NoError(t, json.Unmarshal(artifacts[scenarioID].Gold.Body, &req.ScenarioGold))
	req.GradingMethod = scenario.GetGradingMethod()
	req.ScenarioTools = ScenarioToolExpectations{
		AllowedTools:     append([]string(nil), scenario.GetAllowedTools()...),
		ExpectedTools:    append([]string(nil), scenario.GetExpectedTools()...),
		ForbiddenTools:   append([]string(nil), scenario.GetForbiddenTools()...),
		TrajectoryPolicy: scenario.GetTrajectoryPolicy(),
	}

	ws := mustWorkspace(t, req.Binding.DataOperatorWorkingDirectory, req.Assignment.GetRunId(), req.AttemptID)
	trace := completedHomogeneousTrace(t, "primary")
	evalContext := evalContextOf(trace)
	evalContext["scenario_id"] = scenarioID
	evalContext["workspace"] = workspaceEcho(ws)
	if seed := buildHarnessInvestigationSeed(&req.ScenarioInput.Seed, &ws); seed != nil {
		raw, err := json.Marshal(seed)
		require.NoError(t, err)
		var echoed map[string]any
		require.NoError(t, json.Unmarshal(raw, &echoed))
		evalContext["seed"] = echoed
	}
	trace["model_calls"].([]any)[0].(EvaluationTrace)["tools_declared"] = []any{toolGrep, toolRead, "get_command_constraints"}
	trace["triage_model_call"] = EvaluationTrace{"succeeded": true}
	trace["designated_role_output"] = "Retried against the workspace; the match is AUTH_FAILURE user=svc-deploy."
	trace["tool_calls"] = []any{operatorCall(t, "c1", toolGrep, true, map[string]any{"pattern": "AUTH_FAILURE", "path": ws.Root, "target_operators": []string{"op-1"}}, map[string]any{"loop_turn": float64(2)})}
	trace["governed_actions"] = []any{governedAllow()}

	f := seededImportFixture{req: req, trace: trace, ws: ws}
	if mutate != nil {
		mutate(&f)
	}
	sealTrace(t, f.trace)
	return f
}

func (f seededImportFixture) importResult(t *testing.T) *evalv1.EvaluationAssignmentResult {
	t.Helper()
	result, err := ImportAssignmentResultFromTrace(f.req, f.trace, nil, importNow, importIDs)
	require.NoError(t, err)
	return result
}

func TestImportAssignmentResultFromTrace_SeededScenarioWithAnHonestEchoCompletesAndScores(t *testing.T) {
	t.Parallel()
	f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)

	result := f.importResult(t)

	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, result.GetLifecycleStatus())
	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_RECOVERED, result.GetTrajectoryOutcome())
	assert.Equal(t, uint32(1), result.GetGuidedRetryCount())
	assert.Empty(t, result.GetFailureReason())
	assert.Empty(t, result.GetPublicFailureReason())
	assert.Equal(t, 1.0, findDecomposedScore(result.GetDecomposedScores(), "task_score").GetValue(), "failed grades: %v", failedGradeDetails(result))

	require.Len(t, result.GetModelInferences(), 1)
	assert.True(t, result.GetModelInferences()[0].GetToolsDeclaredReported())
	assert.Contains(t, result.GetModelInferences()[0].GetToolsDeclared(), toolGrep)

	require.Len(t, result.GetToolCalls(), 1)
	record := result.GetToolCalls()[0]
	assert.Equal(t, uint32(2), record.GetLoopTurn())
	assert.False(t, record.GetGuidanceShown())
	assert.NotNil(t, record.GetGovernedBindingRef(), "an operator call with an execution id binds to its receipt")
	require.NoError(t, ValidateAssignmentResultDigest(result))
}

func failedGradeDetails(result *evalv1.EvaluationAssignmentResult) []string {
	var failed []string
	for _, grade := range result.GetDeterministicGrades() {
		if grade.GetCriterionId() != "triage" && grade.GetStatus() != verdictPass {
			failed = append(failed, grade.GetCriterionId()+": "+grade.GetDetail())
		}
	}
	return failed
}

func TestImportAssignmentResultFromTrace_AFailedAttemptCarriesPrivateAndPublicReasonsSeparately(t *testing.T) {
	t.Parallel()
	f := newSeededImportFixture(t, "recovery-error-guided-retry", func(f *seededImportFixture) {
		f.trace["tool_calls"] = []any{operatorCall(t, "c1", toolGrep, false, map[string]any{"pattern": "AUTH_FAILURE"},
			map[string]any{"error_type": "execution.error", "error": "G8E-1607: Tool execution failed for recursive_grep_search: path Field required"})}
		f.trace["governed_actions"] = []any{}
		f.trace["designated_role_output"] = "It failed again, sorry."
	})

	result := f.importResult(t)

	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_IGNORED_GUIDANCE, result.GetTrajectoryOutcome())
	assert.Equal(t, 0.0, findDecomposedScore(result.GetDecomposedScores(), "task_score").GetValue())
	assert.Contains(t, result.GetFailureReason(), "G8E-1607", "the private reason quotes the guidance")
	assert.Contains(t, result.GetFailureReason(), "Its output began: “It failed again, sorry.”.")
	assert.NotContains(t, result.GetPublicFailureReason(), "G8E-1607")
	assert.NotContains(t, result.GetPublicFailureReason(), "It failed again")
	assert.NotEmpty(t, result.GetPublicFailureReason())
	require.Len(t, result.GetToolCalls(), 1)
	assert.True(t, result.GetToolCalls()[0].GetGuidanceShown())
	assert.Equal(t, "execution.error", result.GetToolCalls()[0].GetErrorType())

	// R7: the model-visible error text never reaches the structured record.
	raw, err := json.Marshal(result.GetToolCalls()[0])
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "G8E-1607")
}

func TestImportAssignmentResultFromTrace_AnEchoThatDoesNotMatchTheFrozenFixtureIsNotACompletedAttempt(t *testing.T) {
	t.Parallel()
	failed := evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED
	other := mustWorkspace(t, "/home/operator", "run-1", "attempt-2")
	tests := []struct {
		name   string
		mutate func(f *seededImportFixture)
	}{
		{name: "a seed turn the model never saw", mutate: func(f *seededImportFixture) {
			turns := evalContextOf(f.trace)["seed"].(map[string]any)["turns"].([]any)
			turns[0].(map[string]any)["content"] = "Something the fixture never said."
		}},
		{name: "a case memory planted in the echo", mutate: func(f *seededImportFixture) {
			evalContextOf(f.trace)["seed"].(map[string]any)["case_memory"] = map[string]any{"investigation_summary": "planted"}
		}},
		{name: "no seed echoed at all", mutate: func(f *seededImportFixture) { delete(evalContextOf(f.trace), "seed") }},
		{name: "a workspace belonging to another attempt", mutate: func(f *seededImportFixture) { evalContextOf(f.trace)["workspace"] = workspaceEcho(other) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := newSeededImportFixture(t, "recovery-error-guided-retry", tt.mutate).importResult(t)
			assert.Equal(t, failed, result.GetLifecycleStatus())
			assert.NotEqual(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, findDeterministicGrade(result.GetDeterministicGrades(), "role-invoked").GetStatus())
		})
	}
}

func TestImportAssignmentResultFromTrace_RejectsIncompleteInput(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	trace := completedHomogeneousTrace(t, "primary")

	noAssignment := req
	noAssignment.Assignment = nil
	noAttempt := req
	noAttempt.AttemptID = ""
	noDataSession := req
	noDataSession.Binding.DataOperatorSessionID = ""

	for name, request := range map[string]AssignmentExecutionRequest{"assignment": noAssignment, "attempt": noAttempt} {
		_, err := ImportAssignmentResultFromTrace(request, trace, nil, importNow, importIDs)
		require.Error(t, err, name)
	}
	_, err := ImportAssignmentResultFromTrace(req, EvaluationTrace{}, nil, importNow, importIDs)
	require.Error(t, err)
	_, err = ImportAssignmentResultFromTrace(noDataSession, trace, nil, importNow, importIDs)
	require.Error(t, err)

	t.Run("defaults the clock and id generator", func(t *testing.T) {
		t.Parallel()
		result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Time{}, nil)
		require.NoError(t, err)
		assert.False(t, result.GetCompletedAt().AsTime().IsZero())
	})
}

func TestWorkspaceFromTrace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		trace EvaluationTrace
		want  *ScenarioWorkspace
	}{
		{name: "nil trace", trace: nil},
		{name: "no evaluation context", trace: EvaluationTrace{"status": "completed"}},
		{name: "evaluation context of the wrong type", trace: EvaluationTrace{"evaluation_context": "x"}},
		{name: "no workspace", trace: EvaluationTrace{"evaluation_context": EvaluationTrace{}}},
		{name: "workspace of the wrong type", trace: EvaluationTrace{"evaluation_context": EvaluationTrace{"workspace": "x"}}},
		{name: "empty root", trace: EvaluationTrace{"evaluation_context": EvaluationTrace{"workspace": map[string]any{"root": "", "operator_working_directory": "/r"}}}},
		{name: "empty working directory", trace: EvaluationTrace{"evaluation_context": EvaluationTrace{"workspace": map[string]any{"root": "/r/workspaces/ws-1"}}}},
		{
			name:  "both present",
			trace: EvaluationTrace{"evaluation_context": EvaluationTrace{"workspace": map[string]any{"root": "/r/workspaces/ws-1", "operator_working_directory": "/r"}}},
			want:  &ScenarioWorkspace{Root: "/r/workspaces/ws-1", OperatorWorkingDirectory: "/r"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, workspaceFromTrace(tt.trace))
		})
	}
}

func TestClassifyCampaignTraceOutcome_LifecycleByTraceShape(t *testing.T) {
	t.Parallel()
	const (
		completed = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
		failed    = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED
		provider  = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED
		partial   = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL
	)
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	probeReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{}, nil)
	require.NoError(t, err)

	tests := []struct {
		name       string
		mutate     func(trace EvaluationTrace)
		wantStatus evalv1.EvaluationAssignmentLifecycleStatus
		wantGrade  evalv1.EvaluationVerdictStatus
	}{
		{name: "a valid completed trace", mutate: nil, wantStatus: completed, wantGrade: verdictPass},
		{
			name: "a provider tool rejection is a scored result, not an infrastructure failure",
			mutate: func(trace EvaluationTrace) {
				trace["status"] = "failed"
				trace["provider_tool_rejection"] = EvaluationTrace{"model": "tiny:1b"}
				trace["model_calls"] = []any{}
			},
			wantStatus: completed, wantGrade: verdictFail,
		},
		{name: "a failed trace that never reached a model is a provider failure", mutate: func(trace EvaluationTrace) { trace["status"] = "failed"; trace["model_calls"] = []any{} }, wantStatus: provider, wantGrade: verdictFail},
		{name: "a failed trace that reached a model is a provider failure", mutate: func(trace EvaluationTrace) { trace["status"] = "failed" }, wantStatus: provider, wantGrade: verdictFail},
		{name: "role not invoked is partial", mutate: func(trace EvaluationTrace) { trace["role_outcome"] = "role_not_invoked" }, wantStatus: partial, wantGrade: verdictFail},
		{name: "a trace that is not terminal", mutate: func(trace EvaluationTrace) { trace["status"] = "running" }, wantStatus: failed, wantGrade: verdictFail},
		{name: "a completed trace that fails validation", mutate: func(trace EvaluationTrace) { trace["chat_execution_id"] = "" }, wantStatus: failed, wantGrade: verdictFail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			trace := completedHomogeneousTrace(t, "primary")
			if tt.mutate != nil {
				tt.mutate(trace)
			}
			sealTrace(t, trace)
			status, grade := classifyCampaignTraceOutcome(probeReq, trace)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantGrade, grade.GetStatus())
			assert.Equal(t, "assignment-1:role-invoked", grade.GetGradeId())
		})
	}
}

func inferenceAssignment() *evalv1.EvaluationAssignment {
	return &evalv1.EvaluationAssignment{
		AssignmentId: "assignment-1",
		RunId:        "run-1",
		CampaignId:   "campaign-1",
		ScenarioId:   "instruction-exact-format",
		Lane:         evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
	}
}

func importModelCall(id string, extra map[string]any) EvaluationTrace {
	call := EvaluationTrace{
		"agent_role":              "sage",
		"classification":          "scored_chain",
		"model_role":              "primary",
		"provider":                "G8EProvider",
		"governed_transaction_id": "tx-" + id,
		"governed_result_digest":  "a" + repeatHex('a', 63),
		"provider_attempt_id":     id,
	}
	for k, v := range extra {
		call[k] = v
	}
	return call
}

func inferencesOf(t *testing.T, schema string, calls ...EvaluationTrace) ([]*evalv1.ModelInferenceRecord, *uint64, error) {
	t.Helper()
	list := make([]any, 0, len(calls))
	for _, call := range calls {
		list = append(list, call)
	}
	trace := EvaluationTrace{"schema_version": schema, "model_calls": list}
	return modelInferenceRecordsFromTrace(inferenceAssignment(), "attempt-1", nil, trace, importIDs)
}

func TestModelInferenceRecords_ToolsDeclaredIsReportedOnlyWhenTheTraceSaysSo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		declared     any
		set          bool
		wantReported bool
		wantTools    []string
	}{
		{name: "absent", set: false, wantReported: false},
		{name: "null", set: true, declared: nil, wantReported: false},
		{name: "two tools", set: true, declared: []any{toolGrep, toolRead}, wantReported: true, wantTools: []string{toolGrep, toolRead}},
		{name: "an explicit empty list is reported as empty", set: true, declared: []any{}, wantReported: true, wantTools: []string{}},
		{name: "an in-memory string slice", set: true, declared: []string{toolGrep}, wantReported: true, wantTools: []string{toolGrep}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			extra := map[string]any{}
			if tt.set {
				extra["tools_declared"] = tt.declared
			}
			records, _, err := inferencesOf(t, "6", importModelCall("p1", extra))
			require.NoError(t, err)
			require.Len(t, records, 1)
			assert.Equal(t, tt.wantReported, records[0].GetToolsDeclaredReported())
			assert.Equal(t, tt.wantTools, records[0].GetToolsDeclared())
		})
	}
}

// A malformed tools_declared must not read as "the harness declared no tools".
func TestModelInferenceRecords_ToolsDeclaredThatIsNotAListOfNamesIsAnImportError(t *testing.T) {
	t.Parallel()
	for name, declared := range map[string]any{
		"a string":                 toolGrep,
		"an object":                map[string]any{toolGrep: true},
		"a list with a non-string": []any{toolGrep, 1},
		"a list with a null":       []any{toolGrep, nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, _, err := inferencesOf(t, "6", importModelCall("p1", map[string]any{"tools_declared": declared}))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "tools_declared")
		})
	}
}

func TestModelInferenceRecords_OnlyGovernedSuccessfulCallsWithAnAttemptIdBecomeRecords(t *testing.T) {
	t.Parallel()
	records, _, err := inferencesOf(t, "6",
		importModelCall("kept-1", nil),
		importModelCall("other", map[string]any{"provider": "ollama"}),
		importModelCall("failed", map[string]any{"succeeded": false}),
		importModelCall("", nil),
		importModelCall("kept-2", map[string]any{"agent_role": "triage", "model_role": "lite", "call_site": "triage", "finish_reason": "stop"}),
	)
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "kept-1", records[0].GetProviderAttemptId())
	assert.Equal(t, "sage", records[0].GetAgentPersona())
	assert.Equal(t, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY, records[0].GetModelRole())
	assert.Equal(t, "tx-kept-1", records[0].GetGovernedReceiptRef().GetArtifactId())
	assert.Equal(t, "kept-2", records[1].GetProviderAttemptId())
	assert.Equal(t, "triage", records[1].GetAgentPersona())
	assert.Equal(t, "triage", records[1].GetCallSite())
	assert.Equal(t, "stop", records[1].GetFinishReason())
	assert.Equal(t, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE, records[1].GetModelRole())
	assert.Equal(t, "attempt-1", records[0].GetEvaluationAttemptId())
	assert.True(t, records[0].GetPrivacyAttested())
}

func TestModelInferenceRecords_UsageAndTimingFieldsAreCheckedStrictly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		schema  string
		extra   map[string]any
		wantErr string
		check   func(t *testing.T, record *evalv1.ModelInferenceRecord)
	}{
		{name: "usage reported with counts", schema: "6", extra: map[string]any{"usage_reported": true, "input_tokens": float64(10), "output_tokens": float64(5), "thinking_tokens": float64(2), "cache_tokens": float64(1)}, check: func(t *testing.T, r *evalv1.ModelInferenceRecord) {
			assert.Equal(t, evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED, r.GetUsageAvailability())
			assert.Equal(t, uint32(10), r.GetPromptTokens())
			assert.Equal(t, uint32(5), r.GetCompletionTokens())
			assert.Equal(t, uint32(2), r.GetThinkingTokens())
			assert.Equal(t, uint32(1), r.GetCacheTokens())
		}},
		{name: "optional counters may be absent in schema 6", schema: "6", extra: map[string]any{"usage_reported": true, "input_tokens": float64(1), "output_tokens": float64(1)}, check: func(t *testing.T, r *evalv1.ModelInferenceRecord) {
			assert.Nil(t, r.ThinkingTokens)
			assert.Nil(t, r.CacheTokens)
		}},
		{name: "usage not reported", schema: "6", extra: map[string]any{"usage_reported": false}, check: func(t *testing.T, r *evalv1.ModelInferenceRecord) {
			assert.Equal(t, evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE, r.GetUsageAvailability())
		}},
		{name: "usage_reported must be a boolean", schema: "6", extra: map[string]any{"usage_reported": "yes"}, wantErr: "usage_reported must be a boolean"},
		{name: "reported usage needs input tokens", schema: "6", extra: map[string]any{"usage_reported": true, "output_tokens": float64(1)}, wantErr: "missing input_tokens"},
		{name: "reported usage needs output tokens", schema: "6", extra: map[string]any{"usage_reported": true, "input_tokens": float64(1)}, wantErr: "missing output_tokens"},
		{name: "a negative token count", schema: "6", extra: map[string]any{"usage_reported": true, "input_tokens": float64(-1), "output_tokens": float64(1)}, wantErr: "input_tokens"},
		{name: "a fractional token count", schema: "6", extra: map[string]any{"usage_reported": true, "input_tokens": 1.5, "output_tokens": float64(1)}, wantErr: "input_tokens"},
		{name: "a malformed optional counter", schema: "6", extra: map[string]any{"usage_reported": true, "input_tokens": float64(1), "output_tokens": float64(1), "thinking_tokens": "many"}, wantErr: "thinking_tokens"},
		{name: "a malformed cache counter", schema: "6", extra: map[string]any{"usage_reported": true, "input_tokens": float64(1), "output_tokens": float64(1), "cache_tokens": -3.0}, wantErr: "cache_tokens"},
		{name: "schema 1 ignores optional counters", schema: "1", extra: map[string]any{"usage_reported": true, "input_tokens": float64(1), "output_tokens": float64(1), "thinking_tokens": "garbage"}, check: func(t *testing.T, r *evalv1.ModelInferenceRecord) {
			assert.Nil(t, r.ThinkingTokens)
		}},
		{name: "retry count", schema: "6", extra: map[string]any{"retry_count": float64(3)}, check: func(t *testing.T, r *evalv1.ModelInferenceRecord) { assert.Equal(t, uint32(3), r.GetRetryCount()) }},
		{name: "retry count above the cap", schema: "6", extra: map[string]any{"retry_count": float64(1001)}, wantErr: "retry_count exceeds 1000"},
		{name: "negative retry count", schema: "6", extra: map[string]any{"retry_count": float64(-1)}, wantErr: "retry_count"},
		{name: "retry count of the wrong type", schema: "6", extra: map[string]any{"retry_count": "two"}, wantErr: "retry_count"},
		{name: "durations convert to nanoseconds", schema: "6", extra: map[string]any{"load_duration_seconds": 0.5, "generation_duration_seconds": 2.0, "total_duration_seconds": 3.25}, check: func(t *testing.T, r *evalv1.ModelInferenceRecord) {
			assert.Equal(t, uint64(500_000_000), r.GetLoadDurationNanos())
			assert.Equal(t, uint64(2_000_000_000), r.GetGenerationDurationNanos())
			assert.Equal(t, uint64(3_250_000_000), r.GetTotalDurationNanos())
		}},
		{name: "negative load duration", schema: "6", extra: map[string]any{"load_duration_seconds": -1.0}, wantErr: "load_duration_seconds"},
		{name: "non-finite generation duration", schema: "6", extra: map[string]any{"generation_duration_seconds": "fast"}, wantErr: "generation_duration_seconds"},
		{name: "negative total duration", schema: "6", extra: map[string]any{"total_duration_seconds": -0.1}, wantErr: "total_duration_seconds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			records, _, err := inferencesOf(t, tt.schema, importModelCall("p1", tt.extra))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, records, 1)
			tt.check(t, records[0])
		})
	}
}

func TestModelInferenceRecords_RejectMalformedModelCalls(t *testing.T) {
	t.Parallel()
	_, _, err := modelInferenceRecordsFromTrace(inferenceAssignment(), "a", nil, EvaluationTrace{"model_calls": "x"}, importIDs)
	require.ErrorContains(t, err, "model_calls must be an array")
	_, _, err = modelInferenceRecordsFromTrace(inferenceAssignment(), "a", nil, EvaluationTrace{}, importIDs)
	require.ErrorContains(t, err, "model_calls must be an array")
	_, _, err = modelInferenceRecordsFromTrace(inferenceAssignment(), "a", nil, EvaluationTrace{"model_calls": []any{"junk"}}, importIDs)
	require.ErrorContains(t, err, "model call must be an object")
}

// TestModelInferenceRecords_ScoredSpanCoversOnlyScoredCalls is R9: the memory
// update (agent_role codex) is recorded but never lengthens the scored span.
func TestModelInferenceRecords_ScoredSpanCoversOnlyScoredCalls(t *testing.T) {
	t.Parallel()
	bounds := func(start, end float64) map[string]any {
		return map[string]any{"monotonic_start": start, "monotonic_end": end}
	}
	withRole := func(role string, extra map[string]any) map[string]any {
		if extra == nil {
			extra = map[string]any{}
		}
		extra["agent_role"] = role
		return extra
	}
	postTurnMemory := func(extra map[string]any) map[string]any {
		extra = withRole("codex", extra)
		extra["classification"] = "post_turn"
		return extra
	}
	tests := []struct {
		name     string
		calls    []EvaluationTrace
		wantSpan *uint64
		wantErr  string
		wantRecs int
	}{
		{name: "one scored call", calls: []EvaluationTrace{importModelCall("p1", bounds(10, 12.5))}, wantSpan: ptr(2_500_000_000), wantRecs: 1},
		{
			name:     "the span runs from the earliest start to the latest end across scored calls",
			calls:    []EvaluationTrace{importModelCall("p1", bounds(11, 15)), importModelCall("p2", withRole("triage", bounds(10, 12)))},
			wantSpan: ptr(5_000_000_000), wantRecs: 2,
		},
		{
			name:     "a codex call after the scored work does not extend the span",
			calls:    []EvaluationTrace{importModelCall("p1", bounds(10, 12)), importModelCall("p2", postTurnMemory(bounds(20, 90)))},
			wantSpan: ptr(2_000_000_000), wantRecs: 2,
		},
		{
			name:     "a codex call before the scored work does not extend the span",
			calls:    []EvaluationTrace{importModelCall("p0", postTurnMemory(bounds(0, 1))), importModelCall("p1", bounds(10, 12))},
			wantSpan: ptr(2_000_000_000), wantRecs: 2,
		},
		{
			name:     "a codex call without bounds does not void the span",
			calls:    []EvaluationTrace{importModelCall("p1", bounds(10, 12)), importModelCall("p2", postTurnMemory(nil))},
			wantSpan: ptr(2_000_000_000), wantRecs: 2,
		},
		{name: "only codex calls leave no scored span", calls: []EvaluationTrace{importModelCall("p1", postTurnMemory(bounds(1, 2)))}, wantSpan: nil, wantRecs: 1},
		{name: "a scored call without bounds leaves no span", calls: []EvaluationTrace{importModelCall("p1", bounds(10, 12)), importModelCall("p2", nil)}, wantSpan: nil, wantRecs: 2},
		{name: "a call with only a start has no complete bounds", calls: []EvaluationTrace{importModelCall("p1", map[string]any{"monotonic_start": 10.0})}, wantSpan: nil, wantRecs: 1},
		{name: "end before start is rejected", calls: []EvaluationTrace{importModelCall("p1", bounds(12, 10))}, wantErr: "monotonic_end precedes monotonic_start"},
		{name: "a malformed start is rejected", calls: []EvaluationTrace{importModelCall("p1", map[string]any{"monotonic_start": "now", "monotonic_end": 1.0})}, wantErr: "monotonic_start"},
		{name: "a malformed end is rejected", calls: []EvaluationTrace{importModelCall("p1", map[string]any{"monotonic_start": 1.0, "monotonic_end": -2.0})}, wantErr: "monotonic_end"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			records, span, err := inferencesOf(t, "6", tt.calls...)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, records, tt.wantRecs, "every governed call is still recorded")
			assert.Equal(t, tt.wantSpan, span)
		})
	}
}

func ptr(value uint64) *uint64 { return &value }

func TestToolCallRecords_MapOnlyBoundedFields(t *testing.T) {
	t.Parallel()
	assignment := inferenceAssignment()
	trace := EvaluationTrace{"tool_calls": []any{
		EvaluationTrace{"call_id": "c1", "tool_name": toolGrep, "success": true, "arguments_hash": "h1", "loop_turn": float64(3), "is_operator_tool": true, "execution_id": "e1"},
		EvaluationTrace{"call_id": "c2", "tool_name": toolRead, "success": false, "suggestion": "try a path", "error_type": "validation.error", "loop_turn": "later"},
		EvaluationTrace{"tool_name": toolRun, "success": false, "error": "SECRET model-visible error", "loop_turn": nil},
		EvaluationTrace{"call_id": "c4", "tool_name": toolWrite, "is_operator_tool": true},
		EvaluationTrace{"call_id": "c5"},
		"junk",
	}}

	records := toolCallRecordsFromTrace(assignment, trace, func(prefix string) string { return prefix + "-gen" })

	require.Len(t, records, 4, "a call with no tool name and a non-object entry are skipped")
	assert.Equal(t, uint32(3), records[0].GetLoopTurn())
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, records[0].GetSchemaOutcome())
	assert.Equal(t, "e1", records[0].GetGovernedBindingRef().GetArtifactId())
	assert.False(t, records[0].GetGuidanceShown())

	assert.Equal(t, uint32(0), records[1].GetLoopTurn(), "a malformed loop turn reads as zero")
	assert.True(t, records[1].GetGuidanceShown(), "a suggestion alone is guidance")
	assert.Equal(t, "validation.error", records[1].GetErrorType())
	assert.Nil(t, records[1].GetGovernedBindingRef())

	assert.Equal(t, "tool-call-gen", records[2].GetCallId(), "a missing call id is generated")
	assert.True(t, records[2].GetGuidanceShown())
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, records[2].GetSchemaOutcome())

	assert.Nil(t, records[3].GetGovernedBindingRef(), "an operator call without an execution id has no receipt to bind")

	for _, record := range records {
		raw, err := json.Marshal(record)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "SECRET", "the model-visible error text is never copied into a record")
		assert.NotContains(t, string(raw), "try a path")
	}
}

func TestPolicyDecisionRecords(t *testing.T) {
	t.Parallel()
	assignment := inferenceAssignment()
	valid := map[string]any{"decision_id": "d1", "tool_name": toolRun, "outcome": "deny", "detail": "blocked"}
	with := func(key string, value any) map[string]any {
		out := map[string]any{}
		for k, v := range valid {
			out[k] = v
		}
		if value == nil {
			delete(out, key)
		} else {
			out[key] = value
		}
		return out
	}

	t.Run("absent field is not an error", func(t *testing.T) {
		t.Parallel()
		records, err := policyDecisionRecordsFromTrace(assignment, EvaluationTrace{})
		require.NoError(t, err)
		assert.Nil(t, records)
	})
	t.Run("each outcome maps", func(t *testing.T) {
		t.Parallel()
		want := map[string]evalv1.EvaluationPolicyDecisionOutcome{
			"allow":   evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_ALLOW,
			"deny":    evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_DENY,
			"refused": evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_REFUSED,
		}
		for outcome, expected := range want {
			records, err := policyDecisionRecordsFromTrace(assignment, EvaluationTrace{"policy_decisions": []any{with("outcome", outcome)}})
			require.NoError(t, err)
			require.Len(t, records, 1)
			assert.Equal(t, expected, records[0].GetOutcome())
			assert.Equal(t, "blocked", records[0].GetDetail())
		}
	})
	rejects := map[string]any{
		"not an array":         "x",
		"not an object":        []any{"x"},
		"no decision id":       []any{with("decision_id", nil)},
		"empty decision id":    []any{with("decision_id", "")},
		"tool name not string": []any{with("tool_name", 7)},
		"unknown outcome":      []any{with("outcome", "maybe")},
		"outcome not string":   []any{with("outcome", 3)},
		"detail not a string":  []any{with("detail", 3)},
		"no detail":            []any{with("detail", nil)},
	}
	for name, value := range rejects {
		t.Run("rejects "+name, func(t *testing.T) {
			t.Parallel()
			_, err := policyDecisionRecordsFromTrace(assignment, EvaluationTrace{"policy_decisions": value})
			require.Error(t, err)
		})
	}
}

func TestPartialAssignmentResultFromScoredTrace(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	reported := importModelCall("p1", map[string]any{"usage_reported": true, "input_tokens": float64(4), "output_tokens": float64(2)})
	unreported := importModelCall("p2", map[string]any{"usage_reported": false})

	t.Run("a nil assignment or empty trace yields nothing", func(t *testing.T) {
		t.Parallel()
		bad := req
		bad.Assignment = nil
		partial, ok, err := PartialAssignmentResultFromScoredTrace(bad, EvaluationTrace{"model_calls": []any{reported}}, nil)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Nil(t, partial)
		_, ok, err = PartialAssignmentResultFromScoredTrace(req, EvaluationTrace{}, nil)
		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("no reported usage yet yields nothing", func(t *testing.T) {
		t.Parallel()
		_, ok, err := PartialAssignmentResultFromScoredTrace(req, EvaluationTrace{"model_calls": []any{unreported}}, nil)
		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("a reported call yields a running partial result", func(t *testing.T) {
		t.Parallel()
		partial, ok, err := PartialAssignmentResultFromScoredTrace(req, EvaluationTrace{"schema_version": "6", "model_calls": []any{reported, unreported}}, nil)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING, partial.GetLifecycleStatus())
		assert.Len(t, partial.GetModelInferences(), 2)
		assert.Empty(t, partial.GetDeterministicGrades(), "a partial result is never graded")
	})
	t.Run("malformed reported usage is an error", func(t *testing.T) {
		t.Parallel()
		bad := importModelCall("p3", map[string]any{"usage_reported": true})
		_, _, err := PartialAssignmentResultFromScoredTrace(req, EvaluationTrace{"model_calls": []any{bad}}, nil)
		require.Error(t, err)
	})
}

func TestTraceHasReportedModelInference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		trace EvaluationTrace
		want  bool
	}{
		{name: "no model calls", trace: EvaluationTrace{}, want: false},
		{name: "reported governed call", trace: EvaluationTrace{"model_calls": []any{importModelCall("p", map[string]any{"usage_reported": true})}}, want: true},
		{name: "usage not reported", trace: EvaluationTrace{"model_calls": []any{importModelCall("p", map[string]any{"usage_reported": false})}}, want: false},
		{name: "usage flag of the wrong type", trace: EvaluationTrace{"model_calls": []any{importModelCall("p", map[string]any{"usage_reported": "true"})}}, want: false},
		{name: "reported but failed", trace: EvaluationTrace{"model_calls": []any{importModelCall("p", map[string]any{"usage_reported": true, "succeeded": false})}}, want: false},
		{name: "reported but not governed", trace: EvaluationTrace{"model_calls": []any{importModelCall("p", map[string]any{"usage_reported": true, "provider": "ollama"})}}, want: false},
		{name: "a later call reports", trace: EvaluationTrace{"model_calls": []any{"junk", importModelCall("a", nil), importModelCall("b", map[string]any{"usage_reported": true})}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, TraceHasReportedModelInference(tt.trace))
		})
	}
}

func TestBuildAssignmentTraceEvidenceReference(t *testing.T) {
	t.Parallel()
	trace := EvaluationTrace{"status": "completed", "b": []any{1.0, "x"}, "a": EvaluationTrace{"z": 1.0, "y": 2.0}}
	reordered := EvaluationTrace{"a": map[string]any{"y": 2.0, "z": 1.0}, "b": []any{1.0, "x"}, "status": "completed"}

	first, err := BuildAssignmentTraceEvidenceReference("run-1", "assignment-1", "attempt-1", trace, importNow)
	require.NoError(t, err)
	second, err := BuildAssignmentTraceEvidenceReference("run-1", "assignment-1", "attempt-1", reordered, importNow.Add(time.Hour))
	require.NoError(t, err)

	assert.Equal(t, first.GetArtifactId(), second.GetArtifactId(), "the address is a function of the canonical content, not key order or time")
	assert.Equal(t, first.GetSha256(), second.GetSha256())
	assert.Equal(t, "run-1", first.GetRunId())
	assert.Equal(t, "attempt-1", first.GetAttemptId())
	assert.Equal(t, "run-1", first.GetScopeId())
	assert.Equal(t, "imported", first.GetVerificationStatus())
	assert.Equal(t, "g8ee", first.GetProducerIdentity())

	changed, err := BuildAssignmentTraceEvidenceReference("run-1", "assignment-1", "attempt-1", EvaluationTrace{"status": "failed"}, importNow)
	require.NoError(t, err)
	assert.NotEqual(t, first.GetArtifactId(), changed.GetArtifactId())

	defaulted, err := BuildAssignmentTraceEvidenceReference("run-1", "assignment-1", "attempt-1", trace, time.Time{})
	require.NoError(t, err)
	assert.False(t, defaulted.GetProducedAt().AsTime().IsZero())

	for _, args := range [][3]string{{"", "a", "t"}, {"r", "", "t"}, {"r", "a", ""}} {
		_, err := BuildAssignmentTraceEvidenceReference(args[0], args[1], args[2], trace, importNow)
		require.Error(t, err, fmt.Sprint(args))
	}
	_, err = BuildAssignmentTraceEvidenceReference("r", "a", "t", EvaluationTrace{}, importNow)
	require.Error(t, err)
	_, err = BuildAssignmentTraceEvidenceReference("r", "a", "t", EvaluationTrace{"bad": func() {}}, importNow)
	require.Error(t, err)
}

func TestNumericConversionHelpers(t *testing.T) {
	t.Parallel()
	for _, value := range []any{float64(7), float32(7), int(7), int64(7), uint64(7), uint32(7), int32(7), int16(7), uint(7)} {
		converted, err := uint32Value(value)
		require.NoError(t, err, fmt.Sprintf("%T", value))
		assert.Equal(t, uint32(7), converted)
	}
	for _, value := range []any{"7", nil, true, float64(-1), float64(1.5), float64(5_000_000_000), json.Number("7")} {
		_, err := uint32Value(value)
		require.Error(t, err, fmt.Sprintf("%#v", value))
	}
	nanos, err := durationSecondsToNanosChecked(1.5)
	require.NoError(t, err)
	assert.Equal(t, uint64(1_500_000_000), nanos)
	for _, value := range []any{"x", -0.001, float64(1e30)} {
		_, err := durationSecondsToNanosChecked(value)
		require.Error(t, err, fmt.Sprintf("%#v", value))
	}
	assert.Equal(t, "1", traceSchemaVersion(EvaluationTrace{}))
	assert.Equal(t, "1", traceSchemaVersion(EvaluationTrace{"schema_version": ""}))
	assert.Equal(t, "6", traceSchemaVersion(EvaluationTrace{"schema_version": "6"}))
	assert.Equal(t, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_UNSPECIFIED, parseModelCampaignRole("tribunal"))
	assert.Equal(t, evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_UNSPECIFIED, parseModelCampaignRole(nil))
}
