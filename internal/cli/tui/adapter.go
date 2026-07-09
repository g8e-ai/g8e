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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/internal/cli/sse"
	"github.com/g8e-ai/g8e/internal/constants"
)

// Adapter bridges external event sources to bubbletea messages.
// It does not modify production code — it subscribes to existing event
// sources and translates them into tea.Msg values.
type Adapter struct {
	sseClient *sse.Client
	program   *tea.Program
}

// sseEvent is the top-level envelope for SSE event data.
type sseEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// pipelinePayload is the JSON payload for pipeline.* events.
type pipelinePayload struct {
	Stage  string `json:"stage"`
	Status string `json:"status"`
	TxID   string `json:"tx_id"`
	Detail string `json:"detail"`
}

// ledgerPayload is the JSON payload for ledger.* events.
type ledgerPayload struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// consensusPayload is the JSON payload for consensus.* events.
type consensusPayload struct {
	Member   string `json:"member"`
	Decision bool   `json:"decision"`
	Signed   bool   `json:"signed"`
	Quorum   int    `json:"quorum"`
	Total    int    `json:"total"`
	Result   string `json:"result"`
	Hash     string `json:"hash"`
}

// NewAdapter creates an Adapter that connects to the gateway's SSE stream.
func NewAdapter(sseURL, token string, program *tea.Program, client *http.Client) *Adapter {
	c := sse.NewClient(sseURL, client)
	if token != "" {
		c.SetHeader("Authorization", "Bearer "+token)
	}
	return &Adapter{
		sseClient: c,
		program:   program,
	}
}

// Run starts the adapter goroutine. It connects to the SSE stream and
// translates events into tea.Msg values until the context is cancelled.
// It emits ConnStatusMsg to keep the TUI informed about connection state.
func (a *Adapter) Run(ctx context.Context) {
	if a.sseClient == nil || a.program == nil {
		return
	}

	a.program.Send(ConnStatusMsg{Status: ConnConnecting})

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := a.sseClient.ConnectOnce(ctx, func(eventType, data string) {
			msg := translateSSEEvent(eventType, data)
			if msg != nil {
				a.program.Send(msg)
			}
		})
		if err == nil {
			return
		}

		a.program.Send(ConnStatusMsg{Status: ConnReconnecting, Detail: err.Error()})

		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}

		a.program.Send(ConnStatusMsg{Status: ConnConnecting})
	}
}

// translateSSEEvent maps an SSE event_type + data payload to a tea.Msg.
// The SSE event types are free-form strings; this function maps known
// patterns to the appropriate TUI message types.
func translateSSEEvent(eventType, data string) tea.Msg {
	var raw sseEvent
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return LedgerMsg{Level: LevelInfo, Message: data, Time: timeNow()}
	}

	innerType := raw.Type
	if innerType == "" {
		innerType = eventType
	}

	switch {
	case strings.HasPrefix(innerType, "pipeline."):
		return parsePipelineEvent(raw.Payload)
	case strings.HasPrefix(innerType, "ledger."):
		return parseLedgerEvent(raw.Payload)
	case strings.HasPrefix(innerType, "consensus."):
		return parseConsensusEvent(raw.Payload)
	default:
		return LedgerMsg{Level: LevelInfo, Message: innerType + ": " + string(raw.Payload), Time: timeNow()}
	}
}

// parsePipelineEvent translates a pipeline.* event into a PipelineMsg.
func parsePipelineEvent(payload json.RawMessage) tea.Msg {
	var p pipelinePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("pipeline: unmarshal: %s", err), Time: timeNow()}
	}

	stage := parseStage(p.Stage)
	status := parseStatus(p.Status)

	return PipelineMsg{
		Stage:  stage,
		Status: status,
		TxID:   p.TxID,
		Detail: p.Detail,
	}
}

// parseLedgerEvent translates a ledger.* event into a LedgerMsg.
func parseLedgerEvent(payload json.RawMessage) tea.Msg {
	var p ledgerPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("ledger: unmarshal: %s", err), Time: timeNow()}
	}

	level := LevelInfo
	switch strings.ToLower(p.Level) {
	case "warn", "warning":
		level = LevelWarn
	case "crit", "critical":
		level = LevelCritical
	}

	return LedgerMsg{
		Level:   level,
		Message: p.Message,
		Time:    timeNow(),
	}
}

// parseConsensusEvent translates a consensus.* event into a ConsensusMsg.
func parseConsensusEvent(payload json.RawMessage) tea.Msg {
	var p consensusPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("consensus: unmarshal: %s", err), Time: timeNow()}
	}

	result := ConsensusPending
	switch strings.ToLower(p.Result) {
	case "reached", "approved":
		result = ConsensusReached
	case "rejected":
		result = ConsensusRejected
	}

	return ConsensusMsg{
		Member:   constants.TribunalMember(p.Member),
		Decision: p.Decision,
		Signed:   p.Signed,
		Quorum:   p.Quorum,
		Total:    p.Total,
		Result:   result,
		Hash:     p.Hash,
	}
}

// parseStage converts a string stage name to a pipelineStage constant.
func parseStage(s string) PipelineStage {
	switch strings.ToUpper(s) {
	case "L1":
		return StageL1
	case "L2":
		return StageL2
	case "L3":
		return StageL3
	case "L4":
		return StageL4
	case "L5":
		return StageL5
	default:
		return StageL1
	}
}

// parseStatus converts a string status to a pipelineStatus constant.
func parseStatus(s string) PipelineStatus {
	switch strings.ToLower(s) {
	case "active", "processing":
		return StatusActive
	case "waiting":
		return StatusWaiting
	case "passed", "ok":
		return StatusPassed
	case "failed", "blocked":
		return StatusFailed
	default:
		return StatusIdle
	}
}
