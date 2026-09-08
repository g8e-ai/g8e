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
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

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

func TestComplianceReportCmd_ContainsGenerateAndVerifySubcommands(t *testing.T) {
	cmd := complianceReportCmd()
	require.Len(t, cmd.Commands(), 2)
	assert.Equal(t, "generate", cmd.Commands()[0].Name())
	assert.Equal(t, "verify", cmd.Commands()[1].Name())
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

func TestComplianceReportVerifyCmdWithConfig_PrintsTypedValidReport(t *testing.T) {
	verifiedAt := time.Unix(1_700_000_100, 0).UTC()
	loaderCalled := false
	cmd := complianceReportVerifyCmdWithConfig(
		func(_ context.Context, bundlePath, trustPolicyPath string) (complianceReportBundleInput, error) {
			loaderCalled = true
			assert.Equal(t, "bundle.json", bundlePath)
			assert.Equal(t, "assessed-trust.json", trustPolicyPath)
			return complianceReportBundleInput{}, nil
		},
		func(_ context.Context, request compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
			assert.Equal(t, verifiedAt, request.VerifiedAt)
			return &compliancev1.ComplianceVerificationReport{
				ReportId:               "report-1",
				Valid:                  true,
				VerifiedAt:             requestTimestamp(verifiedAt),
				VerifierId:             constants.ComplianceBundleVerifierID,
				VerifierVersion:        constants.ComplianceBundleVerifierVersion,
				ReproducedChecksumRoot: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}, nil
		},
		func() time.Time { return verifiedAt },
	)
	require.NoError(t, cmd.Flags().Set("trust-policy", "assessed-trust.json"))
	var output bytes.Buffer
	cmd.SetOut(&output)

	require.NoError(t, cmd.RunE(cmd, []string{"bundle.json"}))
	assert.True(t, loaderCalled)
	report := &compliancev1.ComplianceVerificationReport{}
	require.NoError(t, compliancev1.UnmarshalCanonical(bytes.TrimSpace(output.Bytes()), report))
	assert.True(t, report.GetValid())
	assert.Equal(t, "report-1", report.GetReportId())
}

func TestComplianceReportVerifyCmdWithConfig_ReturnsFailureAfterPrintingInvalidReport(t *testing.T) {
	verifiedAt := time.Unix(1_700_000_100, 0).UTC()
	cmd := complianceReportVerifyCmdWithConfig(
		func(context.Context, string, string) (complianceReportBundleInput, error) {
			return complianceReportBundleInput{}, nil
		},
		func(context.Context, compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
			return &compliancev1.ComplianceVerificationReport{
				ReportId:               "report-1",
				Valid:                  false,
				VerifiedAt:             requestTimestamp(verifiedAt),
				VerifierId:             constants.ComplianceBundleVerifierID,
				VerifierVersion:        constants.ComplianceBundleVerifierVersion,
				ReproducedChecksumRoot: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Failures:               []*compliancev1.VerificationFailure{{Code: constants.ErrChecksumMismatch.Error(), SubjectRef: constants.ComplianceBundleAnalysisPath, Reason: "digest mismatch"}},
			}, nil
		},
		func() time.Time { return verifiedAt },
	)
	require.NoError(t, cmd.Flags().Set("trust-policy", "assessed-trust.json"))
	var output bytes.Buffer
	cmd.SetOut(&output)

	err := cmd.RunE(cmd, []string{"bundle.json"})

	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.Contains(t, output.String(), constants.ErrChecksumMismatch.Error())
}

func TestComplianceReportVerifyCmdWithConfig_RequiresExternalTrustPolicy(t *testing.T) {
	cmd := complianceReportVerifyCmdWithConfig(
		func(context.Context, string, string) (complianceReportBundleInput, error) {
			t.Fatal("loader must not run without an explicit trust policy")
			return complianceReportBundleInput{}, nil
		},
		compliancereport.VerifyComplianceReportBundle,
		time.Now,
	)

	err := cmd.RunE(cmd, []string{"bundle.json"})

	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func requestTimestamp(value time.Time) *timestamppb.Timestamp {
	return timestamppb.New(value)
}
