// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version. 2.0.

package client

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
)

// newApprovalTestClient builds a Client whose http transport targets the
// provided ensemble test server. The PublicBaseURL is unused by handleSSEEvent
// (which only POSTs to the ensemble), so it is set to a placeholder.
func newApprovalTestClient(t *testing.T, ensembleURL string) *Client {
	t.Helper()
	c, err := New(config.Config{
		MTLSBaseURL:     ensembleURL,
		PublicBaseURL:   ensembleURL,
		EnsembleBaseURL: ensembleURL,
		Auth:            config.Auth{},
	})
	require.NoError(t, err)
	return c
}

// buildSSEData builds the SSE data frame JSON for a file edit approval event,
// matching the wire shape produced by the gateway: a SSEPushPayload whose
// Event field is the wire event JSON {"type":"...","data":{...}}.
func buildSSEData(t *testing.T, eventType string, approvalID string, extraData map[string]any) string {
	t.Helper()
	dataMap := map[string]any{
		"approval_id":      approvalID,
		"user_id":          "user-123",
		"cli_session_id":   "cli-session-123",
		"case_id":          "case-123",
		"investigation_id": "inv-123",
	}
	for k, v := range extraData {
		dataMap[k] = v
	}
	wireEvent := map[string]any{
		"type": eventType,
		"data": dataMap,
	}
	wireBytes, err := json.Marshal(wireEvent)
	require.NoError(t, err)
	payload := models.SSEPushPayload{
		UserID:       "user-123",
		CliSessionID: "cli-session-123",
		Event:        wireBytes,
	}
	out, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(out)
}

// TestWaitForConnection_AlreadyConnected asserts that WaitForConnection
// returns immediately when the connectedCh is already closed (simulating a
// prior successful SSE connection).
func TestWaitForConnection_AlreadyConnected(t *testing.T) {
	c := newApprovalTestClient(t, "http://example.invalid")
	ap := NewApprovalAutoApprover(c, Persona{ID: "test"}, "http://example.invalid")
	// Simulate a prior onConnect by closing the channel.
	ap.readyOnce.Do(func() { close(ap.connectedCh) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := ap.WaitForConnection(ctx)
	assert.NoError(t, err)
}

// TestWaitForConnection_ContextCancelled asserts that WaitForConnection
// returns the context error when the context is cancelled before the SSE
// subscription connects.
func TestWaitForConnection_ContextCancelled(t *testing.T) {
	c := newApprovalTestClient(t, "http://example.invalid")
	ap := NewApprovalAutoApprover(c, Persona{ID: "test"}, "http://example.invalid")
	// Do NOT close connectedCh; the wait should block until ctx is cancelled.

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := ap.WaitForConnection(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestStartApprovalAutoApprover_NoEnsembleURLReturnsNil asserts that the
// convenience constructor returns nil when EnsembleBaseURL is empty, so
// scenarios fail closed rather than starting an approver with no target.
func TestStartApprovalAutoApprover_NoEnsembleURLReturnsNil(t *testing.T) {
	c, err := New(config.Config{Auth: config.Auth{}})
	require.NoError(t, err)
	ap := c.StartApprovalAutoApprover(context.Background(), Persona{ID: "test"})
	assert.Nil(t, ap)
}

// TestRespondApproval_BoundedTimeoutDoesNotHang asserts that respondApproval
// does not hang indefinitely when the ensemble is unreachable. The bounded
// context (ApprovalRespondTimeout) ensures the POST fails fast. This test
// uses a blackhole server that accepts connections but never responds, with a
// short override of the timeout via a dial-targeted server that closes
// immediately to force a connection error path. We instead validate the
// timeout is finite by pointing at an invalid host and asserting the call
// returns within a reasonable bound.
func TestRespondApproval_BoundedTimeoutDoesNotHang(t *testing.T) {
	c := newApprovalTestClient(t, "http://127.0.0.1:1") // unreachable port
	ap := NewApprovalAutoApprover(c, Persona{ID: "test"}, "http://127.0.0.1:1")

	start := time.Now()
	// Use a goroutine to detect hanging; handleSSEEvent calls respondApproval
	// synchronously. We expect it to return within ApprovalRespondTimeout.
	done := make(chan struct{})
	go func() {
		ap.handleSSEEvent(t.Context(), "", buildSSEData(t, FileEditApprovalEventType, "approval-1", nil))
		close(done)
	}()
	select {
	case <-done:
		elapsed := time.Since(start)
		assert.Less(t, elapsed, ApprovalRespondTimeout+5*time.Second,
			"respondApproval should return within the bounded timeout")
	case <-time.After(ApprovalRespondTimeout + 10*time.Second):
		t.Fatal("respondApproval hung beyond the bounded timeout")
	}
	assert.Equal(t, 0, ap.ApprovedCount(), "unreachable ensemble should not increment approved count")
}

// TestEnsembleApprovalRespondPathConstant asserts the path constant matches
// the canonical ensemble endpoint, guarding against drift.
func TestEnsembleApprovalRespondPathConstant(t *testing.T) {
	assert.Equal(t, "/api/v1/operator/approval/respond", EnsembleApprovalRespondPath)
	assert.True(t, strings.HasPrefix(EnsembleApprovalRespondPath, "/api/v1/"),
		"path should be under the /api/v1 prefix")
}

// TestFileEditApprovalEventTypeConstant asserts the event type constant
// matches the g8e protocol constant for file edit approval requests.
func TestFileEditApprovalEventTypeConstant(t *testing.T) {
	assert.Equal(t, "g8e.v1.operator.file.edit.approval.requested", FileEditApprovalEventType)
}
