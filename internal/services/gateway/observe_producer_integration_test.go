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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// newObserveProducerTestEnv creates a real SQLite DB with the full gateway
// schema (documents + sse_events tables) and returns an ObserveProducerService
// backed by a DocumentStoreService, SSEEventService, and pubsub handler. The
// SSEEventService is also returned so tests can query emitted events.
func newObserveProducerTestEnv(t *testing.T) (*ObserveProducerService, *SSEEventService, *DocumentStoreService) {
	t.Helper()
	logger := testutil.NewTestLogger()
	cfg := sqliteutil.DefaultDBConfig(":memory:")
	db, err := sqliteutil.OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)

	docStore := NewDocumentStoreService(db, logger)
	sseStore := NewSSEEventService(db, logger)
	pubsub := NewGatewayWebSocketHandler(logger)
	producer := NewObserveProducerService(docStore, sseStore, pubsub, logger)
	return producer, sseStore, docStore
}

// producerRoute returns a valid SSERoute for testing with a web session target.
func producerRoute(userID string) SSERoute {
	return SSERoute{UserID: userID, WebSessionID: "web-session-test"}
}

func TestObserveProducer_UpdateAgentState_PersistsProjectionBeforeSSEEvent(t *testing.T) {
	producer, sseStore, docStore := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	payload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-triage",
		DisplayName:   "Triage",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    now,
	}

	err := producer.UpdateAgentState(ctx, "user-1", producerRoute("user-1"), payload)
	require.NoError(t, err)

	// Projection persisted to the document store.
	collection := marshaler.CollectionName(constants.CollectionObserveAgentStates)
	doc, err := docStore.DocGet(collection, "agent-triage")
	require.NoError(t, err)
	require.NotNil(t, doc)
	var proj agentStateProjection
	require.NoError(t, unmarshalDocData(doc, &proj))
	assert.Equal(t, "user-1", proj.UserID)
	assert.Equal(t, "agent-triage", proj.AgentID)
	assert.Equal(t, models.AgentLifecycleStatusRunning, proj.Status)
	assert.Equal(t, models.SnapshotFreshnessObserved, proj.Freshness)

	// SSE event emitted with the correct event type.
	route := producerRoute("user-1")
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAppAgentStatusUpdated), rows[0].EventType)
	assert.Equal(t, "user-1", rows[0].UserID)
	assert.Equal(t, "web-session-test", rows[0].WebSessionID)
}

func TestObserveProducer_UpdateAgentState_SSEPayloadContainsNestedEnvelope(t *testing.T) {
	producer, sseStore, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	payload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-sage",
		DisplayName:   "Sage",
		Role:          "sage",
		Status:        models.AgentLifecycleStatusQueued,
		RunID:         "run-1",
		ObservedAt:    now,
	}

	err := producer.UpdateAgentState(ctx, "user-2", producerRoute("user-2"), payload)
	require.NoError(t, err)

	route := producerRoute("user-2")
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	// The stored payload is a full SSEPushPayload JSON with a nested event.
	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	assert.Equal(t, "user-2", pushPayload.UserID)
	assert.Equal(t, "web-session-test", pushPayload.WebSessionID)

	// The nested event has type and data.
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	assert.Equal(t, string(constants.EventAppAgentStatusUpdated), env.Type)

	var agentPayload models.AgentStatusUpdatedPayload
	require.NoError(t, json.Unmarshal(env.Data, &agentPayload))
	assert.Equal(t, "agent-sage", agentPayload.AgentID)
	assert.Equal(t, models.AgentLifecycleStatusQueued, agentPayload.Status)
	assert.Equal(t, "run-1", agentPayload.RunID)
}

