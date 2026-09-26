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
	ActionType              constants.ActionType
	Command                 string
	Payload                 []byte
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
	Status          int
	Success         bool
	ActionType      constants.ActionType
	CommandResult   *operatorv1.CommandResult
	InventoryResult *operatorv1.OllamaModelInventoryResult
	ResidencyResult *operatorv1.OllamaModelResidencyResult
	ResponseBody    []byte
}

// OllamaModelCommandDispatcher sends governed model maintenance commands to
// one exact Operator session.
type OllamaModelCommandDispatcher interface {
	DispatchOllamaModelCommand(context.Context, OllamaModelCommandDispatchRequest) (*OllamaModelCommandDispatchResult, error)
}

// ValidateOllamaModelCommandResult validates the HTTP and Operator result for
// one EXECUTE_BASH model maintenance command.
func ValidateOllamaModelCommandResult(command string, result *OllamaModelCommandDispatchResult) error {
	if result == nil {
		return fmt.Errorf("%w: Ollama model command %q returned no result", constants.ErrEvaluationDispatchFailed, command)
	}
	if result.Status != http.StatusOK || !result.Success {
		return fmt.Errorf("%w: Ollama model command %q rejected with status %d", constants.ErrEvaluationDispatchFailed, command, result.Status)
	}
	if result.CommandResult == nil {
		return fmt.Errorf("%w: Ollama model command %q missing command result", constants.ErrEvaluationDispatchFailed, command)
	}
	if result.CommandResult.GetStatus() != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		return fmt.Errorf("%w: Ollama model command %q failed: %s", constants.ErrEvaluationDispatchFailed, command, result.CommandResult.GetError())
	}
	if result.CommandResult.GetReturnCode() != 0 {
		return fmt.Errorf("%w: Ollama model command %q exited with code %d", constants.ErrEvaluationDispatchFailed, command, result.CommandResult.GetReturnCode())
	}
	return nil
}

// ValidateOllamaModelInventoryResult validates the HTTP and Operator result
// for one typed inventory query.
func ValidateOllamaModelInventoryResult(result *OllamaModelCommandDispatchResult) error {
	if result == nil {
		return fmt.Errorf("%w: Ollama model inventory returned no result", constants.ErrEvaluationDispatchFailed)
	}
	if result.Status != http.StatusOK || !result.Success {
		return fmt.Errorf("%w: Ollama model inventory rejected with status %d", constants.ErrEvaluationDispatchFailed, result.Status)
	}
	if result.InventoryResult == nil {
		return fmt.Errorf("%w: Ollama model inventory missing typed result", constants.ErrEvaluationDispatchFailed)
	}
	if result.InventoryResult.GetStatus() != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		return fmt.Errorf("%w: Ollama model inventory failed: %s", constants.ErrEvaluationDispatchFailed, result.InventoryResult.GetErrorMessage())
	}
	if len(result.InventoryResult.GetEntries()) == 0 {
		return fmt.Errorf("%w: Ollama model inventory returned no entries", constants.ErrInferenceModelNotFound)
	}
	return nil
}

// ValidateOllamaModelResidencyResult validates the HTTP and Operator result for
// one typed residency query.
func ValidateOllamaModelResidencyResult(result *OllamaModelCommandDispatchResult) error {
	if result == nil {
		return fmt.Errorf("%w: Ollama model residency returned no result", constants.ErrEvaluationDispatchFailed)
	}
	if result.Status != http.StatusOK || !result.Success {
		return fmt.Errorf("%w: Ollama model residency rejected with status %d", constants.ErrEvaluationDispatchFailed, result.Status)
	}
	if result.ResidencyResult == nil {
		return fmt.Errorf("%w: Ollama model residency missing typed result", constants.ErrEvaluationDispatchFailed)
	}
	if result.ResidencyResult.GetStatus() != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		return fmt.Errorf("%w: Ollama model residency failed: %s", constants.ErrEvaluationDispatchFailed, result.ResidencyResult.GetErrorMessage())
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

// MarshalOllamaModelInventoryPayload builds the typed inventory query payload.
func MarshalOllamaModelInventoryPayload(executionID string) ([]byte, error) {
	if executionID == "" {
		return nil, fmt.Errorf("%w: execution ID is required", constants.ErrMissingRequiredField)
	}
	payload, err := proto.Marshal(&operatorv1.OllamaModelInventoryRequested{ExecutionId: executionID})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal Ollama model inventory request: %v", constants.ErrEvaluationDispatchFailed, err)
	}
	return payload, nil
}

