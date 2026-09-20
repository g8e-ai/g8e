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

// OllamaServiceDispatchRequest carries one governed EXECUTE_BASH dispatch to
// the provider-boundary observer operator on the Ollama host.
type OllamaServiceDispatchRequest struct {
	ObserverSessionID string
	Command           string
	ExecutionID       string
	CaseID            string
	InvestigationID   string
	TaskID            string
}

// OllamaServiceDispatchResult is the operator command outcome for one dispatch.
type OllamaServiceDispatchResult struct {
	Status       int
	Success      bool
	Result       *operatorv1.CommandResult
	ResponseBody []byte
}

// OllamaServiceDispatcher executes governed shell commands on the observer
// operator session.
type OllamaServiceDispatcher interface {
	DispatchOllamaServiceCommand(context.Context, OllamaServiceDispatchRequest) (*OllamaServiceDispatchResult, error)
}

// OllamaRestartOutcome summarizes whether a governed provider restart ran.
type OllamaRestartOutcome struct {
	Performed         bool
	SkipReason        string
	ObserverSessionID string
	Platform          string
	CommandCount      int
}

// RestartOllamaViaObserver gracefully restarts the local Ollama provider on the
// provider host through the enrolled observer operator. The observer must have
// started with --ollama; otherwise this returns immediately without dispatching.
func RestartOllamaViaObserver(ctx context.Context, observer *ProviderBoundaryObserverStatus, dispatcher OllamaServiceDispatcher, runID string, newID func(string) string) (OllamaRestartOutcome, error) {
	outcome := OllamaRestartOutcome{}
	if observer == nil || dispatcher == nil || newID == nil {
		outcome.SkipReason = "missing observer or dispatcher"
		return outcome, nil
	}
	outcome.ObserverSessionID = observer.OperatorSessionID
	outcome.Platform = observer.Platform
	if !observer.OllamaEnabled {
		outcome.SkipReason = "observer did not opt in with --ollama"
		return outcome, nil
	}
	commands := operatorcapability.RestartOllamaCommands(observer.Platform)
	outcome.CommandCount = len(commands)
	for index, command := range commands {
		result, err := dispatcher.DispatchOllamaServiceCommand(ctx, OllamaServiceDispatchRequest{
			ObserverSessionID: observer.OperatorSessionID,
			Command:           command,
			ExecutionID:       newID(fmt.Sprintf("ollama-restart-%d", index)),
			CaseID:            runID,
			InvestigationID:   "ollama-service-restart",
			TaskID:            newID("ollama-step"),
		})
		if err != nil {
			return outcome, fmt.Errorf("evaluation: ollama service restart: %w", err)
		}
		if err := validateOllamaServiceDispatchResult(command, result); err != nil {
			return outcome, err
		}
	}
	outcome.Performed = true
	return outcome, nil
}

func validateOllamaServiceDispatchResult(command string, result *OllamaServiceDispatchResult) error {
	if result == nil {
		return fmt.Errorf("%w: ollama service command %q returned no result", constants.ErrEvaluationDispatchFailed, command)
	}
	if result.Status != http.StatusOK || !result.Success {
		return fmt.Errorf("%w: ollama service command %q rejected with status %d", constants.ErrEvaluationDispatchFailed, command, result.Status)
	}
	if result.Result == nil {
		return fmt.Errorf("%w: ollama service command %q missing command result", constants.ErrEvaluationDispatchFailed, command)
	}
	if result.Result.GetStatus() != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		return fmt.Errorf("%w: ollama service command %q failed: %s", constants.ErrEvaluationDispatchFailed, command, result.Result.GetError())
	}
	returnCode := result.Result.GetReturnCode()
	if operatorcapability.ToleratedOllamaRestartExitCode(command, returnCode) {
		return nil
	}
	if command == operatorcapability.OllamaServiceCommandPS && returnCode != 0 {
		return fmt.Errorf("%w: ollama ps exited with code %d", constants.ErrEvaluationDispatchFailed, returnCode)
	}
	if returnCode != 0 {
		return fmt.Errorf("%w: ollama service command %q exited with code %d", constants.ErrEvaluationDispatchFailed, command, returnCode)
	}
	return nil
}

// MarshalOllamaServiceCommandPayload builds the EXECUTE_BASH payload for one
// Ollama service lifecycle command.
func MarshalOllamaServiceCommandPayload(command, executionID string) ([]byte, error) {
	payload, err := proto.Marshal(&operatorv1.CommandRequested{
		Command:       command,
		ExecutionId:   executionID,
		Justification: "evaluation campaign ollama service restart",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal ollama service command: %v", constants.ErrEvaluationDispatchFailed, err)
	}
	return payload, nil
}
