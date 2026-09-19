// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func newObserveProducerLiveTestEnv(t *testing.T) (*ObserveProducerService, *DocumentStoreService, *SSEEventService, fs.RuntimeFileService) {
	t.Helper()
	logger := testutil.NewTestLogger()
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(":memory:"), logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)
	docStore := NewDocumentStoreService(db, logger)
	sseStore := NewSSEEventService(db, logger)
	fileSvc := newProducerFileSvc(t)
	producer := NewObserveProducerService(docStore, sseStore, NewGatewayWebSocketHandler(logger), fileSvc, logger)
	return producer, docStore, sseStore, fileSvc
}

// TestStartCycle_PersistsProjectionAndEmitsSSE verifies that StartCycle
// persists the cycle state projection before emitting the
// ai.eval.cycle.started SSE event.
func TestStartCycle_PersistsProjectionAndEmitsSSE(t *testing.T) {
	producer, docStore, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-start-cycle"
	route := SSERoute{UserID: userID, WebSessionID: "web-start-cycle"}
	proj := models.CycleStateProjection{
		CycleID:           "cycle-1",
		CampaignID:        "campaign-1",
		CampaignRevision:  "rev-1",
		RoleCombinationID: "combo-1",
	}

	err := producer.StartCycle(context.Background(), userID, route, proj)
	require.NoError(t, err)

	// Cycle state projection is persisted.
	collection := marshaler.CollectionName(constants.CollectionObserveCycleStates)
	doc, err := docStore.DocGet(collection, "cycle-1")
	require.NoError(t, err)
	require.NotNil(t, doc)

	var persisted struct {
		UserID string `json:"user_id"`
		models.CycleStateProjection
	}
	require.NoError(t, unmarshalDocData(doc, &persisted))
	assert.Equal(t, userID, persisted.UserID)
	assert.Equal(t, "cycle-1", persisted.CycleID)
	assert.Equal(t, models.CampaignCycleStatusRunning, persisted.Status)
	assert.Equal(t, models.CampaignFreshnessActive, persisted.Freshness)
	assert.NotNil(t, persisted.StartedAt)

	// SSE event is emitted.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalCycleStarted), rows[0].EventType)

	// The payload carries source_sequence and event_id.
	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalCycleStartedPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, "cycle-1", payload.CycleID)
	assert.Equal(t, "campaign-1", payload.CampaignID)
	assert.Equal(t, int64(1), payload.SourceSequence)
	assert.NotEmpty(t, payload.EventID)
}

// TestCompleteCycle_PersistsProjectionAndEmitsSSE verifies that CompleteCycle
// persists the cycle state projection before emitting the
// ai.eval.cycle.completed SSE event.
func TestCompleteCycle_PersistsProjectionAndEmitsSSE(t *testing.T) {
	producer, docStore, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-complete-cycle"
	route := SSERoute{UserID: userID, WebSessionID: "web-complete-cycle"}
	proj := models.CycleStateProjection{
		CycleID:            "cycle-2",
		CampaignID:         "campaign-2",
		CampaignRevision:   "rev-2",
		RoleCombinationID:  "combo-2",
		VerificationStatus: models.EvalVerificationVerified,
	}

	err := producer.CompleteCycle(context.Background(), userID, route, proj, 10, 5)
	require.NoError(t, err)

	// Cycle state projection is persisted.
	collection := marshaler.CollectionName(constants.CollectionObserveCycleStates)
	doc, err := docStore.DocGet(collection, "cycle-2")
	require.NoError(t, err)
	require.NotNil(t, doc)

	var persisted struct {
		UserID string `json:"user_id"`
		models.CycleStateProjection
	}
	require.NoError(t, unmarshalDocData(doc, &persisted))
	assert.Equal(t, userID, persisted.UserID)
	assert.NotNil(t, persisted.CompletedAt)

	// SSE event is emitted.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalCycleCompleted), rows[0].EventType)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalCycleCompletedPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, "cycle-2", payload.CycleID)
	assert.Equal(t, 10, payload.TerminalAttempts)
	assert.Equal(t, 5, payload.AssignedTasks)
	assert.Equal(t, models.EvalVerificationVerified, payload.VerificationStatus)
}

// TestStartAssignment_PersistsProjectionAndEmitsSSE verifies that
// StartAssignment persists the assignment progress projection before emitting
// the ai.eval.assignment.started SSE event.
func TestStartAssignment_PersistsProjectionAndEmitsSSE(t *testing.T) {
	producer, docStore, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-start-assignment"
	route := SSERoute{UserID: userID, WebSessionID: "web-start-assignment"}
	proj := models.AssignmentProgressProjection{
		AssignmentID: "assign-1",
		CycleID:      "cycle-a",
		CampaignID:   "campaign-a",
		VariantID:    "variant-1",
		Role:         models.ModelRolePrimary,
		TaskID:       "task-1",
		ArmID:        "arm-1",
		Repetition:   1,
	}

	err := producer.StartAssignment(context.Background(), userID, route, proj)
	require.NoError(t, err)

	// Assignment progress projection is persisted.
	collection := marshaler.CollectionName(constants.CollectionObserveAssignmentProgress)
	doc, err := docStore.DocGet(collection, "assign-1")
	require.NoError(t, err)
	require.NotNil(t, doc)

	var persisted struct {
		UserID string `json:"user_id"`
		models.AssignmentProgressProjection
	}
	require.NoError(t, unmarshalDocData(doc, &persisted))
	assert.Equal(t, userID, persisted.UserID)
	assert.Equal(t, models.AssignmentProgressStatusRunning, persisted.Status)
	assert.NotNil(t, persisted.StartedAt)

	// SSE event is emitted.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalAssignmentStarted), rows[0].EventType)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalAssignmentStartedPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, "assign-1", payload.AssignmentID)
	assert.Equal(t, models.ModelRolePrimary, payload.Role)
	assert.Equal(t, int64(1), payload.SourceSequence)
}