// MarshalOllamaModelResidencyPayload builds the typed residency query payload.
func MarshalOllamaModelResidencyPayload(executionID string) ([]byte, error) {
	if executionID == "" {
		return nil, fmt.Errorf("%w: execution ID is required", constants.ErrMissingRequiredField)
	}
	payload, err := proto.Marshal(&operatorv1.OllamaModelResidencyRequested{ExecutionId: executionID})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal Ollama model residency request: %v", constants.ErrEvaluationDispatchFailed, err)
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

func dispatchOllamaBashCommand(
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
	executionID := maintenance.NewID(executionPrefix)
	payload, err := MarshalOllamaModelCommandPayload(OllamaModelCommandDispatchRequest{
		Command:        command,
		ExecutionID:    executionID,
		Environment:    maintenance.Environment,
		TimeoutSeconds: maintenance.timeoutSeconds(),
	})
	if err != nil {
		return nil, err
	}
	request := OllamaModelCommandDispatchRequest{
		TargetOperatorSessionID: maintenance.TargetOperatorSessionID,
		ActionType:              constants.ActionTypeExecuteBash,
		Command:                 command,
		Payload:                 payload,
		Environment:             maintenance.Environment,
		TimeoutSeconds:          maintenance.timeoutSeconds(),
		ExecutionID:             executionID,
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

func dispatchOllamaTypedQuery(
	ctx context.Context,
	dispatcher OllamaModelCommandDispatcher,
	maintenance OllamaModelMaintenanceContext,
	actionType constants.ActionType,
	payload []byte,
	investigationID string,
	executionPrefix string,
	validate func(*OllamaModelCommandDispatchResult) error,
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
		ActionType:              actionType,
		Payload:                 payload,
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
	if err := validate(result); err != nil {
		return nil, fmt.Errorf("evaluation: Ollama model maintenance: %w", err)
	}
	return result, nil
}

// PullOllamaModel downloads one model tag through the exact Inference Operator.
func PullOllamaModel(ctx context.Context, dispatcher OllamaModelCommandDispatcher, maintenance OllamaModelMaintenanceContext, modelTag string) error {
	command, err := operatorcapability.OllamaPullCommand(modelTag)
	if err != nil {
		return fmt.Errorf("evaluation: pull Ollama model: %w", err)
	}
	_, err = dispatchOllamaBashCommand(ctx, dispatcher, maintenance, command, "ollama-model-pull", "ollama-pull")
	return err
}

// CopyOllamaModel aliases one model tag through the exact Inference Operator.
func CopyOllamaModel(ctx context.Context, dispatcher OllamaModelCommandDispatcher, maintenance OllamaModelMaintenanceContext, source, destination string) error {
	command, err := operatorcapability.OllamaCopyCommand(source, destination)
	if err != nil {
		return fmt.Errorf("evaluation: copy Ollama model: %w", err)
	}
	_, err = dispatchOllamaBashCommand(ctx, dispatcher, maintenance, command, "ollama-model-copy", "ollama-copy")
	return err
}

// ListOllamaProviderInventory queries provider inventory through the exact
// Inference Operator session.
func ListOllamaProviderInventory(ctx context.Context, dispatcher OllamaModelCommandDispatcher, maintenance OllamaModelMaintenanceContext) ([]inference.ProviderModelInventoryEntry, error) {
	executionID := maintenance.NewID("ollama-inventory")
	payload, err := MarshalOllamaModelInventoryPayload(executionID)
	if err != nil {
		return nil, err
	}
	result, err := dispatchOllamaTypedQuery(
		ctx,
		dispatcher,
		maintenance,
		constants.ActionTypeOllamaModelInventory,
		payload,
		"ollama-model-inventory",
		"ollama-inventory",
		ValidateOllamaModelInventoryResult,
	)
	if err != nil {
		return nil, err
	}
	return inference.ProviderModelInventoryEntriesFromProto(result.InventoryResult.GetEntries()), nil
}

// ReadOllamaProviderResidency queries typed /api/ps residency through the exact
// Inference Operator session.
func ReadOllamaProviderResidency(ctx context.Context, dispatcher OllamaModelCommandDispatcher, maintenance OllamaModelMaintenanceContext) (inference.ProviderResidency, error) {
	executionID := maintenance.NewID("ollama-residency")
	payload, err := MarshalOllamaModelResidencyPayload(executionID)
	if err != nil {
		return inference.ProviderResidency{}, err
	}
	result, err := dispatchOllamaTypedQuery(
		ctx,
		dispatcher,
		maintenance,
		constants.ActionTypeOllamaModelResidency,
		payload,
		"ollama-model-residency",
		"ollama-residency",
		ValidateOllamaModelResidencyResult,
	)
	if err != nil {
		return inference.ProviderResidency{}, err
	}
	residency := inference.ProviderResidencyFromProto(result.ResidencyResult)
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
		executionID := newID(fmt.Sprintf("ollama-release-%d", index))
		payload, err := MarshalOllamaModelCommandPayload(OllamaModelCommandDispatchRequest{
			Command:        command,
			ExecutionID:    executionID,
			Environment:    environment,
			TimeoutSeconds: 60,
		})
		if err != nil {
			return fmt.Errorf("evaluation: release Ollama models: %w", err)
		}
		request := OllamaModelCommandDispatchRequest{
			TargetOperatorSessionID: targetOperatorSessionID,
			ActionType:              constants.ActionTypeExecuteBash,
			Command:                 command,
			Payload:                 payload,
			Environment:             environment,
			TimeoutSeconds:          60,
			ExecutionID:             executionID,
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
