// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestDispatchOllamaModelCommand_RejectsMissingFields(t *testing.T) {
	dispatcher := &harnessOllamaModelCommandDispatcher{client: &harnessclient.Client{}}
	_, err := dispatcher.DispatchOllamaModelCommand(context.Background(), evaluation.OllamaModelCommandDispatchRequest{})
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestDispatchOllamaModelCommand_TargetsInferenceAndCarriesEnvironment(t *testing.T) {
	var received harnessclient.DispatchCommandRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.Header().Set("Content-Type", "application/json")
		commandResult, err := proto.Marshal(&operatorv1.CommandResult{Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED})
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(w).Encode(harnessclient.DispatchCommandResponse{
			Success:       true,
			ResultPayload: commandResult,
		}))
	}))
	t.Cleanup(server.Close)

	client, err := harnessclient.New(harnessconfig.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)
	dispatcher := &harnessOllamaModelCommandDispatcher{
		client: client,
		persona: harnessclient.Persona{
			UserID:       "user-1",
			CLISessionID: "cli-1",
		},
	}
	request := evaluation.OllamaModelCommandDispatchRequest{
		TargetOperatorSessionID: "inference-session",
		Command:                 "ollama stop qwen3:0.6b",
		Environment:             modelCommandEnvironment("http://provider.example:11434"),
		TimeoutSeconds:          30,
		ExecutionID:             "exec-1",
	}
	result, err := dispatcher.DispatchOllamaModelCommand(context.Background(), request)
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.Equal(t, "inference-session", received.TargetOperatorSessionID)
	assert.Equal(t, string(constants.ActionTypeExecuteBash), received.ActionType)
	var command operatorv1.CommandRequested
	require.NoError(t, proto.Unmarshal(received.Payload, &command))
	assert.Equal(t, request.Command, command.GetCommand())
	assert.Equal(t, request.Environment, command.GetEnvironment())
	assert.Equal(t, request.TimeoutSeconds, command.GetTimeoutSeconds())
}

func TestNewHarnessOllamaModelCommandDispatcher_BuildsPersona(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	cfg := &config.Config{}
	authContext := &auth.ClientAuthContext{UserID: "user-1", CLISessionID: "cli-1"}
	dataOperator := &evaluation.DataOperatorStatus{OperatorID: "data-op", OperatorSessionID: "data-session"}
	deps := chatEvalDeps{
		clientFactory: func(harnessconfig.Config) (*harnessclient.Client, error) {
			return harnessclient.New(harnessconfig.Config{MTLSBaseURL: server.URL})
		},
	}

	dispatcher, err := newHarnessOllamaModelCommandDispatcher(cfg, authContext, dataOperator, deps)
	require.NoError(t, err)
	assert.Equal(t, "data-op", dispatcher.persona.OperatorID)
}
