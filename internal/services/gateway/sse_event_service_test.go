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
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSSEEventServiceTest(t *testing.T) (*SSEEventService, *CanonicalDBService) {
	t.Helper()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	vaultDir := filepath.Join(dbDir, constants.VaultDirname)

	db, err := OpenCanonicalDBService(dbDir, secretsDir, vaultDir, logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	return db.SSEStore, db
}

func TestSSERoute_Validate(t *testing.T) {
	t.Run("Validate accepts WebSessionID", func(t *testing.T) {
		route := SSERoute{WebSessionID: "web-session-123"}
		err := route.validate()
		assert.NoError(t, err)
	})

	t.Run("Validate accepts CLISessionID", func(t *testing.T) {
		route := SSERoute{CLISessionID: "cli-session-456"}
		err := route.validate()
		assert.NoError(t, err)
	})

	t.Run("Validate accepts UserID", func(t *testing.T) {
		route := SSERoute{UserID: "user-789"}
		err := route.validate()
		assert.NoError(t, err)
	})

	t.Run("Validate rejects empty route (no IDs set)", func(t *testing.T) {
		route := SSERoute{}
		err := route.validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sse route requires exactly one")
	})

	t.Run("Validate rejects multiple IDs set", func(t *testing.T) {
		route := SSERoute{
			WebSessionID: "web-session-123",
			CLISessionID: "cli-session-456",
		}
		err := route.validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sse route is mutually-exclusive")
	})

	t.Run("Validate rejects all three IDs set", func(t *testing.T) {
		route := SSERoute{
			WebSessionID: "web-session-123",
			CLISessionID: "cli-session-456",
			UserID:       "user-789",
		}
		err := route.validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sse route is mutually-exclusive")
	})
}

func TestNullIfEmpty(t *testing.T) {

	t.Run("nullIfEmpty returns nil for empty string", func(t *testing.T) {
		result := nullIfEmpty("")
		assert.Nil(t, result)
	})

	t.Run("nullIfEmpty returns string for non-empty", func(t *testing.T) {
		result := nullIfEmpty("test-value")
		assert.Equal(t, "test-value", result)
	})

	t.Run("nullIfEmpty returns string for whitespace", func(t *testing.T) {
		result := nullIfEmpty("   ")
		assert.Equal(t, "   ", result)
	})
}

func TestSSEEventService_SSEEventsAppend(t *testing.T) {

	t.Run("SSEEventsAppend with WebSessionID", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-1"}
		err := sseSvc.SSEEventsAppend(route, "test-event", `{"data":"value"}`, "producer-1")
		require.NoError(t, err)

		// Verify event was inserted
		count, err := sseSvc.SSEEventsCount()
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("SSEEventsAppend with CLISessionID", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{CLISessionID: "cli-session-1"}
		err := sseSvc.SSEEventsAppend(route, "cli-event", `{"cli":"data"}`, "producer-2")
		require.NoError(t, err)

		count, err := sseSvc.SSEEventsCount()
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("SSEEventsAppend with UserID", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{UserID: "user-1"}
		err := sseSvc.SSEEventsAppend(route, "user-event", `{"user":"data"}`, "producer-3")
		require.NoError(t, err)

		count, err := sseSvc.SSEEventsCount()
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("SSEEventsAppend with empty producerID", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-2"}
		err := sseSvc.SSEEventsAppend(route, "test-event", `{"data":"value"}`, "")
		require.NoError(t, err)

		count, err := sseSvc.SSEEventsCount()
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("SSEEventsAppend rejects invalid route", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{} // No IDs set
		err := sseSvc.SSEEventsAppend(route, "test-event", `{"data":"value"}`, "producer-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sse route requires exactly one")
	})

	t.Run("SSEEventsAppend with large payload", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-3"}
		largePayload := make([]byte, 1024*100) // 100KB
		for i := range largePayload {
			largePayload[i] = byte(i % 256)
		}
		err := sseSvc.SSEEventsAppend(route, "large-event", string(largePayload), "producer-4")
		require.NoError(t, err)

		count, err := sseSvc.SSEEventsCount()
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("SSEEventsAppend with special characters in payload", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-4"}
		specialPayload := `{"key":"value with \n\t\r\"quotes\" and 'apostrophes'"}`
		err := sseSvc.SSEEventsAppend(route, "special-event", specialPayload, "producer-5")
		require.NoError(t, err)

		events, err := sseSvc.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, specialPayload, events[0].Payload)
	})
}

func TestSSEEventService_SSEEventsCount(t *testing.T) {

	t.Run("SSEEventsCount returns 0 for empty table", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		count, err := sseSvc.SSEEventsCount()
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	t.Run("SSEEventsCount returns correct count after inserts", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-count"}
		for i := 0; i < 5; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)
		}

		count, err := sseSvc.SSEEventsCount()
		require.NoError(t, err)
		assert.Equal(t, int64(5), count)
	})
}

func TestSSEEventService_SSEEventsWipe(t *testing.T) {

	t.Run("SSEEventsWipe deletes all rows", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		// Insert some events
		route := SSERoute{WebSessionID: "web-session-wipe"}
		for i := 0; i < 10; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)
		}

		// Verify count
		count, err := sseSvc.SSEEventsCount()
		require.NoError(t, err)
		assert.Equal(t, int64(10), count)

		// Wipe
		deleted, err := sseSvc.SSEEventsWipe()
		require.NoError(t, err)
		assert.Equal(t, int64(10), deleted)

		// Verify empty
		count, err = sseSvc.SSEEventsCount()
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	t.Run("SSEEventsWipe returns 0 for empty table", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		deleted, err := sseSvc.SSEEventsWipe()
		require.NoError(t, err)
		assert.Equal(t, int64(0), deleted)
	})
}

