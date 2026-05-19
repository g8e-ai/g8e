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

package listen

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/marshaler"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthStatusIndependence(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := NewListenDBService(dbDir, secretsDir, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pkiDir := t.TempDir()
	pki := newPKIAuthority(dbDir, pkiDir, db, logger)
	userSvc := NewUserService(db, logger)
	auth := NewAuthService(db, pki, logger, userSvc, secretsDir)

	t.Run("ValidateOperatorSession succeeds even if status is OFFLINE", func(t *testing.T) {
		sessionID := "test-session-offline"
		opID := "op-offline"
		userID := "user-offline"

		// Create an operator document with OFFLINE status
		// Note: CreatedAt field in the struct is what ValidateOperatorSession checks
		op := &models.OperatorDocumentGo{
			ID:                opID,
			UserID:            userID,
			OperatorSessionID: sessionID,
			Status:            constants.Status.OperatorStatus.Offline,
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
		assert.Equal(t, constants.Status.OperatorStatus.Offline, validatedOp.Status)
	})

	t.Run("ValidateOperatorSession fails if session is expired", func(t *testing.T) {
		sessionID := "test-session-expired"
		opID := "op-expired"
		userID := "user-expired"

		// Create an operator document with an old CreatedAt
		// We must set it in the JSON data because ValidateOperatorSession unmarshals it.
		op := &models.OperatorDocumentGo{
			ID:                opID,
			UserID:            userID,
			OperatorSessionID: sessionID,
			Status:            constants.Status.OperatorStatus.Active,
			CreatedAt:         time.Now().UTC().Add(-48 * time.Hour), // 48h > 24h TTL
		}
		opBytes, _ := json.Marshal(op)
		err = db.DocSet(marshaler.CollectionName(constants.CollectionOperators), opID, opBytes)
		require.NoError(t, err)

		// Manually update created_at in the DB to bypass DocSet's auto-timestamping
		oldTime := time.Now().UTC().Add(-48 * time.Hour)
		_, err = db.db.Exec("UPDATE documents SET created_at = ? WHERE collection = ? AND id = ?",
			oldTime.Format(time.RFC3339Nano), marshaler.CollectionName(constants.CollectionOperators), opID)
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
		sessionID := "test-session-terminated"
		opID := "op-terminated"
		userID := "user-terminated"

		// Create an operator document with TERMINATED status
		op := &models.OperatorDocumentGo{
			ID:                opID,
			UserID:            userID,
			OperatorSessionID: sessionID,
			Status:            constants.Status.OperatorStatus.Terminated,
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
		assert.Equal(t, marshaler.Status(constants.Status.OperatorStatus.Terminated), ae.Reason)
	})
}

func TestApiKeyStatusIndependence(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := NewListenDBService(dbDir, secretsDir, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	apiKeySvc := NewApiKeyService(db, logger)

	t.Run("ValidateKey succeeds even if status is STALE", func(t *testing.T) {
		rawKey := "g8e-test-key-stale-12345678901234"
		docID := rawKey[:20]
		userID := "user-stale"

		// Create an API key document with STALE status
		keyDoc := map[string]interface{}{
			"id":              docID,
			"user_id":         userID,
			"organization_id": "org-1",
			"status":          constants.Status.OperatorStatus.Stale,
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
		rawKey := "g8e-test-key-term-12345678901234"
		docID := rawKey[:20]
		userID := "user-term"

		// Create an API key document with TERMINATED status
		keyDoc := map[string]interface{}{
			"id":              docID,
			"user_id":         userID,
			"organization_id": "org-1",
			"status":          constants.Status.OperatorStatus.Terminated,
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
