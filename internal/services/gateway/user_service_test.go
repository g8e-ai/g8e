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

func TestNewUserService(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	testPaths := testutil.NewTestPathsFromTemp(t)
	db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	userSvc := NewUserService(db, logger)

	require.NotNil(t, userSvc)
	require.Equal(t, db, userSvc.db)
	require.Equal(t, logger, userSvc.logger)
}

func TestUserService_CreateUser(t *testing.T) {
	t.Run("Success - creates regular user", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)
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

func TestUserService_CreateBootstrapUser(t *testing.T) {
	t.Run("Success - creates bootstrap user", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)
		user, err := userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		require.NotNil(t, user)
		require.True(t, user.IsBootstrap)
		require.Equal(t, constants.UserStatusActive, user.Status)
	})

	t.Run("Success - second bootstrap user fails", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)
		// Create first bootstrap user
		_, err = userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		// Second bootstrap user should fail
		_, err = userSvc.CreateBootstrapUser()
		require.Error(t, err)
	})
}

func TestUserService_Disable(t *testing.T) {
	t.Parallel()
	t.Run("Success - disables user", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		// Create a bootstrap user
		user, err := userSvc.CreateBootstrapUser()
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
		results, err := db.DocStore.DocQuery(marshaler.CollectionName(constants.CollectionAuthAdminAudit), filters, "", 0)
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
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		// Create and disable a user
		user, err := userSvc.CreateBootstrapUser()
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
		results, err := db.DocStore.DocQuery(marshaler.CollectionName(constants.CollectionAuthAdminAudit), filters, "", 0)
		require.NoError(t, err)
		require.Len(t, results, 2) // Two audit entries
	})

	t.Run("Error - empty user_id", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		err = userSvc.Disable("", "test_reason", "actor_user_id", "operator_id")
		require.Error(t, err)
	})

	t.Run("Error - user not found", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		userSvc := NewUserService(db, logger)

		err = userSvc.Disable("non-existent-id", "test_reason", "actor_user_id", "operator_id")
		require.Error(t, err)
	})

	t.Run("Success - with auth cache invalidation", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		// Create a mock auth service to test cache invalidation
		mockAuthSvc := &AuthService{}
		userSvc.SetAuthService(mockAuthSvc)

		// Create a bootstrap user
		user, err := userSvc.CreateBootstrapUser()
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
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		// Create a bootstrap user
		user, err := userSvc.CreateBootstrapUser()
		require.NoError(t, err)

		// Close DB to force GetByID error
		db.Close()

		err = userSvc.Disable(user.ID, "test_reason", "actor_user_id", "operator_id")
		require.Error(t, err)
	})
}

func mustMarshal(t *testing.T, v any) []byte {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestUserService_FindBootstrapUser(t *testing.T) {
	t.Parallel()
	t.Run("Success - finds bootstrap user", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		userSvc := NewUserService(db, logger)

		// Create bootstrap user
		created, err := userSvc.CreateBootstrapUser()
		require.NoError(t, err)

		// Find bootstrap user
		found, err := userSvc.FindBootstrapUser()
		require.NoError(t, err)
		require.NotNil(t, found)
		require.Equal(t, created.ID, found.ID)
		require.True(t, found.IsBootstrap)
	})

	t.Run("Success - returns nil when no bootstrap user", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		userSvc := NewUserService(db, logger)

		// Create a non-bootstrap user
		_, err = userSvc.CreateUser()
		require.NoError(t, err)

		// Find bootstrap user should return nil
		found, err := userSvc.FindBootstrapUser()
		require.NoError(t, err)
		require.Nil(t, found)
	})
}

