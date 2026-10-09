// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// ---------------------------------------------------------------------------
// handleInternalSSEStream
// ---------------------------------------------------------------------------

func TestHandleInternalSSEStream_MethodNotAllowed(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sse/stream", nil)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEStream(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleInternalSSEStream_AuthFailure(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEStream(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleInternalSSEStream_BadRequestFromMultipleTargets(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, "user1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEStream(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleInternalSSEStream_SSEHeadersSet(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, _, _ := seedCLISessionCtx(t, h, "stream-headers")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)
	req.Header.Set("Origin", "https://example.com")

	stream := startInternalSSEStream(t, h, req)
	stream.awaitSubscribed()
	stream.stop()
	rr := stream.rec

	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", rr.Header().Get("Connection"))
	assert.Equal(t, "no", rr.Header().Get("X-Accel-Buffering"))
}

func TestHandleInternalSSEStream_SSEHeadersNoOrigin(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, _, _ := seedCLISessionCtx(t, h, "stream-wildcard")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)
	// No Origin header → SSE headers still set, no CORS headers

	stream := startInternalSSEStream(t, h, req)
	stream.awaitSubscribed()
	stream.stop()
	rr := stream.rec

	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", rr.Header().Get("Connection"))
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestHandleInternalSSEStream_LastEventIDOverridesSinceID(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, userID, cliSessionID := seedCLISessionCtx(t, h, "last-event")

	// Push two events
	route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	_, err := h.dataController.sseStore.SSEEventsAppend(route, "event1", `{"event":{"type":"event1"}}`, "app1")
	require.NoError(t, err)
	_, err = h.dataController.sseStore.SSEEventsAppend(route, "event2", `{"event":{"type":"event2"}}`, "app1")
	require.NoError(t, err)

	// Get all events to find the first event's ID
	rows, err := h.dataController.sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	firstID := rows[0].ID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?since_id=0", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", fmt.Sprintf("%d", firstID))

	stream := startInternalSSEStream(t, h, req)
	stream.awaitReplayed()
	body := stream.stop()

	// Should have replayed only events after firstID (i.e., event2)
	assert.Contains(t, body, "event2")
	assert.NotContains(t, body, "event1")
}

func TestHandleInternalSSEStream_ReplaysEventsFromDB(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, userID, cliSessionID := seedCLISessionCtx(t, h, "replay")

	route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	// Push a dummy event first so the real event gets ID > 1
	_, err := h.dataController.sseStore.SSEEventsAppend(route, "dummy_event", `{"event":{"type":"dummy_event"}}`, "app1")
	require.NoError(t, err)
	_, err = h.dataController.sseStore.SSEEventsAppend(route, "replay_event", `{"event":{"type":"replay_event"}}`, "app1")
	require.NoError(t, err)

	// Get all events to find IDs
	rows, err := h.dataController.sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	dummyID := rows[0].ID
	replayID := rows[1].ID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)
	// Use dummy event ID as Last-Event-ID so replay starts after it
	req.Header.Set("Last-Event-ID", fmt.Sprintf("%d", dummyID))

	stream := startInternalSSEStream(t, h, req)
	stream.awaitReplayed()
	body := stream.stop()

	assert.Contains(t, body, "replay_event")
	assert.Contains(t, body, fmt.Sprintf("id: %d", replayID))
	assert.NotContains(t, body, "dummy_event")
}

func TestHandleInternalSSEStream_NoReplayWhenSinceIDZero(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, userID, cliSessionID := seedCLISessionCtx(t, h, "no-replay")

	route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	_, err := h.dataController.sseStore.SSEEventsAppend(route, "no_replay_event", `{"event":{"type":"no_replay_event"}}`, "app1")
	require.NoError(t, err)

	// since_id=0 and no Last-Event-ID → no replay should occur
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?since_id=0", nil).WithContext(ctx)

	stream := startInternalSSEStream(t, h, req)
	stream.awaitSubscribed()
	// A live marker proves the stream reached its live loop, past any replay.
	payload := `{"cli_session_id":"` + cliSessionID + `","event":{"type":"live_marker"}}`
	envelopeJSON, err := json.Marshal(models.SSEPublishedEvent{ID: 0, Payload: json.RawMessage(payload)})
	require.NoError(t, err)
	h.GetGatewayWebSocketHandler().Publish("sse:cli:"+cliSessionID, envelopeJSON)
	stream.awaitBody("live_marker")
	body := stream.stop()

	// Event should NOT be replayed since sinceID=0
	assert.NotContains(t, body, "no_replay_event")
}

