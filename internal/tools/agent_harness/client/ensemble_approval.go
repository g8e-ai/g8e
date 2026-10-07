// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version. 2.0.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	ssepkg "github.com/g8e-ai/g8e/v2/internal/cli/sse"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// EnsembleApprovalRespondPath is the canonical ensemble approval respond
// endpoint, matching InternalAPIPaths.G8EE_OPERATOR_APPROVAL_RESPOND in the
// ensemble (api_paths.json: operator_approval_respond under the /api/v1
// prefix).
const EnsembleApprovalRespondPath = "/api/v1/operator/approval/respond"

// ApprovalRespondTimeout is the max time to wait for the ensemble's approval
// respond endpoint to acknowledge an auto-approval POST. The ensemble resolves
// the pending approval inline and returns; 10s is generous for a local
// round-trip and bounds the wait when the ensemble is unreachable so the
// scenario fails fast instead of hanging on an unbounded context.
const ApprovalRespondTimeout = 10 * time.Second

// FileEditApprovalEventType is the SSE event type string for file edit
// approval requests, matching the g8e protocol constant
// g8e.v1.operator.file.edit.approval.requested.
const FileEditApprovalEventType = string(constants.EventOperatorFileEditApprovalRequested)

// ApprovalAutoApprover is a background SSE listener that auto-approves file
// edit approval requests delivered through the gateway's SSE stream. The
// ensemble's file_create/file_write tools require human approval before
// executing; in CI/headless scenarios there is no human to click "approve",
// so the harness stands in for the human by subscribing to the SSE stream,
// receiving the approval request event, and posting an approval response to
// the ensemble's approval respond endpoint.
//
// The listener runs in a goroutine started by Start and stops when Stop is
// called or the provided context is cancelled. It is safe to call Stop
// multiple times. The auto-approver approves every file edit approval
// request it receives — it does not inspect risk_level or file_path —
// because the harness scenarios are deterministic and the risk analysis has
// already run (the approval gate is after the risk gate in file_service.py).
type ApprovalAutoApprover struct {
	client   *Client
	persona  Persona
	ensemble string // ensemble base URL for approval respond POST
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	err      error // first approval failure, returned to the scenario
	approved int   // count of approvals sent (for diagnostics)
	mu       sync.Mutex
	// connectedCh is closed once the SSE subscription's first HTTP
	// connection succeeds (signaled via the SSE client's SetOnConnect
	// callback). WaitForConnection blocks on it so callers can gate
	// dependent actions (e.g., sending the chat request) on the
	// subscription being established, eliminating the timing race where
	// the near-instant fake provider triggers an approval before the
	// subscription is connected (the gateway would deliver the SSE push
	// to zero listeners).
	connectedCh chan struct{}
	readyOnce   sync.Once
}

// NewApprovalAutoApprover builds an auto-approver wired to the gateway SSE
// stream (via client.cfg.PublicBaseURL) and the ensemble approval respond
// endpoint (via ensembleBaseURL). The persona supplies the CLI session ID
// for SSE routing and the user identity for the approval respond POST.
func NewApprovalAutoApprover(c *Client, p Persona, ensembleBaseURL string) *ApprovalAutoApprover {
	return &ApprovalAutoApprover{
		client:      c,
		persona:     p,
		ensemble:    ensembleBaseURL,
		connectedCh: make(chan struct{}),
	}
}

// Start begins listening for file edit approval requests on the gateway SSE
// stream. The listener runs until Stop is called or ctx is cancelled. Start
// returns immediately; the listener runs in a background goroutine.
func (a *ApprovalAutoApprover) Start(ctx context.Context) {
	listenerCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	sseTLS := a.client.cliTLS
	if sseTLS == nil {
		sseTLS = a.client.tlsCfg
	}
	sseHTTPClient := &http.Client{
		Timeout:   0,
		Transport: &http.Transport{TLSClientConfig: sseTLS.Clone()},
	}

	sseURL := fmt.Sprintf("%s%s?since_id=0",
		a.client.cfg.PublicBaseURL,
		constants.APIPaths.SSEStream)

	sseClient := ssepkg.NewClient(sseURL, sseHTTPClient)
	if a.persona.CLISessionID != "" {
		sseClient.SetHeader(constants.HeaderCLISessionID, a.persona.CLISessionID)
	}
	// Signal readiness on the first successful HTTP connection so
	// callers can block the chat request until the SSE subscription is
	// established. The callback fires once per connection (including
	// reconnects); readyOnce ensures connectedCh is closed only once.
	sseClient.SetOnConnect(func() {
		a.readyOnce.Do(func() {
			close(a.connectedCh)
		})
	})

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		sseClient.Run(listenerCtx, func(eventType, data string) {
			if err := a.handleSSEEvent(listenerCtx, eventType, data); err != nil {
				a.mu.Lock()
				if a.err == nil {
					a.err = err
				}
				a.mu.Unlock()
			}
		})
	}()
}

// Stop signals the SSE listener to stop and waits for it to drain. Safe to
// call multiple times.
func (a *ApprovalAutoApprover) Stop() {
	if a.cancel != nil {
		a.cancel()
		a.wg.Wait()
		a.cancel = nil
	}
}

