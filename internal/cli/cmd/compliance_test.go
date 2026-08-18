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

	"github.com/g8e-ai/g8e/internal/constants"
)

// TestComplianceCmd_Structure asserts that complianceCmd() is non-nil and has
// the expected "export" and "ksi" subcommands.
func TestComplianceCmd_Structure(t *testing.T) {
	cmd := complianceCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "compliance", cmd.Use)

	subcommands := cmd.Commands()
	assert.Len(t, subcommands, 4)

	names := make(map[string]bool, 4)
	for _, sub := range subcommands {
		names[sub.Name()] = true
	}
	assert.True(t, names["export"], "compliance should have 'export' subcommand")
	assert.True(t, names["ksi"], "compliance should have 'ksi' subcommand")
	assert.True(t, names["ksi-history"], "compliance should have 'ksi-history' subcommand")
	assert.True(t, names["overlay"], "compliance should have 'overlay' subcommand")
}

// TestComplianceExportCmd_FileSvcFactoryError asserts that the export subcommand
// wraps fileSvcFactory errors with ErrFileServiceInit.
func TestComplianceExportCmd_FileSvcFactoryError(t *testing.T) {
	cmd := complianceExportCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// TestComplianceKSICmd_FileSvcFactoryError asserts that the ksi subcommand
// wraps fileSvcFactory errors with ErrFileServiceInit.
func TestComplianceKSICmd_FileSvcFactoryError(t *testing.T) {
	cmd := complianceKSICmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// TestComplianceKSIHistoryCmd_FileSvcFactoryError asserts that the ksi-history
// subcommand wraps fileSvcFactory errors with ErrFileServiceInit.
func TestComplianceKSIHistoryCmd_FileSvcFactoryError(t *testing.T) {
	cmd := complianceKSIHistoryCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// TestComplianceExportCmd_InvalidFormat asserts that a non-"oscal" format
// returns a validation error before touching fileSvcFactory.
func TestComplianceExportCmd_InvalidFormat(t *testing.T) {
	cmd := complianceExportCmdWithConfig(failingFileSvcFactory(errFactory))
	cmd.Flags().Set("format", "xml")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Contains(t, err.Error(), "xml")
}

// TestComplianceOverlayCmd_FileSvcFactoryError asserts that the overlay subcommand
// wraps fileSvcFactory errors with ErrFileServiceInit.
func TestComplianceOverlayCmd_FileSvcFactoryError(t *testing.T) {
	cmd := complianceOverlayCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// TestValidateCertClass asserts valid and invalid certification class inputs.
func TestValidateCertClass(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "class A", input: "A", wantErr: false},
		{name: "class B", input: "B", wantErr: false},
		{name: "class C", input: "C", wantErr: false},
		{name: "class D", input: "D", wantErr: false},
		{name: "lowercase c", input: "c", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "two chars", input: "AB", wantErr: true},
		{name: "invalid char", input: "X", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, err := validateCertClass(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, constants.ErrValidationFailed)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.input, string(class))
			}
		})
	}
}
