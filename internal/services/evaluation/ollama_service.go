// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// OllamaModelCommandDispatchRequest carries one governed, unscored model
// maintenance command to the exact Inference Operator session.
type OllamaModelCommandDispatchRequest struct {
	TargetOperatorSessionID string
	Command                 string
	Environment             map[string]string
	WorkingDirectory        string
	TimeoutSeconds          int32
	ExecutionID             string
	CaseID                  string
	InvestigationID         string
	TaskID                  string
}

// OllamaModelCommandDispatchResult is the Operator outcome for one model
// maintenance command.
type OllamaModelCommandDispatchResult struct {
	Status       int
	Success      bool
	Result       *operatorv1.CommandResult
	ResponseBody []byte
}

// OllamaModelCommandDispatcher sends governed model maintenance commands to
// one exact Operator session.
type OllamaModelCommandDispatcher interface {
	DispatchOllamaModelCommand(context.Context, OllamaModelCommandDispatchRequest) (*OllamaModelCommandDispatchResult, error)
}

// ValidateOllamaModelCommandResult validates the HTTP and Operator result for
// one model maintenance command.
func ValidateOllamaModelCommandResult(command string, result *OllamaModelCommandDispatchResult) error {
	if result == nil {
		return fmt.Errorf("%w: Ollama model command %q returned no result", constants.ErrEvaluationDispatchFailed, command)
	}
	if result.Status != http.StatusOK || !result.Success {
		return fmt.Errorf("%w: Ollama model command %q rejected with status %d", constants.ErrEvaluationDispatchFailed, command, result.Status)
	}
	if result.Result == nil {
		return fmt.Errorf("%w: Ollama model command %q missing command result", constants.ErrEvaluationDispatchFailed, command)
	}
	if result.Result.GetStatus() != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		return fmt.Errorf("%w: Ollama model command %q failed: %s", constants.ErrEvaluationDispatchFailed, command, result.Result.GetError())
	}
	if result.Result.GetReturnCode() != 0 {
		return fmt.Errorf("%w: Ollama model command %q exited with code %d", constants.ErrEvaluationDispatchFailed, command, result.Result.GetReturnCode())
	}
	return nil
}

// MarshalOllamaModelCommandPayload builds the typed EXECUTE_BASH payload for
// one unscored model maintenance command.
func MarshalOllamaModelCommandPayload(request OllamaModelCommandDispatchRequest) ([]byte, error) {
	if request.Command == "" || request.ExecutionID == "" {
		return nil, fmt.Errorf("%w: model command and execution ID are required", constants.ErrMissingRequiredField)
	}
	payload, err := proto.Marshal(&operatorv1.CommandRequested{
		Command:          request.Command,
		ExecutionId:      request.ExecutionID,
		Justification:    "evaluation campaign model maintenance",
		Environment:      request.Environment,
		WorkingDirectory: request.WorkingDirectory,
		TimeoutSeconds:   request.TimeoutSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal Ollama model command: %v", constants.ErrEvaluationDispatchFailed, err)
	}
	return payload, nil
}

// ReleaseOllamaModels unloads only the validated campaign-owned model tags
// through the exact Inference Operator session after scored work is complete.
func ReleaseOllamaModels(ctx context.Context, dispatcher OllamaModelCommandDispatcher, targetOperatorSessionID, runID string, modelTags []string, environment map[string]string, newID func(string) string) error {
	if dispatcher == nil || targetOperatorSessionID == "" || runID == "" || newID == nil {
		return fmt.Errorf("evaluation: release Ollama models: %w", constants.ErrMissingRequiredField)
	}
	for index, modelTag := range modelTags {
		command, err := operatorcapability.OllamaStopCommand(modelTag)
		if err != nil {
			return fmt.Errorf("evaluation: release Ollama models: %w", err)
		}
		request := OllamaModelCommandDispatchRequest{
			TargetOperatorSessionID: targetOperatorSessionID,
			Command:                 command,
			Environment:             environment,
			TimeoutSeconds:          60,
			ExecutionID:             newID(fmt.Sprintf("ollama-release-%d", index)),
			CaseID:                  runID,
			InvestigationID:         "ollama-model-release",
			TaskID:                  newID("ollama-release"),
		}
		result, err := dispatcher.DispatchOllamaModelCommand(ctx, request)
		if err != nil {
			return fmt.Errorf("evaluation: release Ollama models: %w", err)
		}
		if err := ValidateOllamaModelCommandResult(command, result); err != nil {
			return fmt.Errorf("evaluation: release Ollama models: %w", err)
		}
	}
	return nil
}
