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
	"io"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type harnessOllamaServiceDispatcher struct {
	client  *harnessclient.Client
	persona harnessclient.Persona
}

func newHarnessOllamaServiceDispatcher(cfg *config.Config, authContext *auth.ClientAuthContext, dataOperator *evaluation.DataOperatorStatus, deps chatEvalDeps) (*harnessOllamaServiceDispatcher, error) {
	client, err := deps.clientFactory(nativeEvalClientConfig(cfg, authContext))
	if err != nil {
		return nil, err
	}
	return &harnessOllamaServiceDispatcher{
		client: client,
		persona: harnessclient.Persona{
			ID:                "g8e-campaign-ollama-service",
			UserAgent:         "g8e-eval-campaign",
			UserID:            authContext.UserID,
			CLISessionID:      authContext.CLISessionID,
			OperatorID:        dataOperator.OperatorID,
			OperatorSessionID: dataOperator.OperatorSessionID,
		},
	}, nil
}

func (d *harnessOllamaServiceDispatcher) DispatchOllamaServiceCommand(ctx context.Context, request evaluation.OllamaServiceDispatchRequest) (*evaluation.OllamaServiceDispatchResult, error) {
	if d == nil || d.client == nil || request.ObserverSessionID == "" || request.Command == "" || request.ExecutionID == "" {
		return nil, fmt.Errorf("evaluation: ollama service dispatch: %w", constants.ErrMissingRequiredField)
	}
	payload, err := evaluation.MarshalOllamaServiceCommandPayload(request.Command, request.ExecutionID)
	if err != nil {
		return nil, err
	}
	status, response, body, err := d.client.DispatchCommand(ctx, d.persona, harnessclient.DispatchCommandRequest{
		TargetOperatorSessionID: request.ObserverSessionID,
		ActionType:              string(constants.ActionTypeExecuteBash),
		Payload:                 payload,
		TargetResource:          "ollama-service",
		CaseID:                  request.CaseID,
		InvestigationID:         request.InvestigationID,
		TaskID:                  request.TaskID,
		CLISessionID:            d.persona.CLISessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluation: ollama service dispatch: %w", err)
	}
	result := &evaluation.OllamaServiceDispatchResult{
		Status:       status,
		Success:      response != nil && response.Success,
		ResponseBody: body,
	}
	if response != nil && len(response.ResultPayload) > 0 {
		commandResult := &operatorv1.CommandResult{}
		if err := proto.Unmarshal(response.ResultPayload, commandResult); err != nil {
			return nil, fmt.Errorf("evaluation: ollama service dispatch: decode command result: %w", err)
		}
		result.Result = commandResult
	}
	return result, nil
}

func restartOllamaViaObserverIfEnabled(
	ctx context.Context,
	operators []models.OperatorDocumentGo,
	runID string,
	dataOperator *evaluation.DataOperatorStatus,
	cfg *config.Config,
	authContext *auth.ClientAuthContext,
	deps chatEvalDeps,
	newID func(string) string,
	logOut io.Writer,
	logErr io.Writer,
) (evaluation.OllamaRestartOutcome, error) {
	observer, err := evaluation.SelectProviderBoundaryObserver(operators, "")
	if err != nil {
		writeOllamaRestartSkip(logErr, err)
		return evaluation.OllamaRestartOutcome{SkipReason: err.Error()}, nil
	}
	if observer == nil || !observer.OllamaEnabled {
		reason := "no active observer with --ollama"
		writeOllamaRestartSkip(logErr, fmt.Errorf("%s", reason))
		return evaluation.OllamaRestartOutcome{SkipReason: reason}, nil
	}
	dispatcher, err := newHarnessOllamaServiceDispatcher(cfg, authContext, dataOperator, deps)
	if err != nil {
		return evaluation.OllamaRestartOutcome{}, err
	}
	outcome, err := evaluation.RestartOllamaViaObserver(ctx, observer, dispatcher, runID, newID)
	if err != nil {
		return outcome, err
	}
	if outcome.Performed && logOut != nil {
		_, _ = fmt.Fprintf(logOut, "Ollama provider restarted via observer %s (%d commands, platform=%s)\n",
			outcome.ObserverSessionID, outcome.CommandCount, outcome.Platform)
	}
	return outcome, nil
}

func writeOllamaRestartSkip(logErr io.Writer, err error) {
	if logErr == nil || err == nil {
		return
	}
	_, _ = fmt.Fprintf(logErr, "warning: governed Ollama restart skipped: %v\n", err)
}
