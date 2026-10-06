// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/sse"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestTranslateSSEEvent(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	originalTimeNow := timeNow
	t.Cleanup(func() { timeNow = originalTimeNow })
	timeNow = func() time.Time { return fixedTime }

	t.Run("unknown event type falls back to LedgerMsg", func(t *testing.T) {
		data := `{"type":"system.heartbeat","data":{"status":"ok"}}`
		msgs := translateSSEEvents("system.heartbeat", data)
		require.Len(t, msgs, 1)
		lm, ok := msgs[0].(LedgerMsg)
		require.True(t, ok)
		assert.Contains(t, lm.Message, "system.heartbeat")
		assert.Equal(t, fixedTime, lm.Time)
	})

	t.Run("event families with no Gateway producer are not special-cased", func(t *testing.T) {
		for _, eventType := range []string{"pipeline.advance", "ledger.entry", "consensus.vote"} {
			msgs := translateSSEEvents("", `{"type":"`+eventType+`","data":{}}`)
			require.Len(t, msgs, 1, eventType)
			_, ok := msgs[0].(LedgerMsg)
			assert.True(t, ok, "%s should fall back to a ledger line", eventType)
		}
	})

	t.Run("non-JSON data falls back to LedgerMsg with raw text", func(t *testing.T) {
		msgs := translateSSEEvents("unknown", "plain text message")
		require.Len(t, msgs, 1)
		lm, ok := msgs[0].(LedgerMsg)
		require.True(t, ok)
		assert.Equal(t, "plain text message", lm.Message)
		assert.Equal(t, LevelInfo, lm.Level)
	})

	t.Run("uses event type from SSE header when payload type is empty", func(t *testing.T) {
		data := `{"type":"","data":{"approval_id":"app-2","file_path":"/etc/hosts"}}`
		msgs := translateSSEEvents(string(constants.EventOperatorFileEditApprovalRequested), data)
		require.Len(t, msgs, 2)
		pm, ok := msgs[0].(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL3, pm.Stage)
		assert.Contains(t, pm.Detail, "/etc/hosts")
	})

	t.Run("R14: extracts type from SSEPushPayload envelope when eventType is empty", func(t *testing.T) {
		innerEvent := `{"type":"` + string(constants.EventOperatorIntentApprovalRequested) + `","data":{"approval_id":"app-3","intent_name":"network.read"}}`
		envelopeJSON, err := json.Marshal(models.SSEPushPayload{CliSessionID: "cli-123", Event: json.RawMessage(innerEvent)})
		require.NoError(t, err)

		msgs := translateSSEEvents("", string(envelopeJSON))
		require.Len(t, msgs, 2)
		pm, ok := msgs[0].(PipelineMsg)
		require.True(t, ok, "expected PipelineMsg from SSEPushPayload envelope, got %T", msgs[0])
		assert.Equal(t, StatusWaiting, pm.Status)
		assert.Equal(t, "app-3", pm.TxID)
		assert.Contains(t, pm.Detail, "network.read")
	})
}

func TestAdapterRunNilSession(t *testing.T) {
	a := NewAdapter(nil, "", &mockSender{})
	done := make(chan struct{})
	go func() {
		a.Run(t.Context())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("adapter.Run with nil session should return immediately")
	}
}

// testSession is a Session over a plain-HTTP test server: the SSE stream is
// served at url, and Gateway requests return the configured JSON (or err).
type testSession struct {
	url           string
	pendingJSON   string
	operatorsJSON string
	healthJSON    string
	statusJSON    string
	err           error
	mu            sync.Mutex
	listCalls     int
	paths         []string

	enrollPendingJSON string
	enrolledJSON      string
	decisionJSON      string
	revokeJSON        string
	stopJSON          string
	postErr           error
	enrollListCalls   int
	posted            []interface{}
}

func (s *testSession) NewSSEClient() *sse.Client { return sse.NewClient(s.url, nil) }

