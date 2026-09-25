// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
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

// OllamaModelMaintenanceContext carries governed model-maintenance dispatch
// fields shared by pull, copy, inventory, and residency commands.
type OllamaModelMaintenanceContext struct {
	TargetOperatorSessionID string
	Environment             map[string]string
	Timeout                 time.Duration
	CaseID                  string
	NewID                   func(string) string
}

func (c OllamaModelMaintenanceContext) validate() error {
	if c.TargetOperatorSessionID == "" || c.NewID == nil {
		return fmt.Errorf("evaluation: Ollama model maintenance: %w", constants.ErrMissingRequiredField)
	}
	return nil
}

func (c OllamaModelMaintenanceContext) timeoutSeconds() int32 {
	if c.Timeout <= 0 {
		return int32(inference.ProviderRequestTimeout / time.Second)
	}
	seconds := int32(c.Timeout / time.Second)
	if seconds <= 0 {
		return 1
	}
	return seconds
}

func dispatchOllamaModelCommand(
	ctx context.Context,
	dispatcher OllamaModelCommandDispatcher,
	maintenance OllamaModelMaintenanceContext,
	command string,
	investigationID string,
	executionPrefix string,
) (*OllamaModelCommandDispatchResult, error) {
	if dispatcher == nil {
		return nil, fmt.Errorf("evaluation: Ollama model maintenance: %w", constants.ErrMissingRequiredField)
	}
	if err := maintenance.validate(); err != nil {
		return nil, err
	}
	caseID := strings.TrimSpace(maintenance.CaseID)
	if caseID == "" {
		caseID = "ollama-model-maintenance"
	}
	request := OllamaModelCommandDispatchRequest{
		TargetOperatorSessionID: maintenance.TargetOperatorSessionID,
		Command:                 command,
		Environment:             maintenance.Environment,
		TimeoutSeconds:          maintenance.timeoutSeconds(),
		ExecutionID:             maintenance.NewID(executionPrefix),
		CaseID:                  caseID,
		InvestigationID:         investigationID,
		TaskID:                  maintenance.NewID(executionPrefix + "-task"),
	}
	result, err := dispatcher.DispatchOllamaModelCommand(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("evaluation: Ollama model maintenance: %w", err)
	}
	if err := ValidateOllamaModelCommandResult(command, result); err != nil {
		return nil, fmt.Errorf("evaluation: Ollama model maintenance: %w", err)
	}
	return result, nil
}

func ollamaModelCommandStdout(result *OllamaModelCommandDispatchResult) ([]byte, error) {
	if result == nil || result.Result == nil {
		return nil, fmt.Errorf("evaluation: Ollama model maintenance: %w", constants.ErrMissingRequiredField)
	}
	stdout := strings.TrimSpace(result.Result.GetStdout())
	if stdout == "" {
		return nil, fmt.Errorf("evaluation: Ollama model maintenance: empty command stdout")
	}
	return []byte(stdout), nil
}

// PullOllamaModel downloads one model tag through the exact Inference Operator.
func PullOllamaModel(ctx context.Context, dispatcher OllamaModelCommandDispatcher, maintenance OllamaModelMaintenanceContext, modelTag string) error {
	command, err := operatorcapability.OllamaPullCommand(modelTag)
	if err != nil {
		return fmt.Errorf("evaluation: pull Ollama model: %w", err)
	}
	_, err = dispatchOllamaModelCommand(ctx, dispatcher, maintenance, command, "ollama-model-pull", "ollama-pull")
	return err
}

// CopyOllamaModel aliases one model tag through the exact Inference Operator.
func CopyOllamaModel(ctx context.Context, dispatcher OllamaModelCommandDispatcher, maintenance OllamaModelMaintenanceContext, source, destination string) error {
	command, err := operatorcapability.OllamaCopyCommand(source, destination)
	if err != nil {
		return fmt.Errorf("evaluation: copy Ollama model: %w", err)
	}
	_, err = dispatchOllamaModelCommand(ctx, dispatcher, maintenance, command, "ollama-model-copy", "ollama-copy")
	return err
}

