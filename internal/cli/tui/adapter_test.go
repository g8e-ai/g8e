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

package tui

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
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
		data := `{"type":"pipeline.advance","payload":{"stage":"L1","status":"active","tx_id":"tx-001","detail":"doctrine check"}}`
		msg := translateSSEEvent("pipeline.advance", data)
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok, "expected PipelineMsg, got %T", msg)
		assert.Equal(t, StageL1, pm.Stage)
		assert.Equal(t, StatusActive, pm.Status)
		assert.Equal(t, "tx-001", pm.TxID)
		assert.Equal(t, "doctrine check", pm.Detail)
	})

	t.Run("pipeline.waiting maps to PipelineMsg waiting", func(t *testing.T) {
		data := `{"type":"pipeline.waiting","payload":{"stage":"L3","status":"waiting","detail":"FIDO2 touch required"}}`
		msg := translateSSEEvent("pipeline.waiting", data)
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL3, pm.Stage)
		assert.Equal(t, StatusWaiting, pm.Status)
		assert.Equal(t, "FIDO2 touch required", pm.Detail)
	})

	t.Run("pipeline.failed maps to PipelineMsg failed", func(t *testing.T) {
		data := `{"type":"pipeline.failed","payload":{"stage":"L1","status":"failed","detail":"PII EGRESS BLOCKED"}}`
		msg := translateSSEEvent("pipeline.failed", data)
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL1, pm.Stage)
		assert.Equal(t, StatusFailed, pm.Status)
		assert.Equal(t, "PII EGRESS BLOCKED", pm.Detail)
	})

	t.Run("ledger.entry maps to LedgerMsg with level", func(t *testing.T) {
		data := `{"type":"ledger.entry","payload":{"level":"critical","message":"PII EGRESS BLOCKED"}}`
		msg := translateSSEEvent("ledger.entry", data)
		lm, ok := msg.(LedgerMsg)
		require.True(t, ok)
		assert.Equal(t, LevelCritical, lm.Level)
		assert.Equal(t, "PII EGRESS BLOCKED", lm.Message)
		assert.Equal(t, fixedTime, lm.Time)
	})

	t.Run("ledger.entry maps warn level", func(t *testing.T) {
		data := `{"type":"ledger.entry","payload":{"level":"warn","message":"approaching threshold"}}`
		msg := translateSSEEvent("ledger.entry", data)
		lm, ok := msg.(LedgerMsg)
		require.True(t, ok)
		assert.Equal(t, LevelWarn, lm.Level)
	})

	t.Run("consensus.vote maps to ConsensusMsg", func(t *testing.T) {
		data := `{"type":"consensus.vote","payload":{"member":"axiom","decision":true,"signed":true,"quorum":3,"total":5}}`
		msg := translateSSEEvent("consensus.vote", data)
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
		data := `{"type":"consensus.result","payload":{"result":"rejected","hash":"abcdef1234567890"}}`
		msg := translateSSEEvent("consensus.result", data)
		cm, ok := msg.(ConsensusMsg)
		require.True(t, ok)
		assert.Equal(t, ConsensusRejected, cm.Result)
		assert.Equal(t, "abcdef1234567890", cm.Hash)
	})

	t.Run("consensus.result reached", func(t *testing.T) {
		data := `{"type":"consensus.result","payload":{"result":"reached","hash":"abc123"}}`
		msg := translateSSEEvent("consensus.result", data)
		cm, ok := msg.(ConsensusMsg)
		require.True(t, ok)
		assert.Equal(t, ConsensusReached, cm.Result)
	})

	t.Run("unknown event type falls back to LedgerMsg", func(t *testing.T) {
		data := `{"type":"system.heartbeat","payload":{"status":"ok"}}`
		msg := translateSSEEvent("system.heartbeat", data)
		lm, ok := msg.(LedgerMsg)
		require.True(t, ok)
		assert.Contains(t, lm.Message, "system.heartbeat")
	})

	t.Run("non-JSON data falls back to LedgerMsg with raw text", func(t *testing.T) {
		msg := translateSSEEvent("unknown", "plain text message")
		lm, ok := msg.(LedgerMsg)
		require.True(t, ok)
		assert.Equal(t, "plain text message", lm.Message)
		assert.Equal(t, LevelInfo, lm.Level)
	})

	t.Run("uses event type from SSE header when payload type is empty", func(t *testing.T) {
		data := `{"type":"","payload":{"stage":"L2","status":"active"}}`
		msg := translateSSEEvent("pipeline.advance", data)
		pm, ok := msg.(PipelineMsg)
		require.True(t, ok)
		assert.Equal(t, StageL2, pm.Stage)
		assert.Equal(t, StatusActive, pm.Status)
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

func TestAdapterNewAdapter(t *testing.T) {
	t.Run("creates adapter with nil client default", func(t *testing.T) {
		a := NewAdapter("http://localhost:8080/sse", "token", nil, nil)
		assert.NotNil(t, a.sseClient)
		assert.Nil(t, a.program)
	})

	t.Run("creates adapter with provided http client", func(t *testing.T) {
		a := NewAdapter("url", "", nil, &http.Client{Timeout: 5 * time.Second})
		assert.NotNil(t, a.sseClient)
	})
}

func TestAdapterRunEmptyURL(t *testing.T) {
	a := NewAdapter("", "", nil, nil)
	done := make(chan struct{})
	go func() {
		a.Run(t.Context())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("adapter.Run with empty URL should return immediately")
	}
}