func TestUser_IsActive(t *testing.T) {
	t.Parallel()
	t.Run("Active status returns true", func(t *testing.T) {
		t.Parallel()
		user := &models.User{
			Status: constants.UserStatusActive,
		}
		require.True(t, user.IsActive())
	})

	t.Run("Disabled status returns false", func(t *testing.T) {
		t.Parallel()
		user := &models.User{
			Status: constants.UserStatusDisabled,
		}
		require.False(t, user.IsActive())
	})

	t.Run("Empty status returns true (backward compatibility)", func(t *testing.T) {
		t.Parallel()
		user := &models.User{
			Status: "",
		}
		require.True(t, user.IsActive())
	})

	t.Run("Nil user returns false", func(t *testing.T) {
		t.Parallel()
		var user *models.User = nil
		require.False(t, user.IsActive())
	})
}

func TestUserService_HasAnyUsers(t *testing.T) {
	t.Parallel()

	t.Run("False when no users exist", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)
		hasUsers, err := userSvc.HasAnyUsers()
		require.NoError(t, err)
		require.False(t, hasUsers)
	})

	t.Run("True when user exists", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)
		_, err = userSvc.CreateUser()
		require.NoError(t, err)

		hasUsers, err := userSvc.HasAnyUsers()
		require.NoError(t, err)
		require.True(t, hasUsers)
	})

	t.Run("Error - DocQuery failure", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		// Close DB to force DocQuery error
		db.Close()

		hasUsers, err := userSvc.HasAnyUsers()
		require.Error(t, err)
		require.False(t, hasUsers)
	})
}

func TestDefaultPersonaDefinitions(t *testing.T) {
	t.Parallel()

	personas := DefaultPersonaDefinitions()
	require.NotEmpty(t, personas)

	// Verify expected personas exist
	personaIDs := make(map[string]bool)
	for _, p := range personas {
		personaIDs[p.ID] = true
		require.NotEmpty(t, p.Name)
		require.NotEmpty(t, p.Description)
		// Default persona can have empty roles
		if p.ID != "default" {
			require.NotEmpty(t, p.Roles)
		}
	}

	// Check for expected persona IDs (admin role maps to admin persona)
	require.True(t, personaIDs[string(constants.UserRoleAdmin)] || personaIDs["admin"])
	require.True(t, personaIDs["security-analyst"])
	require.True(t, personaIDs["developer"])
	require.True(t, personaIDs["auditor"])
	require.True(t, personaIDs["default"])
}

func TestPersonaService_CreatePersona(t *testing.T) {
	t.Parallel()

	t.Run("Success - creates persona", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

		persona := &models.Persona{
			ID:          "test-persona",
			Name:        "Test Persona",
			Description: "A test persona",
			Roles:       []string{"test-role"},
		}

		err = personaSvc.CreatePersona(persona)
		require.NoError(t, err)

		// Verify persona was created
		retrieved, err := personaSvc.GetByID("test-persona")
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		require.Equal(t, "Test Persona", retrieved.Name)
		require.Equal(t, "A test persona", retrieved.Description)
		require.Equal(t, []string{"test-role"}, retrieved.Roles)
	})
}

func TestPersonaService_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("Success - retrieves existing persona", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

		persona := &models.Persona{
			ID:          "test-persona",
			Name:        "Test Persona",
			Description: "A test persona",
			Roles:       []string{"test-role"},
		}
		err = personaSvc.CreatePersona(persona)
		require.NoError(t, err)

		retrieved, err := personaSvc.GetByID("test-persona")
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		require.Equal(t, "test-persona", retrieved.ID)
	})

	t.Run("Success - returns nil for non-existent persona", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

		retrieved, err := personaSvc.GetByID("non-existent")
		require.NoError(t, err)
		require.Nil(t, retrieved)
	})
}

