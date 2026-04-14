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
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPortService(t *testing.T) (*PortService, *MockG8esPubSubClient) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockG8esPubSubClient()
	t.Cleanup(func() { db.Close() })
	return NewPortService(cfg, logger, db), db
}

// ---------------------------------------------------------------------------
// NewPortService
// ---------------------------------------------------------------------------

func TestNewPortService_CreatesService(t *testing.T) {
	ps, _ := newTestPortService(t)
	require.NotNil(t, ps)
	assert.NotNil(t, ps.config)
	assert.NotNil(t, ps.logger)
	assert.NotNil(t, ps.client)
}

// ---------------------------------------------------------------------------
// HandlePortCheckRequest — invalid payload
// ---------------------------------------------------------------------------

func TestPortService_HandlePortCheckRequest_InvalidPayload(t *testing.T) {
	ps, db := newTestPortService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-port-1",
		CaseID:  "case-1",
		Payload: []byte(`{invalid}`),
	}
	ps.HandlePortCheckRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published, "error response must be published for invalid payload")
	assert.Contains(t, string(published.Data), constants.Event.Operator.PortCheck.Failed)
}

// ---------------------------------------------------------------------------
// HandlePortCheckRequest — port out of range
// ---------------------------------------------------------------------------

func TestPortService_HandlePortCheckRequest_InvalidPort_Zero(t *testing.T) {
	ps, db := newTestPortService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-port-2",
		CaseID:  "case-1",
		Payload: mustMarshalJSON(t, models.PortCheckRequestPayload{Port: 0}),
	}
	ps.HandlePortCheckRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.PortCheck.Failed)
}

func TestPortService_HandlePortCheckRequest_InvalidPort_TooHigh(t *testing.T) {
	ps, db := newTestPortService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-port-3",
		CaseID:  "case-1",
		Payload: mustMarshalJSON(t, models.PortCheckRequestPayload{Port: 65536}),
	}
	ps.HandlePortCheckRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.PortCheck.Failed)
}

// ---------------------------------------------------------------------------
// HandlePortCheckRequest — closed port returns completed (not error)
// ---------------------------------------------------------------------------

func TestPortService_HandlePortCheckRequest_ClosedPort_ReturnsCompleted(t *testing.T) {
	ps, db := newTestPortService(t)

	msg := PubSubCommandMessage{
		ID:     "msg-port-4",
		CaseID: "case-1",
		Payload: mustMarshalJSON(t, models.PortCheckRequestPayload{
			Host:     "127.0.0.1",
			Port:     19998,
			Protocol: "tcp",
		}),
	}
	ps.HandlePortCheckRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.PortCheck.Completed)

	var wireMsg struct {
		Payload json.RawMessage `json:"payload"`
	}
	require.NoError(t, json.Unmarshal(published.Data, &wireMsg))

	var result models.PortCheckResultPayload
	require.NoError(t, json.Unmarshal(wireMsg.Payload, &result))
	require.Len(t, result.Results, 1)
	assert.False(t, result.Results[0].Open)
}

// ---------------------------------------------------------------------------
// HandlePortCheckRequest — open port returns completed with latency
// ---------------------------------------------------------------------------

func TestPortService_HandlePortCheckRequest_OpenPort_ReturnsLatency(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			time.Sleep(10 * time.Millisecond)
			conn.Close()
		}
	}()

	ps, db := newTestPortService(t)

	msg := PubSubCommandMessage{
		ID:     "msg-port-5",
		CaseID: "case-1",
		Payload: mustMarshalJSON(t, models.PortCheckRequestPayload{
			Host:     "127.0.0.1",
			Port:     port,
			Protocol: "tcp",
		}),
	}
	ps.HandlePortCheckRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.PortCheck.Completed)

	var wireMsg struct {
		Payload json.RawMessage `json:"payload"`
	}
	require.NoError(t, json.Unmarshal(published.Data, &wireMsg))

	var result models.PortCheckResultPayload
	require.NoError(t, json.Unmarshal(wireMsg.Payload, &result))
	require.Len(t, result.Results, 1)
	assert.True(t, result.Results[0].Open)
	assert.NotNil(t, result.Results[0].LatencyMs)
}

// ---------------------------------------------------------------------------
// HandlePortCheckRequest — default host and protocol
// ---------------------------------------------------------------------------

func TestPortService_HandlePortCheckRequest_DefaultsHostAndProtocol(t *testing.T) {
	ps, db := newTestPortService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-port-6",
		CaseID:  "case-1",
		Payload: mustMarshalJSON(t, models.PortCheckRequestPayload{Port: 19997}),
	}
	ps.HandlePortCheckRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.PortCheck.Completed)
}

// ---------------------------------------------------------------------------
// HandlePortCheckRequest — executionID from payload overrides msg.ID
// ---------------------------------------------------------------------------

func TestPortService_HandlePortCheckRequest_ExecutionIDOverridesMsgID(t *testing.T) {
	ps, db := newTestPortService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-id-original",
		CaseID:  "case-1",
		Payload: mustMarshalJSON(t, models.PortCheckRequestPayload{Port: 19996, ExecutionID: "exec-override"}),
	}
	ps.HandlePortCheckRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), "exec-override")
}