func TestHandleInternalSSEStream_PubSubEventDelivery(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, _, cliSessionID := seedCLISessionCtx(t, h, "pubsub")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)

	stream := startInternalSSEStream(t, h, req)
	stream.awaitSubscribed()

	// Publish an event to the pubsub channel. The stream handler expects a
	// models.SSEPublishedEvent envelope (R1) carrying the DB row ID and the
	// raw SSEPushPayload JSON as the payload.
	payload := `{"cli_session_id":"` + cliSessionID + `","event":{"type":"pubsub_event","data":"hello"}}`
	pubEvent := models.SSEPublishedEvent{ID: 1, Payload: json.RawMessage(payload)}
	envelopeJSON, err := json.Marshal(pubEvent)
	require.NoError(t, err)
	h.GetGatewayWebSocketHandler().Publish("sse:cli:"+cliSessionID, envelopeJSON)

	stream.awaitBody("data: " + payload)
	body := stream.stop()
	assert.Contains(t, body, "pubsub_event")
	assert.Contains(t, body, "data: "+payload)
	assert.Contains(t, body, "id: 1")
}

// Ephemeral events are never persisted, so they are published with ID 0. They
// must still reach the client (without an `id:` field), including after a
// persisted event has advanced the dedup cursor.
func TestHandleInternalSSEStream_EphemeralEventDelivered(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, _, cliSessionID := seedCLISessionCtx(t, h, "ephemeral")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)
	stream := startInternalSSEStream(t, h, req)
	stream.awaitSubscribed()

	publish := func(id int64, name string) {
		payload := `{"cli_session_id":"` + cliSessionID + `","event":{"type":"` + name + `"}}`
		envelopeJSON, err := json.Marshal(models.SSEPublishedEvent{ID: id, Payload: json.RawMessage(payload)})
		require.NoError(t, err)
		h.GetGatewayWebSocketHandler().Publish("sse:cli:"+cliSessionID, envelopeJSON)
	}
	publish(0, "ephemeral_first")
	publish(5, "persisted")
	publish(0, "ephemeral_after_persisted")

	// Delivery is FIFO, so the last event arriving means the others did too.
	stream.awaitBody("ephemeral_after_persisted")
	body := stream.stop()
	assert.Contains(t, body, "ephemeral_first")
	assert.Contains(t, body, "persisted")
	assert.Contains(t, body, "ephemeral_after_persisted")
	assert.NotContains(t, body, "id: 0")
}

func TestHandleInternalSSEStream_HeartbeatSent(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	// Override to a short interval so the test can observe a real heartbeat.
	h.sseController.heartbeat = 50 * time.Millisecond

	ctx, _, _ := seedCLISessionCtx(t, h, "heartbeat")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)

	stream := startInternalSSEStream(t, h, req)
	stream.awaitBody(": heartbeat\n\n")
	body := stream.stop()
	rr := stream.rec

	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
	assert.Contains(t, body, ": heartbeat\n\n", "expected at least one heartbeat comment in SSE stream")
}

func TestHandleInternalSSEStream_ClientLabelOperatorSession(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, _, _ := seedCLISessionCtx(t, h, "label")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)

	stream := startInternalSSEStream(t, h, req)
	stream.awaitSubscribed()
	stream.stop()
	rr := stream.rec

	// Stream should be established — just verify it didn't error
	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
}

func TestHandleInternalSSEStream_ClientLabelWebSession(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	webSessionID := "web-label"
	userID := "user-web-label"

	ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, webSessionID)
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	// Security: web_session_id is NOT passed in the URL. It is derived from
	// the context (set by the auth middleware from the session cookie).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)

	stream := startInternalSSEStream(t, h, req)
	stream.awaitSubscribed()
	stream.stop()
	rr := stream.rec

	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
}

