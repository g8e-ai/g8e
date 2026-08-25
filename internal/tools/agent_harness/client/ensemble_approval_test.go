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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/tools/agent_harness/config"
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

// TestHandleSSEEvent_FileEditApprovalDispatchesApproval asserts that a valid
// file edit approval SSE event triggers an approval POST to the ensemble's
// approval respond endpoint with the expected body and proxy headers.
func TestHandleSSEEvent_FileEditApprovalDispatchesApproval(t *testing.T) {
	var (
		receivedBody   map[string]any
		receivedUserID string
		receivedCLI    string
		receivedEmail  string
		approveID      = "approval-abc-123"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, EnsembleApprovalRespondPath, r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &receivedBody))
		receivedUserID = r.Header.Get(HeaderProxyUserID)
		receivedCLI = r.Header.Get(HeaderProxyCLISessionID)
		receivedEmail = r.Header.Get(HeaderProxyUserEmail)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newApprovalTestClient(t, srv.URL)
	ap := NewApprovalAutoApprover(c, Persona{
		ID:           "test",
		UserID:       "user-123",
		CLISessionID: "cli-session-123",
	}, srv.URL)

	ap.handleSSEEvent("", buildSSEData(t, FileEditApprovalEventType, approveID, nil))

	assert.Equal(t, 1, ap.ApprovedCount(), "approval count should increment on 200 response")
	assert.Equal(t, approveID, receivedBody["approval_id"])
	assert.Equal(t, true, receivedBody["approved"])
	assert.Equal(t, "Auto-approved by harness", receivedBody["reason"])
	ctxObj, ok := receivedBody["context"].(map[string]any)
	require.True(t, ok, "context should be an object")
	assert.Equal(t, "CLIENT", ctxObj["source_component"])
	assert.Equal(t, "user-123", receivedUserID)
	assert.Equal(t, "cli-session-123", receivedCLI)
	assert.Equal(t, "user-123"+ProxyUserEmailSyntheticDomain, receivedEmail)
}

// TestHandleSSEEvent_BoundOperatorStampsContext asserts that when the persona
// carries an OperatorID, the approval POST body includes a bound_operators
// entry with the operator identity.
func TestHandleSSEEvent_BoundOperatorStampsContext(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &receivedBody))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newApprovalTestClient(t, srv.URL)
	ap := NewApprovalAutoApprover(c, Persona{
		ID:                "test",
		UserID:            "user-123",
		CLISessionID:      "cli-session-123",
		OperatorID:        "operator-456",
		OperatorSessionID: "operator-session-789",
	}, srv.URL)

	ap.handleSSEEvent("", buildSSEData(t, FileEditApprovalEventType, "approval-1", nil))

	ctxObj, ok := receivedBody["context"].(map[string]any)
	require.True(t, ok)
	bound, ok := ctxObj["bound_operators"].([]any)
	require.True(t, ok)
	require.Len(t, bound, 1)
	entry, ok := bound[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "operator-456", entry["operator_id"])
	assert.Equal(t, "operator-session-789", entry["operator_session_id"])
	assert.Equal(t, "bound", entry["status"])
}

// TestHandleSSEEvent_NonMatchingEventTypeDoesNotDispatch asserts that SSE
// events whose type is not the file edit approval requested type do not
// trigger an approval POST.
func TestHandleSSEEvent_NonMatchingEventTypeDoesNotDispatch(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newApprovalTestClient(t, srv.URL)
	ap := NewApprovalAutoApprover(c, Persona{ID: "test"}, srv.URL)

	// A different event type (file edit completed, not approval requested).
	ap.handleSSEEvent("", buildSSEData(t, string(constants.EventOperatorFileEditCompleted), "approval-1", nil))
	// Event type passed via the SSE event: field but inner wire type differs.
	ap.handleSSEEvent(string(constants.EventOperatorFileEditCompleted),
		buildSSEData(t, string(constants.EventOperatorFileEditCompleted), "approval-1", nil))

	assert.Equal(t, int32(0), atomic.LoadInt32(&calls), "no approval POST should be made for non-matching event types")
	assert.Equal(t, 0, ap.ApprovedCount())
}