// WaitForConnection blocks until the SSE subscription's first HTTP connection
// is established (signaled via the SetOnConnect callback) or ctx is cancelled.
// Callers must call this before sending the chat request so the near-instant
// fake provider's approval request is not pushed before the subscription is
// connected (the gateway delivers SSE pushes to zero listeners when no
// subscription is established, causing the approval to never be received and
// the scenario to time out).
func (a *ApprovalAutoApprover) WaitForConnection(ctx context.Context) error {
	select {
	case <-a.connectedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ApprovedCount returns the number of approval responses successfully sent.
func (a *ApprovalAutoApprover) ApprovedCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.approved
}

// Err returns the first failure reported by the SSE approval listener.
func (a *ApprovalAutoApprover) Err() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}

type ensembleFileEditApproval struct {
	ApprovalID      string `json:"approval_id"`
	UserID          string `json:"user_id"`
	CLISessionID    string `json:"cli_session_id"`
	CaseID          string `json:"case_id"`
	InvestigationID string `json:"investigation_id"`
}

type ensembleApprovalResponse struct {
	Context    EnsembleRequestContext `json:"context"`
	ApprovalID string                 `json:"approval_id"`
	Approved   bool                   `json:"approved"`
	Reason     string                 `json:"reason"`
}

// handleSSEEvent parses an SSE data frame and, if it is a file edit approval
// request, posts an approval response to the ensemble. The SSE data is a
// SSEPushPayload JSON; the inner Event field is a SessionEventWire JSON whose
// event.type identifies the event and event.data carries the
// FileEditApprovalEvent payload (with approval_id, file_path, etc.).
func (a *ApprovalAutoApprover) handleSSEEvent(ctx context.Context, eventType, data string) error {
	var payload models.SSEPushPayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		//nolint:nilerr // intentional fallback: ignore unparseable SSE frame
		return nil
	}

	// SSEPushPayload.Event is the wire event JSON directly:
	// {"type":"...","data":{...}}. Parse it to extract the inner event
	// type and data.
	var wire struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload.Event, &wire); err != nil {
		//nolint:nilerr // intentional fallback: ignore non-matching wire event
		return nil
	}

	// When the server omits the event: field (R14), eventType is empty.
	// Extract the type from the inner payload instead.
	innerType := eventType
	if innerType == "" {
		innerType = wire.Type
	}
	if innerType != FileEditApprovalEventType {
		return nil
	}

	// The data field is the FileEditApprovalEvent dict (plus routing
	// metadata injected by SessionEventWire.from_session_event). Extract
	// the approval_id and context fields needed for the respond POST.
	var approvalData ensembleFileEditApproval

	if err := json.Unmarshal(wire.Data, &approvalData); err != nil {
		//nolint:nilerr // intentional fallback: ignore unparseable approval payload
		return nil
	}
	if approvalData.ApprovalID == "" {
		return nil
	}

	return a.respondApproval(ctx, approvalData)
}

// respondApproval authenticates and submits the typed Ensemble approval response.
func (a *ApprovalAutoApprover) respondApproval(parent context.Context, ad ensembleFileEditApproval) error {
	userID := ad.UserID
	if userID == "" {
		userID = a.persona.UserID
	}
	cliSessionID := ad.CLISessionID
	if cliSessionID == "" {
		cliSessionID = a.persona.CLISessionID
	}

	body := ensembleApprovalResponse{
		Context:    EnsembleRequestContext{CLISessionID: cliSessionID, UserID: userID, CaseID: ad.CaseID, InvestigationID: ad.InvestigationID, SourceComponent: "CLIENT"},
		ApprovalID: ad.ApprovalID, Approved: true, Reason: "Auto-approved by harness",
	}
	if a.persona.OperatorID != "" {
		body.Context.BoundOperators = []EnsembleBoundOperator{{OperatorID: a.persona.OperatorID, OperatorSessionID: a.persona.OperatorSessionID, Status: "bound"}}
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("respond to approval %s: %w", ad.ApprovalID, err)
	}

	url := a.ensemble + EnsembleApprovalRespondPath
	ctx, cancel := context.WithTimeout(parent, ApprovalRespondTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("respond to approval %s: %w", ad.ApprovalID, err)
	}
	req.Header.Set("Content-Type", constants.HeaderValueApplicationJSON)
	p := a.persona
	if userID != "" {
		p.UserID = userID
	}
	if cliSessionID != "" {
		p.CLISessionID = cliSessionID
	}
	applyEnsemblePersonaHeaders(req, p)

	resp, err := a.client.http.Do(req)
	if err != nil {
		return fmt.Errorf("respond to approval %s: %w", ad.ApprovalID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: approval %s returned status %d", constants.ErrHTTPStatusError, ad.ApprovalID, resp.StatusCode)
	}
	a.mu.Lock()
	a.approved++
	a.mu.Unlock()
	return nil
}

// StartApprovalAutoApprover is a convenience that starts an
// ApprovalAutoApprover for the given persona and returns it. The caller
// must call Stop when the scenario is done (typically deferred).
func (c *Client) StartApprovalAutoApprover(ctx context.Context, p Persona) *ApprovalAutoApprover {
	if c.cfg.EnsembleBaseURL == "" {
		return nil
	}
	ap := NewApprovalAutoApprover(c, p, c.cfg.EnsembleBaseURL)
	ap.Start(ctx)
	return ap
}
