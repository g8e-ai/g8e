// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

// setupGatewayTestEnv creates a temp dir with minimal .g8e structure so that
// loadConfig succeeds but ProcessManager finds no running gateway.
func setupGatewayTestEnv(t *testing.T) string {
	t.Helper()
	tmpDir := chdirTemp(t)

	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "pki"), constants.PermDirStandard))
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "secrets"), constants.PermDirPrivate))
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "pki", "trust"), constants.PermDirStandard))

	protocolDir := filepath.Join(tmpDir, "protocol", "constants")
	require.NoError(t, os.MkdirAll(protocolDir, constants.PermDirStandard))
	require.NoError(t, os.WriteFile(filepath.Join(protocolDir, "paths.json"), []byte(minimalPathsJSON(t)), constants.PermFilePublic))

	return tmpDir
}

func TestGatewayStopCmd_NoConfigReturnsError(t *testing.T) {
	chdirTemp(t)

	originalLoader := configLoad
	configLoad = func(string) (*config.Config, error) {
		return nil, errors.New("no config")
	}
	t.Cleanup(func() { configLoad = originalLoader })

	cmd := gatewayStopCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewayStatusCmd_NoConfigReturnsError(t *testing.T) {
	chdirTemp(t)

	originalLoader := configLoad
	configLoad = func(string) (*config.Config, error) {
		return nil, errors.New("no config")
	}
	t.Cleanup(func() { configLoad = originalLoader })

	cmd := gatewayStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewaySettingsCmd_NoConfigReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := gatewaySettingsCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewayResetCmd_NoConfigReturnsError(t *testing.T) {
	chdirTemp(t)

	originalLoader := configLoad
	configLoad = func(string) (*config.Config, error) {
		return nil, errors.New("no config")
	}
	t.Cleanup(func() { configLoad = originalLoader })

	cmd := gatewayResetCmd()
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewayCleanCmd_NoConfigReturnsError(t *testing.T) {
	chdirTemp(t)

	originalLoader := configLoad
	configLoad = func(string) (*config.Config, error) {
		return nil, errors.New("no config")
	}
	t.Cleanup(func() { configLoad = originalLoader })

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewayCleanCmd_AbortsOnNoResponse(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	originalStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.Write([]byte("n\n"))
	w.Close()
	t.Cleanup(func() { os.Stdin = originalStdin })

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
}

func TestGatewayResetCmd_AbortsOnNoResponse(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	originalStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.Write([]byte("n\n"))
	w.Close()
	t.Cleanup(func() { os.Stdin = originalStdin })

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
}

func TestGatewayStartCmd_InvalidPostureReturnsError(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayStartCmd()
	cmd.Flags().Set("posture", "invalid")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidPosture)
}

func TestGatewayStopCmd_NotRunningReturnsNil(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayStopCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "not running")
}

func TestGatewayStatusCmd_NotRunningReturnsStopped(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "STOPPED")
}

func TestGatewayLogsCmd_NoLogFileReturnsMessage(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayLogsCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No log file found")
}

func TestGatewayCleanCmd_ForceFlagSkipsPrompt(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Clean complete")
}

func TestGatewayCleanCmd_AbortedOutputContainsNoDestructiveAction(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	originalStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.Write([]byte("\n"))
	w.Close()
	t.Cleanup(func() { os.Stdin = originalStdin })

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
	assert.NotContains(t, buf.String(), "Clean complete")
}

func TestGatewayResetCmd_WarningMessagesPrintedBeforePrompt(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	originalStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.Write([]byte("n\n"))
	w.Close()
	t.Cleanup(func() { os.Stdin = originalStdin })

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "This command will:")
	assert.Contains(t, output, "Stop all running g8e services")
	assert.Contains(t, output, "Wipe the SQLite databases")
	assert.Contains(t, output, "Aborted")
}

func TestGatewayCleanCmd_WarningMessagesPrintedBeforePrompt(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	originalStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.Write([]byte("n\n"))
	w.Close()
	t.Cleanup(func() { os.Stdin = originalStdin })

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "WARNING")
	assert.Contains(t, output, "permanently destroyed")
	assert.Contains(t, output, "Aborted")
}

func TestGatewayStatusCmd_OutputContainsHeader(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "g8e Gateway Status")
	assert.True(t, strings.Contains(buf.String(), "========================"))
}