func (s *testSession) DoRequestContext(_ context.Context, method, path string, body interface{}) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paths = append(s.paths, path)
	if s.err != nil {
		return nil, s.err
	}
	if method == http.MethodPost {
		s.posted = append(s.posted, body)
		if s.postErr != nil {
			return nil, s.postErr
		}
		switch path {
		case constants.APIPaths.AuthPlatformEnrollmentDecision:
			return []byte(orDefault(s.decisionJSON, `{"request_id":"req-1","state":"approved"}`)), nil
		case constants.APIPaths.AuthPlatformEnrollmentRevoke:
			return []byte(orDefault(s.revokeJSON, `{"request_id":"req-1","component_kind":"operator","state":"revoked"}`)), nil
		case constants.APIPaths.OperatorsStop:
			return []byte(orDefault(s.stopJSON, `{"success":true,"operator_id":"op-remote","operator_session_id":"sess-remote"}`)), nil
		}
		return nil, constants.ErrNotFound
	}
	if method != http.MethodGet {
		return nil, constants.ErrNotFound
	}
	switch {
	case path == constants.APIPaths.AuthPlatformEnrollmentPending:
		s.enrollListCalls++
		return []byte(orDefault(s.enrollPendingJSON, `{"requests":[]}`)), nil
	case path == constants.APIPaths.AuthPlatformEnrollmentEnrolled:
		return []byte(orDefault(s.enrolledJSON, `{"enrollments":[]}`)), nil
	case path == constants.APIPaths.ApprovalsCLIList:
		s.listCalls++
		return []byte(orDefault(s.pendingJSON, `{"transactions":[]}`)), nil
	case strings.HasPrefix(path, constants.APIPaths.Operators):
		return []byte(orDefault(s.operatorsJSON, `{"success":true,"operators":[]}`)), nil
	case path == constants.APIPaths.Health:
		return []byte(orDefault(s.healthJSON, `{"status":"ok","posture":"notary"}`)), nil
	case strings.HasPrefix(path, constants.APIPaths.ApprovalsCLIStatus):
		return []byte(orDefault(s.statusJSON, `{"status":"approved"}`)), nil
	}
	return nil, constants.ErrNotFound
}

func (s *testSession) enrollmentListCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enrollListCalls
}

func (s *testSession) pendingListCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listCalls
}

func (s *testSession) requestedPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// mockSender captures tea.Msg values sent by the adapter for test assertions.
type mockSender struct {
	mu       sync.Mutex
	messages []tea.Msg
}

func (m *mockSender) Send(msg tea.Msg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
}

func (m *mockSender) snapshot() []tea.Msg {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]tea.Msg, len(m.messages))
	copy(out, m.messages)
	return out
}

func TestTranslateSSEEvents_ChatAndApprovals(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	originalTimeNow := timeNow
	t.Cleanup(func() { timeNow = originalTimeNow })
	timeNow = func() time.Time { return fixedTime }

	t.Run("chat iteration started", func(t *testing.T) {
		msgs := translateSSEEvents(string(constants.EventAiLLMChatIterationStarted), `{"type":"g8e.v1.ai.llm.chat.iteration.started"}`)
		require.Len(t, msgs, 2)
		pm, ok := msgs[0].(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL1, pm.Stage)
		assert.Equal(t, StatusActive, pm.Status)
		lm, ok := msgs[1].(LedgerMsg)
		require.True(t, ok)
		assert.Contains(t, lm.Message, "AI chat iteration started")
	})

	t.Run("chat text chunk received", func(t *testing.T) {
		msgs := translateSSEEvents(string(constants.EventAiLLMChatIterationTextChunkReceived), `{"type":"g8e.v1.ai.llm.chat.iteration.text.chunk.received","data":{"chunk":"hello world"}}`)
		require.Len(t, msgs, 1)
		lm, ok := msgs[0].(LedgerMsg)
		require.True(t, ok)
		assert.Equal(t, "hello world", lm.Message)
	})

	t.Run("command approval requested", func(t *testing.T) {
		msgs := translateSSEEvents(string(constants.EventOperatorCommandApprovalRequested), `{"type":"g8e.v1.operator.command.approval.requested","data":{"approval_id":"app-1","command":"rm -rf /tmp/test"}}`)
		require.Len(t, msgs, 2)
		pm, ok := msgs[0].(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL3, pm.Stage)
		assert.Equal(t, StatusWaiting, pm.Status)
		assert.Equal(t, "app-1", pm.TxID)
		assert.Contains(t, pm.Detail, "rm -rf /tmp/test")
		lm, ok := msgs[1].(LedgerMsg)
		require.True(t, ok)
		assert.Contains(t, lm.Message, "APPROVAL REQUIRED")
	})

	t.Run("approval completed", func(t *testing.T) {
		msgs := translateSSEEvents(constants.SSEEventTypeApprovalCompleted, `{"type":"approval.completed","tx_hash":"tx-999"}`)
		assert.Equal(t, []tea.Msg{ApprovalCompletedMsg{TxHash: "tx-999"}}, msgs)
	})
}

