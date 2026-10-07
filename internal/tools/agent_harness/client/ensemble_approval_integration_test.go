// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version. 2.0.

//go:build integration

package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

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

	ap.handleSSEEvent(t.Context(), "", buildSSEData(t, FileEditApprovalEventType, approveID, nil))

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

	ap.handleSSEEvent(t.Context(), "", buildSSEData(t, FileEditApprovalEventType, "approval-1", nil))

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
	ap.handleSSEEvent(t.Context(), "", buildSSEData(t, string(constants.EventOperatorFileEditCompleted), "approval-1", nil))
	// Event type passed via the SSE event: field but inner wire type differs.
	ap.handleSSEEvent(t.Context(), string(constants.EventOperatorFileEditCompleted),
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
	ap.handleSSEEvent(t.Context(), string(constants.EventOperatorFileEditCompleted),
		buildSSEData(t, FileEditApprovalEventType, "approval-1", nil))
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))

	// SSE event: field is empty, inner wire says "approval.requested".
	// The inner wire type is used as the fallback, so approval is dispatched.
	ap.handleSSEEvent(t.Context(), "", buildSSEData(t, FileEditApprovalEventType, "approval-2", nil))
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
	ap.handleSSEEvent(t.Context(), "", data)

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
		ap.handleSSEEvent(t.Context(), "", bad)
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

	ap.handleSSEEvent(t.Context(), "", buildSSEData(t, FileEditApprovalEventType, "approval-1", nil))
	assert.Equal(t, 0, ap.ApprovedCount(), "approved count should not increment on non-2xx response")
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
	ap.handleSSEEvent(t.Context(), "", data)

	ctxObj, ok := receivedBody["context"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "persona-user", ctxObj["user_id"])
	assert.Equal(t, "persona-cli", ctxObj["cli_session_id"])
	assert.Equal(t, "persona-user", receivedUserID)
	assert.Equal(t, "persona-cli", receivedCLI)
}

func TestApprovalResponseRequiresBearerSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer data-session" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	ap := NewApprovalAutoApprover(newApprovalTestClient(t, srv.URL), Persona{UserID: "user-123", CLISessionID: "cli-session-123", OperatorSessionID: "data-session"}, srv.URL)
	ap.handleSSEEvent(t.Context(), "", buildSSEData(t, FileEditApprovalEventType, "approval-1", nil))
	assert.Equal(t, 1, ap.ApprovedCount())
}

func TestApprovalResponseRejectsRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusFound) }))
	t.Cleanup(srv.Close)
	ap := NewApprovalAutoApprover(newApprovalTestClient(t, srv.URL), Persona{}, srv.URL)
	ap.handleSSEEvent(t.Context(), "", buildSSEData(t, FileEditApprovalEventType, "approval-1", nil))
	assert.Zero(t, ap.ApprovedCount())
}