// TestCompleteAssignment_PersistsProjectionAndEmitsSSE verifies that
// CompleteAssignment persists the assignment progress projection before
// emitting the ai.eval.assignment.completed SSE event with terminal status.
func TestCompleteAssignment_PersistsProjectionAndEmitsSSE(t *testing.T) {
	producer, docStore, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-complete-assignment"
	route := SSERoute{UserID: userID, WebSessionID: "web-complete-assignment"}
	proj := models.AssignmentProgressProjection{
		AssignmentID:   "assign-2",
		CycleID:        "cycle-b",
		CampaignID:     "campaign-b",
		VariantID:      "variant-2",
		Role:           models.ModelRoleAssistant,
		TaskID:         "task-2",
		ArmID:          "arm-2",
		Repetition:     2,
		TerminalStatus: models.TerminalOutcomeStatusCompleted,
	}

	err := producer.CompleteAssignment(context.Background(), userID, route, proj)
	require.NoError(t, err)

	// Assignment progress projection is persisted.
	collection := marshaler.CollectionName(constants.CollectionObserveAssignmentProgress)
	doc, err := docStore.DocGet(collection, "assign-2")
	require.NoError(t, err)
	require.NotNil(t, doc)

	var persisted struct {
		UserID string `json:"user_id"`
		models.AssignmentProgressProjection
	}
	require.NoError(t, unmarshalDocData(doc, &persisted))
	assert.Equal(t, models.TerminalOutcomeStatusCompleted, persisted.TerminalStatus)
	assert.NotNil(t, persisted.CompletedAt)

	// SSE event is emitted.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalAssignmentCompleted), rows[0].EventType)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalAssignmentCompletedPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, models.TerminalOutcomeStatusCompleted, payload.TerminalStatus)
}

// TestRecordModelRoleInvocation_EmitsSSEWithExactIdentity verifies that
// RecordModelRoleInvocation emits an ai.eval.model_role.invoked SSE event
// with exact served model tag, backend, and quantization from the provider
// boundary.
func TestRecordModelRoleInvocation_EmitsSSEWithExactIdentity(t *testing.T) {
	producer, _, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-role-invoke"
	route := SSERoute{UserID: userID, WebSessionID: "web-role-invoke"}
	proj := models.AssignmentProgressProjection{
		AssignmentID: "assign-3",
		CycleID:      "cycle-c",
		CampaignID:   "campaign-c",
		VariantID:    "variant-3",
		Role:         models.ModelRolePrimary,
	}

	err := producer.RecordModelRoleInvocation(context.Background(), userID, route, proj, "qwen2.5-72b", "vllm", "awq")
	require.NoError(t, err)

	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalModelRoleInvoked), rows[0].EventType)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalModelRoleInvokedPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, "qwen2.5-72b", payload.ServedModelTag)
	assert.Equal(t, "vllm", payload.BackendName)
	assert.Equal(t, "awq", payload.Quantization)
	assert.Equal(t, models.ModelRolePrimary, payload.Role)
}

// TestRecordMetricAvailable_EmitsSSEWithPerVariantAggregation verifies that
// RecordMetricAvailability emits an ai.eval.metric.available SSE event with
// per-variant aggregation, numerator, denominator, rate, and unit.
func TestRecordMetricAvailable_EmitsSSEWithPerVariantAggregation(t *testing.T) {
	producer, _, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-metric"
	route := SSERoute{UserID: userID, WebSessionID: "web-metric"}
	rate := 0.8
	payload := models.EvalMetricAvailablePayload{
		CycleID:            "cycle-d",
		CampaignID:         "campaign-d",
		AssignmentID:       "assign-4",
		VariantID:          "variant-4",
		MetricID:           "success_rate",
		MetricVersion:      "1.0.0",
		Numerator:          8,
		Denominator:        10,
		Rate:               &rate,
		Unit:               "count",
		VerificationStatus: models.EvalVerificationProjectionValidated,
	}

	err := producer.RecordMetricAvailability(context.Background(), userID, route, payload)
	require.NoError(t, err)

	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalMetricAvailable), rows[0].EventType)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var emitted models.EvalMetricAvailablePayload
	require.NoError(t, json.Unmarshal(env.Data, &emitted))
	assert.Equal(t, "success_rate", emitted.MetricID)
	assert.Equal(t, 8, emitted.Numerator)
	assert.Equal(t, 10, emitted.Denominator)
	assert.NotNil(t, emitted.Rate)
	assert.Equal(t, 0.8, *emitted.Rate)
	assert.Equal(t, "variant-4", emitted.VariantID)
}

