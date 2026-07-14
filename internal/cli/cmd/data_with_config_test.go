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
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
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
	assert.Equal(t, []string{"/api/users"}, client.getCalls)
}

func TestDataOperatorsCmdWithConfig_ValidResponse(t *testing.T) {
	env := newDataTestEnv(t)

	operators := []map[string]interface{}{
		{"id": "op1", "cloud_subtype": "aws", "status": "active"},
	}
	opsJSON, _ := json.Marshal(operators)

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
	assert.Equal(t, []string{"/api/operators"}, client.getCalls)
}

func TestDataOperatorsCmdWithConfig_InvalidJSONResponse(t *testing.T) {
	env := newDataTestEnv(t)

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
	client := &mockAPIClient{postResp: []byte(`[{"type":"login"}]`)}
	cmd := dataAuditListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	cmd.Flags().Set("operator-session-id", "sess-123")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Audit events for session sess-123")
	assert.Len(t, client.postCalls, 1)
	assert.Equal(t, "/db/audit_events/_query", client.postCalls[0].path)
}

func TestDataAuditListCmdWithConfig_PostError(t *testing.T) {
	env := newDataTestEnv(t)

	loader := func(string) (*config.Config, error) { return env.cfg, nil }
	client := &mockAPIClient{postErr: errors.New("query failed")}
	cmd := dataAuditListCmdWithConfig(loader, mockClientFactory(client), fileSvcFactoryFor(env.fileSvc))
	cmd.Flags().Set("operator-session-id", "sess-123")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query audit events")
}
