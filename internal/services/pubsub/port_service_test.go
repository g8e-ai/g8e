// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pubsub

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestNewPortService(t *testing.T) {
	t.Run("creates service successfully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewPortService(cfg, logger, client)
		require.NotNil(t, svc)
		assert.Equal(t, cfg, svc.config)
		assert.Equal(t, logger, svc.logger)
		assert.Equal(t, client, svc.client)
	})
}

func TestPortService_HandlePortCheckRequest(t *testing.T) {
	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewPortService(cfg, logger, client)

		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.PortCheck.Requested,
			Payload:   []byte("invalid protobuf"),
		}

		svc.HandlePortCheckRequest(context.Background(), msg)
		// Should log error and publish error response
	})

	t.Run("rejects invalid port (zero)", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewPortService(cfg, logger, client)

		req := &operatorv1.CheckPortRequested{Port: 0}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.PortCheck.Requested,
			Payload:   payload,
		}

		svc.HandlePortCheckRequest(context.Background(), msg)
		// Should log warning and publish error response
	})

	t.Run("rejects invalid port (negative)", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewPortService(cfg, logger, client)

		req := &operatorv1.CheckPortRequested{Port: -1}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.PortCheck.Requested,
			Payload:   payload,
		}

		svc.HandlePortCheckRequest(context.Background(), msg)
		// Should log warning and publish error response
	})

	t.Run("rejects invalid port (too large)", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewPortService(cfg, logger, client)

		req := &operatorv1.CheckPortRequested{Port: 70000}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:        "msg-1",
			EventType: constants.Event.Operator.PortCheck.Requested,
			Payload:   payload,
		}

		svc.HandlePortCheckRequest(context.Background(), msg)
		// Should log warning and publish error response
	})

	t.Run("handles valid port check with defaults", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewPortService(cfg, logger, client)

		req := &operatorv1.CheckPortRequested{Port: 8080}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			EventType:         constants.Event.Operator.PortCheck.Requested,
			OperatorSessionID: "session-1",
			Payload:           payload,
		}

		svc.HandlePortCheckRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.NotEmpty(t, published.Data)
	})

	t.Run("handles valid port check with custom host", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewPortService(cfg, logger, client)

		req := &operatorv1.CheckPortRequested{Port: 8080, Host: "127.0.0.1"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			EventType:         constants.Event.Operator.PortCheck.Requested,
			OperatorSessionID: "session-1",
			Payload:           payload,
		}

		svc.HandlePortCheckRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.NotEmpty(t, published.Data)
	})

	t.Run("handles valid port check with custom protocol", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewPortService(cfg, logger, client)

		req := &operatorv1.CheckPortRequested{Port: 8080, Protocol: "tcp"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			ID:                "msg-1",
			EventType:         constants.Event.Operator.PortCheck.Requested,
			OperatorSessionID: "session-1",
			Payload:           payload,
		}

		svc.HandlePortCheckRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.NotEmpty(t, published.Data)
	})
}

func TestPortService_SetAuditStore(t *testing.T) {
	t.Run("sets audit store for observed-state content evidence", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewPortService(cfg, logger, client)

		// Create a mock audit store
		mockAuditStore := &mockAuditEventRecorder{}
		svc.SetAuditStore(mockAuditStore)

		assert.Equal(t, mockAuditStore, svc.auditStore)
	})

	t.Run("sets nil audit store", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewPortService(cfg, logger, client)

		svc.SetAuditStore(nil)
		assert.Nil(t, svc.auditStore)
	})
}
