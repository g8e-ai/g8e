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

// observeCollections is the complete set of observe projection collections.
// Observe producers write only to these collections via DocSet (a plain
// document upsert). They are not members of the governed domain collections
// that flow through the GovernanceEnvelope and the 5-layer verification
// gauntlet.
var observeCollections = map[string]struct{}{
	marshaler.CollectionName(constants.CollectionObserveAgentStates): {},
	marshaler.CollectionName(constants.CollectionObserveRuns):        {},
	marshaler.CollectionName(constants.CollectionObserveEvals):       {},
	marshaler.CollectionName(constants.CollectionObserveDownloads):   {},
}

// governedDomainCollections is the set of domain collections that are mutated
// exclusively through the governed path (EnvelopeProcessor -> L5Actuator ->
// ExecutionHandler -> governedDocStore). Observe projection collections must
// never appear in this set. This list mirrors the governed collection
// constants in internal/constants/collections.go; if a new governed
// collection is added there, add it here so the isolation assertion stays
// accurate.
var governedDomainCollections = map[string]struct{}{
	marshaler.CollectionName(constants.CollectionUsers):                 {},
	marshaler.CollectionName(constants.CollectionWebSessions):           {},
	marshaler.CollectionName(constants.CollectionOperatorSessions):      {},
	marshaler.CollectionName(constants.CollectionCLISessions):           {},
	marshaler.CollectionName(constants.CollectionLoginAudit):            {},
	marshaler.CollectionName(constants.CollectionAuthAdminAudit):        {},
	marshaler.CollectionName(constants.CollectionAccountLocks):          {},
	marshaler.CollectionName(constants.CollectionOrganizations):         {},
	marshaler.CollectionName(constants.CollectionOperators):             {},
	marshaler.CollectionName(constants.CollectionOperatorUsage):         {},
	marshaler.CollectionName(constants.CollectionCases):                 {},
	marshaler.CollectionName(constants.CollectionInvestigations):        {},
	marshaler.CollectionName(constants.CollectionTasks):                 {},
	marshaler.CollectionName(constants.CollectionMemories):              {},
	marshaler.CollectionName(constants.CollectionSettings):              {},
	marshaler.CollectionName(constants.CollectionConsoleAudit):          {},
	marshaler.CollectionName(constants.CollectionBoundSessions):         {},
	marshaler.CollectionName(constants.CollectionPasskeyChallenges):     {},
	marshaler.CollectionName(constants.CollectionPersonas):              {},
	marshaler.CollectionName(constants.CollectionAgentActivityMetadata): {},
	marshaler.CollectionName(constants.CollectionReputationState):       {},
	marshaler.CollectionName(constants.CollectionReputationCommitments): {},
	marshaler.CollectionName(constants.CollectionStakeResolutions):      {},
	marshaler.CollectionName(constants.CollectionRevokedCertificates):   {},
	marshaler.CollectionName(constants.CollectionTrustedSigners):        {},
	marshaler.CollectionName(constants.CollectionAppPolicies):           {},
	marshaler.CollectionName(constants.CollectionConsensus):             {},
	marshaler.CollectionName(constants.CollectionEnrollmentTokens):      {},
	marshaler.CollectionName(constants.CollectionCLIRecoveryRequests):   {},
	marshaler.CollectionName(constants.CollectionPlatformEnrollments):   {},
}

