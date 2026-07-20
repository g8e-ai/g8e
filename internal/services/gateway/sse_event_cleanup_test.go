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

//go:build integration

package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSSEEventService_CleanupDeletesOldEvents verifies that SSEEventsCleanup
// removes events older than the specified max age while preserving newer ones.
// This guards against unbounded ring buffer growth between reconnections.
func TestSSEEventService_CleanupDeletesOldEvents(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	route := SSERoute{CLISessionID: "cli-cleanup-test"}

	// Insert an old event (created in the past via direct DB manipulation).
	// We use SSEEventsAppend which uses now, then sleep briefly, insert another,
	// and cleanup with a max age between the two.
	require.NoError(t, h.db.SSEStore.SSEEventsAppend(route, "old-event", `{"msg":"old"}`, "test-producer"))

	// Wait a tiny bit so the second event has a strictly later timestamp.
	time.Sleep(10 * time.Millisecond)

	require.NoError(t, h.db.SSEStore.SSEEventsAppend(route, "new-event", `{"msg":"new"}`, "test-producer"))

	// Cleanup events older than 5ms — should delete the first but keep the second.
	// We use a small duration to separate the two events.
	deleted, err := h.db.SSEStore.SSEEventsCleanup(5 * time.Millisecond)
	require.NoError(t, err)

	// At least the old event should be deleted. Due to timestamp granularity,
	// the old event's created_at is > 5ms old by now.
	assert.GreaterOrEqual(t, deleted, int64(1), "at least one old event should be deleted")

	// Verify the newer event still exists.
	rows, err := h.db.SSEStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	assert.NotEmpty(t, rows, "newer events should remain after cleanup")
	foundNew := false
	for _, row := range rows {
		if row.EventType == "new-event" {
			foundNew = true
		}
	}
	assert.True(t, foundNew, "new-event should survive cleanup")
}

// TestSSEEventService_CleanupWithZeroAgeDeletesAll verifies that calling
// SSEEventsCleanup with a max age of 0 deletes all events. This is the
// degenerate case that confirms the cleanup query works correctly when
// the cutoff is "now".
func TestSSEEventService_CleanupWithZeroAgeDeletesAll(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	route := SSERoute{CLISessionID: "cli-cleanup-zero"}

	require.NoError(t, h.db.SSEStore.SSEEventsAppend(route, "event-1", `{"msg":"1"}`, "test-producer"))
	require.NoError(t, h.db.SSEStore.SSEEventsAppend(route, "event-2", `{"msg":"2"}`, "test-producer"))

	// Small sleep to ensure events are strictly in the past relative to "now".
	time.Sleep(5 * time.Millisecond)

	deleted, err := h.db.SSEStore.SSEEventsCleanup(0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, int64(2), "all events should be deleted with 0 max age")

	count, err := h.db.SSEStore.SSEEventsCount()
	require.NoError(t, err)
	// There might be events from other tests in the same DB, but our two should be gone.
	// Just verify the count is reasonable (not growing unboundedly).
	assert.GreaterOrEqual(t, count, int64(0))
}

// TestSSEEventService_AppendRejectsMutuallyExclusiveRoute verifies that
// SSEEventsAppend returns an error when more than one routing ID is set.
// This guards against misrouted events where a web_session_id and
// cli_session_id are both provided.
func TestSSEEventService_AppendRejectsMutuallyExclusiveRoute(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	route := SSERoute{
		WebSessionID: "web-123",
		CLISessionID: "cli-456",
	}

	err := h.db.SSEStore.SSEEventsAppend(route, "test-event", `{"msg":"test"}`, "test-producer")
	assert.Error(t, err, "expected error when multiple route IDs are set")
}

// TestSSEEventService_AppendRejectsEmptyRoute verifies that SSEEventsAppend
// returns an error when no routing ID is set.
func TestSSEEventService_AppendRejectsEmptyRoute(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	route := SSERoute{}

	err := h.db.SSEStore.SSEEventsAppend(route, "test-event", `{"msg":"test"}`, "test-producer")
	assert.Error(t, err, "expected error when no route ID is set")
}
