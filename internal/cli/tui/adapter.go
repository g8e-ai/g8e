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
	"errors"
	"fmt"
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

// Adapter bridges the CLI session's SSE stream, pending-approval list,
// Operator list, and Gateway health to bubbletea messages.
type Adapter struct {
	gw     *gateway
	sender messageSender
}

// wireEvent is the inner event of a Gateway SSE frame. Gateway producers
// emit {type, data}; approval.completed is a flat models.ApprovalCompletedEvent,
// so the raw inner JSON is kept alongside the decoded data.
type wireEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
	raw  json.RawMessage
}

// operatorRefreshInterval is how often the Operator list is re-fetched.
// Operator status events (g8e.v1.operator.status.updated.*) are routed only
// to the owner's web sessions, never to a CLI session, so the TUI polls.
const operatorRefreshInterval = 15 * time.Second

// sessionExpiredDetail is the connection detail shown when the Gateway
// rejects the CLI session; it names the same recovery as the rest of the CLI.
const sessionExpiredDetail = "CLI session expired — run 'g8e auth refresh'"

// NewAdapter creates an Adapter over the CLI session. userID scopes the
// Operator list the same way 'g8e gw status' does.
func NewAdapter(session Session, userID string, sender messageSender) *Adapter {
	if session == nil {
		return &Adapter{sender: sender}
	}
	return &Adapter{gw: &gateway{session: session, userID: userID}, sender: sender}
}

// Run streams the CLI session's SSE events into the program until ctx is
// cancelled or the Gateway rejects the CLI session. The stream is live-only
// and approvals.changed is ephemeral, so the pending-approval list (and the
// Operator list and Gateway health) are re-fetched on every connect, and the
// pending list again on every approvals.changed event — the same
// reconciliation the console performs. Reconnects use the shared sse.Client
// backoff.
func (a *Adapter) Run(ctx context.Context) {
	if a.gw == nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	connected := make(chan struct{}, 1)
	approvalsChanged := make(chan struct{}, 1)
	signal := func(ch chan struct{}) {
		select {
		case ch <- struct{}{}:
		default:
		}
	}

	stream := a.gw.session.NewSSEClient()
	stream.SetReconnectOnClose(true)
	stream.SetOnConnect(func() {
		a.sender.Send(ConnStatusMsg{Status: ConnConnected})
		signal(connected)
	})
	stream.SetOnDisconnect(func(err error) {
		if errors.Is(err, constants.ErrCLISessionRefreshRequired) {
			a.sender.Send(ConnStatusMsg{Status: ConnFailed, Detail: sessionExpiredDetail})
			return
		}
		a.sender.Send(ConnStatusMsg{Status: ConnReconnecting, Detail: err.Error()})
	})

	refresherDone := make(chan struct{})
	go func() {
		defer close(refresherDone)
		a.refresh(ctx, connected, approvalsChanged)
	}()
	defer func() { <-refresherDone }()

	a.sender.Send(ConnStatusMsg{Status: ConnConnecting})
	stream.Run(ctx, func(eventType, data string) {
		ev, ok := decodeSSEEvent(eventType, data)
		if !ok {
			a.sender.Send(LedgerMsg{Level: LevelInfo, Message: data, Time: timeNow()})
			return
		}
		if ev.Type == string(constants.EventPlatformApprovalsChanged) {
			signal(approvalsChanged)
		}
		for _, msg := range translateEvent(ev) {
			a.sender.Send(msg)
		}
	})
}

// refresh re-fetches Gateway state until ctx is cancelled: everything on each
// connect, pending approvals on each approvals.changed, and Operators on
// operatorRefreshInterval.
func (a *Adapter) refresh(ctx context.Context, connected, approvalsChanged <-chan struct{}) {
	ticker := time.NewTicker(operatorRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-connected:
			a.sender.Send(a.gw.fetchHealth(ctx))
			a.sender.Send(a.gw.fetchPendingApprovals(ctx))
			a.sender.Send(a.gw.fetchOperators(ctx))
		case <-approvalsChanged:
			a.sender.Send(a.gw.fetchPendingApprovals(ctx))
		case <-ticker.C:
			a.sender.Send(a.gw.fetchOperators(ctx))
		}
	}
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
			FilePath   string `json:"file_path"`
		}
		_ = json.Unmarshal(payload, &p)
		targetPath := firstNonEmpty(p.FilePath, p.ApprovalID)
		return []tea.Msg{
			PipelineMsg{Stage: StageL3, Status: StatusWaiting, TxID: p.ApprovalID, Detail: "File edit approval requested: " + targetPath},
			LedgerMsg{Level: LevelWarn, Message: "APPROVAL REQUIRED: File edit: " + targetPath, Time: timeNow()},
		}
	case innerType == string(constants.EventOperatorIntentApprovalRequested):
		var p struct {
			ApprovalID string `json:"approval_id"`
			IntentName string `json:"intent_name"`
		}
		_ = json.Unmarshal(payload, &p)
		intent := firstNonEmpty(p.IntentName, p.ApprovalID)
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
		return []tea.Msg{ApprovalCompletedMsg{TxHash: p.TxHash}}
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