// ListOllamaProviderInventory queries provider inventory through the exact
// Inference Operator session.
func ListOllamaProviderInventory(ctx context.Context, dispatcher OllamaModelCommandDispatcher, maintenance OllamaModelMaintenanceContext) ([]inference.ProviderModelInventoryEntry, error) {
	command := operatorcapability.OllamaInventoryCommand()
	result, err := dispatchOllamaModelCommand(ctx, dispatcher, maintenance, command, "ollama-model-inventory", "ollama-inventory")
	if err != nil {
		return nil, err
	}
	stdout, err := ollamaModelCommandStdout(result)
	if err != nil {
		return nil, err
	}
	var entries []inference.ProviderModelInventoryEntry
	if err := json.Unmarshal(stdout, &entries); err != nil {
		return nil, fmt.Errorf("evaluation: list Ollama provider inventory: decode: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("evaluation: list Ollama provider inventory: %w", constants.ErrInferenceModelNotFound)
	}
	return entries, nil
}

// ReadOllamaProviderResidency queries typed /api/ps residency through the exact
// Inference Operator session.
func ReadOllamaProviderResidency(ctx context.Context, dispatcher OllamaModelCommandDispatcher, maintenance OllamaModelMaintenanceContext) (inference.ProviderResidency, error) {
	command := operatorcapability.OllamaResidencyCommand()
	result, err := dispatchOllamaModelCommand(ctx, dispatcher, maintenance, command, "ollama-model-residency", "ollama-residency")
	if err != nil {
		return inference.ProviderResidency{}, err
	}
	stdout, err := ollamaModelCommandStdout(result)
	if err != nil {
		return inference.ProviderResidency{}, err
	}
	var residency inference.ProviderResidency
	if err := json.Unmarshal(stdout, &residency); err != nil {
		return inference.ProviderResidency{}, fmt.Errorf("evaluation: read Ollama provider residency: decode: %w", err)
	}
	for _, model := range residency.Models {
		if model.Name == "" {
			return inference.ProviderResidency{}, fmt.Errorf("evaluation: read Ollama provider residency: %w", constants.ErrInferenceProviderResponseInvalid)
		}
	}
	return residency, nil
}

// WaitForOllamaModelsAbsent polls governed residency until named tags are absent.
func WaitForOllamaModelsAbsent(ctx context.Context, dispatcher OllamaModelCommandDispatcher, maintenance OllamaModelMaintenanceContext, modelTags []string) error {
	if len(modelTags) == 0 {
		return nil
	}
	pollMaintenance := maintenance
	if pollMaintenance.Timeout <= 0 {
		pollMaintenance.Timeout = inference.ProviderStatusTimeout
	}
	pollInterval := 2 * time.Second
	for {
		residency, err := ReadOllamaProviderResidency(ctx, dispatcher, pollMaintenance)
		if err != nil {
			return fmt.Errorf("evaluation: wait for Ollama models absent: %w", err)
		}
		absent := true
		for _, resident := range residency.Models {
			for _, wanted := range modelTags {
				if resident.Name == wanted {
					absent = false
					break
				}
			}
			if !absent {
				break
			}
		}
		if absent {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("evaluation: wait for Ollama models absent: %w", ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// ReleaseOllamaModels unloads only the validated campaign-owned model tags
// through the exact Inference Operator session after scored work is complete.
func ReleaseOllamaModels(ctx context.Context, dispatcher OllamaModelCommandDispatcher, targetOperatorSessionID, runID string, modelTags []string, environment map[string]string, newID func(string) string) error {
	if dispatcher == nil || targetOperatorSessionID == "" || runID == "" || newID == nil {
		return fmt.Errorf("evaluation: release Ollama models: %w", constants.ErrMissingRequiredField)
	}
	for index, modelTag := range modelTags {
		command, err := operatorcapability.OllamaReleaseCommand(modelTag)
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
