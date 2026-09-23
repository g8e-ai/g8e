// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type recordingCampaignVerificationPublication struct {
	completionRunIDs []string
	reports          []*evalv1.EvaluationVerificationReport
}

func (p *recordingCampaignVerificationPublication) PublishRunCompletion(_ context.Context, runID string, _ time.Time) (int, error) {
	p.completionRunIDs = append(p.completionRunIDs, runID)
	return 1, nil
}

func (p *recordingCampaignVerificationPublication) PublishRunVerification(_ context.Context, _ string, report *evalv1.EvaluationVerificationReport) (int, error) {
	p.reports = append(p.reports, report)
	return 1, nil
}

func assertPopulationBoundCampaignReport(t *testing.T, report *evalv1.EvaluationVerificationReport, expectedAssignments, verifiedAssignments uint32) {
	t.Helper()
	require.NotNil(t, report)
	assert.Equal(t, constants.CampaignVerifierVersion, report.GetSchemaVersion())
	assert.Equal(t, constants.CampaignVerifierVersion, report.GetVerifierContractVersion())
	assert.Equal(t, constants.EvaluationSourceVersion, report.GetVerifierReleaseVersion())
	assert.Equal(t, expectedAssignments, report.GetExpectedAssignmentCount())
	assert.Equal(t, verifiedAssignments, report.GetVerifiedAssignmentCount())
	assert.NotEmpty(t, report.GetVerifiedPopulationDigest())
	assert.NotEmpty(t, report.GetCampaignDigest())
	assert.NotEmpty(t, report.GetCatalogDigest())
	assert.NotEmpty(t, report.GetModelRegistryDigest())
	require.NotNil(t, report.GetReportDigestRef())
	assert.Len(t, report.GetReportDigestRef().GetSha256(), 64)
}

func prepareCampaignRunForExecute(t *testing.T, deps nativeEvalDeps, cmd *cobra.Command) (runID string, store *evaluation.Store) {
	t.Helper()
	root, err := cmd.Flags().GetString("project-root")
	require.NoError(t, err)

	result, err := runCampaignStartFlow(cmd, deps, campaignStartFlowOptions{
		ModelTag:    "qwen3:4b",
		PrepareOnly: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Plan)

	fileSvc, err := deps.fileSvcFactory(root, slog.Default())
	require.NoError(t, err)
	return result.Plan.RunID, evaluation.NewStore(fileSvc)
}

func designatedRoleLabel(assignment *evalv1.EvaluationAssignment) string {
	homogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous)
	if !ok || homogeneous.Homogeneous == nil {
		return "primary"
	}
	switch homogeneous.Homogeneous.GetDesignatedRole() {
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT:
		return "assistant"
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE:
		return "lite"
	default:
		return "primary"
	}
}

func buildCompletedCampaignTrace(
	assignment *evalv1.EvaluationAssignment,
	attemptID string,
	registryDigest string,
	inferenceSessionID string,
) map[string]any {
	role := designatedRoleLabel(assignment)
	trace := map[string]any{
		"schema_version":    "1",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      "2026-09-15T00:00:00+00:00",
		"role_outcome":      "invoked",
		"evaluation_context": map[string]any{
			"campaign_id":                assignment.GetCampaignId(),
			"run_id":                     assignment.GetRunId(),
			"assignment_id":              assignment.GetAssignmentId(),
			"evaluation_attempt_id":      attemptID,
			"scenario_id":                assignment.GetScenarioId(),
			"model_registry_digest":      registryDigest,
			"target_operator_session_id": inferenceSessionID,
			"evaluation_lane":            "model_role",
			"designated_model_role":      role,
		},
		"controlled_role_assignment": map[string]any{
			"designated_model_role": role,
		},
		"model_calls": []any{
			map[string]any{
				"agent_role":              "sage",
				"model_role":              role,
				"provider":                "G8EProvider",
				"governed_transaction_id": "tx-1",
				"governed_result_digest":  repeatTestHex('a', 64),
				"provider_attempt_id":     "attempt-1",
				"normalized_request_hash": repeatTestHex('b', 64),
				"output_hash":             repeatTestHex('c', 64),
			},
		},
	}
	digest, err := evaluation.ComputeChatProbeTraceDigest(trace)
	if err != nil {
		panic(fmt.Sprintf("buildCompletedCampaignTrace: %v", err))
	}
	trace["trace_digest"] = digest
	return trace
}

func repeatTestHex(ch byte, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = ch
	}
	return string(out)
}

