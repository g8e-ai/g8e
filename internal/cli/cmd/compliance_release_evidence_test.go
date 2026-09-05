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
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// TestValidateReleaseVersion asserts valid and invalid release version inputs.
func TestValidateReleaseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid version", input: "v2.1.3", wantErr: false},
		{name: "valid patch zero", input: "v2.0.0", wantErr: false},
		{name: "valid prerelease", input: "v2.1.3-beta.1", wantErr: false},
		{name: "valid build metadata", input: "v2.1.3+build123", wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "missing v prefix", input: "2.1.3", wantErr: true},
		{name: "lone v", input: "v", wantErr: true},
		{name: "incomplete semver two parts", input: "v2.1", wantErr: true},
		{name: "leading zero in major", input: "v02.1.0", wantErr: true},
		{name: "slash traversal", input: "v2.1.3/../../evil", wantErr: true},
		{name: "backslash traversal", input: `v2.1.3\..\evil`, wantErr: true},
		{name: "parent dir prefix", input: "../v2.1.3", wantErr: true},
		{name: "subpath", input: "v2.1.3/sub", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReleaseVersion(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, constants.ErrValidationFailed)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestReleaseEvidenceReport_OverallPassing verifies the fail-closed gating
// logic across KSI availability, KSI status, and demo-run validity.
func TestReleaseEvidenceReport_OverallPassing(t *testing.T) {
	satisfied := compliance.KSIResult{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied, MethodCount: 2}
	notSatisfied := compliance.KSIResult{ID: "KSI-IAM-05", Status: compliance.KSIStatusNotSatisfied}
	notApplicable := compliance.KSIResult{ID: "KSI-TPR-02", Status: compliance.KSIStatusNotApplicable}
	unknownStatus := compliance.KSIResult{ID: "KSI-UNK-01", Status: "unknown_status"}
	validDemo := &demoRunSummary{RunID: "run-1", Report: &compliancev1.ComplianceVerificationReport{Valid: true}}
	invalidDemo := &demoRunSummary{RunID: "run-2", Report: &compliancev1.ComplianceVerificationReport{Valid: false, Failures: []*compliancev1.VerificationFailure{{Code: "x", Reason: "bad"}}}}

	tests := []struct {
		name     string
		report   *releaseEvidenceReport
		passing  bool
		reasonRe string
	}{
		{
			name:     "ksi unavailable",
			report:   &releaseEvidenceReport{KSIUnavailable: "stores down"},
			passing:  false,
			reasonRe: "KSI evaluation unavailable: stores down",
		},
		{
			name:     "ksi not satisfied",
			report:   &releaseEvidenceReport{KSISet: &compliance.KSIResultSet{Results: []compliance.KSIResult{satisfied, notSatisfied}}},
			passing:  false,
			reasonRe: "KSI KSI-IAM-05 is not satisfied",
		},
		{
			name:     "ksi unknown status",
			report:   &releaseEvidenceReport{KSISet: &compliance.KSIResultSet{Results: []compliance.KSIResult{satisfied, unknownStatus}}},
			passing:  false,
			reasonRe: "KSI KSI-UNK-01 has non-passing status: unknown_status",
		},
		{
			name:     "demo run invalid",
			report:   &releaseEvidenceReport{KSISet: &compliance.KSIResultSet{Results: []compliance.KSIResult{satisfied, notApplicable}}, DemoReports: []*demoRunSummary{validDemo, invalidDemo}},
			passing:  false,
			reasonRe: "demo run run-2 is invalid",
		},
		{
			name:     "demo run verification error",
			report:   &releaseEvidenceReport{KSISet: &compliance.KSIResultSet{Results: []compliance.KSIResult{satisfied, notApplicable}}, DemoReports: []*demoRunSummary{{RunID: "run-3", Err: errors.New("read failed")}}},
			passing:  false,
			reasonRe: "demo run run-3 verification failed: read failed",
		},
		{
			name:     "demo enumeration error",
			report:   &releaseEvidenceReport{KSISet: &compliance.KSIResultSet{Results: []compliance.KSIResult{satisfied}}, DemoEnumerationErr: errors.New("permission denied")},
			passing:  false,
			reasonRe: "failed to enumerate demo runs: permission denied",
		},
		{
			name:     "ksi history error",
			report:   &releaseEvidenceReport{KSISet: &compliance.KSIResultSet{Results: []compliance.KSIResult{satisfied}}, KSIHistoryErr: errors.New("disk failure")},
			passing:  false,
			reasonRe: "failed to read KSI history: disk failure",
		},
		{
			name:     "empty report no ksis and no demo runs",
			report:   &releaseEvidenceReport{KSISet: &compliance.KSIResultSet{Results: []compliance.KSIResult{}}},
			passing:  false,
			reasonRe: "no evidence collected: KSI results and demo runs are both empty",
		},
		{
			name:    "all passing",
			report:  &releaseEvidenceReport{KSISet: &compliance.KSIResultSet{Results: []compliance.KSIResult{satisfied, notApplicable}}, DemoReports: []*demoRunSummary{validDemo}},
			passing: true,
		},
		{
			name:    "all passing no demo runs",
			report:  &releaseEvidenceReport{KSISet: &compliance.KSIResultSet{Results: []compliance.KSIResult{satisfied}}},
			passing: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.passing, tt.report.OverallPassing())
			if !tt.passing {
				assert.Regexp(t, tt.reasonRe, tt.report.FailClosedReason())
			}
		})
	}
}