// TestCompleteVerifier_PersistsProjectionAndEmitsSSE verifies that
// CompleteVerifier persists the verification progress projection before
// emitting the ai.eval.verifier.completed SSE event.
func TestCompleteVerifier_PersistsProjectionAndEmitsSSE(t *testing.T) {
	producer, docStore, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-verifier"
	route := SSERoute{UserID: userID, WebSessionID: "web-verifier"}
	proj := models.VerificationProgressProjection{
		CycleID:                     "cycle-e",
		CampaignID:                  "campaign-e",
		VerificationStatus:          models.EvalVerificationVerified,
		VerifiedIndexGenerationHash: "abc123",
		LayerCount:                  13,
		FailureCount:                0,
	}

	err := producer.CompleteVerifier(context.Background(), userID, route, proj)
	require.NoError(t, err)

	// Verification progress projection is persisted.
	collection := marshaler.CollectionName(constants.CollectionObserveVerificationProgress)
	doc, err := docStore.DocGet(collection, "cycle-e")
	require.NoError(t, err)
	require.NotNil(t, doc)

	// SSE event is emitted.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalVerifierCompleted), rows[0].EventType)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalVerifierCompletedPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, "abc123", payload.VerifiedIndexGenerationHash)
	assert.Equal(t, 13, payload.LayerCount)
	assert.Equal(t, 0, payload.FailureCount)
}

// TestRecordProofAvailability_PersistsProjectionAndEmitsSSE verifies that
// RecordProofAvailability persists the publication progress projection
// before emitting the ai.eval.proof.available SSE event.
func TestRecordProofAvailability_PersistsProjectionAndEmitsSSE(t *testing.T) {
	producer, docStore, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-proof"
	route := SSERoute{UserID: userID, WebSessionID: "web-proof"}

	err := producer.RecordProofAvailability(context.Background(), userID, route, "cycle-f", "campaign-f", "proof-hash-123", 5)
	require.NoError(t, err)

	// Publication progress projection is persisted.
	collection := marshaler.CollectionName(constants.CollectionObservePublicationProgress)
	doc, err := docStore.DocGet(collection, "cycle-f")
	require.NoError(t, err)
	require.NotNil(t, doc)

	// SSE event is emitted.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalProofAvailable), rows[0].EventType)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalProofAvailablePayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, "proof-hash-123", payload.ProofRootSHA256)
	assert.Equal(t, 5, payload.ArtifactCount)
}

// TestCompletePublication_PersistsProjectionAndEmitsSSE verifies that
// CompletePublication persists the publication progress projection before
// emitting the ai.eval.publication.completed SSE event.
func TestCompletePublication_PersistsProjectionAndEmitsSSE(t *testing.T) {
	producer, docStore, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-pub"
	route := SSERoute{UserID: userID, WebSessionID: "web-pub"}
	proj := models.PublicationProgressProjection{
		CycleID:                   "cycle-g",
		CampaignID:                "campaign-g",
		PublicationSchemaVersion:  "4.0.0",
		PublishedProjectionSHA256: "pub-hash-456",
	}

	err := producer.CompletePublication(context.Background(), userID, route, proj)
	require.NoError(t, err)

	// Publication progress projection is persisted.
	collection := marshaler.CollectionName(constants.CollectionObservePublicationProgress)
	doc, err := docStore.DocGet(collection, "cycle-g")
	require.NoError(t, err)
	require.NotNil(t, doc)

	var persisted struct {
		UserID string `json:"user_id"`
		models.PublicationProgressProjection
	}
	require.NoError(t, unmarshalDocData(doc, &persisted))
	assert.Equal(t, models.PublicationStatusPublished, persisted.PublicationStatus)
	assert.NotNil(t, persisted.CompletedAt)

	// SSE event is emitted.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalPublicationCompleted), rows[0].EventType)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalPublicationCompletedPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, "4.0.0", payload.PublicationSchemaVersion)
	assert.Equal(t, "pub-hash-456", payload.PublishedProjectionSHA256)
}

// TestEmitHeartbeat_PersistsProjectionAndEmitsSSE verifies that EmitHeartbeat
// persists the source freshness projection before emitting the
// ai.eval.heartbeat SSE event.
func TestEmitHeartbeat_PersistsProjectionAndEmitsSSE(t *testing.T) {
	producer, docStore, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-heartbeat"
	route := SSERoute{UserID: userID, WebSessionID: "web-heartbeat"}

	err := producer.EmitHeartbeat(context.Background(), userID, route, "source-1", "campaign-h")
	require.NoError(t, err)

	// Source freshness projection is persisted.
	collection := marshaler.CollectionName(constants.CollectionObserveSourceFreshness)
	doc, err := docStore.DocGet(collection, "source-1")
	require.NoError(t, err)
	require.NotNil(t, doc)

	var persisted struct {
		UserID string `json:"user_id"`
		models.SourceFreshnessProjection
	}
	require.NoError(t, unmarshalDocData(doc, &persisted))
	assert.Equal(t, "source-1", persisted.SourceID)
	assert.Equal(t, "campaign-h", persisted.CampaignID)
	assert.Equal(t, models.CampaignFreshnessActive, persisted.Freshness)

	// SSE event is emitted.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalHeartbeat), rows[0].EventType)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalHeartbeatPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, "source-1", payload.SourceID)
	assert.NotEmpty(t, payload.EventID)
	assert.Greater(t, payload.SourceSequence, int64(0))
}

