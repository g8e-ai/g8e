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
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

func TestNewCLISessionService(t *testing.T) {
	infra := setupTestInfrastructure(t, true)

	svc := NewCLISessionService(infra.Stores.DocStore, infra.Logger)

	require.NotNil(t, svc)
	assert.Equal(t, infra.Stores.DocStore, svc.db)
	assert.Equal(t, infra.Logger, svc.logger)
}

func TestCLISessionService_PersistCLISession(t *testing.T) {
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
		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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
		// Note: CreatedAt is stored in the Document metadata, not in the data map
		assert.False(t, doc.CreatedAt.IsZero())
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
		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), "")
		require.NoError(t, err)
		require.NotNil(t, doc)
	})

	t.Run("Empty OperatorSessionID", func(t *testing.T) {
		cliSessionID := "cli-session-empty-operator"
		err := svc.PersistCLISession(cliSessionID, "", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err) // Service doesn't validate empty operatorSessionID

		// Verify it was persisted
		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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
		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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
		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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
		svcNilDB := NewCLISessionService((*DocumentStoreService)(nil), infra.Logger)

		// Nil database causes a panic when accessing DocStore
		assert.Panics(t, func() {
			svcNilDB.PersistCLISession("cli-session-nil-db", "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		})
	})

	t.Run("Nil Logger", func(t *testing.T) {
		svcNilLogger := NewCLISessionService(infra.Stores.DocStore, nil)

		// Should still work without logger (error logging is optional)
		err := svcNilLogger.PersistCLISession("cli-session-nil-logger", "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err)

		// Verify it was persisted
		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), "cli-session-nil-logger")
		require.NoError(t, err)
		require.NotNil(t, doc)
	})

	t.Run("SessionTypeConstant", func(t *testing.T) {
		cliSessionID := "cli-session-type-check"
		err := svc.PersistCLISession(cliSessionID, "operator-session-456", "user-789", "system-fp", "cert-fp", "serial", "mTLS")
		require.NoError(t, err)

		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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

		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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

		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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

		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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
		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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

		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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

		doc, err := infra.Stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
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

// ---------------------------------------------------------------------------
// DeactivateCLISession
// ---------------------------------------------------------------------------

// loadStoredCLISession reads a CLI session directly from the doc store for
// test assertions.
func loadStoredCLISession(t *testing.T, svc *CLISessionService, sessionID string) models.CLISession {
	t.Helper()
	doc, err := svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), sessionID)
	require.NoError(t, err)
	require.NotNil(t, doc)
	session, err := decodeCLISession(doc)
	require.NoError(t, err)
	return *session
}

func TestCLISessionService_DeactivateCLISession_Success(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	require.NoError(t, svc.PersistCLISession("deact-1", "op-1", "user-1", "sys-fp", "cert-fp-1", "serial-1", "mTLS"))
	require.NoError(t, svc.DeactivateCLISession("deact-1"))

	stored := loadStoredCLISession(t, svc, "deact-1")
	assert.False(t, stored.IsActive, "session must be inactive after DeactivateCLISession")
}

func TestCLISessionService_DeactivateCLISession_NotFound(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	err := svc.DeactivateCLISession("nonexistent-session")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrCLISessionNotFound), "expected ErrCLISessionNotFound, got %v", err)
}

func TestCLISessionService_DeactivateCLISession_AlreadyDeactivated(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	require.NoError(t, svc.PersistCLISession("deact-2", "op-2", "user-2", "sys-fp", "cert-fp-2", "serial-2", "mTLS"))
	require.NoError(t, svc.DeactivateCLISession("deact-2"))

	err := svc.DeactivateCLISession("deact-2")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrCLISessionAlreadyDeactivated), "expected ErrCLISessionAlreadyDeactivated, got %v", err)
}

func TestCLISessionService_DeactivateCLISession_EmptyID(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	err := svc.DeactivateCLISession("")
	require.Error(t, err)
}

