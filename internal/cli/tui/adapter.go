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

// Session is the CLI's authenticated Gateway session. *api.Client satisfies
// it, so the TUI uses the same mTLS identity, CLI session header, and SSE
// stream URL as every other CLI command.
type Session interface {
	NewSSEClient() *sse.Client
	DoRequestContext(ctx context.Context, method, path string, body interface{}) ([]byte, error)
}

// messageSender abstracts the Send method of tea.Program so tests can
// capture messages without a real bubbletea program.
type messageSender interface {
	Send(msg tea.Msg)
}

// Adapter bridges the CLI session's SSE stream and pending-approval list to
// bubbletea messages.
type Adapter struct {
	session Session
	sender  messageSender
}

// wireEvent is the inner event of a Gateway SSE frame. Gateway producers
// emit {type, data}; approval.completed is a flat models.ApprovalCompletedEvent,
// so the raw inner JSON is kept alongside the decoded data.
type wireEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
	raw  json.RawMessage
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

// NewAdapter creates an Adapter over the CLI session.
func NewAdapter(session Session, sender messageSender) *Adapter {
	return &Adapter{session: session, sender: sender}
}

// Run streams the CLI session's SSE events into the program until ctx is
// cancelled. The stream is live-only and approvals.changed is ephemeral, so
// the pending-approval list is re-fetched on every connect and on every
// approvals.changed event — the same reconciliation the console performs.
func (a *Adapter) Run(ctx context.Context) {
	if a.session == nil {
		return
	}

	refresh := make(chan struct{}, 1)
	requestRefresh := func() {
		select {
		case refresh <- struct{}{}:
		default:
		}
	}

	stream := a.session.NewSSEClient()
	stream.SetOnConnect(func() {
		a.sender.Send(ConnStatusMsg{Status: ConnConnected})
		requestRefresh()
	})

	refresherDone := make(chan struct{})
	go func() {
		defer close(refresherDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-refresh:
				a.sender.Send(a.fetchPendingApprovals(ctx))
			}
		}
	}()
	defer func() { <-refresherDone }()

	a.sender.Send(ConnStatusMsg{Status: ConnConnecting})
	for {
		err := stream.ConnectOnce(ctx, func(eventType, data string) {
			ev, ok := decodeSSEEvent(eventType, data)
			if !ok {
				a.sender.Send(LedgerMsg{Level: LevelInfo, Message: data, Time: timeNow()})
				return
			}
			if ev.Type == string(constants.EventPlatformApprovalsChanged) {
				requestRefresh()
			}
			for _, msg := range translateEvent(ev) {
				a.sender.Send(msg)
			}
		})
		if err == nil || ctx.Err() != nil {
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

// fetchPendingApprovals lists the session user's pending L3 transactions
// from the same mTLS endpoint the CLI uses.
func (a *Adapter) fetchPendingApprovals(ctx context.Context) PendingApprovalsMsg {
	body, err := a.session.DoRequestContext(ctx, http.MethodGet, constants.APIPaths.ApprovalsCLIList, nil)
	if err != nil {
		return PendingApprovalsMsg{Err: err}
	}
	var resp models.SuspendedTransactionsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return PendingApprovalsMsg{Err: fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)}
	}
	return PendingApprovalsMsg{Transactions: resp.Transactions}
}

// decodeSSEEvent unwraps a Gateway SSE frame. Stored and live events are a
// models.SSEPushPayload whose Event holds the inner event; stream sentinels
// (error, truncated) are the inner event themselves. The Gateway omits the
// event: line (R14), so eventType is only a fallback.
func decodeSSEEvent(eventType, data string) (wireEvent, bool) {
	var envelope models.SSEPushPayload
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		return wireEvent{}, false
	}
	inner := envelope.Event
	if len(inner) == 0 {
		inner = json.RawMessage(data)
	}
	var ev wireEvent
	if err := json.Unmarshal(inner, &ev); err != nil {
		return wireEvent{}, false
	}
	ev.raw = inner
	if ev.Type == "" {
		ev.Type = eventType
	}
	if ev.Type == "" {
		ev.Type = "unknown"
	}
	return ev, true
}

// translateSSEEvents maps a raw SSE frame to tea.Msg values.
func translateSSEEvents(eventType, data string) []tea.Msg {
	ev, ok := decodeSSEEvent(eventType, data)
	if !ok {
		return []tea.Msg{LedgerMsg{Level: LevelInfo, Message: data, Time: timeNow()}}
	}
	return translateEvent(ev)
}