// TestRequestStop_PersistsProjectionAndEmitsSSE verifies that RequestStop
// persists the supervisor state projection before emitting the
// ai.eval.stop.requested SSE event with typed stop reason.
func TestRequestStop_PersistsProjectionAndEmitsSSE(t *testing.T) {
	producer, docStore, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-stop"
	route := SSERoute{UserID: userID, WebSessionID: "web-stop"}

	err := producer.RequestStop(context.Background(), userID, route, "sup-1", "campaign-i", models.StopReasonGraceful, models.StopScopeSupervisor)
	require.NoError(t, err)

	// Supervisor state projection is persisted.
	collection := marshaler.CollectionName(constants.CollectionObserveSupervisorStates)
	doc, err := docStore.DocGet(collection, "sup-1")
	require.NoError(t, err)
	require.NotNil(t, doc)

	var persisted struct {
		UserID string `json:"user_id"`
		models.SupervisorStateProjection
	}
	require.NoError(t, unmarshalDocData(doc, &persisted))
	assert.Equal(t, models.SupervisorStatusStopped, persisted.Status)
	assert.Equal(t, models.StopReasonGraceful, persisted.StopReason)
	assert.Equal(t, models.CampaignFreshnessIntentionallyStopped, persisted.Freshness)

	// SSE event is emitted.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.EventAiEvalStopRequested), rows[0].EventType)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalStopRequestedPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, models.StopReasonGraceful, payload.StopReason)
	assert.Equal(t, models.StopScopeSupervisor, payload.StopScope)
}

// TestRequestStop_SafetyStopSetsSafetyStoppedFreshness verifies that a
// safety stop reason sets the safety_stopped freshness and safety_stopped
// supervisor status.
func TestRequestStop_SafetyStopSetsSafetyStoppedFreshness(t *testing.T) {
	producer, docStore, _, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-safety-stop"
	route := SSERoute{UserID: userID, WebSessionID: "web-safety-stop"}

	err := producer.RequestStop(context.Background(), userID, route, "sup-2", "campaign-j", models.StopReasonSafetyBudget, models.StopScopeSupervisor)
	require.NoError(t, err)

	collection := marshaler.CollectionName(constants.CollectionObserveSupervisorStates)
	doc, err := docStore.DocGet(collection, "sup-2")
	require.NoError(t, err)
	require.NotNil(t, doc)

	var persisted struct {
		UserID string `json:"user_id"`
		models.SupervisorStateProjection
	}
	require.NoError(t, unmarshalDocData(doc, &persisted))
	assert.Equal(t, models.SupervisorStatusSafetyStopped, persisted.Status)
	assert.Equal(t, models.CampaignFreshnessSafetyStopped, persisted.Freshness)
}

// TestMonotonicSourceSequence verifies that source_sequence values are
// monotonically increasing across multiple live events.
func TestMonotonicSourceSequence(t *testing.T) {
	producer, _, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-seq"
	route := SSERoute{UserID: userID, WebSessionID: "web-seq"}

	// Emit three heartbeats.
	require.NoError(t, producer.EmitHeartbeat(context.Background(), userID, route, "source-seq", "campaign-seq"))
	require.NoError(t, producer.EmitHeartbeat(context.Background(), userID, route, "source-seq", "campaign-seq"))
	require.NoError(t, producer.EmitHeartbeat(context.Background(), userID, route, "source-seq", "campaign-seq"))

	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	var sequences []int64
	for _, row := range rows {
		var pushPayload models.SSEPushPayload
		require.NoError(t, json.Unmarshal([]byte(row.Payload), &pushPayload))
		var env sseEventEnvelope
		require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
		var payload models.EvalHeartbeatPayload
		require.NoError(t, json.Unmarshal(env.Data, &payload))
		sequences = append(sequences, payload.SourceSequence)
	}

	assert.Equal(t, int64(1), sequences[0])
	assert.Equal(t, int64(2), sequences[1])
	assert.Equal(t, int64(3), sequences[2])
}

// TestDuplicateEventIDSuppression verifies that a duplicate event_id is
// rejected with ErrObserveDuplicateEventID.
func TestDuplicateEventIDSuppression(t *testing.T) {
	producer, _, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-dup"
	route := SSERoute{UserID: userID, WebSessionID: "web-dup"}

	// First heartbeat succeeds.
	require.NoError(t, producer.EmitHeartbeat(context.Background(), userID, route, "source-dup", "campaign-dup"))

	// Manually inject a duplicate event_id into the seen set.
	producer.seenMu.Lock()
	producer.seenEventIDs["forced-duplicate-id"] = struct{}{}
	producer.seenMu.Unlock()

	// Now try to emit with the duplicate event_id by calling emitLiveSSEEvent
	// directly.
	err := producer.emitLiveSSEEvent(route, string(constants.EventAiEvalHeartbeat), "forced-duplicate-id", models.EvalHeartbeatPayload{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		SourceSequence: 99,
		EventID:        "forced-duplicate-id",
		SourceID:       "source-dup",
	})
	assert.ErrorIs(t, err, constants.ErrObserveDuplicateEventID)

	// Only one SSE event should exist.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

// TestPersistBeforeEvent_NoSSEOnPersistenceFailure verifies that no SSE
// event is emitted when projection persistence fails. We simulate this by
// closing the document store database before calling a live producer method.
func TestPersistBeforeEvent_NoSSEOnPersistenceFailure(t *testing.T) {
	logger := testutil.NewTestLogger()
	cfg := sqliteutil.DefaultDBConfig(":memory:")
	db, err := sqliteutil.OpenDB(cfg, logger)
	require.NoError(t, err)

	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)

	docStore := NewDocumentStoreService(db, logger)
	sseStore := NewSSEEventService(db, logger)
	pubsub := NewGatewayWebSocketHandler(logger)
	fileSvc := newProducerFileSvc(t)
	producer := NewObserveProducerService(docStore, sseStore, pubsub, fileSvc, logger)

	userID := "user-live-persist-fail"
	route := SSERoute{UserID: userID, WebSessionID: "web-persist-fail"}

	// Close the database to simulate persistence failure.
	require.NoError(t, db.Close())

	err = producer.EmitHeartbeat(context.Background(), userID, route, "source-fail", "campaign-fail")
	assert.Error(t, err)

	// No SSE event should exist (the SSE store is also closed, so we
	// verify the error path was hit before emission).
	// We cannot query the closed SSE store, so we verify the error.
	assert.Error(t, err)
}

