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

	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type stubOllamaServiceDispatcher struct {
	requests []OllamaServiceDispatchRequest
}

func (s *stubOllamaServiceDispatcher) DispatchOllamaServiceCommand(ctx context.Context, request OllamaServiceDispatchRequest) (*OllamaServiceDispatchResult, error) {
	_ = ctx
	s.requests = append(s.requests, request)
	return &OllamaServiceDispatchResult{Status: 200, Success: true, Result: operatorv1CompletedCommandResult()}, nil
}

func TestRestartOllamaViaObserver_SkipsWhenNotEnabled(t *testing.T) {
	dispatcher := &stubOllamaServiceDispatcher{}
	outcome, err := RestartOllamaViaObserver(t.Context(), &ProviderBoundaryObserverStatus{OllamaEnabled: false}, dispatcher, "run-1", func(prefix string) string { return prefix })
	require.NoError(t, err)
	assert.False(t, outcome.Performed)
	assert.Empty(t, dispatcher.requests)
}

func TestRestartOllamaViaObserver_DispatchesLifecycleSequence(t *testing.T) {
	dispatcher := &stubOllamaServiceDispatcher{}
	observer := &ProviderBoundaryObserverStatus{
		OperatorSessionID: "sess-obs-1",
		OllamaEnabled:     true,
		Platform:          "windows",
	}
	outcome, err := RestartOllamaViaObserver(t.Context(), observer, dispatcher, "run-1", func(prefix string) string { return prefix + "-id" })
	require.NoError(t, err)
	assert.True(t, outcome.Performed)
	assert.Equal(t, 5, outcome.CommandCount)
	require.Len(t, dispatcher.requests, 5)
	assert.Equal(t, "cmd.exe /C %SystemRoot%/System32/taskkill.exe /IM ollama.exe /F", dispatcher.requests[0].Command)
	assert.Equal(t, "cmd.exe /C %SystemRoot%/System32/timeout.exe /t 3 /nobreak", dispatcher.requests[1].Command)
	assert.Equal(t, operatorcapability.OllamaWindowsStartCommand, dispatcher.requests[2].Command)
	assert.Equal(t, operatorcapability.OllamaRestartReadySettleCommand("windows"), dispatcher.requests[3].Command)
	assert.Equal(t, operatorcapability.OllamaServiceCommandPS, dispatcher.requests[4].Command)
}

func operatorv1CompletedCommandResult() *operatorv1.CommandResult {
	return &operatorv1.CommandResult{Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED}
}
