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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// ---------------------------------------------------------------------------
// handleInternalSSEEvents
// ---------------------------------------------------------------------------

func TestHandleInternalSSEEvents_MethodNotAllowed(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sse/events", nil)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEEvents(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleInternalSSEEvents_AuthFailure(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEEvents(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleInternalSSEEvents_BadRequestFromMultipleTargets(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, "user1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEEvents(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleInternalSSEEvents_Success(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-events-ok"
	userID := "user-events-ok"
	cliSessionID := "cli-events-ok"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	// Push an event first
	route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	_, err := h.dataController.sseStore.SSEEventsAppend(route, "test_event", `{"event":{"type":"test_event"}}`, "test-app")
	require.NoError(t, err)

	// Query events with context-stamped auth
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, cliSessionID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events?since_id=0&limit=10", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEEvents(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp models.SSEEventsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Count)
	assert.Len(t, resp.Events, 1)
	assert.Equal(t, "test_event", resp.Events[0].EventType)
}

func TestHandleInternalSSEEvents_EmptyResult(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-empty"
	userID := "user-empty"
	cliSessionID := "cli-empty"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, cliSessionID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEEvents(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp models.SSEEventsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Count)
	assert.Empty(t, resp.Events)
}