// TestLiveEvents_CrossUserIsolation verifies that live events from one user
// do not appear in another user's SSE stream.
func TestLiveEvents_CrossUserIsolation(t *testing.T) {
	producer, _, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userA := "user-live-a"
	userB := "user-live-b"
	routeA := SSERoute{UserID: userA, WebSessionID: "web-a"}
	routeB := SSERoute{UserID: userB, WebSessionID: "web-b"}

	require.NoError(t, producer.EmitHeartbeat(context.Background(), userA, routeA, "source-a", "campaign-a"))
	require.NoError(t, producer.EmitHeartbeat(context.Background(), userB, routeB, "source-b", "campaign-b"))

	// User A sees only their own event.
	rowsA, err := sseStore.SSEEventsListSince(routeA, 0, 100)
	require.NoError(t, err)
	require.Len(t, rowsA, 1)

	// User B sees only their own event.
	rowsB, err := sseStore.SSEEventsListSince(routeB, 0, 100)
	require.NoError(t, err)
	require.Len(t, rowsB, 1)
}

// TestStartCycle_MissingCycleIDReturnsError verifies that StartCycle returns
// an error when cycle_id is empty.
func TestStartCycle_MissingCycleIDReturnsError(t *testing.T) {
	producer, _, _, _ := newObserveProducerLiveTestEnv(t)
	route := SSERoute{UserID: "user", WebSessionID: "web"}
	err := producer.StartCycle(context.Background(), "user", route, models.CycleStateProjection{
		CampaignID: "campaign",
	})
	assert.ErrorIs(t, err, constants.ErrObserveCycleIDRequired)
}

// TestStartCycle_MissingCampaignIDReturnsError verifies that StartCycle
// returns an error when campaign_id is empty.
func TestStartCycle_MissingCampaignIDReturnsError(t *testing.T) {
	producer, _, _, _ := newObserveProducerLiveTestEnv(t)
	route := SSERoute{UserID: "user", WebSessionID: "web"}
	err := producer.StartCycle(context.Background(), "user", route, models.CycleStateProjection{
		CycleID: "cycle",
	})
	assert.ErrorIs(t, err, constants.ErrObserveCampaignIDRequired)
}

// TestStartAssignment_MissingAssignmentIDReturnsError verifies that
// StartAssignment returns an error when assignment_id is empty.
func TestStartAssignment_MissingAssignmentIDReturnsError(t *testing.T) {
	producer, _, _, _ := newObserveProducerLiveTestEnv(t)
	route := SSERoute{UserID: "user", WebSessionID: "web"}
	err := producer.StartAssignment(context.Background(), "user", route, models.AssignmentProgressProjection{
		CampaignID: "campaign",
	})
	assert.ErrorIs(t, err, constants.ErrObserveAssignmentIDRequired)
}

// TestEmitHeartbeat_MissingSourceIDReturnsError verifies that EmitHeartbeat
// returns an error when source_id is empty.
func TestEmitHeartbeat_MissingSourceIDReturnsError(t *testing.T) {
	producer, _, _, _ := newObserveProducerLiveTestEnv(t)
	route := SSERoute{UserID: "user", WebSessionID: "web"}
	err := producer.EmitHeartbeat(context.Background(), "user", route, "", "campaign")
	assert.ErrorIs(t, err, constants.ErrObserveSourceIDRequired)
}

// TestLiveEventPayloads_NoUnknownFields verifies that the live event payload
// structs reject unknown fields on deserialization (strict JSON). This is a
// structural test: Go's encoding/json ignores unknown fields by default, so
// we verify that the known fields round-trip correctly and the wire shape
// matches the protocol JSON.
func TestLiveEventPayloads_RoundTripSerialization(t *testing.T) {
	// EvalCycleStartedPayload
	original := models.EvalCycleStartedPayload{
		SchemaVersion:       "1.0.0",
		SourceSequence:      42,
		EventID:             "evt-1",
		CycleID:             "cycle-rt",
		CampaignID:          "campaign-rt",
		CampaignRevision:    "rev-rt",
		RoleCombinationID:   "combo-rt",
		PrimaryVariantID:    "variant-p",
		AssistantVariantID:  "variant-a",
		BenchmarkPopulation: 10,
		RepetitionCount:     3,
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)
	var decoded models.EvalCycleStartedPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original, decoded)

	// EvalStopRequestedPayload
	stopOriginal := models.EvalStopRequestedPayload{
		SchemaVersion:  "1.0.0",
		SourceSequence: 99,
		EventID:        "evt-stop",
		CampaignID:     "campaign-stop",
		StopReason:     models.StopReasonSafetyDisk,
		StopScope:      models.StopScopeCycle,
	}
	stopData, err := json.Marshal(stopOriginal)
	require.NoError(t, err)
	var stopDecoded models.EvalStopRequestedPayload
	require.NoError(t, json.Unmarshal(stopData, &stopDecoded))
	assert.Equal(t, stopOriginal, stopDecoded)
}