// translateEvent maps a decoded Gateway event to tea.Msg values.
func translateEvent(ev wireEvent) []tea.Msg {
	innerType, payload := ev.Type, ev.Data

	switch {
	case strings.HasPrefix(innerType, "pipeline."):
		return []tea.Msg{parsePipelineEvent(payload)}
	case strings.HasPrefix(innerType, "ledger."):
		return []tea.Msg{parseLedgerEvent(payload)}
	case strings.HasPrefix(innerType, "consensus."):
		return []tea.Msg{parseConsensusEvent(payload)}

	// Stream sentinels
	case innerType == "error":
		var p models.SSEErrorEvent
		_ = json.Unmarshal(ev.raw, &p)
		return []tea.Msg{LedgerMsg{Level: LevelWarn, Message: "Event stream error: " + p.Reason, Time: timeNow()}}
	case innerType == "truncated":
		var p models.SSETruncationEvent
		_ = json.Unmarshal(ev.raw, &p)
		return []tea.Msg{LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("Event replay truncated at %d events (cursor %d)", p.Limit, p.SinceID), Time: timeNow()}}

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
		_ = json.Unmarshal(payload, &p)
		text := firstNonEmpty(p.Chunk, p.Delta, p.Text, p.Content, string(payload))
		return []tea.Msg{LedgerMsg{Level: LevelInfo, Message: text, Time: timeNow()}}
	case innerType == string(constants.EventAiLLMChatIterationTextReceived):
		var p struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		}
		_ = json.Unmarshal(payload, &p)
		text := firstNonEmpty(p.Text, p.Content, string(payload))
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
		_ = json.Unmarshal(payload, &p)
		errDetail := firstNonEmpty(p.Error, p.Message, p.Reason, "unknown error")
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
		_ = json.Unmarshal(payload, &p)
		txID := firstNonEmpty(p.TxHash, p.ApprovalID)
		cmd := firstNonEmpty(p.Command, txID)
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
		_ = json.Unmarshal(payload, &p)
		targetPath := firstNonEmpty(p.Path, p.File, p.ApprovalID)
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusWaiting, TxID: p.ApprovalID, Detail: "File edit approval requested: " + targetPath},
			LedgerMsg{Level: LevelWarn, Message: "APPROVAL REQUIRED: File edit: " + targetPath, Time: timeNow()},
		}
	case innerType == string(constants.EventOperatorIntentApprovalRequested):
		var p struct {
			ApprovalID string `json:"approval_id"`
			Intent     string `json:"intent"`
		}
		_ = json.Unmarshal(payload, &p)
		intent := firstNonEmpty(p.Intent, p.ApprovalID)
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
		_ = json.Unmarshal(payload, &p)
		txID := firstNonEmpty(p.TxHash, p.ApprovalID)
		tool := firstNonEmpty(p.ToolName, "transaction")
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusWaiting, TxID: txID, Detail: "Notary approval requested: " + tool},
			LedgerMsg{Level: LevelWarn, Message: "APPROVAL REQUIRED: Notary approval for " + tool, Time: timeNow()},
		}
	case innerType == string(constants.EventAiAgentContinueApprovalRequested):
		var p struct {
			ApprovalID string `json:"approval_id"`
			Turn       int    `json:"turn"`
		}
		_ = json.Unmarshal(payload, &p)
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusWaiting, TxID: p.ApprovalID, Detail: fmt.Sprintf("Agent continuation approval requested (turn %d)", p.Turn)},
			LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("APPROVAL REQUIRED: Agent continuation at turn %d", p.Turn), Time: timeNow()},
		}

	// approval.completed is a flat event (models.ApprovalCompletedEvent),
	// the same shape auth.WaitForApprovalSSE consumes.
	case innerType == constants.SSEEventTypeApprovalCompleted:
		var p models.ApprovalCompletedEvent
		_ = json.Unmarshal(ev.raw, &p)
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusPassed, TxID: p.TxHash, Detail: "Approval completed"},
			LedgerMsg{Level: LevelInfo, Message: "Approval completed for tx " + p.TxHash, Time: timeNow()},
		}
	case innerType == string(constants.EventPlatformApprovalsChanged):
		var p models.ApprovalsChangedPayload
		_ = json.Unmarshal(payload, &p)
		if p.Subject == models.ApprovalsChangedEnrollments {
			return []tea.Msg{LedgerMsg{Level: LevelInfo, Message: "Pending platform enrollments changed — review with 'g8e auth enroll pending'", Time: timeNow()}}
		}
		// Transaction changes are reconciled by the adapter's pending-approval refresh.
		return nil

	default:
		return []tea.Msg{LedgerMsg{Level: LevelInfo, Message: innerType + ": " + string(payload), Time: timeNow()}}
	}
}

// firstNonEmpty returns the first non-empty value.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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
