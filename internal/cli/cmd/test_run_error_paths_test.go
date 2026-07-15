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
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestCoverageCmd_PkgFlagChangesTargetPackage(t *testing.T) {
	cmd := testCoverageCmd()
	require.NotNil(t, cmd)

	require.NoError(t, cmd.Flags().Set("pkg", "./internal/services/auth"))
	flag := cmd.Flags().Lookup("pkg")
	require.NotNil(t, flag)
	assert.Equal(t, "./internal/services/auth", flag.Value.String())
}

func TestTestCoverageCmd_VerboseFlagAddsVFlag(t *testing.T) {
	cmd := testCoverageCmd()
	require.NotNil(t, cmd)

	require.NoError(t, cmd.Flags().Set("verbose", "true"))
	flag := cmd.Flags().Lookup("verbose")
	require.NotNil(t, flag)
	assert.Equal(t, "true", flag.Value.String())
}

func TestTestE2ECmd_NoGatewayReturnsGatewayNotRunning(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	cmd := testE2ECmdWithConfig(func(_ string) (*config.Config, error) { return cfg, nil }, fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestTestSummaryCmd_NoTestVaultReturnsMessage(t *testing.T) {
	tmpDir := chdirTemp(t)

	protocolDir := filepath.Join(tmpDir, "protocol", "constants")
	require.NoError(t, os.MkdirAll(protocolDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(protocolDir, "paths.json"), []byte(minimalPathsJSON(t)), 0o644))

	cmd := testSummaryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Test vault directory not found")
}

func TestTestSummaryCmd_EmptyTestVaultReturnsMessage(t *testing.T) {
	tmpDir := chdirTemp(t)

	protocolDir := filepath.Join(tmpDir, "protocol", "constants")
	require.NoError(t, os.MkdirAll(protocolDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(protocolDir, "paths.json"), []byte(minimalPathsJSON(t)), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".g8e", "test-vault"), 0o755))

	cmd := testSummaryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No chaos test runs found")
}

func TestTestUnitCmd_StructureAndFlags(t *testing.T) {
	cmd := testUnitCmd()
	assert.Equal(t, "unit", cmd.Use)
	assert.NotNil(t, cmd.RunE)
}

func TestTestIntegrationCmd_StructureAndFlags(t *testing.T) {
	cmd := testIntegrationCmd()
	assert.Equal(t, "integration", cmd.Use)
	assert.NotNil(t, cmd.RunE)
}
