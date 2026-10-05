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
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/v2/internal/cli/sse"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// messageSender abstracts the Send method of tea.Program so tests can
// capture messages without a real bubbletea program.
type messageSender interface {
	Send(msg tea.Msg)
}

// Adapter bridges external event sources to bubbletea messages.
// It does not modify production code — it subscribes to existing event
// sources and translates them into tea.Msg values.
type Adapter struct {
	sseURL    string
	sseClient *sse.Client
	sender    messageSender
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

// reconnectBackoff is the fixed delay between SSE reconnection attempts.
const reconnectBackoff = 3 * time.Second

// NewAdapter creates an Adapter that connects to the gateway's SSE stream.
func NewAdapter(sseURL, token, cliSessionID string, sender messageSender, client *http.Client) *Adapter {
	c := sse.NewClient(sseURL, client)
	if token != "" {
		c.SetHeader("Authorization", "Bearer "+token)
	}
	if cliSessionID != "" {
		c.SetHeader(constants.HeaderCLISessionID, cliSessionID)
	}
	return &Adapter{
		sseURL:    sseURL,
		sseClient: c,
		sender:    sender,
	}
}

// Run starts the adapter goroutine. It connects to the SSE stream and
// translates events into tea.Msg values until the context is cancelled.
// It emits ConnStatusMsg to keep the TUI informed about connection state.
func (a *Adapter) Run(ctx context.Context) {
	if a.sseURL == "" {
		return
	}
	a.sender.Send(ConnStatusMsg{Status: ConnConnecting})

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		connected := false
		err := a.sseClient.ConnectOnce(ctx, func(eventType, data string) {
			if !connected {
				a.sender.Send(ConnStatusMsg{Status: ConnConnected})
				connected = true
			}
			for _, msg := range translateSSEEvents(eventType, data) {
				if msg != nil {
					a.sender.Send(msg)
				}
			}
		})
		if err == nil {
			return
		}

		a.sender.Send(ConnStatusMsg{Status: ConnReconnecting, Detail: err.Error()})

		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectBackoff):
		}

		a.sender.Send(ConnStatusMsg{Status: ConnConnecting})
	}
}

// translateSSEEvent maps an SSE event_type + data payload to the primary tea.Msg.
// Deprecated: use translateSSEEvents for full multi-message routing.
func translateSSEEvent(eventType, data string) tea.Msg {
	msgs := translateSSEEvents(eventType, data)
	if len(msgs) > 0 {
		return msgs[0]
	}
	return nil
}

