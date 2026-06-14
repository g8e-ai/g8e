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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

func TestNewCLISessionService(t *testing.T) {
	t.Parallel()
	infra := setupTestInfrastructure(t, true)

	svc := NewCLISessionService(infra.DB, infra.Logger)

	require.NotNil(t, svc)
	assert.Equal(t, infra.DB, svc.db)
	assert.Equal(t, infra.Logger, svc.logger)
}

func TestCLISessionService_PersistCLISession(t *testing.T) {
	t.Parallel()
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	t.Run("Success", func(t *testing.T) {
		cliSessionID := "cli-session-123"
		operatorSessionID := "operator-session-456"
		userID := "user-789"
		systemFingerprint := "system-fp-abc"
		certFingerprint := "cert-fp-def"
		certSerial := "serial-123"
		loginMethod := "mTLS"

		err := svc.PersistCLISession(cliSessionID, operatorSessionID, userID, systemFingerprint, certFingerprint, certSerial, loginMethod)
		require.NoError(t, err)

		// Verify the session was persisted in the DB
		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		// Verify the document ID matches
		assert.Equal(t, cliSessionID, doc.ID)

		// Verify the data contains the expected fields (id is stored separately in Document.ID)
		require.Contains(t, doc.Data, "user_id")
		require.Contains(t, doc.Data, "operator_session_id")
		require.Contains(t, doc.Data, "system_fingerprint")
		require.Contains(t, doc.Data, "cert_fingerprint")
		require.Contains(t, doc.Data, "cert_serial")
		require.Contains(t, doc.Data, "session_type")
		require.Contains(t, doc.Data, "is_active")
		require.Contains(t, doc.Data, "login_method")

		// Unmarshal to verify field values
		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)
		stored.ID = doc.ID // Set ID from document

		assert.Equal(t, cliSessionID, stored.ID)
		assert.Equal(t, userID, stored.UserID)
		assert.Equal(t, operatorSessionID, stored.OperatorSessionID)
		assert.Equal(t, systemFingerprint, stored.SystemFingerprint)
		assert.Equal(t, certFingerprint, stored.CertFingerprint)
		assert.Equal(t, certSerial, stored.CertSerial)
		assert.Equal(t, string(constants.SessionTypeCLI), stored.SessionType)
		assert.True(t, stored.IsActive)
		assert.Equal(t, loginMethod, stored.LoginMethod)

		// Verify timestamp fields are set
		assert.False(t, stored.CreatedAt.IsZero())
		assert.False(t, stored.ExpiresAt.IsZero())
		assert.False(t, stored.AbsoluteExpiresAt.IsZero())
		assert.False(t, stored.IdleExpiresAt.IsZero())

		// Verify expiry is approximately 1 hour from now
		expectedExpiry := time.Now().UTC().Add(1 * time.Hour)
		timeDiff := stored.ExpiresAt.Sub(expectedExpiry)
		assert.Less(t, timeDiff.Abs(), 5*time.Second, "expiry should be approximately 1 hour from now")
	})

	t.Run("Empty SessionID", func(t *testing.T) {
		err := svc.PersistCLISession("", "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err) // Service doesn't validate empty sessionID

		// Verify it was still persisted with empty ID
		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), "")
		require.NoError(t, err)
		require.NotNil(t, doc)
	})

	t.Run("Empty OperatorSessionID", func(t *testing.T) {
		cliSessionID := "cli-session-empty-operator"
		err := svc.PersistCLISession(cliSessionID, "", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err) // Service doesn't validate empty operatorSessionID

		// Verify it was persisted
		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)
		assert.Equal(t, "", stored.OperatorSessionID)
	})

	t.Run("Empty UserID", func(t *testing.T) {
		cliSessionID := "cli-session-empty-user"
		err := svc.PersistCLISession(cliSessionID, "operator-session-456", "", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err) // Service doesn't validate empty userID

		// Verify it was persisted
		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)
		assert.Equal(t, "", stored.UserID)
	})

	t.Run("Empty LoginMethod", func(t *testing.T) {
		cliSessionID := "cli-session-empty-login"
		err := svc.PersistCLISession(cliSessionID, "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "")
		require.NoError(t, err) // Service doesn't validate empty loginMethod

		// Verify it was persisted
		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)
		assert.Equal(t, "", stored.LoginMethod)
	})

	t.Run("Nil Database", func(t *testing.T) {
		svcNilDB := NewCLISessionService(nil, infra.Logger)

		// Nil database causes a panic when accessing DocStore
		assert.Panics(t, func() {
			svcNilDB.PersistCLISession("cli-session-nil-db", "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		})
	})

	t.Run("Nil Logger", func(t *testing.T) {
		svcNilLogger := NewCLISessionService(infra.DB, nil)

		// Should still work without logger (error logging is optional)
		err := svcNilLogger.PersistCLISession("cli-session-nil-logger", "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err)

		// Verify it was persisted
		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), "cli-session-nil-logger")
		require.NoError(t, err)
		require.NotNil(t, doc)
	})

	t.Run("SessionTypeConstant", func(t *testing.T) {
		cliSessionID := "cli-session-type-check"
		err := svc.PersistCLISession(cliSessionID, "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)

		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.Equal(t, string(constants.SessionTypeCLI), stored.SessionType)
	})

	t.Run("IsActiveFlag", func(t *testing.T) {
		cliSessionID := "cli-session-active-flag"
		err := svc.PersistCLISession(cliSessionID, "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)

		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.True(t, stored.IsActive)
	})

	t.Run("ExpiryTimestampsConsistency", func(t *testing.T) {
		cliSessionID := "cli-session-expiry-consistency"
		err := svc.PersistCLISession(cliSessionID, "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)

		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		// All three expiry timestamps should be the same (1 hour from creation)
		assert.Equal(t, stored.ExpiresAt, stored.AbsoluteExpiresAt)
		assert.Equal(t, stored.ExpiresAt, stored.IdleExpiresAt)
	})

	t.Run("TimestampOrdering", func(t *testing.T) {
		cliSessionID := "cli-session-timestamp-order"
		err := svc.PersistCLISession(cliSessionID, "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)

		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		// CreatedAt should be before ExpiresAt
		assert.True(t, stored.ExpiresAt.After(stored.CreatedAt) || stored.ExpiresAt.Equal(stored.CreatedAt))
	})

	t.Run("OverwriteExistingSession", func(t *testing.T) {
		cliSessionID := "cli-session-overwrite"

		// Create first session
		err := svc.PersistCLISession(cliSessionID, "operator-session-1", "user-1", "system-fp-1", "cert-fp-1", "serial-1", "mTLS")
		require.NoError(t, err)

		// Overwrite with new session data
		err = svc.PersistCLISession(cliSessionID, "operator-session-2", "user-2", "system-fp-2", "cert-fp-2", "serial-2", "passkey")
		require.NoError(t, err)

		// Verify the session was overwritten
		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)

		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.Equal(t, "operator-session-2", stored.OperatorSessionID)
		assert.Equal(t, "user-2", stored.UserID)
		assert.Equal(t, "system-fp-2", stored.SystemFingerprint)
		assert.Equal(t, "cert-fp-2", stored.CertFingerprint)
		assert.Equal(t, "serial-2", stored.CertSerial)
		assert.Equal(t, "passkey", stored.LoginMethod)
	})

	t.Run("SpecialCharactersInFields", func(t *testing.T) {
		cliSessionID := "cli-session-special-chars"
		specialSystemFP := "system-fp-with-特殊-Characters-😀"
		specialCertFP := "cert-fp-with-特殊-Characters-😀"
		specialSerial := "serial-with-特殊-Characters-😀"

		err := svc.PersistCLISession(cliSessionID, "operator-session-456", "user-789", specialSystemFP, specialCertFP, specialSerial, "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)

		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.Equal(t, specialSystemFP, stored.SystemFingerprint)
		assert.Equal(t, specialCertFP, stored.CertFingerprint)
		assert.Equal(t, specialSerial, stored.CertSerial)
	})

	t.Run("LongFieldValues", func(t *testing.T) {
		cliSessionID := "cli-session-long-fields"
		longString := string(make([]byte, 10000)) // 10KB string
		for i := range longString {
			longString = longString[:i] + "a" + longString[i+1:]
		}

		err := svc.PersistCLISession(cliSessionID, "operator-session-456", "user-789", longString, longString, longString, "mTLS")
		require.NoError(t, err)

		doc, err := infra.DB.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
		require.NoError(t, err)

		var stored models.CLISession
		dataBytes, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(dataBytes, &stored)
		require.NoError(t, err)

		assert.Equal(t, longString, stored.SystemFingerprint)
		assert.Equal(t, longString, stored.CertFingerprint)
		assert.Equal(t, longString, stored.CertSerial)
	})
}