// TestRenderReleaseEvidenceMarkdown_KSIUnavailableAndDemoRun verifies the
// markdown renderer surfaces a KSI-unavailable gap and a valid demo run.
func TestRenderReleaseEvidenceMarkdown_KSIUnavailableAndDemoRun(t *testing.T) {
	report := &releaseEvidenceReport{
		ReleaseVersion: "v2.1.3",
		GeneratedAt:    time.Unix(1_700_000_000, 0).UTC(),
		CertClass:      "C",
		CatalogPath:    constants.DefaultKSICatalogPath,
		KSIUnavailable: "audit store unavailable",
		DemoReports: []*demoRunSummary{{
			RunID: "fedramp-run-1",
			Report: &compliancev1.ComplianceVerificationReport{
				Valid:                  true,
				VerifierId:             constants.DemoRunVerifierID,
				VerifierVersion:        constants.DemoRunVerifierVersion,
				ReproducedChecksumRoot: "abc123",
				VerifiedAt:             timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
			},
		}},
	}

	md := renderReleaseEvidenceMarkdown(report)
	assert.Contains(t, md, "v2.1.3")
	assert.Contains(t, md, "KSI evaluation unavailable: audit store unavailable")
	assert.Contains(t, md, "fedramp-run-1")
	assert.Contains(t, md, "| true |")
	assert.Contains(t, md, constants.DemoRunVerifierID)
	assert.Contains(t, md, "Claim Boundaries")
}

// TestRenderReleaseEvidenceMarkdown_KSISatisfied verifies the markdown renderer
// renders the KSI evaluation table when KSIs are available.
func TestRenderReleaseEvidenceMarkdown_KSISatisfied(t *testing.T) {
	report := &releaseEvidenceReport{
		ReleaseVersion: "v2.1.3",
		GeneratedAt:    time.Unix(1_700_000_000, 0).UTC(),
		CertClass:      "C",
		CatalogPath:    constants.DefaultKSICatalogPath,
		KSISet: &compliance.KSIResultSet{
			Class:         compliance.ClassC,
			EvaluatedAtMs: 1_700_000_000_000,
			Results: []compliance.KSIResult{
				{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied, MethodCount: 2, LastValidatedUnixMs: 1_700_000_000_000},
			},
		},
		KSIHistory: []compliance.KSIResultSet{
			{EvaluatedAtMs: 1_699_990_000_000},
			{EvaluatedAtMs: 1_700_000_000_000},
		},
	}

	md := renderReleaseEvidenceMarkdown(report)
	assert.Contains(t, md, "| KSI ID | Status | Methods | Last Validated |")
	assert.Contains(t, md, "KSI-CMT-01")
	assert.Contains(t, md, "satisfied")
	assert.Contains(t, md, "2 snapshot(s)")
}