// translateSSEEvents maps an SSE event_type + data payload to tea.Msg values.
// Supports pipeline, ledger, consensus, chat, and operator approval events.
func translateSSEEvents(eventType, data string) []tea.Msg {
	var raw sseEvent
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return []tea.Msg{LedgerMsg{Level: LevelInfo, Message: data, Time: timeNow()}}
	}

	innerType := raw.Type
	if innerType == "" {
		innerType = eventType
	}

	// When both the SSE event: field and the top-level JSON type are empty,
	// the data is likely a SSEPushPayload envelope. Extract the inner event
	// JSON and parse it for the type and payload.
	var innerPayload json.RawMessage
	if innerType == "" && raw.Payload == nil {
		var envelope models.SSEPushPayload
		if err := json.Unmarshal([]byte(data), &envelope); err == nil && len(envelope.Event) > 0 {
			var inner sseEvent
			if err := json.Unmarshal(envelope.Event, &inner); err == nil {
				innerType = inner.Type
				innerPayload = inner.Payload
			}
		}
	} else {
		innerPayload = raw.Payload
	}

	if innerType == "" {
		innerType = "unknown"
	}

	switch {
	case strings.HasPrefix(innerType, "pipeline."):
		return []tea.Msg{parsePipelineEvent(innerPayload)}
	case strings.HasPrefix(innerType, "ledger."):
		return []tea.Msg{parseLedgerEvent(innerPayload)}
	case strings.HasPrefix(innerType, "consensus."):
		return []tea.Msg{parseConsensusEvent(innerPayload)}

	// Chat events
	case innerType == string(constants.EventAiLLMChatIterationStarted):
		return []tea.Msg{
			PipelineMsg{Stage: StageL1, Status: StatusActive, Detail: "LLM chat iteration started"},
			LedgerMsg{Level: LevelInfo, Message: "AI chat iteration started", Time: timeNow()},
		}
	case innerType == string(constants.EventAiLLMChatIterationTextChunkReceived):
		var p struct {
			Chunk   string `json:"chunk"`
			Text    string `json:"text"`
			Content string `json:"content"`
			Delta   string `json:"delta"`
		}
		_ = json.Unmarshal(innerPayload, &p)
		text := p.Chunk
		if text == "" {
			text = p.Delta
		}
		if text == "" {
			text = p.Text
		}
		if text == "" {
			text = p.Content
		}
		if text == "" {
			text = string(innerPayload)
		}
		return []tea.Msg{LedgerMsg{Level: LevelInfo, Message: text, Time: timeNow()}}
	case innerType == string(constants.EventAiLLMChatIterationTextReceived):
		var p struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		}
		_ = json.Unmarshal(innerPayload, &p)
		text := p.Text
		if text == "" {
			text = p.Content
		}
		if text == "" {
			text = string(innerPayload)
		}
		return []tea.Msg{LedgerMsg{Level: LevelInfo, Message: text, Time: timeNow()}}
	case innerType == string(constants.EventAiLLMChatIterationCompleted):
		return []tea.Msg{
			PipelineMsg{Stage: StageL5, Status: StatusPassed, Detail: "LLM chat iteration completed"},
			LedgerMsg{Level: LevelInfo, Message: "AI chat iteration completed", Time: timeNow()},
		}
	case innerType == string(constants.EventAiLLMChatIterationFailed):
		var p struct {
			Error   string `json:"error"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
		}
		_ = json.Unmarshal(innerPayload, &p)
		errDetail := p.Error
		if errDetail == "" {
			errDetail = p.Message
		}
		if errDetail == "" {
			errDetail = p.Reason
		}
		if errDetail == "" {
			errDetail = "unknown error"
		}
		return []tea.Msg{
			PipelineMsg{Stage: StageL5, Status: StatusFailed, Detail: "LLM chat iteration failed: " + errDetail},
			LedgerMsg{Level: LevelWarn, Message: "AI chat iteration failed: " + errDetail, Time: timeNow()},
		}

	// Operator approvals requested
	case innerType == string(constants.EventOperatorCommandApprovalRequested):
		var p struct {
			ApprovalID string `json:"approval_id"`
			TxHash     string `json:"tx_hash"`
			Command    string `json:"command"`
		}
		_ = json.Unmarshal(innerPayload, &p)
		txID := p.TxHash
		if txID == "" {
			txID = p.ApprovalID
		}
		cmd := p.Command
		if cmd == "" {
			cmd = txID
		}
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusWaiting, TxID: txID, Detail: "Command approval requested: " + cmd},
			LedgerMsg{Level: LevelWarn, Message: "APPROVAL REQUIRED: Command execution: " + cmd, Time: timeNow()},
		}
	case innerType == string(constants.EventOperatorFileEditApprovalRequested):
		var p struct {
			ApprovalID string `json:"approval_id"`
			Path       string `json:"path"`
			File       string `json:"file"`
		}
		_ = json.Unmarshal(innerPayload, &p)
		targetPath := p.Path
		if targetPath == "" {
			targetPath = p.File
		}
		if targetPath == "" {
			targetPath = p.ApprovalID
		}
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusWaiting, TxID: p.ApprovalID, Detail: "File edit approval requested: " + targetPath},
			LedgerMsg{Level: LevelWarn, Message: "APPROVAL REQUIRED: File edit: " + targetPath, Time: timeNow()},
		}
	case innerType == string(constants.EventOperatorIntentApprovalRequested):
		var p struct {
			ApprovalID string `json:"approval_id"`
			Intent     string `json:"intent"`
		}
		_ = json.Unmarshal(innerPayload, &p)
		intent := p.Intent
		if intent == "" {
			intent = p.ApprovalID
		}
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusWaiting, TxID: p.ApprovalID, Detail: "Intent approval requested: " + intent},
			LedgerMsg{Level: LevelWarn, Message: "APPROVAL REQUIRED: Intent authorization: " + intent, Time: timeNow()},
		}
	case innerType == string(constants.EventOperatorNotaryApprovalRequested):
		var p struct {
			ApprovalID string `json:"approval_id"`
			TxHash     string `json:"tx_hash"`
			ToolName   string `json:"tool_name"`
		}
		_ = json.Unmarshal(innerPayload, &p)
		txID := p.TxHash
		if txID == "" {
			txID = p.ApprovalID
		}
		tool := p.ToolName
		if tool == "" {
			tool = "transaction"
		}
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusWaiting, TxID: txID, Detail: "Notary approval requested: " + tool},
			LedgerMsg{Level: LevelWarn, Message: "APPROVAL REQUIRED: Notary approval for " + tool, Time: timeNow()},
		}
	case innerType == string(constants.EventAiAgentContinueApprovalRequested):
		var p struct {
			ApprovalID string `json:"approval_id"`
			Turn       int    `json:"turn"`
		}
		_ = json.Unmarshal(innerPayload, &p)
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusWaiting, TxID: p.ApprovalID, Detail: fmt.Sprintf("Agent continuation approval requested (turn %d)", p.Turn)},
			LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("APPROVAL REQUIRED: Agent continuation at turn %d", p.Turn), Time: timeNow()},
		}

	// Approval completed and changed
	case innerType == constants.SSEEventTypeApprovalCompleted || strings.HasSuffix(innerType, ".approval.completed"):
		var p struct {
			TxHash   string `json:"tx_hash"`
			UserID   string `json:"user_id"`
			Approved bool   `json:"approved"`
		}
		_ = json.Unmarshal(innerPayload, &p)
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusPassed, TxID: p.TxHash, Detail: "Approval completed"},
			LedgerMsg{Level: LevelInfo, Message: "Approval completed for tx " + p.TxHash, Time: timeNow()},
		}
	case innerType == string(constants.EventPlatformApprovalsChanged):
		return []tea.Msg{LedgerMsg{Level: LevelInfo, Message: "Pending approvals list changed", Time: timeNow()}}

	default:
		return []tea.Msg{LedgerMsg{Level: LevelInfo, Message: innerType + ": " + string(innerPayload), Time: timeNow()}}
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
		Member:   constants.ConsensusMember(p.Member),
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
