// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// inMemoryGovernedDocStore is a faithful in-memory implementation of
// governance.GovernedDocumentStore that mirrors the real
// DocumentStoreService merge/replace/delete semantics. It is used in
// integration tests because the pubsub package cannot import the gateway
// package (import cycle). The real SQLite-backed merge semantics are tested
// in gateway/document_store_service_test.go.
type inMemoryGovernedDocStore struct {
	mu   sync.Mutex
	docs map[string]map[string]map[string]json.RawMessage // collection -> id -> fields
}

func newInMemoryGovernedDocStore() *inMemoryGovernedDocStore {
	return &inMemoryGovernedDocStore{docs: make(map[string]map[string]map[string]json.RawMessage)}
}

func (s *inMemoryGovernedDocStore) DocReplace(collection, id string, data json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.docs[collection] == nil {
		s.docs[collection] = make(map[string]map[string]json.RawMessage)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	delete(parsed, "id")
	delete(parsed, "created_at")
	delete(parsed, "updated_at")
	s.docs[collection][id] = parsed
	return nil
}

func (s *inMemoryGovernedDocStore) DocMerge(collection, id string, fields json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.docs[collection] == nil || s.docs[collection][id] == nil {
		return constants.ErrNotFound
	}
	existing := s.docs[collection][id]
	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(fields, &incoming); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	merged := make(map[string]json.RawMessage)
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range incoming {
		if k == "id" || k == "created_at" || k == "updated_at" {
			continue
		}
		var nullCheck interface{}
		if err := json.Unmarshal(v, &nullCheck); err == nil && nullCheck == nil {
			delete(merged, k)
		} else {
			merged[k] = v
		}
	}
	s.docs[collection][id] = merged
	return nil
}

func (s *inMemoryGovernedDocStore) DocDelete(collection, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.docs[collection] != nil {
		delete(s.docs[collection], id)
	}
	return nil
}

func (s *inMemoryGovernedDocStore) Get(collection, id string) (map[string]json.RawMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.docs[collection] == nil {
		return nil, false
	}
	data, ok := s.docs[collection][id]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent mutation.
	out := make(map[string]json.RawMessage)
	for k, v := range data {
		out[k] = v
	}
	return out, true
}

// TestHandleDocumentUpdateSync_MergePreservesUntouchedFields is the Bug 10
// regression test. It creates a complete investigation document via
// merge=false, then applies a title-only merge=true update, and asserts that
// all original required fields (case_id, user_id, sentinel_mode) survive the
// merge. Before the fix, the handler always called DocSet regardless of
// merge, replacing the complete document with the small patch.
func TestHandleDocumentUpdateSync_MergePreservesUntouchedFields(t *testing.T) {
	docStore := newInMemoryGovernedDocStore()
	f := newPubsubFixture(t)
	f.Svc.governedDocStore = docStore

	// Step 1: Create a complete investigation document via merge=false.
	fullUpdates, err := structpb.NewStruct(map[string]interface{}{
		"case_id":       "case-abc-123",
		"user_id":       "user-xyz-789",
		"sentinel_mode": true,
		"case_title":    "Create a file at /tmp/g8e-smoke-test.txt",
		"status":        "open",
		"history":       []interface{}{"created"},
	})
	require.NoError(t, err)
	createMsg := &PubSubCommandMessage{
		EventType: constants.EventAppInvestigationCreated,
		ID:        "msg-create",
		Payload: mustMarshalProto(t, &operatorv1.DocumentUpdateRequested{
			Collection: "investigations",
			DocumentId: "inv-001",
			Updates:    fullUpdates,
			Merge:      false,
		}),
	}
	summary, err := f.Svc.handleDocumentUpdateSync(context.Background(), createMsg)
	require.NoError(t, err)
	assert.Contains(t, summary, "investigations/inv-001")

	// Step 2: Apply a title-only merge=true update (simulates the concurrent
	// _generate_and_update_title flow that caused Bug 10).
	mergeUpdates, err := structpb.NewStruct(map[string]interface{}{
		"case_title": "Generated Title: Create a file",
		"history":    []interface{}{"created", "title_generated"},
	})
	require.NoError(t, err)
	mergeMsg := &PubSubCommandMessage{
		EventType: constants.EventAppInvestigationUpdated,
		ID:        "msg-merge",
		Payload: mustMarshalProto(t, &operatorv1.DocumentUpdateRequested{
			Collection: "investigations",
			DocumentId: "inv-001",
			Updates:    mergeUpdates,
			Merge:      true,
		}),
	}
	_, err = f.Svc.handleDocumentUpdateSync(context.Background(), mergeMsg)
	require.NoError(t, err)

	// Step 3: Read the document back and verify ALL original fields survived.
	data, ok := docStore.Get("investigations", "inv-001")
	require.True(t, ok, "document must exist after merge")

	// Required fields that were missing after the destructive merge in Bug 10.
	assert.Equal(t, "case-abc-123", rawString(t, data["case_id"]),
		"merge must preserve case_id (Bug 10 regression)")
	assert.Equal(t, "user-xyz-789", rawString(t, data["user_id"]),
		"merge must preserve user_id (Bug 10 regression)")
	assert.Equal(t, true, rawBool(t, data["sentinel_mode"]),
		"merge must preserve sentinel_mode (Bug 10 regression)")

	// Original field that was not in the merge patch.
	assert.Equal(t, "open", rawString(t, data["status"]),
		"merge must preserve untouched fields")

	// Merged fields should have the new values.
	assert.Equal(t, "Generated Title: Create a file", rawString(t, data["case_title"]),
		"merge must apply the updated title")
}

// TestHandleDocumentUpdateSync_ReplaceOverwritesAllFields verifies that
// merge=false replaces the entire document, removing fields not present in
// the new data. This is the expected behavior for create/replacement.
func TestHandleDocumentUpdateSync_ReplaceOverwritesAllFields(t *testing.T) {
	docStore := newInMemoryGovernedDocStore()
	f := newPubsubFixture(t)
	f.Svc.governedDocStore = docStore

	// Create initial document.
	initial, err := structpb.NewStruct(map[string]interface{}{
		"case_id":    "case-1",
		"user_id":    "user-1",
		"case_title": "Initial title",
		"status":     "open",
	})
	require.NoError(t, err)
	createMsg := &PubSubCommandMessage{
		EventType: constants.EventAppCaseCreated,
		ID:        "msg-1",
		Payload: mustMarshalProto(t, &operatorv1.DocumentUpdateRequested{
			Collection: "cases",
			DocumentId: "case-001",
			Updates:    initial,
			Merge:      false,
		}),
	}
	_, err = f.Svc.handleDocumentUpdateSync(context.Background(), createMsg)
	require.NoError(t, err)

	// Replace with a smaller document (merge=false).
	replacement, err := structpb.NewStruct(map[string]interface{}{
		"case_title": "Replaced title",
	})
	require.NoError(t, err)
	replaceMsg := &PubSubCommandMessage{
		EventType: constants.EventAppCaseUpdated,
		ID:        "msg-2",
		Payload: mustMarshalProto(t, &operatorv1.DocumentUpdateRequested{
			Collection: "cases",
			DocumentId: "case-001",
			Updates:    replacement,
			Merge:      false,
		}),
	}
	_, err = f.Svc.handleDocumentUpdateSync(context.Background(), replaceMsg)
	require.NoError(t, err)

	data, ok := docStore.Get("cases", "case-001")
	require.True(t, ok)

	// New field present.
	assert.Equal(t, "Replaced title", rawString(t, data["case_title"]))
	// Old fields removed by replace.
	_, hasCaseID := data["case_id"]
	assert.False(t, hasCaseID, "replace must remove fields not in the new data")
	_, hasStatus := data["status"]
	assert.False(t, hasStatus, "replace must remove fields not in the new data")
}

// TestHandleDocumentDeleteSync_RemovesDocument verifies that a document
// created via merge=false is removed by a subsequent delete operation.
func TestHandleDocumentDeleteSync_RemovesDocument(t *testing.T) {
	docStore := newInMemoryGovernedDocStore()
	f := newPubsubFixture(t)
	f.Svc.governedDocStore = docStore

	// Create a document.
	updates, err := structpb.NewStruct(map[string]interface{}{
		"case_title": "To be deleted",
	})
	require.NoError(t, err)
	createMsg := &PubSubCommandMessage{
		EventType: constants.EventAppCaseCreated,
		ID:        "msg-1",
		Payload: mustMarshalProto(t, &operatorv1.DocumentUpdateRequested{
			Collection: "cases",
			DocumentId: "case-del-001",
			Updates:    updates,
			Merge:      false,
		}),
	}
	_, err = f.Svc.handleDocumentUpdateSync(context.Background(), createMsg)
	require.NoError(t, err)

	// Verify it exists.
	_, ok := docStore.Get("cases", "case-del-001")
	require.True(t, ok)

	// Delete it.
	deleteMsg := &PubSubCommandMessage{
		EventType: constants.EventAppCaseDeleted,
		ID:        "msg-2",
		Payload: mustMarshalProto(t, &operatorv1.DocumentDeleteRequested{
			Collection: "cases",
			DocumentId: "case-del-001",
		}),
	}
	summary, err := f.Svc.handleDocumentDeleteSync(context.Background(), deleteMsg)
	require.NoError(t, err)
	assert.Contains(t, summary, "cases/case-del-001")

	// Verify it's gone.
	_, ok = docStore.Get("cases", "case-del-001")
	assert.False(t, ok, "document must be absent after delete")
}

// TestHandleDocumentUpdateSync_MergeFailsOnMissingDocument verifies that
// merging into a non-existent document fails closed with ErrNotFound rather
// than silently creating a partial document.
func TestHandleDocumentUpdateSync_MergeFailsOnMissingDocument(t *testing.T) {
	docStore := newInMemoryGovernedDocStore()
	f := newPubsubFixture(t)
	f.Svc.governedDocStore = docStore

	mergeUpdates, err := structpb.NewStruct(map[string]interface{}{
		"case_title": "Updated title for nonexistent",
	})
	require.NoError(t, err)
	mergeMsg := &PubSubCommandMessage{
		EventType: constants.EventAppInvestigationUpdated,
		ID:        "msg-1",
		Payload: mustMarshalProto(t, &operatorv1.DocumentUpdateRequested{
			Collection: "investigations",
			DocumentId: "nonexistent-inv",
			Updates:    mergeUpdates,
			Merge:      true,
		}),
	}
	_, err = f.Svc.handleDocumentUpdateSync(context.Background(), mergeMsg)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound,
		"merge into a missing document must fail with ErrNotFound, not silently create a partial document")
}

// rawString decodes a json.RawMessage as a string, failing the test on error.
func rawString(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var s string
	require.NoError(t, json.Unmarshal(raw, &s))
	return s
}

// rawBool decodes a json.RawMessage as a bool, failing the test on error.
func rawBool(t *testing.T, raw json.RawMessage) bool {
	t.Helper()
	var b bool
	require.NoError(t, json.Unmarshal(raw, &b))
	return b
}

// Ensure protojson and proto imports are used (suppress unused import in some build configurations).
var _ = protojson.Marshal
var _ = proto.Marshal
var _ = testutil.NewTestLogger