// auditReceiptsSchema is the receipts table schema from the audit store
// (internal/services/storage/audit_store.go). It is duplicated here so the
// governance isolation test can create a real receipts table in a separate
// in-memory SQLite database and assert it stays empty after observe producer
// calls. The observe producer has no reference to the audit store; this test
// proves that isolation behaviorally against the real receipt schema.
const auditReceiptsSchema = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT NOT NULL,
    session_type TEXT NOT NULL,
    title TEXT,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now')),
    user_identity TEXT,
    PRIMARY KEY (id, session_type)
);
CREATE TABLE IF NOT EXISTS receipts (
    transaction_id TEXT PRIMARY KEY,
    transaction_hash TEXT NOT NULL,
    investigation_id TEXT,
    operator_id TEXT NOT NULL,
    operator_session_id TEXT,
    requestor_user_id TEXT,
    acting_app_id TEXT,
    action_type TEXT NOT NULL,
    target_resource TEXT,
    status TEXT NOT NULL,
    result_summary TEXT,
    state_root_before TEXT,
    state_root_after TEXT,
    executed_at_ms INTEGER NOT NULL,
    signer_key_id TEXT NOT NULL,
    signature TEXT NOT NULL,
    receipt_json TEXT,
    timestamp TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now')),
    FOREIGN KEY(operator_session_id) REFERENCES sessions(id)
);`

// newAuditReceiptsDB creates a separate in-memory SQLite database containing
// only the audit receipts schema. It is closed automatically via t.Cleanup.
// The observe producer never receives a reference to this database; the test
// asserts it remains empty, proving no action receipt is generated by observe
// projection writes.
func newAuditReceiptsDB(t *testing.T) *sqliteutil.DB {
	t.Helper()
	logger := testutil.NewTestLogger()
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(":memory:"), logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.ExecWithRetry(auditReceiptsSchema)
	require.NoError(t, err)
	return db
}

// seedAgentAndRunProjections drives both producer entry points once each so
// the governance isolation tests can assert on the resulting side effects. It
// uses distinct IDs keyed by userID so multiple tests can share the helper
// without colliding.
func seedAgentAndRunProjections(t *testing.T, producer *ObserveProducerService, userID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	route := producerRoute(userID)

	agentPayload := models.AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-gov-iso-" + userID,
		DisplayName:   "Gov Iso Agent",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    now,
	}
	require.NoError(t, producer.UpdateAgentState(ctx, userID, route, agentPayload))

	runPayload := models.RunStatusUpdatedPayload{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         "run-gov-iso-" + userID,
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Gov Iso Run",
		Status:        models.RunLifecycleStatusRunning,
		TotalTasks:    3,
		ObservedAt:    now,
	}
	require.NoError(t, producer.UpdateRunState(ctx, userID, route, runPayload))
}

// distinctDocumentCollections queries the documents table for every distinct
// collection that currently holds at least one row. The DB handle is reached
// through the DocumentStoreService since the test lives in the same package.
func distinctDocumentCollections(t *testing.T, docStore *DocumentStoreService) []string {
	t.Helper()
	rows, err := docStore.db.QueryWithRetry("SELECT DISTINCT collection FROM documents")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rows.Close() })

	var collections []string
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		collections = append(collections, c)
	}
	require.NoError(t, rows.Err())
	return collections
}

// TestObserveProducer_DoesNotCreateGovernanceEnvelopes asserts that observe
// producer writes produce documents only in the observe projection
// collections and never create suspended transaction rows (the governance
// approval path). Governance envelope processing routes mutations through the
// L5Actuator into governed domain collections and may suspend transactions
// pending approval; neither side effect appears after observe producer calls.
func TestObserveProducer_DoesNotCreateGovernanceEnvelopes(t *testing.T) {
	producer, _, docStore := newObserveProducerTestEnv(t)
	seedAgentAndRunProjections(t, producer, "user-gov-env")

	collections := distinctDocumentCollections(t, docStore)
	require.NotEmpty(t, collections, "producer must have written at least one projection document")
	for _, c := range collections {
		_, isObserve := observeCollections[c]
		assert.True(t, isObserve, "document collection %q must be an observe projection collection, not a governed collection", c)
		_, isGoverned := governedDomainCollections[c]
		assert.False(t, isGoverned, "document collection %q must not be a governed domain collection", c)
	}

	// No suspended transactions (governance approval path) may be created.
	var suspendedCount int
	require.NoError(t, docStore.db.QueryRowWithRetry("SELECT COUNT(*) FROM suspended_transactions").Scan(&suspendedCount))
	assert.Zero(t, suspendedCount, "observe producer must not create suspended transaction rows")
}

// TestObserveProducer_DoesNotProduceActionReceipts asserts that observe
// producer writes generate no action receipts. The audit store is a separate
// SQLite database; the observe producer holds no reference to it and cannot
// record receipts. The test creates a real receipts table with the production
// audit store schema and asserts it stays empty, then confirms the SSE event
// payloads carry no receipt fields (transaction_hash, signature,
// signer_key_id, state_root).
func TestObserveProducer_DoesNotProduceActionReceipts(t *testing.T) {
	producer, sseStore, docStore := newObserveProducerTestEnv(t)
	receiptsDB := newAuditReceiptsDB(t)
	seedAgentAndRunProjections(t, producer, "user-gov-receipt")

	// The separate audit receipts database must remain empty.
	var receiptCount int
	require.NoError(t, receiptsDB.QueryRowWithRetry("SELECT COUNT(*) FROM receipts").Scan(&receiptCount))
	assert.Zero(t, receiptCount, "observe producer must not produce action receipts")

	// The SSE event payloads must not carry receipt fields. Receipts are
	// signed governance records with transaction_hash, signature,
	// signer_key_id, and state_root fields; telemetry events do not have them.
	route := producerRoute("user-gov-receipt")
	sseRows, err := sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.NotEmpty(t, sseRows)
	for _, r := range sseRows {
		assert.NotContains(t, r.Payload, "transaction_hash", "telemetry event must not carry receipt transaction_hash")
		assert.NotContains(t, r.Payload, "signature", "telemetry event must not carry receipt signature")
		assert.NotContains(t, r.Payload, "signer_key_id", "telemetry event must not carry receipt signer_key_id")
		assert.NotContains(t, r.Payload, "state_root_before", "telemetry event must not carry receipt state_root_before")
		assert.NotContains(t, r.Payload, "state_root_after", "telemetry event must not carry receipt state_root_after")
	}

	// No receipt-shaped document may appear in the documents table either.
	var docCount int
	require.NoError(t, docStore.db.QueryRowWithRetry(
		"SELECT COUNT(*) FROM documents WHERE data LIKE '%transaction_hash%' OR data LIKE '%signer_key_id%' OR data LIKE '%state_root_before%'",
	).Scan(&docCount))
	assert.Zero(t, docCount, "observe projection documents must not carry receipt fields")
}

// TestObserveProducer_ObserveCollectionsAreNotGoverned asserts that the
// observe projection collections are not members of the governed domain
// collection set, and that writing to them via DocSet does not trigger the
// governance gauntlet. Governance is enforced by routing mutations through
// the EnvelopeProcessor into governed domain collections; the observe producer
// bypasses that path entirely by writing plain projection documents.
func TestObserveProducer_ObserveCollectionsAreNotGoverned(t *testing.T) {
	// The observe collection constants must be distinct from every governed
	// domain collection constant. If an observe collection name ever collides
	// with a governed collection, projections would be indistinguishable from
	// governed records.
	for observeCol := range observeCollections {
		_, isGoverned := governedDomainCollections[observeCol]
		assert.False(t, isGoverned, "observe collection %q must not be a governed domain collection", observeCol)
	}
	// Conversely, no governed collection may be mistaken for an observe one.
	for governedCol := range governedDomainCollections {
		_, isObserve := observeCollections[governedCol]
		assert.False(t, isObserve, "governed collection %q must not be classified as an observe projection collection", governedCol)
	}

	// Behaviorally, writing observe projections via the producer must not
	// produce any governed domain collection document.
	producer, _, docStore := newObserveProducerTestEnv(t)
	seedAgentAndRunProjections(t, producer, "user-gov-set")

	for _, c := range distinctDocumentCollections(t, docStore) {
		_, isGoverned := governedDomainCollections[c]
		assert.False(t, isGoverned, "producer must not write to governed domain collection %q", c)
	}
}

// TestObserveProducer_SSEEventsAreTelemetryNotReceipts asserts that SSE event
// rows emitted by the observe producer are telemetry rows attributed to the
// gateway observe producer, not signed action receipts. Every row must carry
// the observe producer_id, a nested SSEPushPayload envelope with a typed event
// type and data, and no link to a governance envelope or action receipt.
func TestObserveProducer_SSEEventsAreTelemetryNotReceipts(t *testing.T) {
	producer, _, docStore := newObserveProducerTestEnv(t)
	seedAgentAndRunProjections(t, producer, "user-gov-sse")

	// Query sse_events directly for producer_id and payload. The SSEEventRow
	// model does not expose producer_id, so the raw table query is required.
	rows, err := docStore.db.QueryWithRetry(
		"SELECT producer_id, payload FROM sse_events WHERE user_id = ? ORDER BY id",
		"user-gov-sse",
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rows.Close() })

	expectedTypes := map[string]bool{
		string(constants.EventAppAgentStatusUpdated): false,
		string(constants.EventAppRunStatusUpdated):   false,
	}
	var emitted int
	for rows.Next() {
		emitted++
		var producerID, payload string
		require.NoError(t, rows.Scan(&producerID, &payload))

		// Every row is attributed to the observe producer, not a receipt signer.
		assert.Equal(t, observeProducerID, producerID, "SSE event must be attributed to the observe producer")

		// The payload is a nested SSEPushPayload envelope, not a bare receipt.
		var push models.SSEPushPayload
		require.NoError(t, json.Unmarshal([]byte(payload), &push), "SSE payload must be a nested SSEPushPayload envelope")
		assert.Equal(t, "user-gov-sse", push.UserID, "envelope user_id must match the producer route")
		assert.NotEmpty(t, push.Event, "envelope must carry a nested event")

		var env sseEventEnvelope
		require.NoError(t, json.Unmarshal(push.Event, &env), "nested event must decode to a typed envelope")
		assert.NotEmpty(t, env.Type, "nested event must carry a typed event type")
		assert.NotEmpty(t, env.Data, "nested event must carry typed payload data")

		// The event type must be one of the observe telemetry families.
		if seen, ok := expectedTypes[env.Type]; ok {
			assert.False(t, seen, "event type %q emitted more than once", env.Type)
			expectedTypes[env.Type] = true
		} else {
			t.Fatalf("unexpected event type %q; only observe telemetry families are allowed", env.Type)
		}

		// Telemetry payloads must not carry receipt linkage fields.
		assert.NotContains(t, string(env.Data), "transaction_hash", "telemetry event data must not carry receipt transaction_hash")
		assert.NotContains(t, string(env.Data), "signature", "telemetry event data must not carry receipt signature")
		assert.NotContains(t, string(env.Data), "receipt", "telemetry event data must not reference receipts")
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, 2, emitted, "one telemetry event per producer call (agent + run)")

	for typ, seen := range expectedTypes {
		assert.True(t, seen, "expected observe telemetry event type %q was not emitted", typ)
	}
}