func TestPersonaService_GetAll(t *testing.T) {
	t.Parallel()

	t.Run("Success - retrieves all personas", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

		// Create multiple personas
		persona1 := &models.Persona{
			ID:          "persona-1",
			Name:        "Persona 1",
			Description: "First persona",
			Roles:       []string{"role1"},
		}
		persona2 := &models.Persona{
			ID:          "persona-2",
			Name:        "Persona 2",
			Description: "Second persona",
			Roles:       []string{"role2"},
		}

		err = personaSvc.CreatePersona(persona1)
		require.NoError(t, err)
		err = personaSvc.CreatePersona(persona2)
		require.NoError(t, err)

		all, err := personaSvc.GetAll()
		require.NoError(t, err)
		require.Len(t, all, 2)
	})

	t.Run("Success - returns empty list when no personas", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

		all, err := personaSvc.GetAll()
		require.NoError(t, err)
		require.Empty(t, all)
	})
}

func TestPersonaService_docToPersona(t *testing.T) {
	t.Parallel()

	t.Run("Success - converts valid doc to persona", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

		persona := &models.Persona{
			ID:          "test-persona",
			Name:        "Test Persona",
			Description: "A test persona",
			Roles:       []string{"test-role"},
		}
		err = personaSvc.CreatePersona(persona)
		require.NoError(t, err)

		doc, err := db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionPersonas), "test-persona")
		require.NoError(t, err)
		require.NotNil(t, doc)

		converted, err := personaSvc.docToPersona(doc)
		require.NoError(t, err)
		require.NotNil(t, converted)
		require.Equal(t, "test-persona", converted.ID)
		require.Equal(t, "Test Persona", converted.Name)
	})

	t.Run("Error - handles malformed doc", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

		// Create a malformed document
		malformedDoc := &models.Document{
			ID:   "malformed",
			Data: map[string]json.RawMessage{"invalid": json.RawMessage("{invalid json")},
		}

		_, err = personaSvc.docToPersona(malformedDoc)
		require.Error(t, err)
	})
}

func TestPersonaService_MapRolesToPersona(t *testing.T) {
	t.Parallel()

	t.Run("Success - maps role to matching persona", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

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
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

		personaID, err := personaSvc.MapRolesToPersona([]string{})
		require.NoError(t, err)
		require.Equal(t, "default", personaID)
	})

	t.Run("Success - returns default when no match", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

		personaID, err := personaSvc.MapRolesToPersona([]string{"non-existent-role"})
		require.NoError(t, err)
		require.Equal(t, "default", personaID)
	})

	t.Run("Success - returns default when persona load fails", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		personaSvc := NewPersonaService(db, logger)

		// Close DB to force error
		db.Close()

		personaID, err := personaSvc.MapRolesToPersona([]string{"admin"})
		require.NoError(t, err)
		require.Equal(t, "default", personaID)
	})
}

func TestUserService_GetBySub(t *testing.T) {
	t.Parallel()

	t.Run("Success - retrieves user by sub", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		createdUser, err := userSvc.CreateUser()
		require.NoError(t, err)

		retrievedUser, err := userSvc.GetBySub(createdUser.ID)
		require.NoError(t, err)
		require.NotNil(t, retrievedUser)
		require.Equal(t, createdUser.ID, retrievedUser.ID)
	})

	t.Run("Error - empty sub returns error", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		_, err = userSvc.GetBySub("")
		require.Error(t, err)
	})

	t.Run("Success - returns nil for non-existent sub", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		retrievedUser, err := userSvc.GetBySub("non-existent-sub")
		require.NoError(t, err)
		require.Nil(t, retrievedUser)
	})
}