func TestCLISessionService_DeactivateCLISession_ConcurrentOnlyOneSucceeds(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	require.NoError(t, svc.PersistCLISession("deact-concurrent", "op-c", "user-c", "sys-fp", "cert-fp-c", "serial-c", "mTLS"))

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var (
		mu              sync.Mutex
		successes       int
		alreadyDeactErr int
		otherErr        int
	)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			err := svc.DeactivateCLISession("deact-concurrent")
			mu.Lock()
			switch {
			case err == nil:
				successes++
			case errors.Is(err, constants.ErrCLISessionAlreadyDeactivated):
				alreadyDeactErr++
			default:
				otherErr++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, successes, "exactly one concurrent deactivation should succeed")
	assert.Equal(t, goroutines-1, alreadyDeactErr, "all other callers should see already-deactivated")
	assert.Equal(t, 0, otherErr, "no other errors expected")
}

// ---------------------------------------------------------------------------
// ReplaceCLISession
// ---------------------------------------------------------------------------

func TestCLISessionService_ReplaceCLISession_Success(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	require.NoError(t, svc.PersistCLISession("old-1", "op-old-1", "user-1", "sys-fp-old", "cert-fp-old", "serial-old", "mTLS"))

	newSessionID := "new-1"
	newSession, err := svc.ReplaceCLISession("old-1", newSessionID, "cert-fp-new", "serial-new", CLISessionFields{
		OperatorSessionID: "op-old-1",
		UserID:            "user-1",
		SystemFingerprint: "sys-fp-new",
		CertFingerprint:   "cert-fp-new",
		CertSerial:        "serial-new",
		LoginMethod:       string(constants.HeartbeatTypeBootstrap),
	})
	require.NoError(t, err)
	require.NotNil(t, newSession)
	assert.NotEmpty(t, newSession.ID)
	assert.Equal(t, newSessionID, newSession.ID)
	assert.NotEqual(t, "old-1", newSession.ID)
	assert.True(t, newSession.IsActive)
	assert.Equal(t, "user-1", newSession.UserID)
	assert.Equal(t, "op-old-1", newSession.OperatorSessionID)
	assert.Equal(t, "cert-fp-new", newSession.CertFingerprint)
	assert.Equal(t, "serial-new", newSession.CertSerial)

	// Old session must be deactivated.
	oldStored := loadStoredCLISession(t, svc, "old-1")
	assert.False(t, oldStored.IsActive, "old session must be deactivated after replacement")

	// New session must be active and persisted.
	newStored := loadStoredCLISession(t, svc, newSession.ID)
	assert.True(t, newStored.IsActive)
	assert.Equal(t, "cert-fp-new", newStored.CertFingerprint)
}

func TestCLISessionService_ReplaceCLISession_UnknownOldSession(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	_, err := svc.ReplaceCLISession("nonexistent-old", "new-unknown", "cert-fp-new", "serial-new", CLISessionFields{
		OperatorSessionID: "op-1",
		UserID:            "user-1",
		LoginMethod:       "mTLS",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrCLISessionNotFound), "expected ErrCLISessionNotFound, got %v", err)
}

func TestCLISessionService_ReplaceCLISession_AlreadyDeactivatedOld(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	require.NoError(t, svc.PersistCLISession("old-deact", "op-d", "user-d", "sys-fp", "cert-fp", "serial", "mTLS"))
	require.NoError(t, svc.DeactivateCLISession("old-deact"))

	_, err := svc.ReplaceCLISession("old-deact", "new-deact", "cert-fp-new", "serial-new", CLISessionFields{
		OperatorSessionID: "op-d",
		UserID:            "user-d",
		LoginMethod:       "mTLS",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrCLISessionAlreadyDeactivated), "expected ErrCLISessionAlreadyDeactivated, got %v", err)
}

func TestCLISessionService_ReplaceCLISession_MissingUserBinding(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	require.NoError(t, svc.PersistCLISession("old-bind", "op-b", "user-b", "sys-fp", "cert-fp", "serial", "mTLS"))

	// Missing UserID.
	_, err := svc.ReplaceCLISession("old-bind", "new-bind-1", "cert-fp-new", "serial-new", CLISessionFields{
		OperatorSessionID: "op-b",
		LoginMethod:       "mTLS",
	})
	require.Error(t, err)

	// Missing OperatorSessionID.
	_, err = svc.ReplaceCLISession("old-bind", "new-bind-2", "cert-fp-new", "serial-new", CLISessionFields{
		UserID:      "user-b",
		LoginMethod: "mTLS",
	})
	require.Error(t, err)

	// Old session must still be active (no partial mutation).
	stored := loadStoredCLISession(t, svc, "old-bind")
	assert.True(t, stored.IsActive, "old session must remain active on input validation failure")
}