func TestSSEEventService_SSEEventsListSince(t *testing.T) {

	t.Run("SSEEventsListSince with WebSessionID", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route1 := SSERoute{WebSessionID: "web-session-list-1"}
		route2 := SSERoute{WebSessionID: "web-session-list-2"}

		// Insert events for different sessions
		err := sseSvc.SSEEventsAppend(route1, "event-1", `{"session":"1"}`, "producer")
		require.NoError(t, err)
		err = sseSvc.SSEEventsAppend(route2, "event-2", `{"session":"2"}`, "producer")
		require.NoError(t, err)
		err = sseSvc.SSEEventsAppend(route1, "event-3", `{"session":"1"}`, "producer")
		require.NoError(t, err)

		// List events for route1
		events, err := sseSvc.SSEEventsListSince(route1, 0, 10)
		require.NoError(t, err)
		assert.Len(t, events, 2)
		assert.Equal(t, "web-session-list-1", events[0].WebSessionID)
		assert.Equal(t, "event-1", events[0].EventType)
		assert.Equal(t, "event-3", events[1].EventType)
	})

	t.Run("SSEEventsListSince with CLISessionID", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{CLISessionID: "cli-session-list"}
		err := sseSvc.SSEEventsAppend(route, "cli-event", `{"cli":"data"}`, "producer")
		require.NoError(t, err)

		events, err := sseSvc.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "cli-session-list", events[0].CLISessionID)
	})

	t.Run("SSEEventsListSince with UserID", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{UserID: "user-list"}
		err := sseSvc.SSEEventsAppend(route, "user-event", `{"user":"data"}`, "producer")
		require.NoError(t, err)

		events, err := sseSvc.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "user-list", events[0].UserID)
	})

	t.Run("SSEEventsListSince respects sinceID", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-since"}
		for i := 0; i < 5; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)
		}

		// Get all events to find IDs
		allEvents, err := sseSvc.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		require.Len(t, allEvents, 5)

		// List since ID 2 (should return 3 events)
		events, err := sseSvc.SSEEventsListSince(route, allEvents[1].ID, 10)
		require.NoError(t, err)
		assert.Len(t, events, 3)
	})

	t.Run("SSEEventsListSince respects limit", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-limit"}
		for i := 0; i < 10; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)
		}

		events, err := sseSvc.SSEEventsListSince(route, 0, 5)
		require.NoError(t, err)
		assert.Len(t, events, 5)
	})

	t.Run("SSEEventsListSince defaults limit to 200 for invalid values", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-default"}
		for i := 0; i < 5; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)
		}

		// Test with limit 0
		events, err := sseSvc.SSEEventsListSince(route, 0, 0)
		require.NoError(t, err)
		assert.Len(t, events, 5)

		// Test with negative limit
		events, err = sseSvc.SSEEventsListSince(route, 0, -1)
		require.NoError(t, err)
		assert.Len(t, events, 5)

		// Test with limit > 1000
		events, err = sseSvc.SSEEventsListSince(route, 0, 2000)
		require.NoError(t, err)
		assert.Len(t, events, 5)
	})

	t.Run("SSEEventsListSince returns empty for non-existent route", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "non-existent"}
		events, err := sseSvc.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		assert.Len(t, events, 0)
	})

	t.Run("SSEEventsListSince rejects invalid route", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{} // No IDs set
		_, err := sseSvc.SSEEventsListSince(route, 0, 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sse route requires exactly one")
	})

	t.Run("SSEEventsListSince returns events in ascending ID order", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-order"}
		for i := 0; i < 3; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)
		}

		events, err := sseSvc.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		require.Len(t, events, 3)

		// Verify ascending order
		assert.True(t, events[0].ID < events[1].ID)
		assert.True(t, events[1].ID < events[2].ID)
	})
}

