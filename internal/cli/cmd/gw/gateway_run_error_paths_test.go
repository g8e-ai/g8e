// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// setupGatewayTestEnv creates a temp dir with minimal .g8e structure so that
// loadConfig succeeds but ProcessManager finds no running gateway.
func setupGatewayTestEnv(t *testing.T) string {
	t.Helper()
	tmpDir := cmdtest.ChdirTemp(t)

	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "pki"), constants.PermDirStandard))
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "secrets"), constants.PermDirPrivate))
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "pki", "trust"), constants.PermDirStandard))

	protocolDir := filepath.Join(tmpDir, "protocol", "constants")
	require.NoError(t, os.MkdirAll(protocolDir, constants.PermDirStandard))
	require.NoError(t, os.WriteFile(filepath.Join(protocolDir, "paths.json"), []byte(cmdtest.MinimalPathsJSON(t)), constants.PermFilePublic))

	return tmpDir
}

func TestGatewayStopCmd_NoConfigReturnsError(t *testing.T) {
	cmdtest.ChdirTemp(t)

	originalLoader := shared.ConfigLoad
	shared.ConfigLoad = func(string) (*config.Config, error) {
		return nil, fmt.Errorf("no config")
	}
	t.Cleanup(func() { shared.ConfigLoad = originalLoader })

	cmd := gatewayStopCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewayStatusCmd_NoConfigReturnsError(t *testing.T) {
	cmdtest.ChdirTemp(t)

	originalLoader := shared.ConfigLoad
	shared.ConfigLoad = func(string) (*config.Config, error) {
		return nil, fmt.Errorf("no config")
	}
	t.Cleanup(func() { shared.ConfigLoad = originalLoader })

	cmd := gatewayStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewaySettingsCmd_NoConfigReturnsError(t *testing.T) {
	cmdtest.ChdirTemp(t)

	cmd := gatewaySettingsCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewayResetCmd_NoConfigReturnsError(t *testing.T) {
	cmdtest.ChdirTemp(t)

	originalLoader := shared.ConfigLoad
	shared.ConfigLoad = func(string) (*config.Config, error) {
		return nil, fmt.Errorf("no config")
	}
	t.Cleanup(func() { shared.ConfigLoad = originalLoader })

	cmd := gatewayResetCmd()
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestGatewayCleanCmd_NoConfigReturnsError(t *testing.T) {
	cmdtest.ChdirTemp(t)

	originalLoader := shared.ConfigLoad
	shared.ConfigLoad = func(string) (*config.Config, error) {
		return nil, fmt.Errorf("no config")
	}
	t.Cleanup(func() { shared.ConfigLoad = originalLoader })

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
	w.WriteString("n\n")
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
	w.WriteString("n\n")
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

// TestGatewayStatusCmd_DoesNotShowDockerSectionWhenNoContainersRunning verifies that
// `g8e gw status` does not show the Docker Compose Stack section when no containers
// are actually running.
func TestGatewayStatusCmd_DoesNotShowDockerSectionWhenNoContainersRunning(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Localhost Gateway")
	assert.NotContains(t, output, "Docker Compose Stack")
}

// TestGatewayStatusCmd_DockerSectionNotShownWhenDockerMissing
// verifies the docker section is not shown when Docker is not installed.
func TestGatewayStatusCmd_DockerSectionNotShownWhenDockerMissing(t *testing.T) {
	if cmdtest.DockerAvailable() {
		t.Skip("test exercises the no-Docker status note")
	}
	tmpDir := setupGatewayTestEnv(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, constants.DockerComposeFile),
		[]byte("version: '3'\n"), constants.PermFilePublic))

	cmd := gatewayStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "STOPPED")
	assert.NotContains(t, output, "Docker Compose Stack")
}

type statusMockClient struct {
	responses map[string][]byte
}

func (m *statusMockClient) Get(path string) ([]byte, error) {
	if resp, ok := m.responses[path]; ok {
		return resp, nil
	}
	for k, v := range m.responses {
		if strings.HasPrefix(path, k) {
			return v, nil
		}
	}
	return nil, fmt.Errorf("unexpected path: %s", path)
}

func (m *statusMockClient) Post(string, interface{}) ([]byte, error) { return nil, nil }
func (m *statusMockClient) Put(string, interface{}) ([]byte, error)  { return nil, nil }
func (m *statusMockClient) Delete(string) ([]byte, error)            { return nil, nil }

func TestGatewayStatusCmd_ReportsConnectedOperators(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

	healthResp, _ := json.Marshal(models.HealthResponse{
		Status: constants.GatewayModeStatusOK,
		PID:    9999,
	})
	operatorsResp, _ := json.Marshal(models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{
			{
				ID:                "g8e-model-provenance-operator",
				OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleProvenance},
				OperatorType:      constants.OperatorTypeRemote,
				CurrentHostname:   "beepboop",
				OperatorSessionID: "sess-prov-1",
				Status:            constants.OperatorStatusActive,
			},
			{
				ID:                "g8e-provider-boundary-observer",
				OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleObserver},
				OperatorType:      constants.OperatorTypeRemote,
				CurrentHostname:   "beepboop",
				OperatorSessionID: "sess-obs-1",
				Status:            constants.OperatorStatusActive,
			},
			{
				ID:                "g8e-inference-operator",
				OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleInference},
				OperatorType:      constants.OperatorTypeRemote,
				CurrentHostname:   "beepboop",
				OperatorSessionID: "sess-inf-1",
				Status:            constants.OperatorStatusActive,
			},
			{
				ID:            "old-stopped-op",
				OperatorRoles: constants.OperatorRoles{constants.OperatorRoleData},
				OperatorType:  constants.OperatorTypeRemote,
				Status:        constants.OperatorStatusStopped,
			},
		},
	})

	mockClient := &statusMockClient{
		responses: map[string][]byte{
			"/api/v1/health":             healthResp,
			constants.APIPaths.Operators: operatorsResp,
		},
	}

	clientFactory := func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) {
		return mockClient, nil
	}

	cmd := gatewayStatusCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), clientFactory, cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Localhost Gateway")
	assert.Contains(t, out, "State: RUNNING (PID: 9999)")
	assert.Contains(t, out, "Connected Operators")
	assert.Contains(t, out, "g8e-model-provenance-operator")
	assert.Contains(t, out, "provenance")
	assert.Contains(t, out, "g8e-provider-boundary-observer")
	assert.Contains(t, out, "observer")
	assert.Contains(t, out, "g8e-inference-operator")
	assert.Contains(t, out, "inference")
	assert.NotContains(t, out, "old-stopped-op")
	assert.NotContains(t, out, "Docker Compose Stack")
}