// TestHandleSSEEvent_EventTypeFromSSEFieldPreferredOverWire asserts that when
// the SSE event: field is populated, it is used as the type and the inner wire
// type is ignored. This covers the R14 server-omits-event-field path: when the
// SSE field is empty, the inner wire type is used instead.
func TestHandleSSEEvent_EventTypeFromSSEFieldPreferredOverWire(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newApprovalTestClient(t, srv.URL)
	ap := NewApprovalAutoApprover(c, Persona{ID: "test"}, srv.URL)

	// SSE event: field says "completed" but inner wire says "approval.requested".
	// The SSE field wins, so no approval is dispatched.
	ap.handleSSEEvent(string(constants.EventOperatorFileEditCompleted),
		buildSSEData(t, FileEditApprovalEventType, "approval-1", nil))
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))

	// SSE event: field is empty, inner wire says "approval.requested".
	// The inner wire type is used as the fallback, so approval is dispatched.
	ap.handleSSEEvent("", buildSSEData(t, FileEditApprovalEventType, "approval-2", nil))
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
	assert.Equal(t, 1, ap.ApprovedCount())
}

// TestHandleSSEEvent_MissingApprovalIDDoesNotDispatch asserts that an event
// with the correct type but no approval_id in the data payload does not
// trigger an approval POST.
func TestHandleSSEEvent_MissingApprovalIDDoesNotDispatch(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newApprovalTestClient(t, srv.URL)
	ap := NewApprovalAutoApprover(c, Persona{ID: "test"}, srv.URL)

	// Build a payload with an empty approval_id.
	data := buildSSEData(t, FileEditApprovalEventType, "", nil)
	ap.handleSSEEvent("", data)

	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	assert.Equal(t, 0, ap.ApprovedCount())
}

// TestHandleSSEEvent_InvalidJSONDoesNotDispatch asserts that malformed SSE
// data does not trigger an approval POST and does not panic.
func TestHandleSSEEvent_InvalidJSONDoesNotDispatch(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newApprovalTestClient(t, srv.URL)
	ap := NewApprovalAutoApprover(c, Persona{ID: "test"}, srv.URL)

	for _, bad := range []string{
		"not json",
		`{"event":"not-an-object"}`,
		`{"event":"{invalid"}`,
		`{"event":"{\"type\":\""}`,
	} {
		ap.handleSSEEvent("", bad)
	}
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	assert.Equal(t, 0, ap.ApprovedCount())
}

// TestHandleSSEEvent_Non2xxResponseDoesNotIncrementCount asserts that a 4xx/5xx
// response from the ensemble approval respond endpoint does not increment the
// approved counter.
func TestHandleSSEEvent_Non2xxResponseDoesNotIncrementCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	c := newApprovalTestClient(t, srv.URL)
	ap := NewApprovalAutoApprover(c, Persona{ID: "test"}, srv.URL)

	ap.handleSSEEvent("", buildSSEData(t, FileEditApprovalEventType, "approval-1", nil))
	assert.Equal(t, 0, ap.ApprovedCount(), "approved count should not increment on non-2xx response")
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

// TestRespondApproval_PersonaFallback asserts that when the SSE event data
// omits user_id and cli_session_id, the approval POST falls back to the
// persona's UserID and CLISessionID for both the body context and the proxy
// headers.
func TestRespondApproval_PersonaFallback(t *testing.T) {
	var (
		receivedBody   map[string]any
		receivedUserID string
		receivedCLI    string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &receivedBody))
		receivedUserID = r.Header.Get(HeaderProxyUserID)
		receivedCLI = r.Header.Get(HeaderProxyCLISessionID)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newApprovalTestClient(t, srv.URL)
	ap := NewApprovalAutoApprover(c, Persona{
		ID:           "test",
		UserID:       "persona-user",
		CLISessionID: "persona-cli",
	}, srv.URL)

	// Build SSE data with empty user_id and cli_session_id in the event payload.
	data := buildSSEData(t, FileEditApprovalEventType, "approval-1", map[string]any{
		"user_id":        "",
		"cli_session_id": "",
	})
	ap.handleSSEEvent("", data)

	ctxObj, ok := receivedBody["context"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "persona-user", ctxObj["user_id"])
	assert.Equal(t, "persona-cli", ctxObj["cli_session_id"])
	assert.Equal(t, "persona-user", receivedUserID)
	assert.Equal(t, "persona-cli", receivedCLI)
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
		ap.handleSSEEvent("", buildSSEData(t, FileEditApprovalEventType, "approval-1", nil))
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