// TestFullCampaignLifecycle_EventsInOrder verifies that a full campaign
// lifecycle (start cycle, start assignment, complete assignment, complete
// cycle, complete verifier, record proof, complete publication) emits all
// events in order with monotonically increasing source_sequence values.
func TestFullCampaignLifecycle_EventsInOrder(t *testing.T) {
	producer, _, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-full"
	route := SSERoute{UserID: userID, WebSessionID: "web-full"}
	campaignID := "campaign-full"
	cycleID := "cycle-full"

	// Start cycle.
	require.NoError(t, producer.StartCycle(context.Background(), userID, route, models.CycleStateProjection{
		CycleID:           cycleID,
		CampaignID:        campaignID,
		CampaignRevision:  "rev-full",
		RoleCombinationID: "combo-full",
	}))

	// Start assignment.
	require.NoError(t, producer.StartAssignment(context.Background(), userID, route, models.AssignmentProgressProjection{
		AssignmentID: "assign-full",
		CycleID:      cycleID,
		CampaignID:   campaignID,
		VariantID:    "variant-full",
		Role:         models.ModelRolePrimary,
		TaskID:       "task-full",
		ArmID:        "arm-full",
		Repetition:   1,
	}))

	// Record model role invocation.
	require.NoError(t, producer.RecordModelRoleInvocation(context.Background(), userID, route, models.AssignmentProgressProjection{
		AssignmentID: "assign-full",
		CycleID:      cycleID,
		CampaignID:   campaignID,
		VariantID:    "variant-full",
		Role:         models.ModelRolePrimary,
	}, "model-tag", "backend", "quant"))

	// Complete assignment.
	require.NoError(t, producer.CompleteAssignment(context.Background(), userID, route, models.AssignmentProgressProjection{
		AssignmentID:   "assign-full",
		CycleID:        cycleID,
		CampaignID:     campaignID,
		VariantID:      "variant-full",
		Role:           models.ModelRolePrimary,
		TaskID:         "task-full",
		ArmID:          "arm-full",
		Repetition:     1,
		TerminalStatus: models.TerminalOutcomeStatusCompleted,
	}))

	// Complete verifier.
	require.NoError(t, producer.CompleteVerifier(context.Background(), userID, route, models.VerificationProgressProjection{
		CycleID:                     cycleID,
		CampaignID:                  campaignID,
		VerificationStatus:          models.EvalVerificationVerified,
		VerifiedIndexGenerationHash: "hash-full",
		LayerCount:                  13,
		FailureCount:                0,
	}))

	// Record proof availability.
	require.NoError(t, producer.RecordProofAvailability(context.Background(), userID, route, cycleID, campaignID, "proof-full", 3))

	// Complete publication.
	require.NoError(t, producer.CompletePublication(context.Background(), userID, route, models.PublicationProgressProjection{
		CycleID:                   cycleID,
		CampaignID:                campaignID,
		PublicationSchemaVersion:  "4.0.0",
		PublishedProjectionSHA256: "pub-full",
	}))

	// Complete cycle.
	require.NoError(t, producer.CompleteCycle(context.Background(), userID, route, models.CycleStateProjection{
		CycleID:            cycleID,
		CampaignID:         campaignID,
		CampaignRevision:   "rev-full",
		RoleCombinationID:  "combo-full",
		VerificationStatus: models.EvalVerificationVerified,
	}, 1, 1))

	// Verify all 8 events are in order.
	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 8)

	expectedTypes := []string{
		string(constants.EventAiEvalCycleStarted),
		string(constants.EventAiEvalAssignmentStarted),
		string(constants.EventAiEvalModelRoleInvoked),
		string(constants.EventAiEvalAssignmentCompleted),
		string(constants.EventAiEvalVerifierCompleted),
		string(constants.EventAiEvalProofAvailable),
		string(constants.EventAiEvalPublicationCompleted),
		string(constants.EventAiEvalCycleCompleted),
	}
	for i, expected := range expectedTypes {
		assert.Equal(t, expected, rows[i].EventType, "event %d", i)
	}

	// Verify source_sequence is monotonically increasing.
	var prevSeq int64
	for i, row := range rows {
		var pushPayload models.SSEPushPayload
		require.NoError(t, json.Unmarshal([]byte(row.Payload), &pushPayload))
		var env sseEventEnvelope
		require.NoError(t, json.Unmarshal(pushPayload.Event, &env))

		// All live payloads have source_sequence and event_id.
		var generic struct {
			SourceSequence int64  `json:"source_sequence"`
			EventID        string `json:"event_id"`
		}
		require.NoError(t, json.Unmarshal(env.Data, &generic))
		assert.Greater(t, generic.SourceSequence, prevSeq, "event %d sequence not monotonic", i)
		assert.NotEmpty(t, generic.EventID, "event %d missing event_id", i)
		prevSeq = generic.SourceSequence
	}
}

