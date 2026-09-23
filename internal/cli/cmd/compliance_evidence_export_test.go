// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestComplianceEvidenceCmd_ContainsExportSubcommand(t *testing.T) {
	cmd := complianceEvidenceCmd()
	require.Len(t, cmd.Commands(), 1)
	assert.Equal(t, "export", cmd.Commands()[0].Name())
}

func TestComplianceEvidenceExportCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceEvidenceExportCmdWithConfig(failingFileSvcFactory(errFactory))
	require.NoError(t, cmd.Flags().Set("scope", constants.TestInvalidJSONFilename))
	require.NoError(t, cmd.Flags().Set("out", constants.TestCustomComplianceOutDir))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}
