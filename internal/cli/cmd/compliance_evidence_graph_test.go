// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

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

func TestComplianceEvidenceGraphVerifyCmdWithConfig_RecordsImporterFailureAndFailsClosed(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)

	report, err := runEvidenceGraphVerifyCommand(t, fileSvc, &stubProvenanceSource{}, nil, []string{"missing-eval-run"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.False(t, report.Valid)
	require.Len(t, report.ImporterErrors, 1)
	assert.Equal(t, "native-evaluation", report.ImporterErrors[0].SourceID)
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
	projectRoot := writeDemoProvenanceTree(t)
	runID := persistMinimalDemoRunFixture(t, fileSvc, projectRoot)

	report, err := runEvidenceGraphVerifyCommand(t, fileSvc, evidence.NewDemoDirectoryProvenanceSource(projectRoot), []string{runID}, nil)
	require.NoError(t, err)
	assert.False(t, report.VerifiedAt.IsZero())
	assert.NotNil(t, report.NodesByType)
	assert.NotNil(t, report.NodesByScope)
	assert.Empty(t, report.Failures)
	assert.Empty(t, report.ImporterErrors)
}
