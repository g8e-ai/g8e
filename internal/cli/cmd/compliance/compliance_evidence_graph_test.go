// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliancecmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func runEvidenceGraphVerifyCommand(t *testing.T, fileSvc fs.RuntimeFileService, evalRuns []string) (*evidence.EvidenceGraphReport, error) {
	t.Helper()
	cmd := complianceEvidenceGraphVerifyCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))
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

func TestComplianceEvidenceGraphVerifyCmdWithConfig_RecordsImporterFailureAndFailsClosed(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)

	report, err := runEvidenceGraphVerifyCommand(t, fileSvc, []string{"missing-eval-run"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.False(t, report.Valid)
	require.Len(t, report.ImporterErrors, 1)
	assert.Equal(t, "native-evaluation", report.ImporterErrors[0].SourceID)
	assert.Equal(t, "missing-eval-run", report.ImporterErrors[0].RunID)
	assert.NotEmpty(t, report.ImporterErrors[0].Error)
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_RejectsInvalidRunIDs(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	cmd := complianceEvidenceGraphVerifyCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))
	require.NoError(t, cmd.Flags().Set("eval-run", "../invalid"))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPathValidation)
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_RejectsMissingRunFlags(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	cmd := complianceEvidenceGraphVerifyCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_RemovedDemoRunFlag(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	cmd := complianceEvidenceGraphVerifyCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))

	assert.Nil(t, cmd.Flags().Lookup("demo-run"), "the legacy --demo-run flag must not exist")
	require.Error(t, cmd.Flags().Set("demo-run", "any-run"), "setting --demo-run must fail on an unknown flag")
}