func TestSSEEventService_SSEEventsListAllSince(t *testing.T) {

	t.Run("SSEEventsListAllSince returns events across all routes", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		// Insert events for different routes
		route1 := SSERoute{WebSessionID: "web-session-all-1"}
		route2 := SSERoute{CLISessionID: "cli-session-all"}
		route3 := SSERoute{UserID: "user-all"}

		err := sseSvc.SSEEventsAppend(route1, "web-event", `{"type":"web"}`, "producer")
		require.NoError(t, err)
		err = sseSvc.SSEEventsAppend(route2, "cli-event", `{"type":"cli"}`, "producer")
		require.NoError(t, err)
		err = sseSvc.SSEEventsAppend(route3, "user-event", `{"type":"user"}`, "producer")
		require.NoError(t, err)

		// List all events
		events, err := sseSvc.SSEEventsListAllSince(0, 10)
		require.NoError(t, err)
		assert.Len(t, events, 3)

		// Verify we have events from all routes
		hasWeb := false
		hasCLI := false
		hasUser := false
		for _, event := range events {
			if event.WebSessionID != "" {
				hasWeb = true
			}
			if event.CLISessionID != "" {
				hasCLI = true
			}
			if event.UserID != "" {
				hasUser = true
			}
		}
		assert.True(t, hasWeb, "Should have web session event")
		assert.True(t, hasCLI, "Should have CLI session event")
		assert.True(t, hasUser, "Should have user event")
	})

	t.Run("SSEEventsListAllSince respects sinceID", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-all-since"}
		for i := 0; i < 5; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)
		}

		// Get all events to find IDs
		allEvents, err := sseSvc.SSEEventsListAllSince(0, 10)
		require.NoError(t, err)
		require.Len(t, allEvents, 5)

		// List since ID 2
		events, err := sseSvc.SSEEventsListAllSince(allEvents[1].ID, 10)
		require.NoError(t, err)
		assert.Len(t, events, 3)
	})

	t.Run("SSEEventsListAllSince respects limit", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-all-limit"}
		for i := 0; i < 10; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)
		}

		events, err := sseSvc.SSEEventsListAllSince(0, 5)
		require.NoError(t, err)
		assert.Len(t, events, 5)
	})

	t.Run("SSEEventsListAllSince defaults limit to 200 for invalid values", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-all-default"}
		for i := 0; i < 5; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)
		}

		// Test with limit 0
		events, err := sseSvc.SSEEventsListAllSince(0, 0)
		require.NoError(t, err)
		assert.Len(t, events, 5)

		// Test with limit > 1000
		events, err = sseSvc.SSEEventsListAllSince(0, 2000)
		require.NoError(t, err)
		assert.Len(t, events, 5)
	})

	t.Run("SSEEventsListAllSince returns empty for empty table", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		events, err := sseSvc.SSEEventsListAllSince(0, 10)
		require.NoError(t, err)
		assert.Len(t, events, 0)
	})

	t.Run("SSEEventsListAllSince returns events in ascending ID order", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-all-order"}
		for i := 0; i < 3; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)
		}

		events, err := sseSvc.SSEEventsListAllSince(0, 10)
		require.NoError(t, err)
		require.Len(t, events, 3)

		// Verify ascending order
		assert.True(t, events[0].ID < events[1].ID)
		assert.True(t, events[1].ID < events[2].ID)
	})
}