func newTestEnsembleServer(traceFn func(assignmentID, attemptID string) map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == harnessclient.EnsembleChatPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"case_id":"case-1","investigation_id":"inv-1"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/evaluation/trace/"):
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/evaluation/trace/"), "/")
			if len(parts) != 2 || traceFn == nil {
				http.NotFound(w, r)
				return
			}
			trace := traceFn(parts[0], parts[1])
			if trace == nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(harnessclient.EnsembleEvaluationTraceResponse{Trace: trace})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRunCampaignExecute_ExecutesOneAssignment(t *testing.T) {
	root, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()
	defer enableCampaignWitnessGateway(t, root, deps)()

	runID, store := prepareCampaignRunForExecute(t, deps, cmd)
	controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string {
		return prefix + "-" + deps.newID()
	})
	assignment, ok, err := controller.ResumeNextAssignment(context.Background(), runID)
	require.NoError(t, err)
	require.True(t, ok)

	run, err := store.LoadRun(context.Background(), runID)
	require.NoError(t, err)
	spec, err := store.LoadCampaignSpec(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)

	ensemble := newTestEnsembleServer(func(assignmentID, attemptID string) map[string]any {
		if assignmentID != assignment.GetAssignmentId() {
			return nil
		}
		return buildCompletedCampaignTrace(assignment, attemptID, spec.GetModelRegistryDigest(), "infer-session")
	})
	defer ensemble.Close()

	var output bytes.Buffer
	cmd.SetOut(&output)
	executed, err := runCampaignExecute(cmd, deps, campaignExecuteOptions{
		RunID:              runID,
		Limit:              1,
		EnsembleURL:        ensemble.URL,
		NoAutoRefresh:      true,
		InferenceSessionID: "infer-session",
		DataSessionID:      "data-session",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, executed)
	assert.Contains(t, output.String(), assignment.GetAssignmentId())
}

func TestRunCampaignExecute_RejectsMissingRun(t *testing.T) {
	_, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	executed, err := runCampaignExecute(cmd, deps, campaignExecuteOptions{
		RunID: "missing-run-id",
		Limit: 1,
	})
	require.Error(t, err)
	assert.Zero(t, executed)
	assert.Contains(t, err.Error(), "campaign execute")
}

func TestRunCampaignExecute_DefaultsLimitToOne(t *testing.T) {
	root, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()
	defer enableCampaignWitnessGateway(t, root, deps)()

	runID, store := prepareCampaignRunForExecute(t, deps, cmd)
	controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string {
		return prefix + "-" + deps.newID()
	})
	assignment, ok, err := controller.ResumeNextAssignment(context.Background(), runID)
	require.NoError(t, err)
	require.True(t, ok)

	run, err := store.LoadRun(context.Background(), runID)
	require.NoError(t, err)
	spec, err := store.LoadCampaignSpec(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)

	ensemble := newTestEnsembleServer(func(assignmentID, attemptID string) map[string]any {
		if assignmentID != assignment.GetAssignmentId() {
			return nil
		}
		return buildCompletedCampaignTrace(assignment, attemptID, spec.GetModelRegistryDigest(), "infer-session")
	})
	defer ensemble.Close()

	executed, err := runCampaignExecute(cmd, deps, campaignExecuteOptions{
		RunID:              runID,
		EnsembleURL:        ensemble.URL,
		NoAutoRefresh:      true,
		InferenceSessionID: "infer-session",
		DataSessionID:      "data-session",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, executed)
}

func TestVerifyCampaignRun_PersistsAndPublishesPopulationBoundReport(t *testing.T) {
	root, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()
	defer enableCampaignWitnessGateway(t, root, deps)()
	publication := &recordingCampaignVerificationPublication{}
	deps.campaignPublicationFactory = func(*cobra.Command, fs.RuntimeFileService) (campaignVerificationPublication, error) {
		return publication, nil
	}

	runID, store := prepareCampaignRunForExecute(t, deps, cmd)
	controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string {
		return prefix + "-" + deps.newID()
	})
	assignment, ok, err := controller.ResumeNextAssignment(context.Background(), runID)
	require.NoError(t, err)
	require.True(t, ok)

	run, err := store.LoadRun(context.Background(), runID)
	require.NoError(t, err)
	spec, err := store.LoadCampaignSpec(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)

	ensemble := newTestEnsembleServer(func(assignmentID, attemptID string) map[string]any {
		if assignmentID != assignment.GetAssignmentId() {
			return nil
		}
		return buildCompletedCampaignTrace(assignment, attemptID, spec.GetModelRegistryDigest(), "infer-session")
	})
	defer ensemble.Close()

	executed, err := runCampaignExecute(cmd, deps, campaignExecuteOptions{
		RunID:              runID,
		Limit:              1,
		EnsembleURL:        ensemble.URL,
		NoAutoRefresh:      true,
		InferenceSessionID: "infer-session",
		DataSessionID:      "data-session",
	})
	require.NoError(t, err)
	require.Equal(t, 1, executed)

	var output bytes.Buffer
	cmd.SetOut(&output)
	report, err := verifyCampaignRun(cmd, deps, runID, false, false, false)
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, report.GetStatus())

	loaded, err := store.LoadCampaignVerification(context.Background(), runID)
	require.NoError(t, err)
	assert.Equal(t, report.GetStatus(), loaded.GetStatus())
	assertPopulationBoundCampaignReport(t, loaded, 75, 1)
	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM, loaded.GetProviderObservationPolicy())
	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM, loaded.GetModelProvenancePolicy())
	assert.Equal(t, spec.GetCampaignDigest(), loaded.GetCampaignDigest())
	assert.Equal(t, spec.GetCatalogDigest(), loaded.GetCatalogDigest())
	assert.Equal(t, spec.GetModelRegistryDigest(), loaded.GetModelRegistryDigest())
	assert.Equal(t, []string{runID}, publication.completionRunIDs)
	require.Len(t, publication.reports, 1)
	assertPopulationBoundCampaignReport(t, publication.reports[0], 75, 1)
	assert.Equal(t, loaded.GetReportDigestRef().GetSha256(), publication.reports[0].GetReportDigestRef().GetSha256())
}

