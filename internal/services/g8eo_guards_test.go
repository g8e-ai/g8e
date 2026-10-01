// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmanagerv1 "github.com/g8e-ai/g8e/v2/internal/adapters/lattice/gen/anduril/taskmanager/v1"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// capturingLogger returns a logger whose JSON records can be inspected, so
// tests assert on what the service actually reported.
func capturingLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func decodeLogRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	dec := json.NewDecoder(buf)
	for dec.More() {
		var record map[string]any
		require.NoError(t, dec.Decode(&record))
		records = append(records, record)
	}
	return records
}

func TestG8eoService_LatticeTaskHandler_AcknowledgesTaskWithoutExecutingAnything(t *testing.T) {
	logger, buf := capturingLogger()
	service, err := NewG8eoService(testutil.NewTestConfig(t), logger, newTestTLSConfig(t), newTestFileSvc(t), newTestPubSubClientFactory())
	require.NoError(t, err)
	task := &taskmanagerv1.Task{Version: &taskmanagerv1.TaskVersion{TaskId: "task-lattice-7"}}

	err = service.latticeTaskHandler(t.Context(), task)

	require.NoError(t, err, "the task handler only acknowledges receipt")
	var found bool
	for _, record := range decodeLogRecords(t, buf) {
		if record["msg"] == "Lattice task received" {
			found = true
			assert.Equal(t, "task-lattice-7", record["task_id"])
		}
	}
	assert.True(t, found, "receipt of a Lattice task must be logged with its task id")
}

func TestG8eoService_LatticeTaskHandler_ToleratesTasksWithoutVersion(t *testing.T) {
	tests := []struct {
		name string
		task *taskmanagerv1.Task
	}{
		{"nil task", nil},
		{"task without version", &taskmanagerv1.Task{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := capturingLogger()
			service, err := NewG8eoService(testutil.NewTestConfig(t), logger, newTestTLSConfig(t), newTestFileSvc(t), newTestPubSubClientFactory())
			require.NoError(t, err)

			require.NoError(t, service.latticeTaskHandler(t.Context(), tt.task))

			for _, record := range decodeLogRecords(t, buf) {
				if record["msg"] == "Lattice task received" {
					assert.Equal(t, "", record["task_id"])
				}
			}
		})
	}
}

func TestG8eoService_Start_RejectsSecondStartWithoutDisturbingRunningService(t *testing.T) {
	service, err := NewG8eoService(testutil.NewTestConfig(t), testutil.NewTestLogger(), newTestTLSConfig(t), newTestFileSvc(t), newTestPubSubClientFactory())
	require.NoError(t, err)
	service.running = true

	err = service.Start(t.Context())

	require.ErrorIs(t, err, constants.ErrServiceUnavailable)
	assert.True(t, service.running, "a rejected second start must leave the service running")
	assert.Nil(t, service.cancel, "a rejected start must not create a new service context")
	assert.Nil(t, service.pubSubClient, "a rejected start must not open a second pub/sub client")
}

func TestG8eoService_Start_RequiresRuntimeFileService(t *testing.T) {
	service, err := NewG8eoService(testutil.NewTestConfig(t), testutil.NewTestLogger(), newTestTLSConfig(t), nil, newTestPubSubClientFactory())
	require.NoError(t, err)

	err = service.Start(t.Context())

	require.ErrorIs(t, err, constants.ErrInternal)
	assert.Contains(t, err.Error(), "fileSvc must be provided")
	assert.False(t, service.running)
	assert.Nil(t, service.pubSubClient, "nothing may be connected before the file service is validated")
	if service.cancel != nil {
		service.cancel()
	}
}

func TestG8eoService_Stop_NeverStartedServiceIsANoOp(t *testing.T) {
	logger, buf := capturingLogger()
	service, err := NewG8eoService(testutil.NewTestConfig(t), logger, newTestTLSConfig(t), newTestFileSvc(t), newTestPubSubClientFactory())
	require.NoError(t, err)

	require.NoError(t, service.Stop(t.Context()))

	assert.False(t, service.running)
	for _, record := range decodeLogRecords(t, buf) {
		assert.NotEqual(t, "g8e Operator shutting down...", record["msg"], "stopping a service that never started must not run the shutdown sequence")
	}
}

func TestG8eoService_Stop_CancelsServiceContextAndMarksStopped(t *testing.T) {
	service, err := NewG8eoService(testutil.NewTestConfig(t), testutil.NewTestLogger(), newTestTLSConfig(t), newTestFileSvc(t), newTestPubSubClientFactory())
	require.NoError(t, err)
	service.ctx, service.cancel = context.WithCancel(t.Context())
	service.running = true

	require.NoError(t, service.Stop(t.Context()))

	assert.False(t, service.running)
	assert.Error(t, service.ctx.Err(), "stop must cancel the service context so background work exits")
	require.NoError(t, service.Stop(t.Context()), "a second stop is a no-op")
}