func TestSSEEventService_RouteIsolation(t *testing.T) {

	t.Run("Events are isolated by route type", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		webRoute := SSERoute{WebSessionID: "web-session-iso"}
		cliRoute := SSERoute{CLISessionID: "cli-session-iso"}
		userRoute := SSERoute{UserID: "user-iso"}

		// Insert events for each route type
		err := sseSvc.SSEEventsAppend(webRoute, "web-event", `{"type":"web"}`, "producer")
		require.NoError(t, err)
		err = sseSvc.SSEEventsAppend(cliRoute, "cli-event", `{"type":"cli"}`, "producer")
		require.NoError(t, err)
		err = sseSvc.SSEEventsAppend(userRoute, "user-event", `{"type":"user"}`, "producer")
		require.NoError(t, err)

		// Verify each route only sees its own events
		webEvents, err := sseSvc.SSEEventsListSince(webRoute, 0, 10)
		require.NoError(t, err)
		assert.Len(t, webEvents, 1)
		assert.Equal(t, "web-event", webEvents[0].EventType)

		cliEvents, err := sseSvc.SSEEventsListSince(cliRoute, 0, 10)
		require.NoError(t, err)
		assert.Len(t, cliEvents, 1)
		assert.Equal(t, "cli-event", cliEvents[0].EventType)

		userEvents, err := sseSvc.SSEEventsListSince(userRoute, 0, 10)
		require.NoError(t, err)
		assert.Len(t, userEvents, 1)
		assert.Equal(t, "user-event", userEvents[0].EventType)
	})

	t.Run("Events are isolated by session ID within same type", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route1 := SSERoute{WebSessionID: "web-session-iso-1"}
		route2 := SSERoute{WebSessionID: "web-session-iso-2"}

		err := sseSvc.SSEEventsAppend(route1, "event-1", `{"session":"1"}`, "producer")
		require.NoError(t, err)
		err = sseSvc.SSEEventsAppend(route2, "event-2", `{"session":"2"}`, "producer")
		require.NoError(t, err)

		events1, err := sseSvc.SSEEventsListSince(route1, 0, 10)
		require.NoError(t, err)
		assert.Len(t, events1, 1)
		assert.Equal(t, "event-1", events1[0].EventType)

		events2, err := sseSvc.SSEEventsListSince(route2, 0, 10)
		require.NoError(t, err)
		assert.Len(t, events2, 1)
		assert.Equal(t, "event-2", events2[0].EventType)
	})
}

func TestSSEEventService_EventDataIntegrity(t *testing.T) {

	t.Run("Event fields are preserved correctly", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-integrity"}
		eventType := "test-event-type"
		payload := `{"key":"value","number":123,"nested":{"field":"data"}}`
		producerID := "test-producer-123"

		err := sseSvc.SSEEventsAppend(route, eventType, payload, producerID)
		require.NoError(t, err)

		events, err := sseSvc.SSEEventsListSince(route, 0, 10)
		require.NoError(t, err)
		require.Len(t, events, 1)

		event := events[0]
		assert.Equal(t, eventType, event.EventType)
		assert.Equal(t, payload, event.Payload)
		assert.Equal(t, "web-session-integrity", event.WebSessionID)
		assert.NotEmpty(t, event.CreatedAt)
		assert.Greater(t, event.ID, int64(0))
	})

	t.Run("Event ID increments correctly", func(t *testing.T) {
		sseSvc, _ := setupSSEEventServiceTest(t)
		route := SSERoute{WebSessionID: "web-session-increment"}
		var lastID int64 = 0

		for i := 0; i < 5; i++ {
			err := sseSvc.SSEEventsAppend(route, "event", `{"data":"value"}`, "producer")
			require.NoError(t, err)

			events, err := sseSvc.SSEEventsListSince(route, 0, 10)
			require.NoError(t, err)
			currentID := events[len(events)-1].ID
			assert.Greater(t, currentID, lastID)
			lastID = currentID
		}
	})
}
