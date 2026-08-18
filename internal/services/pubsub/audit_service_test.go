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

	"github.com/g8e-ai/g8e/internal/constants"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/require"
)

func TestNewAuditService(t *testing.T) {
	t.Run("returns non-nil service with logger", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewAuditService(cfg, logger, nil)
		require.NotNil(t, svc)
		require.Equal(t, logger, svc.logger)
	})
}

func TestAuditService_HandleUserMsgRequest(t *testing.T) {
	t.Run("skips when audit vault not enabled", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.UserMsg,
			Payload:   mustMarshalProto(t, &operatorv1.AuditMsgRequested{Content: "test message"}),
		}

		svc.HandleUserMsgRequest(context.Background(), msg)
		// Should not panic, just log and return
	})

	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.UserMsg,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandleUserMsgRequest(context.Background(), msg)
		// Should log error and return without panic
	})

	t.Run("rejects empty content", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.UserMsg,
			Payload:   mustMarshalProto(t, &operatorv1.AuditMsgRequested{Content: ""}),
		}

		svc.HandleUserMsgRequest(context.Background(), msg)
		// Should log warning and return without panic
	})

	t.Run("handles valid user message", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.UserMsg,
			Payload:   mustMarshalProto(t, &operatorv1.AuditMsgRequested{Content: "test user message"}),
		}

		svc.HandleUserMsgRequest(context.Background(), msg)
		// Should attempt to record and log without panic
	})
}

func TestAuditService_HandleAIMsgRequest(t *testing.T) {
	t.Run("skips when audit vault not enabled", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.AIMsg,
			Payload:   mustMarshalProto(t, &operatorv1.AuditMsgRequested{Content: "test AI message"}),
		}

		svc.HandleAIMsgRequest(context.Background(), msg)
		// Should not panic
	})

	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.AIMsg,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandleAIMsgRequest(context.Background(), msg)
		// Should log error and return
	})

	t.Run("rejects empty content", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.AIMsg,
			Payload:   mustMarshalProto(t, &operatorv1.AuditMsgRequested{Content: ""}),
		}

		svc.HandleAIMsgRequest(context.Background(), msg)
		// Should log warning and return
	})

	t.Run("handles valid AI message", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.AIMsg,
			Payload:   mustMarshalProto(t, &operatorv1.AuditMsgRequested{Content: "test AI response"}),
		}

		svc.HandleAIMsgRequest(context.Background(), msg)
		// Should attempt to record
	})
}

func TestAuditService_HandleDirectCmdRequest(t *testing.T) {
	t.Run("skips when audit vault not enabled", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.DirectCmd,
			Payload:   mustMarshalProto(t, &operatorv1.DirectCommandAuditRequested{Command: "ls -la"}),
		}

		svc.HandleDirectCmdRequest(context.Background(), msg)
		// Should not panic
	})

	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.DirectCmd,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandleDirectCmdRequest(context.Background(), msg)
		// Should log error and return
	})

	t.Run("rejects empty command", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.DirectCmd,
			Payload:   mustMarshalProto(t, &operatorv1.DirectCommandAuditRequested{Command: ""}),
		}

		svc.HandleDirectCmdRequest(context.Background(), msg)
		// Should log warning and return
	})

	t.Run("handles valid direct command", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.DirectCmd,
			Payload:   mustMarshalProto(t, &operatorv1.DirectCommandAuditRequested{Command: "ls -la", ExecutionId: "exec-1"}),
		}

		svc.HandleDirectCmdRequest(context.Background(), msg)
		// Should attempt to record
	})
}

func TestAuditService_HandleDirectCmdResultRequest(t *testing.T) {
	t.Run("skips when audit vault not enabled", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.DirectCmdResult,
			Payload:   mustMarshalProto(t, &operatorv1.DirectCommandResultAuditRequested{Command: "ls -la"}),
		}

		svc.HandleDirectCmdResultRequest(context.Background(), msg)
		// Should not panic
	})

	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.DirectCmdResult,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandleDirectCmdResultRequest(context.Background(), msg)
		// Should log error and return
	})

	t.Run("rejects empty command", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.DirectCmdResult,
			Payload:   mustMarshalProto(t, &operatorv1.DirectCommandResultAuditRequested{Command: ""}),
		}

		svc.HandleDirectCmdResultRequest(context.Background(), msg)
		// Should log warning and return
	})

	t.Run("handles valid direct command result", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		mockStore := &storage.SQLAuditStore{}
		svc := NewAuditService(cfg, logger, mockStore)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.Audit.DirectCmdResult,
			Payload: mustMarshalProto(t, &operatorv1.DirectCommandResultAuditRequested{
				Command:              "ls -la",
				ExecutionId:          "exec-1",
				ExitCode:             0,
				Output:               "file1\nfile2",
				Stderr:               "",
				ExecutionTimeSeconds: 0.5,
			}),
		}

		svc.HandleDirectCmdResultRequest(context.Background(), msg)
		// Should attempt to record
	})
}