// gatewayFrame builds an SSE data payload exactly as the Gateway's
// SSEEventPublisher does: SSEPushPayload{event: {type, data}}.
func gatewayFrame(t *testing.T, eventType string, data any) string {
	t.Helper()
	dataJSON, err := json.Marshal(data)
	require.NoError(t, err)
	inner, err := json.Marshal(struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}{Type: eventType, Data: dataJSON})
	require.NoError(t, err)
	frame, err := json.Marshal(models.SSEPushPayload{UserID: "user-1", CliSessionID: "cli-1", Event: inner})
	require.NoError(t, err)
	return string(frame)
}

func TestTranslateSSEEvents_GatewayWireShape(t *testing.T) {
	t.Run("publisher envelope payload is decoded from data", func(t *testing.T) {
		frame := gatewayFrame(t, string(constants.EventOperatorCommandApprovalRequested), map[string]string{"approval_id": "app-7", "command": "systemctl restart nginx"})
		msgs := translateSSEEvents("", frame)
		require.Len(t, msgs, 2)
		pm, ok := msgs[0].(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, "app-7", pm.TxID)
		assert.Contains(t, pm.Detail, "systemctl restart nginx")
	})

	t.Run("approval.completed is read from the flat event", func(t *testing.T) {
		event, err := json.Marshal(models.ApprovalCompletedEvent{Type: constants.SSEEventTypeApprovalCompleted, UserID: "user-1", TxHash: "tx-abc"})
		require.NoError(t, err)
		frame, err := json.Marshal(models.SSEPushPayload{UserID: "user-1", CliSessionID: "cli-1", Event: event})
		require.NoError(t, err)
		msgs := translateSSEEvents("", string(frame))
		assert.Equal(t, []tea.Msg{ApprovalCompletedMsg{TxHash: "tx-abc"}}, msgs)
	})

	t.Run("transaction approvals.changed produces no message", func(t *testing.T) {
		frame := gatewayFrame(t, string(constants.EventPlatformApprovalsChanged), models.ApprovalsChangedPayload{Subject: models.ApprovalsChangedTransactions})
		assert.Empty(t, translateSSEEvents("", frame))
	})

	t.Run("enrollment approvals.changed produces no message", func(t *testing.T) {
		// The adapter re-lists enrollments; the model announces new requests.
		frame := gatewayFrame(t, string(constants.EventPlatformApprovalsChanged), models.ApprovalsChangedPayload{Subject: models.ApprovalsChangedEnrollments})
		assert.Empty(t, translateSSEEvents("", frame))
	})

	t.Run("replay error sentinel surfaces as a warning", func(t *testing.T) {
		sentinel, err := json.Marshal(models.SSEErrorEvent{Type: "error", Reason: "replay_failed"})
		require.NoError(t, err)
		msgs := translateSSEEvents("", string(sentinel))
		require.Len(t, msgs, 1)
		lm, ok := msgs[0].(LedgerMsg)
		require.True(t, ok)
		assert.Equal(t, LevelWarn, lm.Level)
		assert.Contains(t, lm.Message, "replay_failed")
	})
}
