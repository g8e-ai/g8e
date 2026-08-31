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
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
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

func TestTestCoverageCmd_DelegatesThresholdEnforcementToMakefile(t *testing.T) {
	var capturedName string
	var capturedArgs []string
	runner := func(name string, args ...string) error {
		capturedName = name
		capturedArgs = args
		return nil
	}
	cmd := testCoverageCmdWithRunner(runner)
	require.NoError(t, cmd.Flags().Set("pkg", "./internal/services/auth"))
	require.NoError(t, cmd.Flags().Set("verbose", "true"))

	require.NoError(t, cmd.RunE(cmd, nil))

	assert.Equal(t, "make", capturedName)
	assert.Equal(t, []string{"test-coverage", "PKG=./internal/services/auth", "VERBOSE=1"}, capturedArgs)
}

func TestTestCoverageCmd_MakefileFailureWrapsCoverageError(t *testing.T) {
	runnerErr := errors.New("coverage threshold failed")
	cmd := testCoverageCmdWithRunner(func(string, ...string) error { return runnerErr })

	err := cmd.RunE(cmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCoverageTestsFailed)
	assert.ErrorIs(t, err, runnerErr)
}

// recordingE2ERunner captures the arguments passed to the runner and returns
// a fixed (code, err) pair. Used to verify argument propagation without
// starting platform tests.
func recordingE2ERunner(code int, err error, captured *[]string) e2eCommandRunner {
	return func(_ context.Context, _ string, args ...string) (int, error) {
		*captured = args
		return code, err
	}
}

func TestTestE2ECmd_PropagatesArgumentsAndRaceFlag(t *testing.T) {
	var captured []string
	cmd := testE2ECmdWithRunner(recordingE2ERunner(0, nil, &captured))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.RunE(cmd, nil))

	// Core flags are always present.
	assert.Contains(t, captured, "test")
	assert.Contains(t, captured, "-tags=e2e")
	assert.Contains(t, captured, "-count=1")
	assert.Contains(t, captured, "-parallel=1")
	assert.Contains(t, captured, "./test/e2e/...")
	// -race is present on non-Windows platforms.
	if runtime.GOOS != "windows" {
		assert.Contains(t, captured, "-race")
	}
	// --run is omitted when unset.
	assert.NotContains(t, captured, "-run")
}

func TestTestE2ECmd_RunFlagAppendsRegexp(t *testing.T) {
	var captured []string
	cmd := testE2ECmdWithRunner(recordingE2ERunner(0, nil, &captured))
	require.NoError(t, cmd.Flags().Set("run", "TestApproved"))

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.RunE(cmd, nil))

	idx := -1
	for i, a := range captured {
		if a == "-run" {
			idx = i
			break
		}
	}
	require.GreaterOrEqual(t, idx, 0, "captured args should contain -run: %v", captured)
	require.True(t, len(captured) > idx+1, "captured args should have value after -run: %v", captured)
	assert.Equal(t, "TestApproved", captured[idx+1])
}

func TestTestE2ECmd_NonzeroExitWrapsErrE2ETestsFailed(t *testing.T) {
	runnerErr := errors.New("child process failed")
	cmd := testE2ECmdWithRunner(recordingE2ERunner(2, runnerErr, &[]string{}))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrE2ETestsFailed)
	assert.ErrorIs(t, err, runnerErr)
}

func TestTestE2ECmd_NonzeroExitWithoutErrorWrapsErrE2ETestsFailed(t *testing.T) {
	cmd := testE2ECmdWithRunner(recordingE2ERunner(1, nil, &[]string{}))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrE2ETestsFailed)
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
