// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/stretchr/testify/require"
)

func TestBriefStatusDistinguishesUnknownAndEmptyRegistry(t *testing.T) {
	for _, tc := range []struct {
		name      string
		operators []byte
		want      string
	}{
		{"unavailable", nil, "Operators  unavailable"},
		{"malformed", []byte("not JSON"), "Operators  unavailable"},
		{"rejected", []byte(`{"success":false}`), "Operators  unavailable"},
		{"empty", []byte(`{"success":true,"operators":[]}`), "Operators  0 connected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
			mock := &statusMockClient{responses: map[string][]byte{"/api/v1/health": []byte(`{"status":"ok"}`)}}
			if tc.operators != nil {
				mock.responses[constants.APIPaths.Operators] = tc.operators
			}
			factory := func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) { return mock, nil }
			cmd := gatewayStatusCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), factory, cmdtest.FileSvcFactoryFor(fileSvc))
			cmd.SetArgs([]string{"--brief"})
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			require.NoError(t, cmd.Execute())
			require.Contains(t, buf.String(), "Gateway    online")
			require.Contains(t, buf.String(), tc.want)
			require.NotContains(t, buf.String(), "Endpoints:")
		})
	}
}

func TestBriefStatusOnlyShowsConnectedOperators(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{
		{OperatorRoles: constants.OperatorRoles{constants.OperatorRoleInference}, CurrentHostname: "gpu-host", Status: constants.OperatorStatusActive},
		{CurrentHostname: "stopped-host", Status: constants.OperatorStatusStopped},
		{CurrentHostname: "unclaimed-host", Status: constants.OperatorStatusActive, IsSlot: true},
	}})
	require.NoError(t, err)
	mock := &statusMockClient{responses: map[string][]byte{"/api/v1/health": []byte(`{"status":"ok"}`), constants.APIPaths.Operators: body}}
	factory := func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) { return mock, nil }
	cmd, buf := newOutputCmd()
	require.NoError(t, printBriefStatus(cmd, cfg, factory, cmdtest.FileSvcFactoryFor(fileSvc)))
	require.Contains(t, buf.String(), "Operators  1 connected")
	require.Contains(t, buf.String(), "inference on gpu-host")
	require.NotContains(t, buf.String(), "stopped-host")
	require.NotContains(t, buf.String(), "unclaimed-host")
}

func TestQuietStartKeepsRunningGatewayAndSuppressesGuidance(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	pidPath := filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
	pid := []byte(strconv.Itoa(os.Getpid()))
	require.NoError(t, fileSvc.WriteFile(context.Background(), pidPath, pid, constants.PermFilePrivate))
	cmd := gatewayStartCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FileSvcFactoryFor(fileSvc), nil)
	cmd.SetArgs([]string{"--quiet", "--cert-mode", "localhost"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	require.NoError(t, cmd.Execute())
	require.Empty(t, buf.String())
	saved, err := fileSvc.ReadFile(context.Background(), pidPath)
	require.NoError(t, err)
	require.Equal(t, pid, saved)
	cmd.Println("output restored")
	require.Contains(t, buf.String(), "output restored")
}
