// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	clioperator "github.com/g8e-ai/g8e/v2/internal/cli/operator"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestParseOperatorHeartbeatView_HeartbeatResultSnapshot(t *testing.T) {
	raw := json.RawMessage(`{
		"timestamp": "2026-09-18T12:00:00Z",
		"status": "automatic",
		"system_identity": {
			"hostname": "worker-1",
			"os": "linux",
			"architecture": "amd64",
			"current_user": "bob",
			"cpu_count": 8,
			"memory_mb": 16384
		},
		"performance_metrics": {
			"cpu_percent": 12.5,
			"memory_percent": 45.0,
			"disk_percent": 60.0
		},
		"uptime_info": {
			"uptime": "2h 15m",
			"uptime_seconds": 8100
		}
	}`)

	view := clioperator.ParseHeartbeatView(raw)
	require.NotNil(t, view)
	assert.Equal(t, "worker-1", view.SystemIdentity.Hostname)
	assert.Equal(t, constants.Platform("linux"), view.SystemIdentity.OS)
	assert.Equal(t, "automatic", view.HeartbeatType)
	assert.Equal(t, 12.5, view.PerformanceMetrics.CPUPercent)
	assert.Equal(t, "2h 15m", view.UptimeInfo.Uptime)
}

func TestParseOperatorHeartbeatView_AcceptsProtojsonCamelCase(t *testing.T) {
	raw := json.RawMessage(`{
		"timestamp": "2026-09-18T12:00:00Z",
		"status": "automatic",
		"systemIdentity": {
			"hostname": "legacy-host"
		},
		"performanceMetrics": {
			"cpuPercent": 5.0
		}
	}`)

	view := clioperator.ParseHeartbeatView(raw)
	require.NotNil(t, view)
	assert.Equal(t, "legacy-host", view.SystemIdentity.Hostname)
	assert.Equal(t, 5.0, view.PerformanceMetrics.CPUPercent)
}

func TestParseOperatorHeartbeatView_RejectsLegacyPythonShape(t *testing.T) {
	raw := json.RawMessage(`{
		"timestamp": "2026-09-18T12:00:00Z",
		"heartbeat_type": "automatic",
		"system_identity": {"hostname": "legacy-host"},
		"performance": {"cpu_percent": 5.0}
	}`)

	assert.Nil(t, clioperator.ParseHeartbeatView(raw))
}

func TestFindOperatorByIDOrSession(t *testing.T) {
	operators := []*operatorv1.OperatorDocument{
		{Id: "op-1", OperatorSessionId: "session-1"},
		{Id: "op-2", OperatorSessionId: "session-2"},
	}

	assert.NotNil(t, findOperatorByIDOrSession(operators, "op-2"))
	assert.NotNil(t, findOperatorByIDOrSession(operators, "session-1"))
	assert.Nil(t, findOperatorByIDOrSession(operators, "missing"))
}

func TestOperatorShowCmdWithConfig_NotFound(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	slotResp := models.OperatorSlotResponse{
		Success: true,
		Operators: []*operatorv1.OperatorDocument{
			{Id: "op-1", OperatorSessionId: "session-1", Status: string(constants.OperatorStatusActive)},
		},
	}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{GetResp: respJSON}
	cmd := operatorShowCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"missing-id"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrRegistrationOperatorNotFound)
}

func TestOperatorShowCmdWithConfig_PrintsHeartbeatDetails(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	heartbeat := json.RawMessage(`{
		"timestamp": "2026-09-18T12:00:00Z",
		"status": "automatic",
		"system_identity": {
			"hostname": "dev-host",
			"os": "linux",
			"architecture": "amd64",
			"current_user": "bob",
			"pwd": "/home/bob"
		},
		"performance_metrics": {
			"cpu_percent": 10.0,
			"memory_percent": 50.0
		},
		"version_info": {
			"operator_version": "v2.1.8"
		}
	}`)

	slotResp := models.OperatorSlotResponse{
		Success: true,
		Operators: []*operatorv1.OperatorDocument{
			{
				ID:                "286d7a56-b961-4cff-9c69-063a84b69afd",
				OperatorSessionID: "74a859f4-2443-4a4b-ae9c-35da879c1ad3",
				OperatorType:      constants.OperatorTypeRemote,
				Status:            constants.OperatorStatusActive,
				LatestHeartbeat:   heartbeat,
				UpdatedAt:         time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{GetResp: respJSON}
	cmd := operatorShowCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"286d7a56-b961-4cff-9c69-063a84b69afd"})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "286d7a56-b961-4cff-9c69-063a84b69afd")
	assert.Contains(t, output, "dev-host")
	assert.Contains(t, output, "Performance")
	assert.Contains(t, output, "CPU:")
	assert.Contains(t, output, "v2.1.8")
}