func TestUserService_CreateUserFromInvitation(t *testing.T) {
	t.Parallel()

	t.Run("Success - creates user from invitation", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		invitation := &models.Invitation{
			ID:             "test-invitation",
			OrganizationID: "org-123",
			Sub:            "user-sub",
			Roles:          []string{"admin"},
			CreatedBy:      "creator",
			CreatedAt:      time.Now().UTC(),
			ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
			IsConsumed:     false,
		}

		user, err := userSvc.CreateUserFromInvitation("user-sub", invitation)
		require.NoError(t, err)
		require.NotNil(t, user)
		require.Equal(t, "user-sub", user.ID)
		require.Equal(t, string(constants.AuthProviderJWT), user.Provider)
		require.Equal(t, "org-123", user.OrganizationID)
		require.Equal(t, []string{"admin"}, user.Roles)
		require.False(t, user.IsBootstrap)
	})

	t.Run("Error - empty sub returns error", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		invitation := &models.Invitation{
			ID:             "test-invitation",
			OrganizationID: "org-123",
			Sub:            "user-sub",
			Roles:          []string{"admin"},
			CreatedBy:      "creator",
			CreatedAt:      time.Now().UTC(),
			ExpiresAt:      time.Now().UTC().Add(24 * time.Hour),
			IsConsumed:     false,
		}

		_, err = userSvc.CreateUserFromInvitation("", invitation)
		require.Error(t, err)
	})

	t.Run("Error - nil invitation returns error", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		_, err = userSvc.CreateUserFromInvitation("user-sub", nil)
		require.Error(t, err)
	})
}

func TestUserService_UpdatePasskeyCredentials(t *testing.T) {
	t.Parallel()

	t.Run("Success - updates passkey credentials", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		user, err := userSvc.CreateUser()
		require.NoError(t, err)

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
		require.NoError(t, err)

		// Verify credentials were updated
		updatedUser, err := userSvc.GetByID(user.ID)
		require.NoError(t, err)
		require.Len(t, updatedUser.PasskeyCredentials, 1)
		require.Equal(t, []byte("cred-1"), updatedUser.PasskeyCredentials[0].ID)
	})

	t.Run("Error - DocUpdate failure", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

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

func TestUserService_DeleteUser(t *testing.T) {
	t.Parallel()

	t.Run("Success - deletes user", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		err = userSvc.DeleteUser(user.ID)
		require.NoError(t, err)

		// Verify user was deleted
		deletedUser, err := userSvc.GetByID(user.ID)
		require.NoError(t, err)
		require.Nil(t, deletedUser)
	})

	t.Run("Success - with auth cache invalidation", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

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

	t.Run("Error - deleting non-existent user returns error", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		err = userSvc.DeleteUser("non-existent-id")
		require.Error(t, err)
	})
}

func TestUserService_docToUser(t *testing.T) {
	t.Parallel()

	t.Run("Success - converts valid doc to user", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		doc, err := db.DocStore.DocGet(marshaler.CollectionName(constants.CollectionUsers), user.ID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		converted, err := userSvc.docToUser(doc)
		require.NoError(t, err)
		require.NotNil(t, converted)
		require.Equal(t, user.ID, converted.ID)
	})

	t.Run("Error - handles malformed doc", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		// Create a malformed document
		malformedDoc := &models.Document{
			ID:   "malformed",
			Data: map[string]json.RawMessage{"invalid": json.RawMessage("{invalid json")},
		}

		_, err = userSvc.docToUser(malformedDoc)
		require.Error(t, err)
	})
}

func TestUserService_CreateInvitation(t *testing.T) {
	t.Parallel()

	t.Run("Success - creates invitation", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		invitation, err := userSvc.CreateInvitation("org-123", "user-sub", "creator", []string{"admin"}, 24*time.Hour)
		require.NoError(t, err)
		require.NotNil(t, invitation)
		require.Equal(t, "org-123", invitation.OrganizationID)
		require.Equal(t, "user-sub", invitation.Sub)
		require.Equal(t, "creator", invitation.CreatedBy)
		require.Equal(t, []string{"admin"}, invitation.Roles)
		require.False(t, invitation.IsConsumed)
		require.True(t, invitation.ExpiresAt.After(time.Now().UTC()))
	})

	t.Run("Success - creates invitation with default roles", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		invitation, err := userSvc.CreateInvitation("org-123", "user-sub", "creator", nil, 24*time.Hour)
		require.NoError(t, err)
		require.NotNil(t, invitation)
		require.Equal(t, []string{"user"}, invitation.Roles)
	})

	t.Run("Error - missing required fields", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		_, err = userSvc.CreateInvitation("", "user-sub", "creator", []string{"admin"}, 24*time.Hour)
		require.Error(t, err)

		_, err = userSvc.CreateInvitation("org-123", "", "creator", []string{"admin"}, 24*time.Hour)
		require.Error(t, err)

		_, err = userSvc.CreateInvitation("org-123", "user-sub", "", []string{"admin"}, 24*time.Hour)
		require.Error(t, err)
	})
}

