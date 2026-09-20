// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type harnessOllamaModelCommandDispatcher struct {
	client  *harnessclient.Client
	persona harnessclient.Persona
}

func newHarnessOllamaModelCommandDispatcher(cfg *config.Config, authContext *auth.ClientAuthContext, dataOperator *evaluation.DataOperatorStatus, deps chatEvalDeps) (*harnessOllamaModelCommandDispatcher, error) {
	client, err := deps.clientFactory(nativeEvalClientConfig(cfg, authContext))
	if err != nil {
		return nil, err
	}
	return &harnessOllamaModelCommandDispatcher{
		client: client,
		persona: harnessclient.Persona{
			ID:                "g8e-campaign-model-maintenance",
			UserAgent:         "g8e-eval-campaign",
			UserID:            authContext.UserID,
			CLISessionID:      authContext.CLISessionID,
			OperatorID:        dataOperator.OperatorID,
			OperatorSessionID: dataOperator.OperatorSessionID,
		},
	}, nil
}

func (d *harnessOllamaModelCommandDispatcher) DispatchOllamaModelCommand(ctx context.Context, request evaluation.OllamaModelCommandDispatchRequest) (*evaluation.OllamaModelCommandDispatchResult, error) {
	if d == nil || d.client == nil || request.TargetOperatorSessionID == "" || request.Command == "" || request.ExecutionID == "" {
		return nil, fmt.Errorf("evaluation: Ollama model command dispatch: %w", constants.ErrMissingRequiredField)
	}
	payload, err := evaluation.MarshalOllamaModelCommandPayload(request)
	if err != nil {
		return nil, err
	}
	status, response, body, err := d.client.DispatchCommand(ctx, d.persona, harnessclient.DispatchCommandRequest{
		TargetOperatorSessionID: request.TargetOperatorSessionID,
		ActionType:              string(constants.ActionTypeExecuteBash),
		Payload:                 payload,
		TargetResource:          "ollama-model",
		CaseID:                  request.CaseID,
		InvestigationID:         request.InvestigationID,
		TaskID:                  request.TaskID,
		CLISessionID:            d.persona.CLISessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluation: Ollama model command dispatch: %w", err)
	}
	result := &evaluation.OllamaModelCommandDispatchResult{
		Status:       status,
		Success:      response != nil && response.Success,
		ResponseBody: body,
	}
	if response != nil && len(response.ResultPayload) > 0 {
		commandResult := &operatorv1.CommandResult{}
		if err := proto.Unmarshal(response.ResultPayload, commandResult); err != nil {
			return nil, fmt.Errorf("evaluation: Ollama model command dispatch: decode command result: %w", err)
		}
		result.Result = commandResult
	}
	return result, nil
}

func modelCommandEnvironment(endpoint string) map[string]string {
	return map[string]string{"OLLAMA_HOST": endpoint}
}