// ---------------------------------------------------------------------------
// R14 regression: stream emits id: and data: only, no event: field
// ---------------------------------------------------------------------------

func TestHandleInternalSSEStream_ReplayEmitsNoEventField(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, userID, cliSessionID := seedCLISessionCtx(t, h, "no-event-field")

	route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	_, err := h.dataController.sseStore.SSEEventsAppend(route, "test_type", `{"event":{"type":"test_type"}}`, "app1")
	require.NoError(t, err)

	rows, err := h.dataController.sseStore.SSEEventsListSince(route, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	eventID := rows[0].ID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)

	stream := startInternalSSEStream(t, h, req)
	stream.awaitReplayed()
	body := stream.stop()

	assert.Contains(t, body, fmt.Sprintf("id: %d", eventID))
	assert.Contains(t, body, "data: ")
	// R14: no SSE "event:" field should be present at the start of any line.
	// The JSON payload may contain "event": as a key, so we check for the SSE
	// field prefix specifically (line starts with "event:").
	for _, line := range strings.Split(body, "\n") {
		assert.False(t, strings.HasPrefix(line, "event:"), "stream should not emit SSE event: field, found: %s", line)
	}
}

// ---------------------------------------------------------------------------
// R1/R2 regression: duplicate suppression by lastEmittedID
// ---------------------------------------------------------------------------

func TestHandleInternalSSEStream_DuplicatePubSubEventSuppressed(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, userID, cliSessionID := seedCLISessionCtx(t, h, "dedup")

	route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	// Insert two events: event1 (ID 1) and event2 (ID 2).
	_, err := h.dataController.sseStore.SSEEventsAppend(route, "event1_type", `{"event":{"type":"event1_type"}}`, "app1")
	require.NoError(t, err)
	_, err = h.dataController.sseStore.SSEEventsAppend(route, "event2_type", `{"event":{"type":"event2_type"}}`, "app1")
	require.NoError(t, err)

	rows, err := h.dataController.sseStore.SSEEventsListSince(route, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	event2ID := rows[1].ID

	// Last-Event-ID=event1.ID so replay returns only event2 and sets lastEmittedID=event2.ID.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", fmt.Sprintf("%d", rows[0].ID))

	stream := startInternalSSEStream(t, h, req)
	// Replay complete: event2 is replayed, lastEmittedID = event2ID.
	stream.awaitReplayed()

	// Publish a duplicate event with the same ID as the replayed row.
	// The stream handler should suppress it via lastEmittedID dedup.
	dupPayload := `{"event":{"type":"event2_type"}}`
	dupEvent := models.SSEPublishedEvent{ID: event2ID, Payload: json.RawMessage(dupPayload)}
	dupJSON, err := json.Marshal(dupEvent)
	require.NoError(t, err)
	h.GetGatewayWebSocketHandler().Publish("sse:cli:"+cliSessionID, dupJSON)

	// Also publish a new event with a higher ID — should be delivered.
	newPayload := `{"event":{"type":"live_type"}}`
	newEvent := models.SSEPublishedEvent{ID: event2ID + 1, Payload: json.RawMessage(newPayload)}
	newJSON, err := json.Marshal(newEvent)
	require.NoError(t, err)
	h.GetGatewayWebSocketHandler().Publish("sse:cli:"+cliSessionID, newJSON)

	// Delivery is FIFO, so the live event arriving means the duplicate before
	// it was already handled.
	stream.awaitBody("live_type")
	body := stream.stop()
	// event2_type should appear exactly once (via replay, not via pub/sub duplicate).
	count := strings.Count(body, "event2_type")
	assert.Equal(t, 1, count, "event2_type should appear exactly once (duplicate suppressed), got %d\nbody: %s", count, body)
	// The new live event should be delivered.
	assert.Contains(t, body, "live_type")
}

// Row IDs are assigned in commit order but published after commit by
// concurrent writers, so live events reach a stream out of ID order. A lower
// ID arriving after a higher one was not replayed and must still be delivered.
func TestHandleInternalSSEStream_OutOfOrderLiveEventsDelivered(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, _, cliSessionID := seedCLISessionCtx(t, h, "outoforder")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?since_id=0", nil).WithContext(ctx)
	stream := startInternalSSEStream(t, h, req)
	stream.awaitSubscribed()

	for _, e := range []struct {
		id   int64
		kind string
	}{{12, "higher_type"}, {11, "lower_type"}} {
		raw, err := json.Marshal(models.SSEPublishedEvent{ID: e.id, Payload: json.RawMessage(fmt.Sprintf(`{"event":{"type":%q}}`, e.kind))})
		require.NoError(t, err)
		h.GetGatewayWebSocketHandler().Publish("sse:cli:"+cliSessionID, raw)
	}

	stream.awaitBody("lower_type")
	body := stream.stop()
	assert.Contains(t, body, "higher_type")
	assert.Contains(t, body, "lower_type", "an event with a lower ID than one already emitted live must not be dropped as a duplicate\nbody: %s", body)
}

// ---------------------------------------------------------------------------
// R3 regression: write error terminates stream goroutine
// ---------------------------------------------------------------------------

func TestHandleInternalSSEStream_WriteErrorTerminatesGoroutine(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, userID, cliSessionID := seedCLISessionCtx(t, h, "write-err")

	route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	_, err := h.dataController.sseStore.SSEEventsAppend(route, "test_type", `{"event":{"type":"test_type"}}`, "app1")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)
	// Set Last-Event-ID to 0 so replay is triggered.
	req.Header.Set("Last-Event-ID", "0")

	// errorWriter always returns an error on Write to simulate a broken pipe.
	ew := &errorWriter{header: make(http.Header)}
	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(ew, req)
		close(done)
	}()

	// The handler should return quickly because the first Fprintf fails.
	select {
	case <-done:
		// Success: goroutine terminated on write error.
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("stream goroutine did not terminate within 2s after write error")
	}
	cancel()
	<-done
}

