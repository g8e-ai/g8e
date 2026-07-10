// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
