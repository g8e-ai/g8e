// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

func TestWebSessionService_CreateWebSession(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.WebSessionSvc

	t.Run("Success", func(t *testing.T) {
		userID := "user-123"
		session, err := svc.CreateWebSession(userID)
		require.NoError(t, err)
		require.NotNil(t, session)

		assert.NotEmpty(t, session.ID)
		assert.Equal(t, userID, session.UserID)
		assert.True(t, session.CreatedAtUnixMs > 0)
		assert.True(t, session.ExpiresAtUnixMs > session.CreatedAtUnixMs)

		// Verify it's actually in the DB
		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionWebSessions), session.ID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		var stored models.WebSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)
		stored.ID = doc.ID
		assert.Equal(t, session.ID, stored.ID)
		assert.Equal(t, userID, stored.UserID)
	})

	t.Run("Empty UserID", func(t *testing.T) {
		session, err := svc.CreateWebSession("")
		require.NoError(t, err) // Service currently doesn't validate empty userID
		require.NotNil(t, session)
		assert.Equal(t, "", session.UserID)
	})
}

func TestWebSessionService_ValidateWebSession(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.WebSessionSvc

	t.Run("Success", func(t *testing.T) {
		userID := "user-456"
		created, err := svc.CreateWebSession(userID)
		require.NoError(t, err)

		validated, err := svc.ValidateWebSession(created.ID)
		require.NoError(t, err)
		require.NotNil(t, validated)
		assert.Equal(t, created.ID, validated.ID)
		assert.Equal(t, userID, validated.UserID)
	})

	t.Run("Not Found", func(t *testing.T) {
		validated, err := svc.ValidateWebSession("non-existent-id")
		require.Error(t, err)
		assert.Nil(t, validated)
	})

	t.Run("Expired Session", func(t *testing.T) {
		userID := "user-789"
		// Manually create an expired session in DB
		sessionID := "expired-session"
		expiredSession := &models.WebSession{
			ID:              sessionID,
			UserID:          userID,
			CreatedAtUnixMs: time.Now().Add(-48 * time.Hour).UnixMilli(),
			ExpiresAtUnixMs: time.Now().Add(-24 * time.Hour).UnixMilli(),
		}

		data, err := json.Marshal(expiredSession)
		require.NoError(t, err)
		err = infra.Stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionWebSessions), sessionID, data)
		require.NoError(t, err)

		validated, err := svc.ValidateWebSession(sessionID)
		require.Error(t, err)
		assert.Nil(t, validated)
	})

	t.Run("Malformed Data in DB", func(t *testing.T) {
		sessionID := "malformed-session"
		// Set some invalid data that cannot be unmarshaled into models.WebSession
		err := infra.Stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionWebSessions), sessionID, []byte(`{"expires_at_unix_ms": "invalid-type"}`))
		require.NoError(t, err)

		validated, err := svc.ValidateWebSession(sessionID)
		require.Error(t, err)
		assert.Nil(t, validated)
	})
}
