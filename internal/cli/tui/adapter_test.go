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

func TestParseStage(t *testing.T) {
	tests := []struct {
		input string
		want  PipelineStage
	}{
		{"L1", StageL1},
		{"L2", StageL2},
		{"L3", StageL3},
		{"L4", StageL4},
		{"L5", StageL5},
		{"l1", StageL1},
		{"l3", StageL3},
		{"unknown", StageL1},
		{"", StageL1},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, parseStage(tt.input))
		})
	}
}

func TestParseStatus(t *testing.T) {
	tests := []struct {
		input string
		want  PipelineStatus
	}{
		{"active", StatusActive},
		{"processing", StatusActive},
		{"waiting", StatusWaiting},
		{"passed", StatusPassed},
		{"ok", StatusPassed},
		{"failed", StatusFailed},
		{"blocked", StatusFailed},
		{"idle", StatusIdle},
		{"unknown", StatusIdle},
		{"", StatusIdle},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, parseStatus(tt.input))
		})
	}
}

func TestTranslateSSEEvent(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	originalTimeNow := timeNow
	t.Cleanup(func() { timeNow = originalTimeNow })
	timeNow = func() time.Time { return fixedTime }

	t.Run("pipeline.advance maps to PipelineMsg active", func(t *testing.T) {
		data := `{"type":"pipeline.advance","data":{"stage":"L1","status":"active","tx_id":"tx-001","detail":"doctrine check"}}`
		msgs := translateSSEEvents("pipeline.advance", data)
		msg := msgs[0]
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok, "expected PipelineMsg, got %T", msg)
		assert.Equal(t, StageL1, pm.Stage)
		assert.Equal(t, StatusActive, pm.Status)
		assert.Equal(t, "tx-001", pm.TxID)
		assert.Equal(t, "doctrine check", pm.Detail)
	})

	t.Run("pipeline.waiting maps to PipelineMsg waiting", func(t *testing.T) {
		data := `{"type":"pipeline.waiting","data":{"stage":"L3","status":"waiting","detail":"FIDO2 touch required"}}`
		msgs := translateSSEEvents("pipeline.waiting", data)
		msg := msgs[0]
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL3, pm.Stage)
		assert.Equal(t, StatusWaiting, pm.Status)
		assert.Equal(t, "FIDO2 touch required", pm.Detail)
	})

	t.Run("pipeline.failed maps to PipelineMsg failed", func(t *testing.T) {
		data := `{"type":"pipeline.failed","data":{"stage":"L1","status":"failed","detail":"PII EGRESS BLOCKED"}}`
		msgs := translateSSEEvents("pipeline.failed", data)
		msg := msgs[0]
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL1, pm.Stage)
		assert.Equal(t, StatusFailed, pm.Status)
		assert.Equal(t, "PII EGRESS BLOCKED", pm.Detail)
	})

	t.Run("ledger.entry maps to LedgerMsg with level", func(t *testing.T) {
		data := `{"type":"ledger.entry","data":{"level":"critical","message":"PII EGRESS BLOCKED"}}`
		msgs := translateSSEEvents("ledger.entry", data)
		msg := msgs[0]
		lm, ok := msg.(LedgerMsg)
		require.True(t, ok)
		assert.Equal(t, LevelCritical, lm.Level)
		assert.Equal(t, "PII EGRESS BLOCKED", lm.Message)
		assert.Equal(t, fixedTime, lm.Time)
	})

	t.Run("ledger.entry maps warn level", func(t *testing.T) {
		data := `{"type":"ledger.entry","data":{"level":"warn","message":"approaching threshold"}}`
		msgs := translateSSEEvents("ledger.entry", data)
		msg := msgs[0]
		lm, ok := msg.(LedgerMsg)
		require.True(t, ok)
		assert.Equal(t, LevelWarn, lm.Level)
	})

	t.Run("consensus.vote maps to ConsensusMsg", func(t *testing.T) {
		data := `{"type":"consensus.vote","data":{"member":"axiom","decision":true,"signed":true,"quorum":3,"total":5}}`
		msgs := translateSSEEvents("consensus.vote", data)
		msg := msgs[0]
		cm, ok := msg.(ConsensusMsg)
		require.True(t, ok)
		assert.Equal(t, constants.ConsensusMemberAxiom, cm.Member)
		assert.True(t, cm.Decision)
		assert.True(t, cm.Signed)
		assert.Equal(t, 3, cm.Quorum)
		assert.Equal(t, 5, cm.Total)
		assert.Equal(t, ConsensusPending, cm.Result)
	})

	t.Run("consensus.result maps to ConsensusMsg with result", func(t *testing.T) {
		data := `{"type":"consensus.result","data":{"result":"rejected","hash":"abcdef1234567890"}}`
		msgs := translateSSEEvents("consensus.result", data)
		msg := msgs[0]
		cm, ok := msg.(ConsensusMsg)
		require.True(t, ok)
		assert.Equal(t, ConsensusRejected, cm.Result)
		assert.Equal(t, "abcdef1234567890", cm.Hash)
	})

	t.Run("consensus.result reached", func(t *testing.T) {
		data := `{"type":"consensus.result","data":{"result":"reached","hash":"abc123"}}`
		msgs := translateSSEEvents("consensus.result", data)
		msg := msgs[0]
		cm, ok := msg.(ConsensusMsg)
		require.True(t, ok)
		assert.Equal(t, ConsensusReached, cm.Result)
	})

	t.Run("unknown event type falls back to LedgerMsg", func(t *testing.T) {
		data := `{"type":"system.heartbeat","data":{"status":"ok"}}`
		msgs := translateSSEEvents("system.heartbeat", data)
		msg := msgs[0]
		lm, ok := msg.(LedgerMsg)
		require.True(t, ok)
		assert.Contains(t, lm.Message, "system.heartbeat")
	})

	t.Run("non-JSON data falls back to LedgerMsg with raw text", func(t *testing.T) {
		msgs := translateSSEEvents("unknown", "plain text message")
		msg := msgs[0]
		lm, ok := msg.(LedgerMsg)
		require.True(t, ok)
		assert.Equal(t, "plain text message", lm.Message)
		assert.Equal(t, LevelInfo, lm.Level)
	})

	t.Run("uses event type from SSE header when payload type is empty", func(t *testing.T) {
		data := `{"type":"","data":{"stage":"L2","status":"active"}}`
		msgs := translateSSEEvents("pipeline.advance", data)
		msg := msgs[0]
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL2, pm.Stage)
		assert.Equal(t, StatusActive, pm.Status)
	})

	t.Run("R14: extracts type from SSEPushPayload envelope when eventType is empty", func(t *testing.T) {
		// When the server omits the event: field (R14), eventType is empty
		// and the top-level JSON has no "type" field. The data is a
		// SSEPushPayload envelope wrapping the inner event JSON. The adapter
		// must extract the type from the inner event.
		innerEvent := `{"type":"pipeline.advance","data":{"stage":"L3","status":"waiting","detail":"FIDO2 touch"}}`
		envelope := models.SSEPushPayload{
			CliSessionID: "cli-123",
			Event:        json.RawMessage(innerEvent),
		}
		envelopeJSON, err := json.Marshal(envelope)
		require.NoError(t, err)

		msgs := translateSSEEvents("", string(envelopeJSON))
		msg := msgs[0]
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok, "expected PipelineMsg from SSEPushPayload envelope, got %T", msg)
		assert.Equal(t, StageL3, pm.Stage)
		assert.Equal(t, StatusWaiting, pm.Status)
		assert.Equal(t, "FIDO2 touch", pm.Detail)
	})

	t.Run("R14: extracts consensus type from SSEPushPayload envelope when eventType is empty", func(t *testing.T) {
		innerEvent := `{"type":"consensus.vote","data":{"member":"axiom","decision":true,"signed":true,"quorum":3,"total":5}}`
		envelope := models.SSEPushPayload{
			UserID: "user-123",
			Event:  json.RawMessage(innerEvent),
		}
		envelopeJSON, err := json.Marshal(envelope)
		require.NoError(t, err)

		msgs := translateSSEEvents("", string(envelopeJSON))
		msg := msgs[0]
		cm, ok := msg.(ConsensusMsg)
		require.True(t, ok, "expected ConsensusMsg from SSEPushPayload envelope, got %T", msg)
		assert.Equal(t, constants.ConsensusMemberAxiom, cm.Member)
		assert.True(t, cm.Decision)
	})
}