// ---------------------------------------------------------------------------
// R6 regression: truncation sentinel when replay hits the limit
// ---------------------------------------------------------------------------

func TestHandleInternalSSEStream_TruncationSentinelOnFullReplay(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx, userID, cliSessionID := seedCLISessionCtx(t, h, "truncation")

	route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	// Insert exactly 1000 events to trigger the truncation sentinel.
	for i := 0; i < 1000; i++ {
		_, err := h.dataController.sseStore.SSEEventsAppend(route, "trunc_type", `{"event":{"type":"trunc_type"}}`, "app1")
		require.NoError(t, err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", "0")

	stream := startInternalSSEStream(t, h, req)
	stream.awaitBody(`"type":"truncated"`)
	body := stream.stop()

	// R6: the truncation sentinel must be present.
	assert.Contains(t, body, `"type":"truncated"`)
	assert.Contains(t, body, `"limit":1000`)
}

// ---------------------------------------------------------------------------
// Browser cookie auth through the full router + auth middleware
// ---------------------------------------------------------------------------

// TestSSEStream_BrowserWebSessionCookieThroughRouter drives the real router
// (auth middleware included) with only a web session cookie, the way the
// Console's EventSource does. Every other stream test seeds context directly
// and so cannot catch a middleware-to-handler identity gap.
func TestSSEStream_BrowserWebSessionCookieThroughRouter(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)
	require.NotNil(t, h.authMiddleware)

	userID := "user-browser-sse"
	webSessionID := "web-browser-sse"
	userBytes, err := json.Marshal(&models.User{ID: userID, Status: constants.UserStatusActive})
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(t.Context(), marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))
	sessBytes, err := json.Marshal(&models.WebSession{
		ID:              webSessionID,
		UserID:          userID,
		CreatedAtUnixMs: time.Now().UnixMilli(),
		ExpiresAtUnixMs: time.Now().Add(time.Hour).UnixMilli(),
	})
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(t.Context(), marshaler.CollectionName(constants.CollectionWebSessions), webSessionID, sessBytes))

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.SSEStream, nil)
	req.AddCookie(&http.Cookie{Name: constants.WebSessionCookieName, Value: webSessionID})

	stream := startSSEStream(t, h, req)
	stream.awaitSubscribed()
	body := stream.stop()
	rr := stream.rec

	assert.Equal(t, http.StatusOK, rr.Code(), "body: %s", body)
	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
}