func TestUserService_GetInvitationByID(t *testing.T) {
	t.Parallel()

	t.Run("Success - retrieves invitation by ID", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		created, err := userSvc.CreateInvitation("org-123", "user-sub", "creator", []string{"admin"}, 24*time.Hour)
		require.NoError(t, err)

		retrieved, err := userSvc.GetInvitationByID(created.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		require.Equal(t, created.ID, retrieved.ID)
		require.Equal(t, "org-123", retrieved.OrganizationID)
	})

	t.Run("Success - returns nil for non-existent invitation", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		retrieved, err := userSvc.GetInvitationByID("non-existent")
		require.NoError(t, err)
		require.Nil(t, retrieved)
	})
}

func TestUserService_FindActiveInvitationBySub(t *testing.T) {
	t.Parallel()

	t.Run("Success - finds active invitation", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		created, err := userSvc.CreateInvitation("org-123", "user-sub", "creator", []string{"admin"}, 24*time.Hour)
		require.NoError(t, err)

		found, err := userSvc.FindActiveInvitationBySub("user-sub")
		require.NoError(t, err)
		require.NotNil(t, found)
		require.Equal(t, created.ID, found.ID)
		require.Equal(t, "user-sub", found.Sub)
	})

	t.Run("Success - returns nil for consumed invitation", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		created, err := userSvc.CreateInvitation("org-123", "user-sub", "creator", []string{"admin"}, 24*time.Hour)
		require.NoError(t, err)

		// Consume the invitation
		err = userSvc.ConsumeInvitation(created.ID)
		require.NoError(t, err)

		// Should not find it anymore
		found, err := userSvc.FindActiveInvitationBySub("user-sub")
		require.NoError(t, err)
		require.Nil(t, found)
	})

	t.Run("Success - returns nil for expired invitation", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		// Create an expired invitation
		_, err = userSvc.CreateInvitation("org-123", "user-sub", "creator", []string{"admin"}, -1*time.Hour)
		require.NoError(t, err)

		found, err := userSvc.FindActiveInvitationBySub("user-sub")
		require.NoError(t, err)
		require.Nil(t, found)
	})

	t.Run("Success - returns nil when no invitation exists", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		found, err := userSvc.FindActiveInvitationBySub("non-existent-sub")
		require.NoError(t, err)
		require.Nil(t, found)
	})

	t.Run("Error - empty sub returns error", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		_, err = userSvc.FindActiveInvitationBySub("")
		require.Error(t, err)
	})
}

func TestUserService_ConsumeInvitation(t *testing.T) {
	t.Parallel()

	t.Run("Success - consumes invitation", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		created, err := userSvc.CreateInvitation("org-123", "user-sub", "creator", []string{"admin"}, 24*time.Hour)
		require.NoError(t, err)
		require.False(t, created.IsConsumed)

		err = userSvc.ConsumeInvitation(created.ID)
		require.NoError(t, err)

		// Verify invitation is now consumed
		consumed, err := userSvc.GetInvitationByID(created.ID)
		require.NoError(t, err)
		require.NotNil(t, consumed)
		require.True(t, consumed.IsConsumed)
		require.NotNil(t, consumed.ConsumedAt)
	})

	t.Run("Error - consuming non-existent invitation returns error", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		testPaths := testutil.NewTestPathsFromTemp(t)
		db, err := OpenCanonicalDBService(testPaths.DataDir, testPaths.SecretsDir, testPaths.VaultDir, logger, true, "", false, nil)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)

		err = userSvc.ConsumeInvitation("non-existent")
		require.Error(t, err)
	})
}