func TestParsePipelineEvent(t *testing.T) {
	t.Run("parses all fields", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]string{
			"stage":  "L4",
			"status": "passed",
			"tx_id":  "tx-xyz",
			"detail": "warden verified",
		})
		msg := parsePipelineEvent(payload)
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL4, pm.Stage)
		assert.Equal(t, StatusPassed, pm.Status)
		assert.Equal(t, "tx-xyz", pm.TxID)
		assert.Equal(t, "warden verified", pm.Detail)
	})

	t.Run("defaults to idle for unknown status", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]string{
			"stage":  "L1",
			"status": "bogus",
		})
		msg := parsePipelineEvent(payload)
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StatusIdle, pm.Status)
	})
}

func TestParseLedgerEvent(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	originalTimeNow := timeNow
	t.Cleanup(func() { timeNow = originalTimeNow })
	timeNow = func() time.Time { return fixedTime }

	tests := []struct {
		name      string
		level     string
		wantLevel LedgerLevel
	}{
		{"info", "info", LevelInfo},
		{"warn", "warn", LevelWarn},
		{"warning", "warning", LevelWarn},
		{"crit", "crit", LevelCritical},
		{"critical", "critical", LevelCritical},
		{"unknown defaults to info", "bogus", LevelInfo},
		{"empty defaults to info", "", LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]string{
				"level":   tt.level,
				"message": "test message",
			})
			msg := parseLedgerEvent(payload)
			lm, ok := msg.(LedgerMsg)
			require.True(t, ok)
			assert.Equal(t, tt.wantLevel, lm.Level)
			assert.Equal(t, "test message", lm.Message)
			assert.Equal(t, fixedTime, lm.Time)
		})
	}
}