func TestGatewayStatusCmd_ReportsNoConnectedOperatorsWhenEmpty(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

	healthResp, _ := json.Marshal(models.HealthResponse{
		Status: constants.GatewayModeStatusOK,
		PID:    9999,
	})
	operatorsResp, _ := json.Marshal(models.OperatorSlotResponse{
		Success:   true,
		Operators: []models.OperatorDocumentGo{},
	})

	mockClient := &statusMockClient{
		responses: map[string][]byte{
			"/api/v1/health":             healthResp,
			constants.APIPaths.Operators: operatorsResp,
		},
	}

	clientFactory := func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) {
		return mockClient, nil
	}

	cmd := gatewayStatusCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), clientFactory, cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Localhost Gateway")
	assert.Contains(t, out, "Connected Operators")
	assert.Contains(t, out, "No connected operators")
	assert.NotContains(t, out, "Docker Compose Stack")
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
	w.WriteString("\n")
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
	w.WriteString("n\n")
	w.Close()
	t.Cleanup(func() { os.Stdin = originalStdin })

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "destructive operation")
	assert.Contains(t, output, "Stop all running g8e services")
	assert.Contains(t, output, "Rename the runtime directory (.g8e) aside")
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
	w.WriteString("n\n")
	w.Close()
	t.Cleanup(func() { os.Stdin = originalStdin })

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "WARNING")
	assert.Contains(t, output, "Rename the runtime directory (.g8e) aside")
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