// TestRenderReleaseEvidenceCSV_Rows verifies the CSV renderer emits a header
// row and one row per evidence item with the expected columns.
func TestRenderReleaseEvidenceCSV_Rows(t *testing.T) {
	report := &releaseEvidenceReport{
		ReleaseVersion: "v2.1.3",
		GeneratedAt:    time.Unix(1_700_000_000, 0).UTC(),
		KSISet: &compliance.KSIResultSet{
			EvaluatedAtMs: 1_700_000_000_000,
			Results: []compliance.KSIResult{
				{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied, MethodCount: 2, LastValidatedUnixMs: 1_700_000_000_000},
			},
		},
		DemoReports: []*demoRunSummary{{
			RunID: "fedramp-run-1",
			Report: &compliancev1.ComplianceVerificationReport{
				Valid:                  true,
				VerifierId:             constants.DemoRunVerifierID,
				VerifierVersion:        constants.DemoRunVerifierVersion,
				ReproducedChecksumRoot: "abc123",
				VerifiedAt:             timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
			},
		}},
	}

	csvBytes, err := renderReleaseEvidenceCSV(report)
	require.NoError(t, err)
	r := csv.NewReader(strings.NewReader(string(csvBytes)))
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3)
	assert.Equal(t, []string{"evidence_type", "identifier", "status", "valid", "method_count", "last_validated", "failure_count", "verifier_id", "verifier_version", "checksum_root", "evaluated_at"}, records[0])
	assert.Equal(t, "ksi", records[1][0])
	assert.Equal(t, "KSI-CMT-01", records[1][1])
	assert.Equal(t, "satisfied", records[1][2])
	assert.Equal(t, "demo-run", records[2][0])
	assert.Equal(t, "fedramp-run-1", records[2][1])
	assert.Equal(t, "true", records[2][3])
	assert.Equal(t, constants.DemoRunVerifierID, records[2][7])
}

// TestRenderReleaseEvidenceCSV_KSIUnavailable verifies the CSV records a KSI
// unavailable row when KSI evaluation could not run.
func TestRenderReleaseEvidenceCSV_KSIUnavailable(t *testing.T) {
	report := &releaseEvidenceReport{
		ReleaseVersion: "v2.1.3",
		GeneratedAt:    time.Unix(1_700_000_000, 0).UTC(),
		KSIUnavailable: "stores down",
	}
	csvBytes, err := renderReleaseEvidenceCSV(report)
	require.NoError(t, err)
	r := csv.NewReader(strings.NewReader(string(csvBytes)))
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "ksi", records[1][0])
	assert.Equal(t, "unavailable", records[1][2])
}