func TestObserveProducer_UpdateRunState_PersistsProjectionBeforeSSEEvent(t *testing.T) {
	producer, sseStore, docStore := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	startedAt := now.Add(-1 * time.Minute)
	payload := models.RunStatusUpdatedPayload{
		SchemaVersion:  constants.ObserveAPIReadModelSchemaVersion,
		RunID:          "run-investigation-1",
		RunKind:        models.RunKindInvestigation,
		DisplayName:    "Investigation Alpha",
		Status:         models.RunLifecycleStatusRunning,
		CompletedTasks: 1,
		TotalTasks:     5,
		StartedAt:      &startedAt,
		ObservedAt:     now,
	}

	err := producer.UpdateRunState(ctx, "user-3", producerRoute("user-3"), payload)
	require.NoError(t, err)

	// Projection persisted.
	collection := marshaler.CollectionName(constants.CollectionObserveRuns)
	doc, err := docStore.DocGet(collection, "run-investigation-1")
	require.NoError(t, err)
	require.NotNil(t, doc)
	var proj runProjection
	require.NoError(t, unmarshalDocData(doc, &proj))
	assert.Equal(t, "user-3", proj.UserID)
	assert.Equal(t, "run-investigation-1", proj.RunID)
	assert.Equal(t, models.RunKindInvestigation, proj.RunKind)
	assert.Equal(t, models.RunLifecycleStatusRunning, proj.Status)
	assert.Equal(t, 1, proj.CompletedTasks)
	assert.Equal(t, 5, proj.TotalTasks)

	// SSE event emitted.
	route := producerRoute("user-3")
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAppRunStatusUpdated), rows[0].EventType)
}

func TestObserveProducer_UpdateRunState_SSEPayloadContainsNestedEnvelope(t *testing.T) {
	producer, sseStore, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	payload := models.RunStatusUpdatedPayload{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         "run-eval-1",
		RunKind:       models.RunKindEval,
		DisplayName:   "Eval Run",
		Status:        models.RunLifecycleStatusCompleted,
		TotalTasks:    10,
		ObservedAt:    now,
	}

	err := producer.UpdateRunState(ctx, "user-4", producerRoute("user-4"), payload)
	require.NoError(t, err)

	route := producerRoute("user-4")
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))

	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	assert.Equal(t, string(constants.EventAppRunStatusUpdated), env.Type)

	var runPayload models.RunStatusUpdatedPayload
	require.NoError(t, json.Unmarshal(env.Data, &runPayload))
	assert.Equal(t, "run-eval-1", runPayload.RunID)
	assert.Equal(t, models.RunKindEval, runPayload.RunKind)
	assert.Equal(t, models.RunLifecycleStatusCompleted, runPayload.Status)
}

func TestObserveProducer_InvalidTransitionRejected_NoProjectionNoEvent(t *testing.T) {
	producer, sseStore, docStore := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed a completed run.
	seedPayload := models.RunStatusUpdatedPayload{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         "run-terminal-1",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Terminal Run",
		Status:        models.RunLifecycleStatusCompleted,
		TotalTasks:    3,
		ObservedAt:    now,
	}
	require.NoError(t, producer.UpdateRunState(ctx, "user-5", producerRoute("user-5"), seedPayload))

	// Attempt to regress from completed to running.
	regressPayload := models.RunStatusUpdatedPayload{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         "run-terminal-1",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Terminal Run",
		Status:        models.RunLifecycleStatusRunning,
		TotalTasks:    3,
		ObservedAt:    now.Add(1 * time.Second),
	}
	err := producer.UpdateRunState(ctx, "user-5", producerRoute("user-5"), regressPayload)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveInvalidTransition))

	// Projection remains in completed state.
	collection := marshaler.CollectionName(constants.CollectionObserveRuns)
	doc, err := docStore.DocGet(collection, "run-terminal-1")
	require.NoError(t, err)
	var proj runProjection
	require.NoError(t, unmarshalDocData(doc, &proj))
	assert.Equal(t, models.RunLifecycleStatusCompleted, proj.Status, "projection must not regress")

	// Only one SSE event (the initial completion), no event for the rejected transition.
	route := producerRoute("user-5")
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	assert.Len(t, rows, 1, "no SSE event should be emitted for rejected transition")
}

