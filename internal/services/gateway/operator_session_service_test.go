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
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

func TestNewOperatorSessionService(t *testing.T) {
	infra := setupTestInfrastructure(t, true)

	svc := NewOperatorSessionService(infra.DB, infra.Logger)

	require.NotNil(t, svc)
	assert.Equal(t, infra.DB, svc.db)
	assert.Equal(t, infra.Logger, svc.logger)
}

func TestOperatorSessionService_PersistOperatorSession(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.OperatorSessionSvc

	t.Run("Success_AllFieldsPersisted", func(t *testing.T) {
		operatorSessionID := "op-session-123"
		userID := "user-789"
		orgID := "org-abc"
		operatorID := "operator-def"
		loginMethod := "mTLS"

		err := svc.PersistOperatorSession(operatorSessionID, userID, orgID, operatorID, loginMethod)
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		assert.Equal(t, operatorSessionID, doc.ID)

		require.Contains(t, doc.Data, "session_type")
		require.Contains(t, doc.Data, "user_id")
		require.Contains(t, doc.Data, "organization_id")
		require.Contains(t, doc.Data, "operator_id")
		require.Contains(t, doc.Data, "is_active")
		require.Contains(t, doc.Data, "absolute_expires_at")
		require.Contains(t, doc.Data, "idle_expires_at")
		require.Contains(t, doc.Data, "last_activity")
		require.Contains(t, doc.Data, "login_method")

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)
		stored.ID = doc.ID

		assert.Equal(t, operatorSessionID, stored.ID)
		assert.Equal(t, userID, stored.UserID)
		assert.Equal(t, orgID, stored.OrganizationID)
		assert.Equal(t, operatorID, stored.OperatorID)
		assert.Equal(t, string(constants.SessionTypeOperator), stored.SessionType)
		assert.True(t, stored.IsActive)
		assert.Equal(t, loginMethod, stored.LoginMethod)
	})

	t.Run("SessionTypeConstant_IsOperator", func(t *testing.T) {
		operatorSessionID := "op-session-type-check"

		err := svc.PersistOperatorSession(operatorSessionID, "user-1", "org-1", "operator-1", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.Equal(t, string(constants.SessionTypeOperator), stored.SessionType)
	})

	t.Run("IsActiveFlag_True", func(t *testing.T) {
		operatorSessionID := "op-session-active-flag"

		err := svc.PersistOperatorSession(operatorSessionID, "user-1", "org-1", "operator-1", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.True(t, stored.IsActive)
	})

	t.Run("ExpiryTimestamps_ApproximatelyOneHour", func(t *testing.T) {
		operatorSessionID := "op-session-expiry"

		err := svc.PersistOperatorSession(operatorSessionID, "user-1", "org-1", "operator-1", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		created := doc.CreatedAt
		absExpiry, err := time.Parse(time.RFC3339, stored.AbsoluteExpiresAt)
		require.NoError(t, err)
		idleExpiry, err := time.Parse(time.RFC3339, stored.IdleExpiresAt)
		require.NoError(t, err)
		lastActivity, err := time.Parse(time.RFC3339, stored.LastActivity)
		require.NoError(t, err)

		expectedExpiry := time.Now().UTC().Add(1 * time.Hour)
		assert.Less(t, absExpiry.Sub(expectedExpiry).Abs(), 5*time.Second, "absolute_expires_at should be approximately 1 hour from now")
		assert.Less(t, idleExpiry.Sub(expectedExpiry).Abs(), 5*time.Second, "idle_expires_at should be approximately 1 hour from now")

		assert.True(t, absExpiry.After(created) || absExpiry.Equal(created), "absolute_expires_at should be after or equal to created_at")
		assert.True(t, idleExpiry.After(created) || idleExpiry.Equal(created), "idle_expires_at should be after or equal to created_at")
		assert.True(t, lastActivity.Equal(created) || lastActivity.Sub(created).Abs() < time.Second, "last_activity should be approximately equal to created_at")
	})

	t.Run("AbsoluteAndIdleExpiry_AreEqual", func(t *testing.T) {
		operatorSessionID := "op-session-expiry-consistency"

		err := svc.PersistOperatorSession(operatorSessionID, "user-1", "org-1", "operator-1", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.Equal(t, stored.AbsoluteExpiresAt, stored.IdleExpiresAt)
	})

	t.Run("EmptySessionID_PersistsWithoutError", func(t *testing.T) {
		err := svc.PersistOperatorSession("", "user-1", "org-1", "operator-1", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), "")
		require.NoError(t, err)
		require.NotNil(t, doc)
	})

	t.Run("EmptyUserID_PersistsWithoutError", func(t *testing.T) {
		operatorSessionID := "op-session-empty-user"

		err := svc.PersistOperatorSession(operatorSessionID, "", "org-1", "operator-1", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)
		assert.Equal(t, "", stored.UserID)
	})

	t.Run("EmptyOrgID_PersistsWithoutError", func(t *testing.T) {
		operatorSessionID := "op-session-empty-org"

		err := svc.PersistOperatorSession(operatorSessionID, "user-1", "", "operator-1", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)
		assert.Equal(t, "", stored.OrganizationID)
	})

	t.Run("EmptyOperatorID_PersistsWithoutError", func(t *testing.T) {
		operatorSessionID := "op-session-empty-operator"

		err := svc.PersistOperatorSession(operatorSessionID, "user-1", "org-1", "", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)
		assert.Equal(t, "", stored.OperatorID)
	})

	t.Run("EmptyLoginMethod_PersistsWithoutError", func(t *testing.T) {
		operatorSessionID := "op-session-empty-login"

		err := svc.PersistOperatorSession(operatorSessionID, "user-1", "org-1", "operator-1", "")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)
		assert.Equal(t, "", stored.LoginMethod)
	})

	t.Run("NilDatabase_PanicsOnPersist", func(t *testing.T) {
		svcNilDB := NewOperatorSessionService(nil, infra.Logger)

		assert.Panics(t, func() {
			svcNilDB.PersistOperatorSession("op-session-nil-db", "user-1", "org-1", "operator-1", "mTLS")
		})
	})

	t.Run("NilLogger_PersistsWithoutError", func(t *testing.T) {
		svcNilLogger := NewOperatorSessionService(infra.DB, nil)

		err := svcNilLogger.PersistOperatorSession("op-session-nil-logger", "user-1", "org-1", "operator-1", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), "op-session-nil-logger")
		require.NoError(t, err)
		require.NotNil(t, doc)
	})

	t.Run("OverwriteExistingSession_ReplacesData", func(t *testing.T) {
		operatorSessionID := "op-session-overwrite"

		err := svc.PersistOperatorSession(operatorSessionID, "user-1", "org-1", "operator-1", "mTLS")
		require.NoError(t, err)

		err = svc.PersistOperatorSession(operatorSessionID, "user-2", "org-2", "operator-2", "passkey")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.Equal(t, "user-2", stored.UserID)
		assert.Equal(t, "org-2", stored.OrganizationID)
		assert.Equal(t, "operator-2", stored.OperatorID)
		assert.Equal(t, "passkey", stored.LoginMethod)
	})

	t.Run("SpecialCharactersInFields_PersistCorrectly", func(t *testing.T) {
		operatorSessionID := "op-session-special-chars"
		specialUserID := "user-with-特殊-Characters-😀"
		specialOrgID := "org-with-特殊-Characters-😀"
		specialOperatorID := "operator-with-特殊-Characters-😀"

		err := svc.PersistOperatorSession(operatorSessionID, specialUserID, specialOrgID, specialOperatorID, "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.Equal(t, specialUserID, stored.UserID)
		assert.Equal(t, specialOrgID, stored.OrganizationID)
		assert.Equal(t, specialOperatorID, stored.OperatorID)
	})

	t.Run("LongFieldValues_PersistCorrectly", func(t *testing.T) {
		operatorSessionID := "op-session-long-fields"
		longString := make([]byte, 10000)
		for i := range longString {
			longString[i] = 'a'
		}
		longVal := string(longString)

		err := svc.PersistOperatorSession(operatorSessionID, longVal, longVal, longVal, "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.Equal(t, longVal, stored.UserID)
		assert.Equal(t, longVal, stored.OrganizationID)
		assert.Equal(t, longVal, stored.OperatorID)
	})

	t.Run("MultipleSessions_IndependentLookup", func(t *testing.T) {
		sessionIDs := []string{"op-session-multi-1", "op-session-multi-2", "op-session-multi-3"}
		for i, sid := range sessionIDs {
			err := svc.PersistOperatorSession(sid, "user-"+sid, "org-"+sid, "operator-"+sid, "mTLS")
			require.NoError(t, err, "failed for session index %d", i)
		}

		for _, sid := range sessionIDs {
			doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), sid)
			require.NoError(t, err)
			require.NotNil(t, doc)

			var stored models.OperatorSession
			dataBytes, err := json.Marshal(doc.Data)
			require.NoError(t, err)
			err = json.Unmarshal(dataBytes, &stored)
			require.NoError(t, err)

			assert.Equal(t, "user-"+sid, stored.UserID)
			assert.Equal(t, "org-"+sid, stored.OrganizationID)
			assert.Equal(t, "operator-"+sid, stored.OperatorID)
		}
	})

	t.Run("PasskeyLoginMethod_PersistedCorrectly", func(t *testing.T) {
		operatorSessionID := "op-session-passkey"

		err := svc.PersistOperatorSession(operatorSessionID, "user-1", "org-1", "operator-1", "passkey")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), operatorSessionID)
		require.NoError(t, err)

		var stored models.OperatorSession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.Equal(t, "passkey", stored.LoginMethod)
	})
}