func TestOperatorShowCmdWithConfig_JSONOutput(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	heartbeat := json.RawMessage(`{"system_identity":{"hostname":"json-host"}}`)
	slotResp := models.OperatorSlotResponse{
		Success: true,
		Operators: []*operatorv1.OperatorDocument{
			{
				ID:                "op-json",
				OperatorSessionID: "session-json",
				Status:            constants.OperatorStatusActive,
				LatestHeartbeat:   heartbeat,
			},
		},
	}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{GetResp: respJSON}
	cmd := operatorShowCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	cmdtest.EnableGlobalJSON(t, cmd)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"session-json"})
	require.NoError(t, err)

	var payload operatorShowOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &payload))
	assert.Equal(t, "op-json", payload.OperatorID)
	require.NotNil(t, payload.Heartbeat)
	require.NotNil(t, payload.Heartbeat.SystemIdentity)
	assert.Equal(t, "json-host", payload.Heartbeat.SystemIdentity.Hostname)
}

func TestOperatorShowCmdWithConfig_ReportsLastHeartbeatAndStaleStatus(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	lastHeartbeat := time.Date(2026, time.September, 30, 12, 34, 56, 0, time.UTC)
	slotResp := models.OperatorSlotResponse{
		Success: true,
		Operators: []*operatorv1.OperatorDocument{
			{
				ID:                "op-stale",
				OperatorSessionID: "session-stale",
				Status:            constants.OperatorStatusStale,
				LastHeartbeatAt:   &lastHeartbeat,
			},
		},
	}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{GetResp: respJSON}

	textCmd := operatorShowCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	var textBuf bytes.Buffer
	textCmd.SetOut(&textBuf)
	textCmd.SetErr(&textBuf)
	require.NoError(t, textCmd.RunE(textCmd, []string{"session-stale"}))
	assert.Contains(t, textBuf.String(), "Status:    stale")
	assert.Contains(t, textBuf.String(), "Last heartbeat: 2026-09-30 12:34:56 UTC")

	jsonCmd := operatorShowCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	cmdtest.EnableGlobalJSON(t, jsonCmd)
	var jsonBuf bytes.Buffer
	jsonCmd.SetOut(&jsonBuf)
	jsonCmd.SetErr(&jsonBuf)
	require.NoError(t, jsonCmd.RunE(jsonCmd, []string{"session-stale"}))
	var payload operatorShowOutput
	require.NoError(t, json.Unmarshal(jsonBuf.Bytes(), &payload))
	assert.Equal(t, constants.OperatorStatusStale, payload.Status)
	require.NotNil(t, payload.LastHeartbeatAt)
	assert.True(t, payload.LastHeartbeatAt.Equal(lastHeartbeat))
}

func TestOperatorShowCmdWithConfig_ConfigLoadError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, fmt.Errorf("config load error")
	}

	cmd := operatorShowCmdWithConfig(failLoader, authcmd.DefaultAPIClientFactory, shared.NewFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"op-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config load error")
}

func TestOperatorListCmdWithConfig_IncludesHostname(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	heartbeat := json.RawMessage(`{"system_identity":{"hostname":"list-host"}}`)
	slotResp := models.OperatorSlotResponse{
		Success: true,
		Operators: []*operatorv1.OperatorDocument{
			{
				ID:                "op-host",
				OperatorSessionID: "session-host",
				OperatorType:      constants.OperatorTypeRemote,
				Status:            constants.OperatorStatusActive,
				LatestHeartbeat:   heartbeat,
			},
		},
	}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{GetResp: respJSON}
	cmd := operatorListCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "list-host")
	assert.Contains(t, buf.String(), "Hostname")
}

func TestOperatorHostnameValue_FallbackToCurrentHostname(t *testing.T) {
	op := operatorv1.OperatorDocument{
		ID:              "op-1",
		CurrentHostname: "cached-worker",
	}
	assert.Equal(t, "cached-worker", operatorHostnameValue(op))
	assert.Equal(t, "cached-worker", operatorHostnameDisplay(op))

	emptyOp := operatorv1.OperatorDocument{ID: "op-2"}
	assert.Equal(t, "", operatorHostnameValue(emptyOp))
	assert.Equal(t, "-", operatorHostnameDisplay(emptyOp))
}