func TestObserveProducer_StaleUpdateRejected(t *testing.T) {
	producer, sseStore, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	later := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	earlier := time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)

	// Seed an agent state at the later timestamp.
	seedPayload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-stale-1",
		DisplayName:   "Stale Agent",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    later,
	}
	require.NoError(t, producer.UpdateAgentState(ctx, "user-6", producerRoute("user-6"), seedPayload))

	// Attempt to update with an earlier observed_at (stale).
	stalePayload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-stale-1",
		DisplayName:   "Stale Agent",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusWaiting,
		ObservedAt:    earlier,
	}
	err := producer.UpdateAgentState(ctx, "user-6", producerRoute("user-6"), stalePayload)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveStaleUpdate))

	// Only one SSE event (the initial seed), no event for the stale update.
	route := producerRoute("user-6")
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	assert.Len(t, rows, 1, "no SSE event should be emitted for stale update")
}

func TestObserveProducer_TerminalStateIdempotentRefreshAllowed(t *testing.T) {
	producer, sseStore, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed a completed run.
	seedPayload := models.RunStatusUpdatedPayload{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         "run-idempotent-1",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Idempotent Run",
		Status:        models.RunLifecycleStatusCompleted,
		TotalTasks:    3,
		ObservedAt:    now,
	}
	require.NoError(t, producer.UpdateRunState(ctx, "user-7", producerRoute("user-7"), seedPayload))

	// Refresh with same status at a later observed_at — should succeed.
	refreshPayload := models.RunStatusUpdatedPayload{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         "run-idempotent-1",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Idempotent Run",
		Status:        models.RunLifecycleStatusCompleted,
		TotalTasks:    3,
		ObservedAt:    now.Add(1 * time.Second),
	}
	err := producer.UpdateRunState(ctx, "user-7", producerRoute("user-7"), refreshPayload)
	require.NoError(t, err, "same-status refresh of terminal state should be idempotent")

	// Two SSE events (seed + refresh).
	route := producerRoute("user-7")
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	assert.Len(t, rows, 2, "idempotent refresh should emit a new SSE event")
}

func TestObserveProducer_CrossUserIsolation_AgentOwnedByDifferentUserRejected(t *testing.T) {
	producer, _, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed an agent for user-a.
	seedPayload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-cross-1",
		DisplayName:   "Cross Agent",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusIdle,
		ObservedAt:    now,
	}
	require.NoError(t, producer.UpdateAgentState(ctx, "user-a", producerRoute("user-a"), seedPayload))

	// user-b attempts to update user-a's agent.
	updatePayload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-cross-1",
		DisplayName:   "Cross Agent",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    now.Add(1 * time.Second),
	}
	err := producer.UpdateAgentState(ctx, "user-b", producerRoute("user-b"), updatePayload)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveAgentNotFound))
}

func TestObserveProducer_CrossUserIsolation_RunOwnedByDifferentUserRejected(t *testing.T) {
	producer, _, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed a run for user-a.
	seedPayload := models.RunStatusUpdatedPayload{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         "run-cross-1",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Cross Run",
		Status:        models.RunLifecycleStatusQueued,
		TotalTasks:    3,
		ObservedAt:    now,
	}
	require.NoError(t, producer.UpdateRunState(ctx, "user-a", producerRoute("user-a"), seedPayload))

	// user-b attempts to update user-a's run.
	updatePayload := models.RunStatusUpdatedPayload{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         "run-cross-1",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Cross Run",
		Status:        models.RunLifecycleStatusRunning,
		TotalTasks:    3,
		ObservedAt:    now.Add(1 * time.Second),
	}
	err := producer.UpdateRunState(ctx, "user-b", producerRoute("user-b"), updatePayload)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveRunNotFound))
}

