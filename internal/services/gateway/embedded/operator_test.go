// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package embedded

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// memStore mirrors the document-store rules the substrate depends on:
// DocGet returns nil for a missing row, DocSet strips id and timestamp
// keys from the body, and DocUpdate merges fields while skipping those
// same keys. Gateway integration tests still run this path against
// DocumentStoreService.
type memStore struct {
	docs map[string]map[string]map[string]json.RawMessage
}

func newMemStore() *memStore {
	return &memStore{docs: map[string]map[string]map[string]json.RawMessage{}}
}

func (s *memStore) DocGet(collection, id string) (*models.Document, error) {
	col := s.docs[collection]
	if col == nil {
		return nil, nil
	}
	data, ok := col[id]
	if !ok {
		return nil, nil
	}
	return &models.Document{ID: id, Collection: collection, Data: cloneRawMap(data)}, nil
}

func (s *memStore) DocSet(collection, id string, data json.RawMessage) error {
	parsed, err := decodeRawObject(data)
	if err != nil {
		return err
	}
	delete(parsed, "id")
	delete(parsed, "created_at")
	delete(parsed, "updated_at")
	if s.docs[collection] == nil {
		s.docs[collection] = map[string]map[string]json.RawMessage{}
	}
	s.docs[collection][id] = parsed
	return nil
}

func (s *memStore) DocUpdate(collection, id string, fields json.RawMessage) (*models.Document, error) {
	col := s.docs[collection]
	if col == nil || col[id] == nil {
		return nil, constants.ErrNotFound
	}
	incoming, err := decodeRawObject(fields)
	if err != nil {
		return nil, err
	}
	doc := col[id]
	for k, v := range incoming {
		if k == "id" || k == "created_at" || k == "updated_at" {
			continue
		}
		var nullCheck any
		if err := json.Unmarshal(v, &nullCheck); err == nil && nullCheck == nil {
			delete(doc, k)
			continue
		}
		doc[k] = v
	}
	return &models.Document{ID: id, Collection: collection, Data: cloneRawMap(doc)}, nil
}

func (s *memStore) count(collection string) int {
	return len(s.docs[collection])
}

type persistCall struct {
	operatorSessionID string
	userID            string
	orgID             string
	operatorID        string
	loginMethod       string
}

type memSessions struct {
	calls []persistCall
	err   error
}

func (m *memSessions) PersistOperatorSession(operatorSessionID, userID, orgID, operatorID, loginMethod string) error {
	m.calls = append(m.calls, persistCall{
		operatorSessionID: operatorSessionID,
		userID:            userID,
		orgID:             orgID,
		operatorID:        operatorID,
		loginMethod:       loginMethod,
	})
	return m.err
}

func decodeRawObject(data json.RawMessage) (map[string]json.RawMessage, error) {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	if parsed == nil {
		parsed = map[string]json.RawMessage{}
	}
	return parsed, nil
}

func cloneRawMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	for k, v := range in {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}

func loadOperator(t *testing.T, store *memStore) *models.OperatorDocumentGo {
	t.Helper()
	doc, err := store.DocGet(marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
	require.NoError(t, err)
	require.NotNil(t, doc)
	b, err := json.Marshal(doc.Data)
	require.NoError(t, err)
	var op models.OperatorDocumentGo
	require.NoError(t, json.Unmarshal(b, &op))
	op.ID = doc.ID
	return &op
}

func TestRegisterPending_Idempotent(t *testing.T) {
	store := newMemStore()
	svc := New(store, &memSessions{})

	require.NoError(t, svc.RegisterPending())

	op := loadOperator(t, store)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), op.ID)
	assert.Equal(t, constants.OperatorTypeEmbedded, op.OperatorType)
	assert.Equal(t, constants.ComponentNameG8EO, op.Component)
	assert.Equal(t, constants.OperatorStatusAvailable, op.Status)
	assert.False(t, op.Claimed)
	assert.False(t, op.IsSlot)
	assert.Empty(t, op.UserID)
	assert.Empty(t, op.OperatorSessionID)
	assert.Equal(t, 1, store.count(marshaler.CollectionName(constants.CollectionOperators)))

	require.NoError(t, svc.RegisterPending())
	assert.Equal(t, 1, store.count(marshaler.CollectionName(constants.CollectionOperators)))

	now := time.Now().UTC()
	_, sessionID, err := svc.Claim("user-claim", "fp", now)
	require.NoError(t, err)
	require.NoError(t, svc.RegisterPending())
	claimed := loadOperator(t, store)
	assert.True(t, claimed.Claimed)
	assert.Equal(t, "user-claim", claimed.UserID)
	assert.Equal(t, sessionID, claimed.OperatorSessionID)
	assert.Equal(t, "fp", claimed.SystemFingerprint)
}

func TestClaim_SameUserReclaimIsIdempotent(t *testing.T) {
	store := newMemStore()
	svc := New(store, &memSessions{})
	now := time.Now().UTC()

	operatorID, sessionID, err := svc.Claim("user-same", "fp", now)
	require.NoError(t, err)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), operatorID)
	assert.NotEmpty(t, sessionID)

	operatorID2, sessionID2, err := svc.Claim("user-same", "fp", now)
	require.NoError(t, err)
	assert.Equal(t, operatorID, operatorID2)
	assert.Equal(t, sessionID, sessionID2)
}

func TestClaim_DifferentUserRejected(t *testing.T) {
	store := newMemStore()
	svc := New(store, &memSessions{})
	now := time.Now().UTC()

	_, _, err := svc.Claim("user-owner", "fp", now)
	require.NoError(t, err)

	_, _, err = svc.Claim("user-other", "fp", now)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEmbeddedOperatorClaimed)
}

func TestClaim_MissingPendingDoc_ClaimsAnyway(t *testing.T) {
	store := newMemStore()
	svc := New(store, &memSessions{})

	operatorID, sessionID, err := svc.Claim("user-heal", "fp-heal", time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), operatorID)
	assert.NotEmpty(t, sessionID)

	op := loadOperator(t, store)
	assert.True(t, op.Claimed)
	assert.Equal(t, "user-heal", op.UserID)
	assert.Equal(t, sessionID, op.OperatorSessionID)
	assert.Equal(t, "fp-heal", op.SystemFingerprint)
	assert.Equal(t, constants.OperatorTypeEmbedded, op.OperatorType)
	assert.Equal(t, constants.OperatorStatusActive, op.Status)
}

func TestClaimEmbeddedOperator_PersistsBootstrapSession(t *testing.T) {
	store := newMemStore()
	sessions := &memSessions{}
	svc := New(store, sessions)

	operatorID, sessionID, err := svc.ClaimEmbeddedOperator("user-browser")
	require.NoError(t, err)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), operatorID)
	require.Len(t, sessions.calls, 1)
	assert.Equal(t, persistCall{
		operatorSessionID: sessionID,
		userID:            "user-browser",
		orgID:             "user-browser",
		operatorID:        operatorID,
		loginMethod:       string(constants.HeartbeatTypeBootstrap),
	}, sessions.calls[0])

	operatorID2, sessionID2, err := svc.ClaimEmbeddedOperator("user-browser")
	require.NoError(t, err)
	assert.Equal(t, operatorID, operatorID2)
	assert.Equal(t, sessionID, sessionID2)
	require.Len(t, sessions.calls, 2)
	assert.Equal(t, sessions.calls[0], sessions.calls[1])

	_, _, err = svc.ClaimEmbeddedOperator("user-other")
	require.ErrorIs(t, err, constants.ErrEmbeddedOperatorClaimed)
	assert.Len(t, sessions.calls, 2)
}
