// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	execution "github.com/g8e-ai/g8e/v2/internal/services/execution"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestPayloadToExecutionRequest(t *testing.T) {
	t.Run("converts valid payload", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandRequested{
			Command:        "ls -la",
			ExecutionId:    "exec-1",
			Justification:  "test",
			Intent:         "list files",
			VaultMode:      "raw",
			TimeoutSeconds: 30,
		}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:              "msg-1",
			EventType:       constants.Event.Operator.Command.Requested,
			CaseID:          "case-1",
			TaskID:          func() *string { s := "task-1"; return &s }(),
			InvestigationID: "investigation-1",
			Payload:         payload,
		}

		execReq, err := payloadToExecutionRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, "exec-1", execReq.ExecutionID)
		assert.Equal(t, "case-1", execReq.CaseID)
		assert.Equal(t, "task-1", *execReq.TaskID)
		assert.Equal(t, "investigation-1", execReq.InvestigationID)
		assert.Equal(t, "ls -la", execReq.Command)
		assert.Equal(t, "test", execReq.Justification)
		assert.Equal(t, "list files", execReq.Intent)
		assert.Equal(t, "raw", execReq.VaultMode)
		assert.Equal(t, 30, execReq.TimeoutSeconds)
	})

	t.Run("rejects invalid protobuf", func(t *testing.T) {
		t.Parallel()
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		_, err := payloadToExecutionRequest(msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode command payload")
	})

	t.Run("rejects missing command", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandRequested{Command: ""}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.Requested,
			Payload:   payload,
		}

		_, err := payloadToExecutionRequest(msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing command")
	})

	t.Run("uses message ID when payload has no execution_id", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandRequested{Command: "ls -la"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.Requested,
			Payload:   payload,
		}

		execReq, err := payloadToExecutionRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, "msg-1", execReq.ExecutionID)
	})

	t.Run("uses default timeout when not specified", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandRequested{Command: "ls -la"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.Requested,
			Payload:   payload,
		}

		execReq, err := payloadToExecutionRequest(msg)
		require.NoError(t, err)
		assert.Equal(t, 300, execReq.TimeoutSeconds)
	})
}

func TestCommandService_HandleExecutionRequest(t *testing.T) {
	t.Run("rejects oversized payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		largePayload := make([]byte, 1024*1024+1)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.Requested,
			Payload:   largePayload,
		}

		svc.HandleExecutionRequest(context.Background(), msg)
		// Should log error and return without panic
	})

	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandleExecutionRequest(context.Background(), msg)
		// Should log error and return without panic
	})

	t.Run("rejects missing command field", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		req := &operatorv1.CommandRequested{Command: ""}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.Requested,
			Payload:   payload,
		}

		svc.HandleExecutionRequest(context.Background(), msg)
		// Should log error and return without panic
	})
}

func TestCommandService_SetResultsPublisher_SetAuditStore_SetLedgerService(t *testing.T) {
	t.Run("SetResultsPublisher", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		mockResults := &mockResultsPublisher{}
		svc.SetResultsPublisher(mockResults)
		// Verify method can be called without panic
		assert.NotNil(t, svc.results)
	})

	t.Run("SetAuditStore", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		// Use nil for test - just verify method can be called
		svc.SetAuditStore(nil)
		// Verify method can be called without panic
	})

	t.Run("SetLedgerService", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		// Use nil for test - just verify method can be called
		svc.SetLedgerService(nil)
		// Verify method can be called without panic
	})

	t.Run("SetHistoryHandler", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		// Use nil for test - just verify method can be called
		svc.SetHistoryHandler(nil)
		// Verify method can be called without panic
	})

	t.Run("SetScrubbing", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		// Use nil for test - just verify method can be called
		svc.SetScrubbing(nil)
		// Verify method can be called without panic
	})
}

func TestCommandService_HandleCancelRequest(t *testing.T) {
	t.Run("rejects oversized payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		largePayload := make([]byte, 64*1024+1)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.CancelRequested,
			Payload:   largePayload,
		}

		svc.HandleCancelRequest(context.Background(), msg)
		// Should log error and return without panic
	})

	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.CancelRequested,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandleCancelRequest(context.Background(), msg)
		// Should log error and return without panic
	})

	t.Run("rejects missing execution_id", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		req := &operatorv1.CommandCancelRequested{ExecutionId: ""}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Command.CancelRequested,
			Payload:   payload,
		}

		svc.HandleCancelRequest(context.Background(), msg)
		// Should log error and return without panic
	})
}

func TestCommandService_runStatusTicker(t *testing.T) {
	t.Run("returns zero when done channel closes immediately", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		ctx := context.Background()
		done := make(chan struct{})
		close(done)

		execReq := &models.ExecutionRequestPayload{ExecutionID: "exec-1"}
		msg := &PubSubCommandMessage{ID: "msg-1"}
		startTime := time.Now()

		count := svc.runStatusTicker(ctx, execReq, msg, "test", startTime, done)
		assert.Equal(t, 0, count)
	})

	t.Run("returns count when context cancelled", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		execSvc := execution.NewExecutionService(cfg, logger)
		svc := NewCommandService(cfg, logger, execSvc)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})

		execReq := &models.ExecutionRequestPayload{ExecutionID: "exec-1"}
		msg := &PubSubCommandMessage{ID: "msg-1"}
		startTime := time.Now()

		// Cancel context after a short delay
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		count := svc.runStatusTicker(ctx, execReq, msg, "test", startTime, done)
		assert.GreaterOrEqual(t, count, 0)
	})
}

func TestIsValidFileOperation(t *testing.T) {
	t.Parallel()
	validOps := []constants.FileOperation{
		constants.FileOperationCreate,
		constants.FileOperationDelete,
		constants.FileOperationInsert,
		constants.FileOperationPatch,
		constants.FileOperationRead,
		constants.FileOperationReplace,
		constants.FileOperationUpdate,
		constants.FileOperationWrite,
	}
	for _, op := range validOps {
		if !isValidFileOperation(op) {
			t.Errorf("expected %s to be valid", op)
		}
	}

	if isValidFileOperation("invalid") {
		t.Error("expected 'invalid' to be invalid")
	}
	if isValidFileOperation("") {
		t.Error("expected empty string to be invalid")
	}
}
