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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

type evidenceGraphEvalManifest struct {
	SchemaVersion       string    `json:"schema_version"`
	RunID               string    `json:"run_id"`
	SuiteID             string    `json:"suite_id"`
	SuiteVersion        string    `json:"suite_version"`
	CreatedAt           time.Time `json:"created_at"`
	OrchestratorVersion string    `json:"orchestrator_version"`
}

type evidenceGraphEvalTask struct {
	SchemaVersion string `json:"schema_version"`
	TaskID        string `json:"task_id"`
	SuiteID       string `json:"suite_id"`
	SuiteVersion  string `json:"suite_version"`
	PromptHash    string `json:"prompt_hash"`
}

type evidenceGraphEvalAttempt struct {
	SchemaVersion string    `json:"schema_version"`
	AttemptID     string    `json:"attempt_id"`
	RunID         string    `json:"run_id"`
	TaskID        string    `json:"task_id"`
	ArmID         string    `json:"arm_id"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at"`
	ReceiptRefs   []string  `json:"receipt_refs"`
	GradeRefs     []string  `json:"grade_refs"`
}

type evidenceGraphEvalMetric struct {
	SchemaVersion      string   `json:"schema_version"`
	MetricID           string   `json:"metric_id"`
	MetricVersion      string   `json:"metric_version"`
	AttemptID          string   `json:"attempt_id"`
	RunID              string   `json:"run_id"`
	ArmID              string   `json:"arm_id"`
	TaskID             string   `json:"task_id"`
	Value              float64  `json:"value"`
	Unit               string   `json:"unit"`
	Eligible           bool     `json:"eligible"`
	VerificationStatus string   `json:"verification_status"`
	GraderClass        string   `json:"grader_class"`
	EvidenceRefs       []string `json:"evidence_refs"`
}

func persistMinimalEvidenceGraphEvalFixture(t *testing.T, fileSvc fs.RuntimeFileService) string {
	t.Helper()
	const (
		runID     = "evidence-graph-eval-run"
		suiteID   = "evidence-graph-suite"
		taskID    = "evidence-graph-task"
		attemptID = "evidence-graph-attempt"
		metricID  = "evidence-graph-metric"
	)
	createdAt := time.Unix(1_700_000_000, 0).UTC()
	runDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.EvalRunsDirname, runID)
	ctx := context.Background()
	manifestBody, err := json.Marshal(evidenceGraphEvalManifest{SchemaVersion: "1.40.0", RunID: runID, SuiteID: suiteID, SuiteVersion: "1.0.0", CreatedAt: createdAt, OrchestratorVersion: "g8e-evals-test"})
	require.NoError(t, err)
	taskBody, err := json.Marshal(evidenceGraphEvalTask{SchemaVersion: "1.40.0", TaskID: taskID, SuiteID: suiteID, SuiteVersion: "1.0.0", PromptHash: "prompt-sha256"})
	require.NoError(t, err)
	attemptBody, err := json.Marshal(evidenceGraphEvalAttempt{SchemaVersion: "1.40.0", AttemptID: attemptID, RunID: runID, TaskID: taskID, ArmID: "direct", StartedAt: createdAt.Add(time.Second), EndedAt: createdAt.Add(2 * time.Second), ReceiptRefs: []string{}, GradeRefs: []string{metricID}})
	require.NoError(t, err)
	metricBody, err := json.Marshal(evidenceGraphEvalMetric{SchemaVersion: "1.40.0", MetricID: metricID, MetricVersion: "1.0.0", AttemptID: attemptID, RunID: runID, ArmID: "direct", TaskID: taskID, Value: 1, Unit: "boolean", Eligible: true, VerificationStatus: "verified", GraderClass: "deterministic", EvidenceRefs: []string{}})
	require.NoError(t, err)
	records := []struct {
		filename string
		body     []byte
	}{
		{constants.EvalRunManifestFilename, manifestBody},
		{constants.EvalRunTasksFilename, append(taskBody, '\n')},
		{constants.EvalRunAttemptsFilename, append(attemptBody, '\n')},
		{constants.EvalRunMetricsFilename, append(metricBody, '\n')},
	}
	require.NoError(t, fileSvc.MkdirAll(ctx, runDir, constants.PermDirStandard))
	for _, record := range records {
		require.NoError(t, fileSvc.WriteFile(ctx, filepath.Join(runDir, record.filename), record.body, constants.PermFileReadOnly))
	}
	for _, filename := range []string{constants.EvalRunReceiptsFilename, constants.EvalRunStagesFilename, constants.EvalRunEvidenceIndexFilename} {
		require.NoError(t, fileSvc.WriteFile(ctx, filepath.Join(runDir, filename), nil, constants.PermFileReadOnly))
	}
	return runID
}

