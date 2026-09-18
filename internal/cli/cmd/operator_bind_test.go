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

type stubOperatorBindClient struct {
	result auth.CLISessionBind
	err    error
	called bool
	arg    string
}

func (s *stubOperatorBindClient) Bind(_ context.Context, _ fs.RuntimeFileService, operatorSessionID string) (auth.CLISessionBind, error) {
	s.called = true
	s.arg = operatorSessionID
	return s.result, s.err
}

func TestOperatorBindCmdWithConfig_Success(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	listBody, err := json.Marshal(models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{{
			ID:                "op-data",
			OperatorSessionID: "899b5d27-4599-4b5c-ac5a-beb86b256e7d",
			OperatorType:      constants.OperatorTypeRemote,
			Status:            constants.OperatorStatusActive,
		}},
	})
	require.NoError(t, err)

	stub := &stubOperatorBindClient{result: auth.CLISessionBind{
		CLISessionID:      "cli-new",
		UserID:            "user-001",
		OperatorSessionID: "899b5d27-4599-4b5c-ac5a-beb86b256e7d",
		OperatorID:        "op-data",
	}}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	cmd := operatorBindCmdWithConfig(
		loader,
		mockClientFactory(&mockAPIClient{getResp: listBody}),
		func(*config.Config) operatorBindClient { return stub },
		fileSvcFactoryFor(fileSvc),
	)
	cmd.SetArgs([]string{"899b5d27-4599-4b5c-ac5a-beb86b256e7d", "--yes"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())
	assert.True(t, stub.called)
	assert.Equal(t, "899b5d27-4599-4b5c-ac5a-beb86b256e7d", stub.arg)
	assert.Contains(t, buf.String(), "CLI session bound to operator session")

	loaded, err := auth.LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "cli-new", loaded.CLISessionID)
	assert.Equal(t, "899b5d27-4599-4b5c-ac5a-beb86b256e7d", loaded.OperatorSessionID)
	assert.Equal(t, "op-data", loaded.OperatorID)
}

func TestOperatorBindCmdWithConfig_NotAuthenticated(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	loader := func(string) (*config.Config, error) { return cfg, nil }
	cmd := operatorBindCmdWithConfig(
		loader,
		mockClientFactory(&mockAPIClient{}),
		func(*config.Config) operatorBindClient { return &stubOperatorBindClient{} },
		fileSvcFactoryFor(fileSvc),
	)
	cmd.SetArgs([]string{"899b5d27-4599-4b5c-ac5a-beb86b256e7d", "--yes"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
}

func TestOperatorBindCmdWithConfig_OperatorNotFound(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	listBody, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{}})
	require.NoError(t, err)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	cmd := operatorBindCmdWithConfig(
		loader,
		mockClientFactory(&mockAPIClient{getResp: listBody}),
		func(*config.Config) operatorBindClient { return &stubOperatorBindClient{} },
		fileSvcFactoryFor(fileSvc),
	)
	cmd.SetArgs([]string{"899b5d27-4599-4b5c-ac5a-beb86b256e7d", "--yes"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no operator found with session id")
}

func TestOperatorBindCmdWithConfig_BindError(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	listBody, err := json.Marshal(models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{{
			ID:                "op-data",
			OperatorSessionID: "899b5d27-4599-4b5c-ac5a-beb86b256e7d",
			Status:            constants.OperatorStatusActive,
		}},
	})
	require.NoError(t, err)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	cmd := operatorBindCmdWithConfig(
		loader,
		mockClientFactory(&mockAPIClient{getResp: listBody}),
		func(*config.Config) operatorBindClient {
			return &stubOperatorBindClient{err: errors.New("gateway rejected bind")}
		},
		fileSvcFactoryFor(fileSvc),
	)
	cmd.SetArgs([]string{"899b5d27-4599-4b5c-ac5a-beb86b256e7d", "--yes"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gateway rejected bind")
}
