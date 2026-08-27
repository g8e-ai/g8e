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

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// dataTestEnv holds the pre-aligned (fileSvc, cfg) pair for data tests.
type dataTestEnv struct {
	fileSvc fs.RuntimeFileService
	cfg     *config.Config
}

func newDataTestEnv(t *testing.T) dataTestEnv {
	t.Helper()
	fileSvc, cfg := newCmdTestEnv(t)
	return dataTestEnv{fileSvc: fileSvc, cfg: cfg}
}

func mockClientFactory(client apiClient) apiClientFactory {
	return func(_ fs.RuntimeFileService, cfg *config.Config) (apiClient, error) {
		return client, nil
	}
}

func failingClientFactory(err error) apiClientFactory {
	return func(_ fs.RuntimeFileService, cfg *config.Config) (apiClient, error) {
		return nil, err
	}
}

func TestDataUsersCmdWithConfig_ConfigLoadError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, errors.New("config load error")
	}

	cmd := dataUsersCmdWithConfig(failLoader, defaultAPIClientFactory, newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestDataUsersCmdWithConfig_ClientCreationError(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	cmd := dataUsersCmdWithConfig(loader, failingClientFactory(errors.New("client creation error")), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client creation error")
}

func TestDataUsersCmdWithConfig_GetRequestError(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getErr: errors.New("network error")}
	cmd := dataUsersCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch users")
}

func TestDataUsersCmdWithConfig_InvalidJSONResponse(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getResp: []byte("invalid json")}
	cmd := dataUsersCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestDataUsersCmdWithConfig_ValidResponse(t *testing.T) {
	env := newDataTestEnv(t)

	users := []map[string]interface{}{{"id": "user1"}, {"id": "user2"}}
	usersJSON, _ := json.Marshal(users)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getResp: usersJSON}
	cmd := dataUsersCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Users (2 total)")
	assert.Contains(t, buf.String(), "user1")
	assert.Contains(t, buf.String(), "user2")
	assert.Equal(t, []string{constants.APIPaths.Users}, client.getCalls)
}

// saveDataTestCredentials writes a Credentials with the given userID so that
// dataOperatorsCmdWithConfig can load it for the user_id query parameter.
func saveDataTestCredentials(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config, userID string) {
	t.Helper()
	creds := &auth.Credentials{UserID: userID, CLISessionID: "cli-sess-test"}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
}

func TestDataOperatorsCmdWithConfig_ValidResponse(t *testing.T) {
	env := newDataTestEnv(t)
	saveDataTestCredentials(t, env.fileSvc, env.cfg, "user-001")

	slotResp := models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{
			{ID: "op1", CloudSubtype: "aws", Status: "active"},
		},
	}
	opsJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getResp: opsJSON}
	cmd := dataOperatorsCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Operators (1 total)")
	assert.Contains(t, buf.String(), "op1")
	assert.Equal(t, []string{constants.APIPaths.Operators + "?user_id=user-001"}, client.getCalls)
}

func TestDataOperatorsCmdWithConfig_InvalidJSONResponse(t *testing.T) {
	env := newDataTestEnv(t)
	saveDataTestCredentials(t, env.fileSvc, env.cfg, "user-001")

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getResp: []byte("not json")}
	cmd := dataOperatorsCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestDataOperatorsCmdWithConfig_NotAuthenticatedWithoutCredentials(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{}
	cmd := dataOperatorsCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
	assert.Empty(t, client.getCalls, "client must not be called when credentials are missing")
}

func TestDataOperatorsCmdWithConfig_SendsUserIDQueryParameter(t *testing.T) {
	env := newDataTestEnv(t)
	saveDataTestCredentials(t, env.fileSvc, env.cfg, "user-distinct-456")

	slotResp := models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{}}
	opsJSON, _ := json.Marshal(slotResp)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getResp: opsJSON}
	cmd := dataOperatorsCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	require.Len(t, client.getCalls, 1)
	assert.Contains(t, client.getCalls[0], "user_id=user-distinct-456")
	assert.Equal(t, constants.APIPaths.Operators+"?user_id=user-distinct-456", client.getCalls[0])
}

func TestDataSettingsCmdWithConfig_ValidResponse(t *testing.T) {
	env := newDataTestEnv(t)

	settings := map[string]interface{}{
		"settings":   map[string]interface{}{"key": "value"},
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-01T00:00:00Z",
	}
	settingsJSON, _ := json.Marshal(settings)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getResp: settingsJSON}
	cmd := dataSettingsCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Platform Settings")
	assert.Equal(t, []string{"/db/settings/platform_settings"}, client.getCalls)
}

func TestDataSettingsCmdWithConfig_InvalidJSONResponse(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getResp: []byte("invalid")}
	cmd := dataSettingsCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestDataStoreCmdWithConfig_NoCollectionReturnsError(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{}
	cmd := dataStoreCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCollectionRequired)
}

func TestDataStoreCmdWithConfig_ListModeCallsQuery(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{postResp: []byte(`[{"id":"doc1"}]`)}
	cmd := dataStoreCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	cmd.Flags().Set("collection", "test_collection")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Documents in collection 'test_collection'")
	assert.Len(t, client.postCalls, 1)
	assert.Contains(t, client.postCalls[0].path, "/db/test_collection/_query")
}

func TestDataStoreCmdWithConfig_DocumentModeCallsGet(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getResp: []byte(`{"id":"doc1","data":"hello"}`)}
	cmd := dataStoreCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	cmd.Flags().Set("collection", "test_collection")
	cmd.Flags().Set("document-id", "doc1")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Document test_collection/doc1")
	assert.Equal(t, []string{"/db/test_collection/doc1"}, client.getCalls)
}

func TestDataStoreCmdWithConfig_PostError(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{postErr: errors.New("query failed")}
	cmd := dataStoreCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	cmd.Flags().Set("collection", "test_collection")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query collection")
}

func TestDataStoreCmdWithConfig_GetError(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getErr: errors.New("fetch failed")}
	cmd := dataStoreCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	cmd.Flags().Set("collection", "test_collection")
	cmd.Flags().Set("document-id", "doc1")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch document")
}

func TestDataAuditListCmdWithConfig_NoSessionIDReturnsError(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{}
	cmd := dataAuditListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOperatorSessionIDRequired)
}

func TestDataAuditListCmdWithConfig_ValidQuery(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getResp: []byte(`{"success":true,"events":[{"id":1,"operator_session_id":"sess-123","timestamp":"2026-08-27T10:00:00Z","type":"login","command_raw":"ls","command_exit_code":0}],"count":1}`)}
	cmd := dataAuditListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	cmd.Flags().Set("operator-session-id", "sess-123")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Audit events for session sess-123")
	assert.Len(t, client.getCalls, 1)
	assert.Equal(t, constants.APIPaths.AuditEvents+"?limit=100&operator_session_id=sess-123", client.getCalls[0])
}

func TestDataAuditListCmdWithConfig_GetError(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{getErr: errors.New("query failed")}
	cmd := dataAuditListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	cmd.Flags().Set("operator-session-id", "sess-123")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch audit events")
}