func TestObserveProducer_UpdateAgentState_MissingAgentIDRejected(t *testing.T) {
	producer, _, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()

	payload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "",
		DisplayName:   "No ID",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusIdle,
		ObservedAt:    time.Now().UTC(),
	}
	err := producer.UpdateAgentState(ctx, "user-x", producerRoute("user-x"), payload)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveAgentIDRequired))
}

func TestObserveProducer_UpdateRunState_MissingRunIDRejected(t *testing.T) {
	producer, _, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()

	payload := models.RunStatusUpdatedPayload{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         "",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "No ID",
		Status:        models.RunLifecycleStatusQueued,
		TotalTasks:    3,
		ObservedAt:    time.Now().UTC(),
	}
	err := producer.UpdateRunState(ctx, "user-x", producerRoute("user-x"), payload)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveRunIDRequired))
}

func TestObserveProducer_UpdateAgentState_InvalidRouteRejected(t *testing.T) {
	producer, _, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()

	payload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-route-1",
		DisplayName:   "Route Agent",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusIdle,
		ObservedAt:    time.Now().UTC(),
	}
	// Route with no session ID.
	invalidRoute := SSERoute{UserID: "user-x"}
	err := producer.UpdateAgentState(ctx, "user-x", invalidRoute, payload)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewaySSERouteSessionRequired))
}

func TestObserveProducer_UpdateRunState_PreservesTasksAndEvidenceFromExisting(t *testing.T) {
	producer, _, docStore := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed a run with tasks and evidence.
	collection := marshaler.CollectionName(constants.CollectionObserveRuns)
	initialProj := runProjection{
		UserID:        "user-preserve",
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         "run-preserve-1",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Preserve Run",
		Status:        models.RunLifecycleStatusRunning,
		TotalTasks:    3,
		Tasks: []models.RunTask{
			{TaskID: "task-1", Status: models.RunLifecycleStatusCompleted},
			{TaskID: "task-2", Status: models.RunLifecycleStatusRunning},
		},
		EvidenceSafeLinks: []models.EvidenceSafeLink{
			{ArtifactID: "art-1", Label: "Evidence", MediaType: "application/jsonl"},
		},
		HasReceipts:   true,
		EvidenceCount: 1,
		ObservedAt:    now,
	}
	b, err := json.Marshal(initialProj)
	require.NoError(t, err)
	require.NoError(t, docStore.DocSet(collection, "run-preserve-1", b))

	// Update run state via producer — tasks and evidence should be preserved.
	updatePayload := models.RunStatusUpdatedPayload{
		SchemaVersion:  constants.ObserveAPIReadModelSchemaVersion,
		RunID:          "run-preserve-1",
		RunKind:        models.RunKindInvestigation,
		DisplayName:    "Preserve Run",
		Status:         models.RunLifecycleStatusCompleted,
		CompletedTasks: 3,
		TotalTasks:     3,
		ObservedAt:     now.Add(1 * time.Second),
	}
	err = producer.UpdateRunState(ctx, "user-preserve", producerRoute("user-preserve"), updatePayload)
	require.NoError(t, err)

	doc, err := docStore.DocGet(collection, "run-preserve-1")
	require.NoError(t, err)
	var proj runProjection
	require.NoError(t, unmarshalDocData(doc, &proj))
	assert.Equal(t, models.RunLifecycleStatusCompleted, proj.Status)
	assert.Len(t, proj.Tasks, 2, "tasks must be preserved from existing projection")
	assert.Len(t, proj.EvidenceSafeLinks, 1, "evidence links must be preserved")
	assert.True(t, proj.HasReceipts, "has_receipts must be preserved")
	assert.Equal(t, 1, proj.EvidenceCount, "evidence_count must be preserved")
}

