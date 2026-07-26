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
	"fmt"
	"os/user"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestUserService_CreateUser_Integration(t *testing.T) {
	t.Run("Success - creates regular user with OS user info", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(stores.DocStore, logger)
		newUser, err := userSvc.CreateUser()
		require.NoError(t, err)
		require.NotNil(t, newUser)
		require.False(t, newUser.IsBootstrap)
		require.Equal(t, constants.UserStatusActive, newUser.Status)

		// Verify local OS user info is stored
		currentUser, err := user.Current()
		if err == nil {
			require.NotNil(t, newUser.LocalOSUser)
			require.Equal(t, currentUser.Uid, newUser.LocalOSUser.UID)
			require.Equal(t, currentUser.Gid, newUser.LocalOSUser.GID)
		}
	})
}

func TestUserService_Disable_Integration(t *testing.T) {
	t.Run("Success - disables user with audit entry", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(stores.DocStore, logger)

		// Create a bootstrap user
		user, err := userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)

		err = userSvc.Disable(user.ID, "test_reason", "actor_user_id", "operator_id")
		require.NoError(t, err)

		// Verify user is disabled
		disabledUser, err := userSvc.GetByID(user.ID)
		require.NoError(t, err)
		require.NotNil(t, disabledUser)
		require.Equal(t, constants.UserStatusDisabled, disabledUser.Status)
		require.False(t, disabledUser.IsActive())

		// Verify audit entry was created in the correct collection
		filters := []models.DocFilter{
			{Field: "target", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", user.ID))},
		}
		results, err := stores.DocStore.DocQuery(marshaler.CollectionName(constants.CollectionAuthAdminAudit), filters, "", 0)
		require.NoError(t, err)
		require.Len(t, results, 1)

		var auditEntry models.AdminAuditEntry
		err = json.Unmarshal(mustMarshal(t, results[0].ForWire()), &auditEntry)
		require.NoError(t, err)
		require.NotNil(t, auditEntry.Details)
		require.Equal(t, "test_reason", auditEntry.Details.Reason)
		require.Equal(t, "actor_user_id", auditEntry.Actor)
		require.Equal(t, "operator_id", auditEntry.OperatorID)
	})

	t.Run("Success - idempotent when already disabled", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(stores.DocStore, logger)

		// Create and disable a user
		user, err := userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)
		err = userSvc.Disable(user.ID, "test_reason", "actor_user_id", "operator_id")
		require.NoError(t, err)

		// Disable again (should be idempotent)
		err = userSvc.Disable(user.ID, "test_reason_2", "actor_user_id_2", "operator_id_2")
		require.NoError(t, err)

		// Verify audit entry was created for the noop
		filters := []models.DocFilter{
			{Field: "target", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", user.ID))},
		}
		results, err := stores.DocStore.DocQuery(marshaler.CollectionName(constants.CollectionAuthAdminAudit), filters, "", 0)
		require.NoError(t, err)
		require.Len(t, results, 2) // Two audit entries
	})

	t.Run("Success - with auth cache invalidation", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(stores.DocStore, logger)

		// Create a mock auth service to test cache invalidation
		mockAuthSvc := &AuthService{}
		userSvc.SetAuthService(mockAuthSvc)

		// Create a bootstrap user
		user, err := userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)

		err = userSvc.Disable(user.ID, "test_reason", "actor_user_id", "operator_id")
		require.NoError(t, err)

		// Verify user is disabled
		disabledUser, err := userSvc.GetByID(user.ID)
		require.NoError(t, err)
		require.NotNil(t, disabledUser)
		require.Equal(t, constants.UserStatusDisabled, disabledUser.Status)
	})

	t.Run("Error - GetByID failure when DB closed", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(stores.DocStore, logger)

		// Create a bootstrap user
		user, err := userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)

		// Close DB to force GetByID error
		db.Close()

		err = userSvc.Disable(user.ID, "test_reason", "actor_user_id", "operator_id")
		require.Error(t, err)
	})
}