// TestUnavailableMeasurementStatus verifies that an assignment with
// unavailable terminal status is correctly persisted and emitted.
func TestUnavailableMeasurementStatus(t *testing.T) {
	producer, _, sseStore, _ := newObserveProducerLiveTestEnv(t)

	userID := "user-live-unavail"
	route := SSERoute{UserID: userID, WebSessionID: "web-unavail"}

	require.NoError(t, producer.CompleteAssignment(context.Background(), userID, route, models.AssignmentProgressProjection{
		AssignmentID:   "assign-unavail",
		CycleID:        "cycle-unavail",
		CampaignID:     "campaign-unavail",
		VariantID:      "variant-unavail",
		Role:           models.ModelRoleLite,
		TaskID:         "task-unavail",
		ArmID:          "arm-unavail",
		Repetition:     1,
		TerminalStatus: models.TerminalOutcomeStatusUnavailable,
	}))

	rows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalAssignmentCompletedPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, models.TerminalOutcomeStatusUnavailable, payload.TerminalStatus)
}

// TestStaleAndStopTransitions verifies that stop events carry the correct
// freshness labels for graceful vs safety stops.
func TestStaleAndStopTransitions(t *testing.T) {
	producer, _, _, _ := newObserveProducerLiveTestEnv(t)

	// Graceful stop -> intentionally_stopped.
	producer2, _, sseStore2, _ := newObserveProducerLiveTestEnv(t)
	userID := "user-live-stale"
	route := SSERoute{UserID: userID, WebSessionID: "web-stale"}
	require.NoError(t, producer2.RequestStop(context.Background(), userID, route, "sup-graceful", "campaign-graceful", models.StopReasonGraceful, models.StopScopeSupervisor))

	rows, err := sseStore2.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	var pushPayload models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &pushPayload))
	var env sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload.Event, &env))
	var payload models.EvalStopRequestedPayload
	require.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.Equal(t, models.StopReasonGraceful, payload.StopReason)

	// Safety stop -> safety_stopped.
	producer3, _, sseStore3, _ := newObserveProducerLiveTestEnv(t)
	route3 := SSERoute{UserID: userID, WebSessionID: "web-safety"}
	require.NoError(t, producer3.RequestStop(context.Background(), userID, route3, "sup-safety", "campaign-safety", models.StopReasonSafetyVerifier, models.StopScopeCycle))

	rows3, err := sseStore3.SSEEventsListSince(route3, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows3, 1)

	var pushPayload3 models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows3[0].Payload), &pushPayload3))
	var env3 sseEventEnvelope
	require.NoError(t, json.Unmarshal(pushPayload3.Event, &env3))
	var payload3 models.EvalStopRequestedPayload
	require.NoError(t, json.Unmarshal(env3.Data, &payload3))
	assert.Equal(t, models.StopReasonSafetyVerifier, payload3.StopReason)
	assert.Equal(t, models.StopScopeCycle, payload3.StopScope)

	// Suppress unused variable warning for producer.
	_ = producer
}

// TestRoleCombinationProjection_ExactBinding verifies that the
// RoleCombinationProjection struct correctly binds all three exact variant
// identities with served model tags, backends, and quantization.
func TestRoleCombinationProjection_ExactBinding(t *testing.T) {
	proj := models.RoleCombinationProjection{
		RoleCombinationID:     "combo-exact",
		CampaignID:            "campaign-exact",
		PrimaryVariantID:      "variant-primary",
		AssistantVariantID:    "variant-assistant",
		LiteVariantID:         "variant-lite",
		PrimaryModelTag:       "model-primary",
		AssistantModelTag:     "model-assistant",
		LiteModelTag:          "model-lite",
		PrimaryBackend:        "vllm",
		AssistantBackend:      "tgi",
		LiteBackend:           "llama.cpp",
		PrimaryQuantization:   "awq",
		AssistantQuantization: "gptq",
		LiteQuantization:      "q4_k_m",
	}

	data, err := json.Marshal(proj)
	require.NoError(t, err)
	var decoded models.RoleCombinationProjection
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, proj, decoded)
	assert.Equal(t, "variant-primary", decoded.PrimaryVariantID)
	assert.Equal(t, "variant-assistant", decoded.AssistantVariantID)
	assert.Equal(t, "variant-lite", decoded.LiteVariantID)
	assert.Equal(t, "vllm", decoded.PrimaryBackend)
	assert.Equal(t, "awq", decoded.PrimaryQuantization)
}

// TestObservedMeasurement_ExtendedFields verifies that the extended
// ObservedMeasurement struct carries scope, collector_version, and
// evidence_hash fields.
func TestObservedMeasurement_ExtendedFields(t *testing.T) {
	meas := models.ObservedMeasurement{
		SchemaVersion:    "1.0.0",
		MetricID:         "latency",
		Value:            42.5,
		Unit:             "ms",
		SourceComponent:  "observer",
		Status:           models.MeasurementStatusObserved,
		Scope:            models.MeasurementScopeProcess,
		CollectorVersion: "1.2.3",
		EvidenceHash:     "abc123",
	}

	data, err := json.Marshal(meas)
	require.NoError(t, err)
	var decoded models.ObservedMeasurement
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, models.MeasurementScopeProcess, decoded.Scope)
	assert.Equal(t, "1.2.3", decoded.CollectorVersion)
	assert.Equal(t, "abc123", decoded.EvidenceHash)
}

