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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/operator"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestOperatorRunCmdWithConfig_Success(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	sessionA := "4881d566-90a9-44c9-9e3e-c6bb51e07f5c"
	sessionB := "286d7a56-b961-4cff-9c69-063a84b69afd"

	listBody, err := json.Marshal(models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{
			{ID: "op-a", OperatorSessionID: sessionA, OperatorType: constants.OperatorTypeRemote, Status: constants.OperatorStatusActive},
			{ID: "op-b", OperatorSessionID: sessionB, OperatorType: constants.OperatorTypeRemote, Status: constants.OperatorStatusActive},
		},
	})
	require.NoError(t, err)

	mockClient := &mockAPIClient{
		getResp:  listBody,
		postResp: mustMarshalDispatchSuccess(t, "welcome to host-a"),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	cmd := operatorRunCmdWithConfig(
		loader,
		func(_ fs.RuntimeFileService, _ *config.Config, _ time.Duration) (apiClient, error) {
			return mockClient, nil
		},
		fileSvcFactoryFor(fileSvc),
	)
	cmd.SetArgs([]string{sessionA, sessionB, "--cmd", "echo 'welcome to ' $(hostname)"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())
	assert.Len(t, mockClient.postCalls, 2)
	assert.Contains(t, buf.String(), "exit: 0")
	assert.Contains(t, buf.String(), "welcome to host-a")
}

func TestOperatorRunCmdWithConfig_OperatorNotFound(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	listBody, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{}})
	require.NoError(t, err)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	cmd := operatorRunCmdWithConfig(
		loader,
		func(_ fs.RuntimeFileService, _ *config.Config, _ time.Duration) (apiClient, error) {
			return &mockAPIClient{getResp: listBody}, nil
		},
		fileSvcFactoryFor(fileSvc),
	)
	cmd.SetArgs([]string{"4881d566-90a9-44c9-9e3e-c6bb51e07f5c", "--cmd", "echo hi"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no operator found with session id")
}

func TestOperatorStopCmdWithConfig_SendsTargetedShutdown(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	sessionID := "4881d566-90a9-44c9-9e3e-c6bb51e07f5c"
	listBody, err := json.Marshal(models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{{
			ID: "op-a", OperatorSessionID: sessionID, OperatorType: constants.OperatorTypeRemote, Status: constants.OperatorStatusActive,
		}},
	})
	require.NoError(t, err)
	stopBody, err := json.Marshal(models.StopOperatorResponse{Success: true, OperatorID: "op-a", OperatorSessionID: sessionID})
	require.NoError(t, err)
	mockClient := &mockAPIClient{getResp: listBody, postResp: stopBody}

	cmd := operatorStopCmdWithConfig(configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(fileSvc))
	cmd.SetArgs([]string{sessionID, "--reason", "maintenance"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())
	require.Len(t, mockClient.postCalls, 1)
	assert.Equal(t, constants.APIPaths.OperatorsStop, mockClient.postCalls[0].path)
	request := mockClient.postCalls[0].body.(models.StopOperatorRequest)
	assert.Equal(t, sessionID, request.OperatorSessionID)
	assert.Equal(t, "maintenance", request.Reason)
	assert.Contains(t, buf.String(), "Stop requested")
}

func TestOperatorStopCmdWithConfig_RejectsEmbeddedOperator(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	listBody, err := json.Marshal(models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{{
			ID: string(constants.DocIDEmbeddedOperator), OperatorSessionID: "embedded-session", OperatorType: constants.OperatorTypeEmbedded, Status: constants.OperatorStatusActive,
		}},
	})
	require.NoError(t, err)
	mockClient := &mockAPIClient{getResp: listBody}
	cmd := operatorStopCmdWithConfig(configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(fileSvc))
	cmd.SetArgs([]string{"embedded-session"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOperatorStopEmbedded)
	assert.Empty(t, mockClient.postCalls)
}

func TestDedupeOperatorSessionIDs(t *testing.T) {
	ids := dedupeOperatorSessionIDs([]string{
		"4881d566-90a9-44c9-9e3e-c6bb51e07f5c",
		"4881d566-90a9-44c9-9e3e-c6bb51e07f5c",
		"286d7a56-b961-4cff-9c69-063a84b69afd",
	})
	assert.Equal(t, []string{
		"4881d566-90a9-44c9-9e3e-c6bb51e07f5c",
		"286d7a56-b961-4cff-9c69-063a84b69afd",
	}, ids)
}

func mustMarshalDispatchSuccess(t *testing.T, stdout string) []byte {
	t.Helper()
	resultPayload, err := proto.Marshal(&operatorv1.CommandResult{
		Status:     operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		Stdout:     stdout,
		ReturnCode: 0,
	})
	require.NoError(t, err)
	body, err := json.Marshal(operator.DispatchResponse{
		Success:       true,
		TransactionID: "tx-1",
		EventType:     "g8e.v1.operator.command.completed",
		ActionType:    "EXECUTE_BASH_RESULT",
		ResultPayload: resultPayload,
	})
	require.NoError(t, err)
	return body
}
