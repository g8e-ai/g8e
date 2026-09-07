// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func runComplianceReportGenerateCommand(t *testing.T, evalRuns []string) ([]byte, error) {
	t.Helper()
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil))
	windowStart := time.Unix(1_699_999_999, 0).UTC()
	windowEnd := time.Unix(1_700_000_100, 0).UTC()
	require.NoError(t, cmd.Flags().Set("scope-id", evidence.EvalScopeID("evidence-graph-suite")))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", strconv.FormatInt(windowStart.UnixMilli(), 10)))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", strconv.FormatInt(windowEnd.UnixMilli(), 10)))
	for _, runID := range evalRuns {
		require.NoError(t, cmd.Flags().Set("eval-run", runID))
	}
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	return bytes.TrimSpace(output.Bytes()), cmd.RunE(cmd, nil)
}

func TestComplianceReportCmd_ContainsGenerateSubcommand(t *testing.T) {
	cmd := complianceReportCmd()
	require.Len(t, cmd.Commands(), 1)
	assert.Equal(t, "generate", cmd.Commands()[0].Name())
}

func TestComplianceReportGenerateCmdWithConfig_ProducesCanonicalAnalysisFromVerifiedEvalEvidence(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	runID := persistMinimalEvidenceGraphEvalFixture(t, fileSvc)
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil))
	windowStart := time.Unix(1_699_999_999, 0).UTC()
	windowEnd := time.Unix(1_700_000_100, 0).UTC()
	require.NoError(t, cmd.Flags().Set("scope-id", evidence.EvalScopeID("evidence-graph-suite")))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", strconv.FormatInt(windowStart.UnixMilli(), 10)))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", strconv.FormatInt(windowEnd.UnixMilli(), 10)))
	require.NoError(t, cmd.Flags().Set("eval-run", runID))
	var output bytes.Buffer
	cmd.SetOut(&output)

	require.NoError(t, cmd.RunE(cmd, nil))
	analysis := &compliancev1.ComplianceAnalysis{}
	require.NoError(t, compliancev1.UnmarshalCanonical(bytes.TrimSpace(output.Bytes()), analysis))
	assert.Equal(t, evidence.EvalScopeID("evidence-graph-suite"), analysis.GetScopeRef())
	assert.Equal(t, constants.AnalysisBuilderID, analysis.GetGeneratorIdentity())
	assert.True(t, analysis.GetEvidenceGraphValid())
	assert.NotEmpty(t, analysis.GetAssertionAssessments())
	assert.NotEmpty(t, analysis.GetFrameworkAssessments())
}

func TestComplianceReportGenerateCmdWithConfig_RendersRequestedFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   compliancereport.Format
		contains string
	}{
		{name: "canonical JSON", format: compliancereport.FormatJSON, contains: "analysis_id"},
		{name: "OSCAL", format: compliancereport.FormatOSCAL, contains: "g8e Compliance Assessment Results"},
		{name: "Markdown", format: compliancereport.FormatMarkdown, contains: "# g8e Compliance Report"},
		{name: "HTML", format: compliancereport.FormatHTML, contains: "<!doctype html>"},
		{name: "CLI", format: compliancereport.FormatCLI, contains: "Compliance analysis:"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fileSvc, _ := newCmdTestEnv(t)
			runID := persistMinimalEvidenceGraphEvalFixture(t, fileSvc)
			cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil))
			windowStart := time.Unix(1_699_999_999, 0).UTC()
			windowEnd := time.Unix(1_700_000_100, 0).UTC()
			require.NoError(t, cmd.Flags().Set("scope-id", evidence.EvalScopeID("evidence-graph-suite")))
			require.NoError(t, cmd.Flags().Set("window-start-unix-ms", strconv.FormatInt(windowStart.UnixMilli(), 10)))
			require.NoError(t, cmd.Flags().Set("window-end-unix-ms", strconv.FormatInt(windowEnd.UnixMilli(), 10)))
			require.NoError(t, cmd.Flags().Set("eval-run", runID))
			require.NoError(t, cmd.Flags().Set("format", string(test.format)))
			var output bytes.Buffer
			cmd.SetOut(&output)

			require.NoError(t, cmd.RunE(cmd, nil))
			assert.Contains(t, output.String(), test.contains)
		})
	}
}

func TestComplianceReportGenerateCmdWithConfig_RejectsUnsupportedFormat(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil))
	require.NoError(t, cmd.Flags().Set("scope-id", "scope-1"))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", "1700000000000"))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", "1700000001000"))
	require.NoError(t, cmd.Flags().Set("eval-run", "run-1"))
	require.NoError(t, cmd.Flags().Set("format", "yaml"))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestComplianceReportGenerateCmdWithConfig_RejectsMissingEvidenceRuns(t *testing.T) {
	body, err := runComplianceReportGenerateCommand(t, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Empty(t, body)
}

func TestComplianceReportGenerateCmdWithConfig_FailsClosedOnImporterFailure(t *testing.T) {
	body, err := runComplianceReportGenerateCommand(t, []string{"missing-eval-run"})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.Empty(t, body)
}

func TestComplianceReportGenerateCmdWithConfig_RejectsInvalidEvidenceWindow(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil))
	require.NoError(t, cmd.Flags().Set("scope-id", "scope-1"))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", "1700000001000"))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", "1700000000000"))
	require.NoError(t, cmd.Flags().Set("eval-run", "run-1"))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}