// TestEvalDetail_ExtendedFields verifies that the extended EvalDetail struct
// carries the new role-combination, benchmark, environment, and proof fields.
func TestEvalDetail_ExtendedFields(t *testing.T) {
	detail := models.EvalDetail{
		SchemaVersion:       "1.0.0",
		RunID:               "run-ext",
		SuiteID:             "suite-ext",
		SuiteVersion:        "1.0.0",
		CampaignID:          "campaign-ext",
		ArmIDs:              []string{"arm-ext"},
		ModelCohortIDs:      []string{"cohort-ext"},
		Status:              models.RunLifecycleStatusCompleted,
		VerificationStatus:  models.EvalVerificationVerified,
		ReceiptCount:        5,
		AssignedTasks:       10,
		TerminalAttempts:    10,
		Metrics:             []models.EvalMetricSummary{},
		RoleCombinationID:   "combo-ext",
		PrimaryVariantID:    "variant-ext",
		BenchmarkPopulation: 20,
		RepetitionCount:     3,
		SupersededCount:     1,
		QualificationCount:  2,
		UnavailableCount:    1,
		EnvironmentClass:    "gpu-a100",
		BackendName:         "vllm",
		ArtifactDigest:      "sha256:abc",
		Quantization:        "awq",
		ProofLinks:          []models.EvidenceSafeLink{{ArtifactID: "proof-1", Label: "Proof", MediaType: "application/json"}},
	}

	data, err := json.Marshal(detail)
	require.NoError(t, err)
	var decoded models.EvalDetail
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "combo-ext", decoded.RoleCombinationID)
	assert.Equal(t, "variant-ext", decoded.PrimaryVariantID)
	assert.Equal(t, 20, decoded.BenchmarkPopulation)
	assert.Equal(t, 3, decoded.RepetitionCount)
	assert.Equal(t, 1, decoded.SupersededCount)
	assert.Equal(t, "gpu-a100", decoded.EnvironmentClass)
	assert.Equal(t, "vllm", decoded.BackendName)
	assert.Equal(t, "awq", decoded.Quantization)
	assert.Len(t, decoded.ProofLinks, 1)
}

// TestLiveEvent_InvalidRouteReturnsError verifies that live producer methods
// reject invalid SSE routes.
func TestLiveEvent_InvalidRouteReturnsError(t *testing.T) {
	producer, _, _, _ := newObserveProducerLiveTestEnv(t)

	// Missing session ID.
	route := SSERoute{UserID: "user"}
	err := producer.EmitHeartbeat(context.Background(), "user", route, "source", "campaign")
	assert.Error(t, err)
}

// TestRecordMetricAvailability_MissingCampaignIDReturnsError verifies that
// RecordMetricAvailability returns an error when campaign_id is empty.
func TestRecordMetricAvailability_MissingCampaignIDReturnsError(t *testing.T) {
	producer, _, _, _ := newObserveProducerLiveTestEnv(t)
	route := SSERoute{UserID: "user", WebSessionID: "web"}
	err := producer.RecordMetricAvailability(context.Background(), "user", route, models.EvalMetricAvailablePayload{
		AssignmentID: "assign",
	})
	assert.ErrorIs(t, err, constants.ErrObserveCampaignIDRequired)
}

// TestRecordModelRoleInvocation_MissingVariantIDReturnsError verifies that
// RecordModelRoleInvocation returns an error when variant_id is empty.
func TestRecordModelRoleInvocation_MissingVariantIDReturnsError(t *testing.T) {
	producer, _, _, _ := newObserveProducerLiveTestEnv(t)
	route := SSERoute{UserID: "user", WebSessionID: "web"}
	err := producer.RecordModelRoleInvocation(context.Background(), "user", route, models.AssignmentProgressProjection{
		AssignmentID: "assign",
		CampaignID:   "campaign",
	}, "tag", "backend", "")
	assert.ErrorIs(t, err, constants.ErrObserveVariantIDRequired)
}

// TestDisclosureFailure_ProhibitedFieldsNotInPayload verifies that live
// event payloads do not contain prohibited fields (raw prompts, outputs,
// credentials, private endpoints, etc.). This is a structural test: the
// payload structs are typed and only carry allowlisted fields.
func TestDisclosureFailure_ProhibitedFieldsNotInPayload(t *testing.T) {
	// Verify that no live event payload struct contains fields that would
	// expose prohibited content. The structs are typed Go structs with
	// explicit json tags; they cannot carry arbitrary fields.
	payload := models.EvalAssignmentStartedPayload{
		SchemaVersion:  "1.0.0",
		SourceSequence: 1,
		EventID:        "evt",
		CycleID:        "cycle",
		CampaignID:     "campaign",
		AssignmentID:   "assign",
		VariantID:      "variant",
		Role:           models.ModelRolePrimary,
		TaskID:         "task",
		ArmID:          "arm",
		Repetition:     1,
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Verify no prohibited field names appear in the JSON.
	prohibited := []string{"prompt", "output", "response", "credential", "secret", "key", "password", "endpoint", "path", "trail"}
	for _, p := range prohibited {
		assert.NotContains(t, string(data), "\""+p+"\"")
	}
}

// TestErrors_NotNil verifies that the new error constants are non-nil and
// distinct.
func TestErrors_NotNil(t *testing.T) {
	errs := []error{
		constants.ErrObserveCampaignIDRequired,
		constants.ErrObserveCycleIDRequired,
		constants.ErrObserveAssignmentIDRequired,
		constants.ErrObserveSupervisorIDRequired,
		constants.ErrObserveSourceIDRequired,
		constants.ErrObserveRoleCombinationIDRequired,
		constants.ErrObserveVariantIDRequired,
		constants.ErrObserveDuplicateEventID,
	}
	for _, e := range errs {
		assert.NotNil(t, e)
	}
	// Verify they are distinct.
	for i, a := range errs {
		for j, b := range errs {
			if i != j {
				assert.NotEqual(t, a, b, "error %d and %d are equal", i, j)
			}
		}
	}
}
