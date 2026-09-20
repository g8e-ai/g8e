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
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestDispatchOllamaServiceCommand_RejectsMissingFields(t *testing.T) {
	dispatcher := &harnessOllamaServiceDispatcher{client: &harnessclient.Client{}}
	_, err := dispatcher.DispatchOllamaServiceCommand(context.Background(), evaluation.OllamaServiceDispatchRequest{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestDispatchOllamaServiceCommand_ReturnsCommandResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, constants.APIPaths.OperatorsCommands, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		commandResult := &operatorv1.CommandResult{ReturnCode: 0}
		payload, err := proto.Marshal(commandResult)
		require.NoError(t, err)
		response := harnessclient.DispatchCommandResponse{
			Success:       true,
			ResultPayload: payload,
		}
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	t.Cleanup(server.Close)

	client, err := harnessclient.New(harnessconfig.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)
	dispatcher := &harnessOllamaServiceDispatcher{
		client: client,
		persona: harnessclient.Persona{
			ID:                "g8e-campaign-ollama-service",
			UserAgent:         "g8e-eval-campaign",
			UserID:            "user-1",
			CLISessionID:      "cli-1",
			OperatorID:        "data-op",
			OperatorSessionID: "data-session",
		},
	}

	result, err := dispatcher.DispatchOllamaServiceCommand(context.Background(), evaluation.OllamaServiceDispatchRequest{
		ObserverSessionID: "observer-session",
		Command:           "restart",
		ExecutionID:       "exec-1",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.NotEmpty(t, result.ResponseBody)
}

func TestNewHarnessOllamaServiceDispatcher_BuildsPersona(t *testing.T) {
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

	dispatcher, err := newHarnessOllamaServiceDispatcher(cfg, authContext, dataOperator, deps)
	require.NoError(t, err)
	require.NotNil(t, dispatcher)
	assert.Equal(t, "data-op", dispatcher.persona.OperatorID)
}

func TestRestartOllamaViaObserverIfEnabled_SkipsWhenNoObserver(t *testing.T) {
	outcome, err := restartOllamaViaObserverIfEnabled(
		context.Background(),
		campaignOrchestrateOperators(),
		"run-1",
		&evaluation.DataOperatorStatus{OperatorID: "data-op", OperatorSessionID: "data-session"},
		&config.Config{},
		&auth.ClientAuthContext{UserID: "user-1", CLISessionID: "cli-1"},
		chatEvalDeps{},
		func(prefix string) string { return prefix + "-1" },
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.False(t, outcome.Performed)
}

func TestRestartOllamaViaObserverIfEnabled_SkipsWhenDisabled(t *testing.T) {
	operators := []models.OperatorDocumentGo{
		{
			ID:                "observer-op",
			OperatorSessionID: "observer-session",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig: &models.RuntimeConfig{
				ProviderBoundaryObserverEnabled: true,
			},
		},
	}
	outcome, err := restartOllamaViaObserverIfEnabled(
		context.Background(),
		operators,
		"run-1",
		&evaluation.DataOperatorStatus{OperatorID: "data-op", OperatorSessionID: "data-session"},
		&config.Config{},
		&auth.ClientAuthContext{UserID: "user-1", CLISessionID: "cli-1"},
		chatEvalDeps{},
		func(prefix string) string { return prefix + "-1" },
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.False(t, outcome.Performed)
}