// TestWriteReleaseEvidenceArtifacts_WritesBothFiles verifies the markdown and
// CSV files are written to the output directory with the version-derived
// filenames.
func TestWriteReleaseEvidenceArtifacts_WritesBothFiles(t *testing.T) {
	report := &releaseEvidenceReport{
		ReleaseVersion: "v2.1.3",
		GeneratedAt:    time.Unix(1_700_000_000, 0).UTC(),
		KSISet:         &compliance.KSIResultSet{Results: []compliance.KSIResult{{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied}}},
	}
	outDir := t.TempDir()
	require.NoError(t, writeReleaseEvidenceArtifacts(report, outDir))

	mdPath := filepath.Join(outDir, "v2.1.3"+constants.ReleaseEvidenceMarkdownSuffix)
	csvPath := filepath.Join(outDir, "v2.1.3"+constants.ReleaseEvidenceCSVSuffix)
	info, err := os.Stat(mdPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
	info, err = os.Stat(csvPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestWriteReleaseEvidenceArtifacts_EmptyOutDirFails verifies a missing --out
// is rejected.
func TestWriteReleaseEvidenceArtifacts_EmptyOutDirFails(t *testing.T) {
	report := &releaseEvidenceReport{ReleaseVersion: "v2.1.3"}
	err := writeReleaseEvidenceArtifacts(report, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

// TestWriteReleaseEvidenceArtifacts_PathTraversalRejected verifies that release
// versions attempting directory traversal are rejected and cannot escape outDir.
func TestWriteReleaseEvidenceArtifacts_PathTraversalRejected(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{name: "parent dir traversal", version: "../../evil"},
		{name: "subpath traversal", version: "foo/../../evil"},
	}
	outDir := t.TempDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &releaseEvidenceReport{ReleaseVersion: tt.version}
			err := writeReleaseEvidenceArtifacts(report, outDir)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrPathValidation)
		})
	}
}

type errorReadDirFileService struct {
	fs.RuntimeFileService
	err error
}

func (m *errorReadDirFileService) ReadDir(ctx context.Context, relPath string) ([]os.DirEntry, error) {
	return nil, m.err
}

// TestEnumerateDemoRunIDs_ErrorPropagation verifies that non-not-found errors
// from ReadDir are propagated while ErrNotFound returns an empty list.
func TestEnumerateDemoRunIDs_ErrorPropagation(t *testing.T) {
	ctx := context.Background()

	t.Run("propagates non-not-found error", func(t *testing.T) {
		mockSvc := &errorReadDirFileService{err: errors.New("permission denied")}
		runIDs, err := enumerateDemoRunIDs(ctx, mockSvc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
		assert.Nil(t, runIDs)
	})

	t.Run("returns empty list on not found", func(t *testing.T) {
		mockSvc := &errorReadDirFileService{err: constants.ErrNotFound}
		runIDs, err := enumerateDemoRunIDs(ctx, mockSvc)
		require.NoError(t, err)
		assert.Nil(t, runIDs)
	})
}

// TestComplianceReleaseEvidenceCmdWithConfig_GeneratesReportWithDemoRun
// exercises the full command against a hermetic runtime tree: KSI evaluation
// runs against the test catalog (KSIs report not_satisfied since the test
// audit store has no real governance data), one demo run fixture is persisted
// and verifies valid, and the markdown + CSV are written to the output
// directory.
func TestComplianceReleaseEvidenceCmdWithConfig_GeneratesReportWithDemoRun(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	projectRoot := writeDemoProvenanceTree(t)
	runID := persistMinimalDemoRunFixture(t, fileSvc, projectRoot)

	catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)
	source := evidence.NewDemoDirectoryProvenanceSource(projectRoot)
	cmd := complianceReleaseEvidenceCmdWithConfig(
		fileSvcFactoryFor(fileSvc),
		func(string) evidence.ProvenanceSource { return source },
	)
	outDir := t.TempDir()
	require.NoError(t, cmd.Flags().Set("version", "v2.1.3"))
	require.NoError(t, cmd.Flags().Set("out", outDir))
	require.NoError(t, cmd.Flags().Set("catalog", catPath))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	mdPath := filepath.Join(outDir, "v2.1.3"+constants.ReleaseEvidenceMarkdownSuffix)
	csvPath := filepath.Join(outDir, "v2.1.3"+constants.ReleaseEvidenceCSVSuffix)
	md, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	csvData, err := os.ReadFile(csvPath)
	require.NoError(t, err)

	mdStr := string(md)
	assert.Contains(t, mdStr, "v2.1.3")
	assert.Contains(t, mdStr, "KSI evaluation | available")
	assert.Contains(t, mdStr, runID)
	assert.Contains(t, mdStr, "| true |")
	assert.Contains(t, buf.String(), "wrote "+mdPath)
	assert.Contains(t, buf.String(), "wrote "+csvPath)

	// CSV has header + 2 KSI rows + 1 demo-run row (test catalog has 2 KSIs).
	r := csv.NewReader(strings.NewReader(string(csvData)))
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 4)
	assert.Equal(t, "evidence_type", records[0][0])
	assert.Equal(t, "ksi", records[1][0])
	assert.Equal(t, "not_satisfied", records[1][2])
	assert.Equal(t, "ksi", records[2][0])
	assert.Equal(t, "demo-run", records[3][0])
	assert.Equal(t, runID, records[3][1])
	assert.Equal(t, "true", records[3][3])
}

