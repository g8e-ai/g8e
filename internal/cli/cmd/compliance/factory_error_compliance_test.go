// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliancecmd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
)

var errFactory = fmt.Errorf("factory boom")

func TestComplianceKSIHistoryCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceKSIHistoryCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestComplianceOverlayCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceOverlayCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestComplianceDemoRunVerifyCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceDemoRunVerifyCmdWithConfig(
		cmdtest.FailingFileSvcFactory(errFactory),
		func(string) evidence.ProvenanceSource {
			panic("provenance source should not be created when fileSvcFactory fails")
		},
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, []string{"any-run"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceEvidenceGraphVerifyCmdWithConfig(
		cmdtest.FailingFileSvcFactory(errFactory),
		func(string) evidence.ProvenanceSource {
			panic("provenance source should not be created when fileSvcFactory fails")
		},
	)
	require.NoError(t, cmd.Flags().Set("demo-run", "any-run"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestComplianceReportGenerateCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceReportGenerateCmdWithConfig(
		cmdtest.FailingFileSvcFactory(errFactory),
		func(string) evidence.ProvenanceSource {
			panic("provenance source should not be created when fileSvcFactory fails")
		},
		func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
			panic("signing identity should not be loaded when fileSvcFactory fails")
		},
		time.Now,
	)
	require.NoError(t, cmd.Flags().Set("scope", constants.ComplianceBundleScopeFilename))
	require.NoError(t, cmd.Flags().Set("demo-run", "any-run"))
	require.NoError(t, cmd.Flags().Set("report-id", "report-1"))
	require.NoError(t, cmd.Flags().Set("signing-metadata", constants.ComplianceReportSigningMetadataTestFilename))
	require.NoError(t, cmd.Flags().Set("signing-private-key", constants.ComplianceReportSigningPrivateKeyTestFilename))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}
