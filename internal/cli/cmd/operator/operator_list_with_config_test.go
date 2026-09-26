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

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"

	// saveTestCredentials writes a Credentials with the given userID so that
	// operatorListCmdWithConfig can load it for the user_id query parameter.
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
)

func saveTestCredentials(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config, userID string) {
	t.Helper()
	creds := &auth.Credentials{UserID: userID, CLISessionID: "cli-sess-test"}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
}

func TestOperatorListCmdWithConfig_ConfigLoadError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, fmt.Errorf("config load error")
	}

	cmd := operatorListCmdWithConfig(failLoader, authcmd.DefaultAPIClientFactory, shared.NewFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config load error")
}

func TestOperatorListCmdWithConfig_ClientCreationError(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	loader := func(string) (*config.Config, error) { return cfg, nil }
	cmd := operatorListCmdWithConfig(loader, authcmd.FailingClientFactory(fmt.Errorf("client creation error")), cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client creation error")
}

func TestOperatorListCmdWithConfig_GetRequestError(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{GetErr: fmt.Errorf("network error")}
	cmd := operatorListCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network error")
}

func TestOperatorListCmdWithConfig_InvalidJSONResponse(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{GetResp: []byte("not json")}
	cmd := operatorListCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestOperatorListCmdWithConfig_NotAuthenticatedWithoutCredentials(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{}
	cmd := operatorListCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
	assert.Empty(t, client.GetCalls, "client must not be called when credentials are missing")
}

func TestOperatorListCmdWithConfig_EmptyOperatorsListPrintsNoOperators(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	slotResp := models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{}}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{GetResp: respJSON}
	cmd := operatorListCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No operators found")
	assert.Equal(t, []string{constants.APIPaths.Operators + "?user_id=user-001"}, client.GetCalls)
}

func TestOperatorListCmdWithConfig_ValidResponsePrintsOperatorTable(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	slotResp := models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{
			{ID: "op-001", OperatorSessionID: "session-001", OperatorType: "embedded", Status: "active"},
			{ID: "op-002", OperatorSessionID: "session-002", OperatorType: "remote", Status: "standby"},
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

	output := buf.String()
	assert.Contains(t, output, "Operators (2 total)")
	assert.Contains(t, output, "op-001")
	assert.Contains(t, output, "op-002")
	assert.Contains(t, output, "session-001")
	assert.Contains(t, output, "session-002")
	assert.Contains(t, output, "embedded")
	assert.Contains(t, output, "remote")
	assert.Equal(t, []string{constants.APIPaths.Operators + "?user_id=user-001"}, client.GetCalls)
}

func TestOperatorListCmdWithConfig_JSONOutputIncludesRuntimeFlags(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	slotResp := models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{
			{
				ID:                "data-op",
				OperatorSessionID: "data-session",
				OperatorType:      constants.OperatorTypeRemote,
				Status:            constants.OperatorStatusActive,
				RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: false},
			},
			{
				ID:                "infer-op",
				OperatorSessionID: "infer-session",
				OperatorType:      constants.OperatorTypeRemote,
				Status:            constants.OperatorStatusActive,
				RuntimeConfig: &models.RuntimeConfig{
					InferenceEnabled:                true,
					ProviderBoundaryObserverEnabled: false,
				},
			},
		},
	}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{GetResp: respJSON}
	cmd := operatorListCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	cmdtest.EnableGlobalJSON(t, cmd)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	var payload operatorListOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &payload))
	require.Len(t, payload.Operators, 2)
	assert.Equal(t, "data-session", payload.Operators[0].OperatorSessionID)
	require.NotNil(t, payload.Operators[0].InferenceEnabled)
	assert.False(t, *payload.Operators[0].InferenceEnabled)
	assert.Equal(t, "infer-session", payload.Operators[1].OperatorSessionID)
	require.NotNil(t, payload.Operators[1].InferenceEnabled)
	assert.True(t, *payload.Operators[1].InferenceEnabled)
}

func TestOperatorListCmdWithConfig_SendsUserIDQueryParameter(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-distinct-123")

	slotResp := models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{}}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &cmdtest.MockAPIClient{GetResp: respJSON}
	cmd := operatorListCmdWithConfig(loader, authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	require.Len(t, client.GetCalls, 1)
	assert.Contains(t, client.GetCalls[0], "user_id=user-distinct-123")
	assert.Equal(t, constants.APIPaths.Operators+"?user_id=user-distinct-123", client.GetCalls[0])
}
