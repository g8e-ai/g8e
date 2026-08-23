// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// saveTestCredentials writes a Credentials with the given userID so that
// operatorListCmdWithConfig can load it for the user_id query parameter.
func saveTestCredentials(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config, userID string) {
	t.Helper()
	creds := &auth.Credentials{UserID: userID, CLISessionID: "cli-sess-test"}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
}

func TestOperatorListCmdWithConfig_ConfigLoadError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, errors.New("config load error")
	}

	cmd := operatorListCmdWithConfig(failLoader, defaultAPIClientFactory, newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config load error")
}

func TestOperatorListCmdWithConfig_ClientCreationError(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	loader := func(string) (*config.Config, error) { return cfg, nil }
	cmd := operatorListCmdWithConfig(loader, failingClientFactory(errors.New("client creation error")), fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client creation error")
}

func TestOperatorListCmdWithConfig_GetRequestError(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getErr: errors.New("network error")}
	cmd := operatorListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network error")
}

func TestOperatorListCmdWithConfig_InvalidJSONResponse(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getResp: []byte("not json")}
	cmd := operatorListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestOperatorListCmdWithConfig_NotAuthenticatedWithoutCredentials(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{}
	cmd := operatorListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
	assert.Empty(t, client.getCalls, "client must not be called when credentials are missing")
}

func TestOperatorListCmdWithConfig_EmptyOperatorsListPrintsNoOperators(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	slotResp := models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{}}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getResp: respJSON}
	cmd := operatorListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No operators found")
	assert.Equal(t, []string{constants.APIPaths.Operators + "?user_id=user-001"}, client.getCalls)
}

func TestOperatorListCmdWithConfig_ValidResponsePrintsOperatorTable(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	slotResp := models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{
			{ID: "op-001", CloudSubtype: "aws", Status: "active"},
			{ID: "op-002", CloudSubtype: "gcp", Status: "standby"},
		},
	}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getResp: respJSON}
	cmd := operatorListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Operators (2 total)")
	assert.Contains(t, output, "op-001")
	assert.Contains(t, output, "op-002")
	assert.Contains(t, output, "aws")
	assert.Contains(t, output, "gcp")
	assert.Equal(t, []string{constants.APIPaths.Operators + "?user_id=user-001"}, client.getCalls)
}

func TestOperatorListCmdWithConfig_SendsUserIDQueryParameter(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-distinct-123")

	slotResp := models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{}}
	respJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getResp: respJSON}
	cmd := operatorListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	require.Len(t, client.getCalls, 1)
	assert.Contains(t, client.getCalls[0], "user_id=user-distinct-123")
	assert.Equal(t, constants.APIPaths.Operators+"?user_id=user-distinct-123", client.getCalls[0])
}
