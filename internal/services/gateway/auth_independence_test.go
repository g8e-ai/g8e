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

package gateway

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthStatusIndependence(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	// Use WAL mode to avoid SQLITE_BUSY errors in parallel tests
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	// Enable WAL mode to reduce SQLITE_BUSY contention
	_, err = db.db.Exec("PRAGMA journal_mode=WAL")
	require.NoError(t, err)

	pkiDir := t.TempDir()
	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)
	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	userSvc := NewUserService(db, logger)
	resp := responder.New(logger)
	auth := NewAuthService(db, pki, logger, userSvc, resp, secretsDir)

	t.Run("ValidateOperatorSession succeeds even if status is OFFLINE", func(t *testing.T) {
		t.Parallel()
		sessionID := "test-session-offline"
		opID := "op-offline"
		userID := "user-offline"

		// Create an operator document with OFFLINE status
		// Note: CreatedAt field in the struct is what ValidateOperatorSession checks
		op := &models.OperatorDocumentGo{
			ID:                opID,
			UserID:            userID,
			OperatorSessionID: sessionID,
			Status:            constants.OperatorStatusOffline,
			CreatedAt:         time.Now().UTC(),
		}
		opBytes, _ := json.Marshal(op)
		err = db.DocSet(marshaler.CollectionName(constants.CollectionOperators), opID, opBytes)
		require.NoError(t, err)

		// Create the linked user
		user := &models.User{
			ID:     userID,
			Status: constants.UserStatusActive,
		}
		userBytes, _ := json.Marshal(user)
		err = db.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes)
		require.NoError(t, err)

		// Validate session - should succeed despite OFFLINE status
		validatedOp, err := auth.ValidateOperatorSession(sessionID)
		assert.NoError(t, err)
		assert.NotNil(t, validatedOp)
		assert.Equal(t, opID, validatedOp.ID)
		assert.Equal(t, constants.OperatorStatusOffline, validatedOp.Status)
	})

	t.Run("ValidateOperatorSession fails if session is expired", func(t *testing.T) {
		t.Parallel()
		sessionID := "test-session-expired"
		opID := "op-expired"
		userID := "user-expired"

		// Create an operator document with an old CreatedAt using the test hook
		oldTime := time.Now().UTC().Add(-48 * time.Hour) // 48h > 24h TTL
		op := &models.OperatorDocumentGo{
			ID:                opID,
			UserID:            userID,
			OperatorSessionID: sessionID,
			Status:            constants.OperatorStatusActive,
			CreatedAt:         oldTime,
		}
		opBytes, _ := json.Marshal(op)
		err = db.DocSetWithTimestamps(marshaler.CollectionName(constants.CollectionOperators), opID, opBytes, oldTime, oldTime)
		require.NoError(t, err)

		// Validate session - should fail
		validatedOp, err := auth.ValidateOperatorSession(sessionID)
		require.Error(t, err)
		assert.Nil(t, validatedOp)

		// AuthError.Error() returns JSON, so check the message in the JSON or use type assertion
		ae, ok := err.(*AuthError)
		require.True(t, ok, "Error should be of type *AuthError")
		assert.Equal(t, "operator session expired", ae.Message)
		assert.Equal(t, "ttl_exceeded", ae.Reason)
	})

	t.Run("ValidateOperatorSession fails if status is TERMINATED", func(t *testing.T) {
		t.Parallel()
		sessionID := "test-session-terminated"
		opID := "op-terminated"
		userID := "user-terminated"

		// Create an operator document with TERMINATED status
		op := &models.OperatorDocumentGo{
			ID:                opID,
			UserID:            userID,
			OperatorSessionID: sessionID,
			Status:            constants.OperatorStatusTerminated,
			CreatedAt:         time.Now().UTC(),
		}
		opBytes, _ := json.Marshal(op)
		err = db.DocSet(marshaler.CollectionName(constants.CollectionOperators), opID, opBytes)
		require.NoError(t, err)

		// Validate session - should fail
		validatedOp, err := auth.ValidateOperatorSession(sessionID)
		assert.Error(t, err)
		assert.Nil(t, validatedOp)

		ae, ok := err.(*AuthError)
		require.True(t, ok)
		assert.Equal(t, "operator identity disabled", ae.Message)
		assert.Equal(t, marshaler.Status(constants.OperatorStatusTerminated), ae.Reason)
	})
}

func TestAPIKeyStatusIndependence(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	apiKeySvc := NewAPIKeyService(db, logger)

	t.Run("ValidateKey succeeds even if status is STALE", func(t *testing.T) {
		t.Parallel()
		rawKey := "g8e-test-key-stale-12345678901234"
		docID := rawKey[:20]
		userID := "user-stale"

		// Create an API key document with STALE status
		keyDoc := map[string]interface{}{
			"id":              docID,
			"user_id":         userID,
			"organization_id": "org-1",
			"status":          constants.OperatorStatusStale,
			"created_at":      time.Now().UnixMilli(),
		}
		keyBytes, _ := json.Marshal(keyDoc)
		err = db.DocSet(marshaler.CollectionName(constants.CollectionAPIKeys), docID, keyBytes)
		require.NoError(t, err)

		// Validate key - should succeed despite STALE status
		doc, err := apiKeySvc.ValidateKey(rawKey)
		assert.NoError(t, err)
		assert.NotNil(t, doc)
	})

	t.Run("ValidateKey fails if status is TERMINATED", func(t *testing.T) {
		t.Parallel()
		rawKey := "g8e-test-key-term-12345678901234"
		docID := rawKey[:20]
		userID := "user-term"

		// Create an API key document with TERMINATED status
		keyDoc := map[string]interface{}{
			"id":              docID,
			"user_id":         userID,
			"organization_id": "org-1",
			"status":          constants.OperatorStatusTerminated,
			"created_at":      time.Now().UnixMilli(),
		}
		keyBytes, _ := json.Marshal(keyDoc)
		err = db.DocSet(marshaler.CollectionName(constants.CollectionAPIKeys), docID, keyBytes)
		require.NoError(t, err)

		// Validate key - should fail
		doc, err := apiKeySvc.ValidateKey(rawKey)
		assert.Error(t, err)
		assert.Nil(t, doc)
		assert.Contains(t, err.Error(), "terminated")
	})
}