// TestComplianceReleaseEvidenceCmdWithConfig_ExplicitDemoRunFlag verifies the
// --demo-run flag selects a specific run ID rather than enumerating all runs.
func TestComplianceReleaseEvidenceCmdWithConfig_ExplicitDemoRunFlag(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	projectRoot := writeDemoProvenanceTree(t)
	runID := persistMinimalDemoRunFixture(t, fileSvc, projectRoot)

	catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)
	source := evidence.NewDemoDirectoryProvenanceSource(projectRoot)
	cmd := complianceReleaseEvidenceCmdWithConfig(
		fileSvcFactoryFor(fileSvc),
		func(string) evidence.ProvenanceSource { return source },
	)
	outDir := t.TempDir()
	require.NoError(t, cmd.Flags().Set("version", "v2.1.3"))
	require.NoError(t, cmd.Flags().Set("out", outDir))
	require.NoError(t, cmd.Flags().Set("catalog", catPath))
	require.NoError(t, cmd.Flags().Set("demo-run", runID))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	mdPath := filepath.Join(outDir, "v2.1.3"+constants.ReleaseEvidenceMarkdownSuffix)
	md, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	assert.Contains(t, string(md), runID)
}

// TestComplianceReleaseEvidenceCmdWithConfig_InvalidVersionFails verifies the
// command rejects an invalid version before touching the file service.
func TestComplianceReleaseEvidenceCmdWithConfig_InvalidVersionFails(t *testing.T) {
	cmd := complianceReleaseEvidenceCmdWithConfig(
		failingFileSvcFactory(errFactory),
		stubProvenanceSourceFactory(nil),
	)
	require.NoError(t, cmd.Flags().Set("version", "2.1.3"))
	require.NoError(t, cmd.Flags().Set("out", t.TempDir()))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

// TestComplianceReleaseEvidenceCmdWithConfig_FailClosedExitsNonzero verifies
// the --fail-closed flag surfaces a non-passing report as a command error.
func TestComplianceReleaseEvidenceCmdWithConfig_FailClosedExitsNonzero(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)

	cmd := complianceReleaseEvidenceCmdWithConfig(
		fileSvcFactoryFor(fileSvc),
		stubProvenanceSourceFactory(nil),
	)
	outDir := t.TempDir()
	require.NoError(t, cmd.Flags().Set("version", "v2.1.3"))
	require.NoError(t, cmd.Flags().Set("out", outDir))
	require.NoError(t, cmd.Flags().Set("catalog", catPath))
	require.NoError(t, cmd.Flags().Set("fail-closed", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrComplianceReleaseEvidence)
	// Report is still written despite fail-closed exit.
	_, statErr := os.Stat(filepath.Join(outDir, "v2.1.3"+constants.ReleaseEvidenceMarkdownSuffix))
	require.NoError(t, statErr)
}

// TestEnumerateDemoRunIDs_NoDirReturnsEmpty verifies a missing demo-evidence
// directory yields no run IDs rather than an error.
func TestEnumerateDemoRunIDs_NoDirReturnsEmpty(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	runIDs, err := enumerateDemoRunIDs(context.Background(), fileSvc)
	require.NoError(t, err)
	assert.Empty(t, runIDs)
}