func TestCLISessionService_ReplaceCLISession_EmptyOldID(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	_, err := svc.ReplaceCLISession("", "new-empty-old", "cert-fp-new", "serial-new", CLISessionFields{
		OperatorSessionID: "op-1",
		UserID:            "user-1",
		LoginMethod:       "mTLS",
	})
	require.Error(t, err)
}

// TestCLISessionService_ReplaceCLISession_ConcurrentOnlyOneSucceeds verifies
// that exactly one of 10 concurrent replacements wins the conditional update
// on the old session's is_active=true flag. The winner gets nil; the others
// get constants.ErrCLISessionAlreadyDeactivated. Losers may fail at either
// the read step (old session already deactivated, no new session written) or
// the conditional update step (new session written but old already
// deactivated by the winner, so the new session is deleted and the typed
// error is returned) — both paths return constants.ErrCLISessionAlreadyDeactivated.
// This matches the plan's invariant: "concurrent replacement (10 goroutines,
// exactly 1 success)".
func TestCLISessionService_ReplaceCLISession_ConcurrentOnlyOneSucceeds(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	require.NoError(t, svc.PersistCLISession("old-concurrent", "op-c", "user-c", "sys-fp", "cert-fp-old", "serial-old", "mTLS"))

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var (
		mu              sync.Mutex
		successes       int
		alreadyDeactErr int
		otherErr        int
		winnerSessionID string
	)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			newSessionID := fmt.Sprintf("new-concurrent-%d", idx)
			newSession, err := svc.ReplaceCLISession("old-concurrent", newSessionID, "cert-fp-new", "serial-new", CLISessionFields{
				OperatorSessionID: "op-c",
				UserID:            "user-c",
				SystemFingerprint: "sys-fp-new",
				LoginMethod:       string(constants.HeartbeatTypeBootstrap),
			})
			mu.Lock()
			switch {
			case err == nil:
				successes++
				if newSession != nil {
					winnerSessionID = newSession.ID
				}
			case errors.Is(err, constants.ErrCLISessionAlreadyDeactivated):
				alreadyDeactErr++
			default:
				otherErr++
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	assert.Equal(t, 1, successes, "exactly one concurrent replacement should succeed")
	assert.Equal(t, goroutines-1, alreadyDeactErr, "all other callers should see already-deactivated")
	assert.Equal(t, 0, otherErr, "no other errors expected")
	assert.NotEmpty(t, winnerSessionID, "the winner must return a new session ID")

	// The old session must be deactivated regardless of which caller won.
	oldStored := loadStoredCLISession(t, svc, "old-concurrent")
	assert.False(t, oldStored.IsActive, "old session must be deactivated after concurrent replacement")

	// The winner's new session must be active and persisted.
	winnerStored := loadStoredCLISession(t, svc, winnerSessionID)
	assert.True(t, winnerStored.IsActive, "winner's new session must be active")
	assert.Equal(t, "user-c", winnerStored.UserID)
	assert.Equal(t, "cert-fp-new", winnerStored.CertFingerprint)
}

// TestCLISessionService_ReplaceCLISession_RevocationWriteFailureRollback
// verifies the documented partial-failure semantics: when ReplaceCLISession
// succeeds, the old session is inactive and the new session is active. The
// caller's responsibility is to revoke the old cert AFTER this success. If
// the caller's subsequent revocation fails, the old session remains inactive
// (the session side is already committed) — the caller retries revocation
// idempotently. This test simulates the post-success revocation failure by
// confirming the session state is already final when ReplaceCLISession
// returns nil.
func TestCLISessionService_ReplaceCLISession_RevocationWriteFailureRollback(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	require.NoError(t, svc.PersistCLISession("old-rollback", "op-rb", "user-rb", "sys-fp", "cert-fp-old", "serial-old", "mTLS"))

	newSession, err := svc.ReplaceCLISession("old-rollback", "new-rollback", "cert-fp-new", "serial-new", CLISessionFields{
		OperatorSessionID: "op-rb",
		UserID:            "user-rb",
		SystemFingerprint: "sys-fp-new",
		LoginMethod:       string(constants.HeartbeatTypeBootstrap),
	})
	require.NoError(t, err, "ReplaceCLISession must succeed before revocation is attempted")
	require.NotNil(t, newSession)

	// At this point, the caller would call pki.RevokeCertificate(oldSerial, ...).
	// Simulate that revocation failing — the session state must already be
	// final (old inactive, new active) so a retry of revocation is safe and
	// the user is not locked out.
	oldStored := loadStoredCLISession(t, svc, "old-rollback")
	assert.False(t, oldStored.IsActive, "old session must be inactive before caller attempts revocation")

	newStored := loadStoredCLISession(t, svc, newSession.ID)
	assert.True(t, newStored.IsActive, "new session must be active before caller attempts revocation")

	// A second ReplaceCLISession on the now-deactivated old session must
	// fail with constants.ErrCLISessionAlreadyDeactivated — the caller
	// cannot accidentally double-replace.
	_, err = svc.ReplaceCLISession("old-rollback", "new-rollback-2", "cert-fp-new-2", "serial-new-2", CLISessionFields{
		OperatorSessionID: "op-rb",
		UserID:            "user-rb",
		LoginMethod:       "mTLS",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrCLISessionAlreadyDeactivated))
}

// ---------------------------------------------------------------------------
// RefreshCLISession
// ---------------------------------------------------------------------------

// persistCLISessionForRefresh creates an active CLI session for the given
// user and returns the session ID. Used by RefreshCLISession tests to set
// up the "old session" state.
func persistCLISessionForRefresh(t *testing.T, svc *CLISessionService, sessionID, userID, operatorSessionID string) {
	t.Helper()
	require.NoError(t, svc.PersistCLISession(sessionID, operatorSessionID, userID, "sys-fp", "cert-fp", "serial", "mTLS"))
}

// TestCLISessionService_RefreshCLISession_OldSessionActive_DeactivatedAndNewPersisted
// verifies the primary recovery path: the caller's cert is still valid, the
// old CLI session is active (but server-side expired), and RefreshCLISession
// deactivates the old session and persists a new active one bound to the
// same user and operator session.
func TestCLISessionService_RefreshCLISession_OldSessionActive_DeactivatedAndNewPersisted(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	persistCLISessionForRefresh(t, svc, "refresh-old-active", "user-refresh-1", "op-refresh-1")

	newSession, err := svc.RefreshCLISession("refresh-old-active", "refresh-new-1", CLISessionFields{
		OperatorSessionID: "op-refresh-1",
		UserID:            "user-refresh-1",
		SystemFingerprint: "sys-fp-new",
		CertFingerprint:   "cert-fp-inherited",
		CertSerial:        "serial-inherited",
		LoginMethod:       "mTLS",
	})
	require.NoError(t, err)
	require.NotNil(t, newSession)
	assert.Equal(t, "refresh-new-1", newSession.ID)
	assert.True(t, newSession.IsActive)
	assert.Equal(t, "user-refresh-1", newSession.UserID)
	assert.Equal(t, "op-refresh-1", newSession.OperatorSessionID)
	assert.Equal(t, "cert-fp-inherited", newSession.CertFingerprint)

	// Old session must be deactivated.
	oldStored := loadStoredCLISession(t, svc, "refresh-old-active")
	assert.False(t, oldStored.IsActive, "old session must be deactivated after refresh")

	// New session must be active and persisted.
	newStored := loadStoredCLISession(t, svc, "refresh-new-1")
	assert.True(t, newStored.IsActive)
	assert.Equal(t, "user-refresh-1", newStored.UserID)
}

// TestCLISessionService_RefreshCLISession_OldSessionAlreadyDeactivated_NewPersisted
// verifies that an already-deactivated old session is not an error — the
// caller's cert is the proof of identity, not the old session's state. The
// new session is still persisted.
func TestCLISessionService_RefreshCLISession_OldSessionAlreadyDeactivated_NewPersisted(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	persistCLISessionForRefresh(t, svc, "refresh-old-deact", "user-refresh-2", "op-refresh-2")
	require.NoError(t, svc.DeactivateCLISession("refresh-old-deact"))

	newSession, err := svc.RefreshCLISession("refresh-old-deact", "refresh-new-2", CLISessionFields{
		OperatorSessionID: "op-refresh-2",
		UserID:            "user-refresh-2",
		LoginMethod:       "mTLS",
	})
	require.NoError(t, err)
	require.NotNil(t, newSession)
	assert.Equal(t, "refresh-new-2", newSession.ID)
	assert.True(t, newSession.IsActive)

	// Old session remains deactivated.
	oldStored := loadStoredCLISession(t, svc, "refresh-old-deact")
	assert.False(t, oldStored.IsActive)

	// New session is active.
	newStored := loadStoredCLISession(t, svc, "refresh-new-2")
	assert.True(t, newStored.IsActive)
}

// TestCLISessionService_RefreshCLISession_OldSessionMissing_NewPersisted
// verifies the gateway-volume-reset case: the old session ID from the cert
// URI SAN does not match any persisted session (the gateway volume was
// wiped). RefreshCLISession still persists a new session — the cert is the
// proof of identity, not the old session's state.
func TestCLISessionService_RefreshCLISession_OldSessionMissing_NewPersisted(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	// No old session persisted — simulate a volume reset.
	newSession, err := svc.RefreshCLISession("refresh-old-missing", "refresh-new-3", CLISessionFields{
		OperatorSessionID: "op-refresh-3",
		UserID:            "user-refresh-3",
		LoginMethod:       "mTLS",
	})
	require.NoError(t, err)
	require.NotNil(t, newSession)
	assert.Equal(t, "refresh-new-3", newSession.ID)
	assert.True(t, newSession.IsActive)
	assert.Equal(t, "user-refresh-3", newSession.UserID)

	// New session is persisted.
	newStored := loadStoredCLISession(t, svc, "refresh-new-3")
	assert.True(t, newStored.IsActive)
}

// TestCLISessionService_RefreshCLISession_EmptyOldSessionID_NewPersisted
// verifies that an empty oldSessionID (the caller's cert has no URI SAN
// session ID, or the controller passed an empty string) still results in a
// new session. Only the new session is persisted; there is no old session
// to deactivate.
func TestCLISessionService_RefreshCLISession_EmptyOldSessionID_NewPersisted(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	newSession, err := svc.RefreshCLISession("", "refresh-new-4", CLISessionFields{
		OperatorSessionID: "op-refresh-4",
		UserID:            "user-refresh-4",
		LoginMethod:       "mTLS",
	})
	require.NoError(t, err)
	require.NotNil(t, newSession)
	assert.Equal(t, "refresh-new-4", newSession.ID)
	assert.True(t, newSession.IsActive)
}

// TestCLISessionService_RefreshCLISession_MissingUserBinding_ReturnsError
// verifies that a missing UserID or OperatorSessionID in the fields is
// rejected before any session mutation. This is the fail-closed guard
// against a controller bug that tries to refresh a session without binding
// it to the authenticated user.
func TestCLISessionService_RefreshCLISession_MissingUserBinding_ReturnsError(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	// Missing UserID.
	_, err := svc.RefreshCLISession("refresh-old-bind", "refresh-new-bind-1", CLISessionFields{
		OperatorSessionID: "op-bind",
		LoginMethod:       "mTLS",
	})
	require.Error(t, err)

	// Missing OperatorSessionID.
	_, err = svc.RefreshCLISession("refresh-old-bind", "refresh-new-bind-2", CLISessionFields{
		UserID:      "user-bind",
		LoginMethod: "mTLS",
	})
	require.Error(t, err)
}

// TestCLISessionService_RefreshCLISession_MissingNewSessionID_ReturnsError
// verifies that an empty newSessionID is rejected. The caller MUST
// pre-generate the new session ID so the cert URI SAN can be checked
// against it by the auth middleware on subsequent requests.
func TestCLISessionService_RefreshCLISession_MissingNewSessionID_ReturnsError(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	svc := infra.CLISessionSvc

	_, err := svc.RefreshCLISession("refresh-old-noid", "", CLISessionFields{
		OperatorSessionID: "op-noid",
		UserID:            "user-noid",
		LoginMethod:       "mTLS",
	})
	require.Error(t, err)
}
