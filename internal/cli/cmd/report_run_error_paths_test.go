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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportAllCmd_EmptyDataReturnsSuccess(t *testing.T) {
	chdirTemp(t)

	cmd := reportAllCmd()
	cmd.SetContext(context.Background())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Verification PASSED")
}

func TestReportVerifyCmd_EmptyDataReturnsSuccess(t *testing.T) {
	chdirTemp(t)

	cmd := reportVerifyCmd()
	cmd.SetContext(context.Background())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Verification PASSED")
}

func TestReportFlags_AddFlags_RegistersAllFlags(t *testing.T) {
	cmd := reportAllCmd()
	expectedFlags := []string{"out", "data-dir", "runtime-dir", "ledger-dir"}
	for _, flagName := range expectedFlags {
		flag := cmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "report all should have --%s flag", flagName)
	}
}

func TestReportVerifyCmd_AddFlags_RegistersAllFlags(t *testing.T) {
	cmd := reportVerifyCmd()
	expectedFlags := []string{"out", "data-dir", "runtime-dir", "ledger-dir"}
	for _, flagName := range expectedFlags {
		flag := cmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "report verify should have --%s flag", flagName)
	}
}

func TestReportAllCmd_OutDirFlagCanBeSet(t *testing.T) {
	cmd := reportAllCmd()
	require.NoError(t, cmd.Flags().Set("out", "/tmp/custom-reports"))
	flag := cmd.Flags().Lookup("out")
	assert.Equal(t, "/tmp/custom-reports", flag.Value.String())
}

func TestReportAllCmd_DataDirFlagCanBeSet(t *testing.T) {
	cmd := reportAllCmd()
	require.NoError(t, cmd.Flags().Set("data-dir", "/tmp/custom-data"))
	flag := cmd.Flags().Lookup("data-dir")
	assert.Equal(t, "/tmp/custom-data", flag.Value.String())
}