func runEvidenceGraphVerifyCommand(t *testing.T, fileSvc fs.RuntimeFileService, source evidence.ProvenanceSource, demoRuns, evalRuns []string) (*evidence.EvidenceGraphReport, error) {
	t.Helper()
	cmd := complianceEvidenceGraphVerifyCmdWithConfig(fileSvcFactoryFor(fileSvc), func(string) evidence.ProvenanceSource { return source })
	for _, runID := range demoRuns {
		require.NoError(t, cmd.Flags().Set("demo-run", runID))
	}
	for _, runID := range evalRuns {
		require.NoError(t, cmd.Flags().Set("eval-run", runID))
	}
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	err := cmd.RunE(cmd, nil)
	var report evidence.EvidenceGraphReport
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(output.Bytes()), &report))
	return &report, err
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_AcceptsValidDemoRun(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	projectRoot := writeDemoProvenanceTree(t)
	runID := persistMinimalDemoRunFixture(t, fileSvc, projectRoot)

	report, err := runEvidenceGraphVerifyCommand(t, fileSvc, evidence.NewDemoDirectoryProvenanceSource(projectRoot), []string{runID}, nil)
	require.NoError(t, err)
	assert.True(t, report.Valid)
	assert.Positive(t, report.NodeCount)
	assert.Positive(t, report.NodesByType[string(evidence.ArtifactTypeDemoManifest)])
	assert.Equal(t, constants.EvidenceGraphVerifierID, report.VerifierID)
	assert.Equal(t, constants.EvidenceGraphVerifierVersion, report.VerifierVersion)
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_AcceptsValidEvalBundle(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	runID := persistMinimalEvidenceGraphEvalFixture(t, fileSvc)

	report, err := runEvidenceGraphVerifyCommand(t, fileSvc, &stubProvenanceSource{}, nil, []string{runID})
	require.NoError(t, err)
	assert.True(t, report.Valid)
	assert.Equal(t, 4, report.NodeCount)
	assert.Equal(t, 1, report.NodesByType[string(evidence.ArtifactTypeEvalManifest)])
	assert.Equal(t, 1, report.NodesByType[string(evidence.ArtifactTypeEvalTask)])
	assert.Equal(t, 1, report.NodesByType[string(evidence.ArtifactTypeEvalAttempt)])
	assert.Equal(t, 1, report.NodesByType[string(evidence.ArtifactTypeEvalMetric)])
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_AcceptsMixedDemoAndEvalGraph(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	projectRoot := writeDemoProvenanceTree(t)
	demoRunID := persistMinimalDemoRunFixture(t, fileSvc, projectRoot)
	evalRunID := persistMinimalEvidenceGraphEvalFixture(t, fileSvc)

	report, err := runEvidenceGraphVerifyCommand(t, fileSvc, evidence.NewDemoDirectoryProvenanceSource(projectRoot), []string{demoRunID}, []string{evalRunID})
	require.NoError(t, err)
	assert.True(t, report.Valid)
	assert.Positive(t, report.NodesByType[string(evidence.ArtifactTypeDemoManifest)])
	assert.Equal(t, 1, report.NodesByType[string(evidence.ArtifactTypeEvalManifest)])
	assert.Len(t, report.NodesByScope, 2)
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_RecordsImporterFailureAndFailsClosed(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)

	report, err := runEvidenceGraphVerifyCommand(t, fileSvc, &stubProvenanceSource{}, nil, []string{"missing-eval-run"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.False(t, report.Valid)
	require.Len(t, report.ImporterErrors, 1)
	assert.Equal(t, "eval-bundle", report.ImporterErrors[0].SourceID)
	assert.Equal(t, "missing-eval-run", report.ImporterErrors[0].RunID)
	assert.NotEmpty(t, report.ImporterErrors[0].Error)
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_RejectsInvalidRunIDs(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{name: "demo run traversal", flag: "demo-run"},
		{name: "eval run traversal", flag: "eval-run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSvc, _ := newCmdTestEnv(t)
			cmd := complianceEvidenceGraphVerifyCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil))
			require.NoError(t, cmd.Flags().Set(tt.flag, "../invalid"))

			err := cmd.RunE(cmd, nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrPathValidation)
		})
	}
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_RejectsMissingRunFlags(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceEvidenceGraphVerifyCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_PrintsTypedReportJSON(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	runID := persistMinimalEvidenceGraphEvalFixture(t, fileSvc)

	report, err := runEvidenceGraphVerifyCommand(t, fileSvc, &stubProvenanceSource{}, nil, []string{runID})
	require.NoError(t, err)
	assert.False(t, report.VerifiedAt.IsZero())
	assert.NotNil(t, report.NodesByType)
	assert.NotNil(t, report.NodesByScope)
	assert.Empty(t, report.Failures)
	assert.Empty(t, report.ImporterErrors)
}