func TestUserService_DeleteUser_Integration(t *testing.T) {
	t.Run("Success - with auth cache invalidation", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(stores.DocStore, logger)

		// Create a mock auth service to test cache invalidation
		mockAuthSvc := &AuthService{}
		userSvc.SetAuthService(mockAuthSvc)

		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		err = userSvc.DeleteUser(user.ID)
		require.NoError(t, err)

		// Verify user was deleted
		deletedUser, err := userSvc.GetByID(user.ID)
		require.NoError(t, err)
		require.Nil(t, deletedUser)
	})
}

func TestUserService_UpdatePasskeyCredentials_Integration(t *testing.T) {
	t.Run("Error - DocUpdate failure", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(stores.DocStore, logger)

		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		// Close DB to force DocUpdate error
		db.Close()

		newCredentials := []models.PasskeyCredential{
			{
				ID:              []byte("cred-1"),
				PublicKey:       []byte("public-key-1"),
				AttestationType: "none",
				Transport:       []protocol.AuthenticatorTransport{},
				Authenticator:   models.Authenticator{},
				CreatedAtUnixMs: time.Now().UTC().UnixMilli(),
			},
		}

		err = userSvc.UpdatePasskeyCredentials(user.ID, newCredentials)
		require.Error(t, err)
	})
}

func TestUserService_HasAnyUsers_Integration(t *testing.T) {
	t.Run("Error - DocQuery failure", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(stores.DocStore, logger)

		// Close DB to force DocQuery error
		db.Close()

		hasUsers, err := userSvc.HasAnyUsers()
		require.Error(t, err)
		require.False(t, hasUsers)
	})
}

func TestPersonaService_MapRolesToPersona_Integration(t *testing.T) {
	t.Run("Success - maps role to matching persona", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(stores.DocStore, logger)

		// Create a persona with specific roles
		persona := &models.Persona{
			ID:          "test-persona",
			Name:        "Test Persona",
			Description: "A test persona",
			Roles:       []string{"admin", "administrator"},
		}
		err = personaSvc.CreatePersona(persona)
		require.NoError(t, err)

		personaID, err := personaSvc.MapRolesToPersona([]string{"admin"})
		require.NoError(t, err)
		require.Equal(t, "test-persona", personaID)
	})

	t.Run("Success - returns default when no roles", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(stores.DocStore, logger)

		personaID, err := personaSvc.MapRolesToPersona([]string{})
		require.NoError(t, err)
		require.Equal(t, "default", personaID)
	})

	t.Run("Success - returns default when no match", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(stores.DocStore, logger)

		personaID, err := personaSvc.MapRolesToPersona([]string{"non-existent-role"})
		require.NoError(t, err)
		require.Equal(t, "default", personaID)
	})

	t.Run("Success - returns default when persona load fails", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(stores.DocStore, logger)

		// Close DB to force error
		db.Close()

		personaID, err := personaSvc.MapRolesToPersona([]string{"admin"})
		require.NoError(t, err)
		require.Equal(t, "default", personaID)
	})
}

func TestUserService_docToUser_Integration(t *testing.T) {
	t.Run("Success - converts valid doc to user", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(stores.DocStore, logger)

		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		doc, err := stores.DocStore.DocGet(marshaler.CollectionName(constants.CollectionUsers), user.ID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		converted, err := userSvc.docToUser(doc)
		require.NoError(t, err)
		require.NotNil(t, converted)
		require.Equal(t, user.ID, converted.ID)
	})

	t.Run("Error - handles malformed doc", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, stores, err := openTestDB(t, testPaths.DataDir, newTestFileSvc(t), logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(stores.DocStore, logger)

		// Create a malformed document
		malformedDoc := &models.Document{
			ID:   "malformed",
			Data: map[string]json.RawMessage{"invalid": json.RawMessage("{invalid json}")},
		}

		_, err = userSvc.docToUser(malformedDoc)
		require.Error(t, err)
	})
}

func mustMarshal(t *testing.T, v any) []byte {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