func TestParseConsensusEvent(t *testing.T) {
	t.Run("parses vote with all fields", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]interface{}{
			"member":   "nemesis",
			"decision": false,
			"signed":   true,
			"quorum":   3,
			"total":    5,
			"result":   "rejected",
			"hash":     "deadbeef",
		})
		msg := parseConsensusEvent(payload)
		cm, ok := msg.(ConsensusMsg)
		require.True(t, ok)
		assert.Equal(t, constants.ConsensusMemberNemesis, cm.Member)
		assert.False(t, cm.Decision)
		assert.True(t, cm.Signed)
		assert.Equal(t, 3, cm.Quorum)
		assert.Equal(t, 5, cm.Total)
		assert.Equal(t, ConsensusRejected, cm.Result)
		assert.Equal(t, "deadbeef", cm.Hash)
	})

	t.Run("approved result maps to reached", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]interface{}{
			"result": "approved",
		})
		msg := parseConsensusEvent(payload)
		cm, ok := msg.(ConsensusMsg)
		require.True(t, ok)
		assert.Equal(t, ConsensusReached, cm.Result)
	})

	t.Run("unknown result defaults to pending", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]interface{}{
			"result": "bogus",
		})
		msg := parseConsensusEvent(payload)
		cm, ok := msg.(ConsensusMsg)
		require.True(t, ok)
		assert.Equal(t, ConsensusPending, cm.Result)
	})
}

func TestAdapterRunNilSession(t *testing.T) {
	a := NewAdapter(nil, &mockSender{})
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
// served at url and the pending-approval list returns pendingJSON.
type testSession struct {
	url         string
	pendingJSON string
	mu          sync.Mutex
	listCalls   int
}

func (s *testSession) NewSSEClient() *sse.Client { return sse.NewClient(s.url, nil) }

func (s *testSession) DoRequestContext(_ context.Context, method, path string, _ interface{}) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if method != http.MethodGet || path != constants.APIPaths.ApprovalsCLIList {
		return nil, constants.ErrNotFound
	}
	s.listCalls++
	if s.pendingJSON == "" {
		return []byte(`{"transactions":[]}`), nil
	}
	return []byte(s.pendingJSON), nil
}

func (s *testSession) pendingListCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listCalls
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
		require.Len(t, msgs, 2)
		pm, ok := msgs[0].(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL3, pm.Stage)
		assert.Equal(t, StatusPassed, pm.Status)
		assert.Equal(t, "tx-999", pm.TxID)
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
		require.Len(t, msgs, 2)
		pm, ok := msgs[0].(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StatusPassed, pm.Status)
		assert.Equal(t, "tx-abc", pm.TxID)
	})

	t.Run("transaction approvals.changed produces no message", func(t *testing.T) {
		frame := gatewayFrame(t, string(constants.EventPlatformApprovalsChanged), models.ApprovalsChangedPayload{Subject: models.ApprovalsChangedTransactions})
		assert.Empty(t, translateSSEEvents("", frame))
	})

	t.Run("enrollment approvals.changed points at the CLI command", func(t *testing.T) {
		frame := gatewayFrame(t, string(constants.EventPlatformApprovalsChanged), models.ApprovalsChangedPayload{Subject: models.ApprovalsChangedEnrollments})
		msgs := translateSSEEvents("", frame)
		require.Len(t, msgs, 1)
		lm, ok := msgs[0].(LedgerMsg)
		require.True(t, ok)
		assert.Contains(t, lm.Message, "g8e auth enroll pending")
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