func TestVerifyCampaignRun_RejectsMissingRun(t *testing.T) {
	_, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	report, err := verifyCampaignRun(cmd, deps, "missing-run-id", false, false, false)
	require.Error(t, err)
	assert.Nil(t, report)
	assert.Contains(t, err.Error(), "campaign verify")
}

type campaignTraceLookup struct {
	root string
	deps nativeEvalDeps
}

func (l *campaignTraceLookup) resolve(assignmentID, attemptID string) map[string]any {
	fileSvc, err := l.deps.fileSvcFactory(l.root, slog.Default())
	if err != nil {
		return nil
	}
	store := evaluation.NewStore(fileSvc)
	active, err := evaluation.LoadActiveCampaignRun(l.root)
	if err != nil || active.RunID == "" {
		return nil
	}
	assignment, err := store.LoadAssignment(context.Background(), active.RunID, assignmentID)
	if err != nil {
		return nil
	}
	run, err := store.LoadRun(context.Background(), active.RunID)
	if err != nil {
		return nil
	}
	spec, err := store.LoadCampaignSpec(context.Background(), run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return nil
	}
	return buildCompletedCampaignTrace(assignment, attemptID, spec.GetModelRegistryDigest(), "infer-session")
}

func TestRunCampaignStartFlow_ExecuteAndVerifyPersistsPopulationBoundReportWithoutPublication(t *testing.T) {
	root, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()
	defer enableCampaignWitnessGateway(t, root, deps)()
	publication := &recordingCampaignVerificationPublication{}
	deps.campaignPublicationFactory = func(*cobra.Command, fs.RuntimeFileService) (campaignVerificationPublication, error) {
		return publication, nil
	}

	lookup := &campaignTraceLookup{root: root, deps: deps}
	ensemble := newTestEnsembleServer(lookup.resolve)
	defer ensemble.Close()

	var output bytes.Buffer
	cmd.SetOut(&output)
	result, err := runCampaignStartFlow(cmd, deps, campaignStartFlowOptions{
		ModelTag:           "qwen3:4b",
		EnsembleURL:        ensemble.URL,
		NoAutoRefresh:      true,
		Verify:             true,
		InferenceSessionID: "infer-session",
		DataSessionID:      "data-session",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Plan)
	assert.Equal(t, 1, result.Executed)
	require.NotNil(t, result.Report)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, result.Report.GetStatus())
	assertPopulationBoundCampaignReport(t, result.Report, 75, 1)
	assert.Contains(t, output.String(), "verification")

	fileSvc, err := deps.fileSvcFactory(root, slog.Default())
	require.NoError(t, err)
	loaded, err := evaluation.NewStore(fileSvc).LoadCampaignVerification(context.Background(), result.Plan.RunID)
	require.NoError(t, err)
	assertPopulationBoundCampaignReport(t, loaded, 75, 1)
	assert.Empty(t, publication.completionRunIDs)
	assert.Empty(t, publication.reports)
}
