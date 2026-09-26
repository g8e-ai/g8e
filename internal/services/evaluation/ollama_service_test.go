// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type recordingOllamaModelCommandDispatcher struct {
	requests []OllamaModelCommandDispatchRequest
}

func (d *recordingOllamaModelCommandDispatcher) DispatchOllamaModelCommand(_ context.Context, request OllamaModelCommandDispatchRequest) (*OllamaModelCommandDispatchResult, error) {
	d.requests = append(d.requests, request)
	return &OllamaModelCommandDispatchResult{
		Status:  200,
		Success: true,
		CommandResult: &operatorv1.CommandResult{
			Status:     operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ReturnCode: 0,
		},
	}, nil
}

func TestReleaseOllamaModels_TargetsExactInferenceSession(t *testing.T) {
	t.Parallel()
	dispatcher := &recordingOllamaModelCommandDispatcher{}
	err := ReleaseOllamaModels(context.Background(), dispatcher, "inference-session", "run-1", []string{"qwen3:0.6b", "registry.example/team/gemma3:4b"}, map[string]string{"OLLAMA_HOST": "http://provider.example:11434"}, func(prefix string) string { return prefix + "-id" })
	require.NoError(t, err)
	require.Len(t, dispatcher.requests, 2)
	assert.Equal(t, "inference-session", dispatcher.requests[0].TargetOperatorSessionID)
	assert.Equal(t, "/g8e operator model release qwen3:0.6b", dispatcher.requests[0].Command)
	assert.Equal(t, "/g8e operator model release registry.example/team/gemma3:4b", dispatcher.requests[1].Command)
	assert.Equal(t, "http://provider.example:11434", dispatcher.requests[0].Environment["OLLAMA_HOST"])
}

func TestMarshalOllamaModelCommandPayload_CarriesExecutionFields(t *testing.T) {
	t.Parallel()
	payload, err := MarshalOllamaModelCommandPayload(OllamaModelCommandDispatchRequest{
		Command:          "ollama stop qwen3:0.6b",
		ExecutionID:      "exec-1",
		Environment:      map[string]string{"OLLAMA_HOST": "http://provider.example:11434"},
		WorkingDirectory: "/root",
		TimeoutSeconds:   30,
	})
	require.NoError(t, err)
	var command operatorv1.CommandRequested
	require.NoError(t, proto.Unmarshal(payload, &command))
	assert.Equal(t, "ollama stop qwen3:0.6b", command.GetCommand())
	assert.Equal(t, "exec-1", command.GetExecutionId())
	assert.Equal(t, map[string]string{"OLLAMA_HOST": "http://provider.example:11434"}, command.GetEnvironment())
	assert.Equal(t, "/root", command.GetWorkingDirectory())
	assert.Equal(t, int32(30), command.GetTimeoutSeconds())
}

func TestValidateOllamaModelCommandResult_RejectsNonzeroExit(t *testing.T) {
	t.Parallel()
	err := ValidateOllamaModelCommandResult("ollama stop qwen3:0.6b", &OllamaModelCommandDispatchResult{
		Status:  200,
		Success: true,
		CommandResult: &operatorv1.CommandResult{
			Status:     operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ReturnCode: 1,
		},
	})
	require.Error(t, err)
}