func TestObserveProducer_AgentStateTransitionSequence_QueuedToRunningToCompleted(t *testing.T) {
	producer, sseStore, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	base := time.Now().UTC()
	route := producerRoute("user-seq")

	// queued -> running -> completed
	steps := []models.AgentLifecycleStatus{
		models.AgentLifecycleStatusQueued,
		models.AgentLifecycleStatusRunning,
		models.AgentLifecycleStatusCompleted,
	}
	for i, status := range steps {
		payload := models.AgentStatusUpdatedPayload{
			SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
			AgentID:       "agent-seq-1",
			DisplayName:   "Seq Agent",
			Role:          "triage",
			Status:        status,
			ObservedAt:    base.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, producer.UpdateAgentState(ctx, "user-seq", route, payload))
	}

	// Three SSE events in order.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	for i, expected := range []string{
		string(constants.EventAppAgentStatusUpdated),
		string(constants.EventAppAgentStatusUpdated),
		string(constants.EventAppAgentStatusUpdated),
	} {
		assert.Equal(t, expected, rows[i].EventType, "event %d should be agent status updated", i)
	}
}

func TestObserveProducer_RunStateTransitionSequence_QueuedToRunningToWaitingToCompleted(t *testing.T) {
	producer, sseStore, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	base := time.Now().UTC()
	route := producerRoute("user-seq-r")

	steps := []models.RunLifecycleStatus{
		models.RunLifecycleStatusQueued,
		models.RunLifecycleStatusRunning,
		models.RunLifecycleStatusWaiting,
		models.RunLifecycleStatusRunning,
		models.RunLifecycleStatusCompleted,
	}
	for i, status := range steps {
		payload := models.RunStatusUpdatedPayload{
			SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
			RunID:         "run-seq-1",
			RunKind:       models.RunKindInvestigation,
			DisplayName:   "Seq Run",
			Status:        status,
			TotalTasks:    5,
			ObservedAt:    base.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, producer.UpdateRunState(ctx, "user-seq-r", route, payload))
	}

	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 5, "one SSE event per transition")
}

func TestObserveProducer_EmittedEventDoesNotContainUserIDInEventData(t *testing.T) {
	producer, sseStore, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	payload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-priv-1",
		DisplayName:   "Priv Agent",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    now,
	}
	require.NoError(t, producer.UpdateAgentState(ctx, "user-priv", producerRoute("user-priv"), payload))

	route := producerRoute("user-priv")
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	// The event data (nested payload) must not contain user_id — it is a
	// routing field on the outer envelope, not a payload field.
	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	assert.NotContains(t, string(env.Data), "user_id",
		"event data must not expose user_id ownership field")
}

func TestObserveProducer_DuplicateEventAtSameObservedAtIsIdempotent(t *testing.T) {
	producer, sseStore, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()
	route := producerRoute("user-dup")

	payload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-dup-1",
		DisplayName:   "Dup Agent",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    now,
	}

	// First update succeeds.
	require.NoError(t, producer.UpdateAgentState(ctx, "user-dup", route, payload))
	// Second update at the same observed_at succeeds (not stale, equal is allowed).
	require.NoError(t, producer.UpdateAgentState(ctx, "user-dup", route, payload))

	// Two SSE events (both are valid emissions — the projection is refreshed).
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	assert.Len(t, rows, 2, "equal observed_at is not stale; both updates emit events")
}

func TestObserveProducer_CLIRouteEmitsToCLIChannel(t *testing.T) {
	producer, sseStore, _ := newObserveProducerTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	cliRoute := SSERoute{UserID: "user-cli", CLISessionID: "cli-session-test"}
	payload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-cli-1",
		DisplayName:   "CLI Agent",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusIdle,
		ObservedAt:    now,
	}
	require.NoError(t, producer.UpdateAgentState(ctx, "user-cli", cliRoute, payload))

	// Event is queryable via the CLI route.
	rows, err := sseStore.SSEEventsListSince(cliRoute, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "user-cli", rows[0].UserID)
	assert.Equal(t, "cli-session-test", rows[0].CLISessionID)
	assert.Empty(t, rows[0].WebSessionID)
}
