// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// newTestDocumentStoreService creates a DocumentStoreService with a test DB.
func newTestDocumentStoreService(t *testing.T) *DocumentStoreService {
	t.Helper()
	tmpDir := testutil.TempDir(t)
	dbPath := filepath.Join(tmpDir, "user_service_test.db")
	cfg := sqliteutil.DefaultDBConfig(dbPath)
	db, err := sqliteutil.OpenDB(cfg, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Create minimal schema for document store operations
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS documents (
			collection TEXT NOT NULL,
			id TEXT NOT NULL,
			data TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (collection, id)
		)
	`)
	require.NoError(t, err)

	return NewDocumentStoreService(db, testutil.NewTestLogger())
}

// newTestCanonicalDBService creates a DocumentStoreService for unit tests.
func newTestCanonicalDBService(t *testing.T) *DocumentStoreService {
	t.Helper()
	return newTestDocumentStoreService(t)
}

// mockAuthService is a mock implementation of AuthService for cache invalidation
type mockAuthService struct {
	invalidateUserCacheFunc func(userID string)
}

func (m *mockAuthService) InvalidateUserCache(userID string) {
	if m.invalidateUserCacheFunc != nil {
		m.invalidateUserCacheFunc(userID)
	}
}

// newNoopLogger creates a no-op logger for testing
func newNoopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewUserService(t *testing.T) {

	mockDB := newTestCanonicalDBService(t)
	logger := newNoopLogger()

	userSvc := NewUserService(mockDB, logger)

	assert.NotNil(t, userSvc)
	assert.NotNil(t, userSvc.db)
	assert.Equal(t, logger, userSvc.logger)
	assert.Nil(t, userSvc.authSvc)
}

func TestUserService_SetAuthService(t *testing.T) {

	mockDB := newTestCanonicalDBService(t)
	logger := newNoopLogger()
	userSvc := NewUserService(mockDB, logger)

	mockAuth := &mockAuthService{}
	userSvc.SetAuthService(mockAuth)

	assert.Equal(t, mockAuth, userSvc.authSvc)
}

func TestUserService_CreateUser(t *testing.T) {
	t.Run("Success - creates regular user", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		user, err := userSvc.CreateUser()

		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, constants.UserStatusActive, user.Status)
		assert.NotEmpty(t, user.ID)
		assert.NotEmpty(t, user.WebAuthnUserID)
		assert.Equal(t, string(constants.AuthProviderPasskey), user.Provider)
	})
}

func TestUserService_CreateUserWithOSUser(t *testing.T) {
	t.Run("Success - attaches provided local OS user info", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		osUser := &models.LocalOSUser{
			Domain:   "CORP",
			Username: "alice",
			UID:      "1001",
			GID:      "1001",
		}

		user, err := userSvc.CreateUserWithOSUser(osUser)

		require.NoError(t, err)
		require.NotNil(t, user)
		assert.Equal(t, constants.UserStatusActive, user.Status)
		assert.NotEmpty(t, user.ID)
		require.NotNil(t, user.LocalOSUser, "LocalOSUser should be attached when provided")
		assert.Equal(t, "CORP", user.LocalOSUser.Domain)
		assert.Equal(t, "alice", user.LocalOSUser.Username)
		assert.Equal(t, "1001", user.LocalOSUser.UID)
		assert.Equal(t, []string{string(constants.UserRoleOwner)}, user.Roles, "first user created via bootstrap gets the owner role")
	})

	t.Run("Success - falls back to gateway local OS user when nil", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		user, err := userSvc.CreateUserWithOSUser(nil)

		require.NoError(t, err)
		require.NotNil(t, user)
		// createUser falls back to getLocalOSUser() when nil is passed;
		// the test runner's OS user info is attached (non-nil on Unix/Windows).
		assert.NotNil(t, user.LocalOSUser, "LocalOSUser should fall back to the gateway's local OS user")
		assert.Equal(t, []string{string(constants.UserRoleOwner)}, user.Roles, "first user created via bootstrap gets the owner role")
	})
}

func TestUserService_Disable(t *testing.T) {
	t.Run("Success - disables user", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		// Create a user first
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		// Disable the user
		err = userSvc.Disable(user.ID, "test reason", "actor-123", "operator-123")
		assert.NoError(t, err)

		// Verify user is disabled
		disabledUser, err := userSvc.GetByID(user.ID)
		assert.NoError(t, err)
		assert.Equal(t, constants.UserStatusDisabled, disabledUser.Status)
	})

	t.Run("Error - empty user ID", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		err := userSvc.Disable("", "test reason", "actor-123", "operator-123")
		assert.Error(t, err)
		assert.Equal(t, constants.ErrUserIDRequired, err)
	})

	t.Run("Error - user not found", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		err := userSvc.Disable("non-existent", "test reason", "actor-123", "operator-123")
		assert.Error(t, err)
		assert.Equal(t, constants.ErrUserNotFound, err)
	})
}

func TestUserService_IsFirstUser(t *testing.T) {
	t.Run("True - sole user is the first user", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		first, err := userSvc.IsFirstUser(user.ID)
		assert.NoError(t, err)
		assert.True(t, first, "the only user in the system is the first user / admin")
	})

	t.Run("False - non-existent user is not the first user", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		// No users exist at all.
		first, err := userSvc.IsFirstUser("non-existent")
		assert.NoError(t, err)
		assert.False(t, first, "a user that does not exist cannot be the first user")
	})

	t.Run("False - second user is not the first user", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		firstUser, err := userSvc.CreateUser()
		require.NoError(t, err)
		secondUser, err := userSvc.CreateUser()
		require.NoError(t, err)

		isFirst, err := userSvc.IsFirstUser(secondUser.ID)
		assert.NoError(t, err)
		assert.False(t, isFirst, "the second user is not the first user / admin")

		// The original first user remains the first user (and admin) even
		// after a second user is created — IsFirstUser reports whether the
		// user IS the first user ever created, not whether they are the
		// only user. The first user retains admin permanently.
		isFirstOriginal, err := userSvc.IsFirstUser(firstUser.ID)
		assert.NoError(t, err)
		assert.True(t, isFirstOriginal, "the first user created remains the first user / admin even after a second user is added")
	})

	t.Run("Error - empty user ID", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		_, err := userSvc.IsFirstUser("")
		assert.Error(t, err)
		assert.Equal(t, constants.ErrUserIDRequired, err)
	})
}

func TestUser_IsActive(t *testing.T) {
	t.Run("Active status returns true", func(t *testing.T) {
		user := &models.User{
			Status: constants.UserStatusActive,
		}
		assert.True(t, user.IsActive())
	})

	t.Run("Disabled status returns false", func(t *testing.T) {
		user := &models.User{
			Status: constants.UserStatusDisabled,
		}
		assert.False(t, user.IsActive())
	})

	t.Run("Empty status returns false", func(t *testing.T) {
		user := &models.User{
			Status: "",
		}
		assert.False(t, user.IsActive())
	})

	t.Run("Nil user returns false", func(t *testing.T) {
		var user *models.User = nil
		assert.False(t, user.IsActive())
	})
}

func TestUserService_HasAnyUsers(t *testing.T) {
	t.Run("True - users exist", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		// Create a user
		_, err := userSvc.CreateUser()
		require.NoError(t, err)

		// Check if users exist
		hasUsers, err := userSvc.HasAnyUsers()
		assert.NoError(t, err)
		assert.True(t, hasUsers)
	})

	t.Run("False - no users exist", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		hasUsers, err := userSvc.HasAnyUsers()
		assert.NoError(t, err)
		assert.False(t, hasUsers)
	})
}

func TestDefaultPersonaDefinitions(t *testing.T) {

	personas := DefaultPersonaDefinitions()
	assert.NotEmpty(t, personas)
	assert.GreaterOrEqual(t, len(personas), 4) // admin, security-analyst, developer, auditor, default
}

func TestPersonaService_CreatePersona(t *testing.T) {
	t.Run("Success - creates persona", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		personaSvc := NewPersonaService(mockDB, logger)

		persona := &models.Persona{
			ID:          "test-persona",
			Name:        "Test Persona",
			Description: "A test persona",
			Roles:       []string{"admin"},
		}

		err := personaSvc.CreatePersona(persona)
		assert.NoError(t, err)

		// Verify persona was created
		found, err := personaSvc.GetByID("test-persona")
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, "test-persona", found.ID)
		assert.Equal(t, "Test Persona", found.Name)
	})
}

func TestPersonaService_GetByID(t *testing.T) {
	t.Run("Success - retrieves existing persona", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		personaSvc := NewPersonaService(mockDB, logger)

		persona := &models.Persona{
			ID:          "test-persona",
			Name:        "Test Persona",
			Description: "A test persona",
			Roles:       []string{"admin"},
		}
		err := personaSvc.CreatePersona(persona)
		require.NoError(t, err)

		retrieved, err := personaSvc.GetByID("test-persona")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, "test-persona", retrieved.ID)
	})

	t.Run("Success - returns nil for non-existent persona", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		personaSvc := NewPersonaService(mockDB, logger)

		retrieved, err := personaSvc.GetByID("non-existent")
		assert.NoError(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestPersonaService_GetAll(t *testing.T) {
	t.Run("Success - retrieves all personas", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		personaSvc := NewPersonaService(mockDB, logger)

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

		err := personaSvc.CreatePersona(persona1)
		require.NoError(t, err)
		err = personaSvc.CreatePersona(persona2)
		require.NoError(t, err)

		all, err := personaSvc.GetAll()
		assert.NoError(t, err)
		assert.Len(t, all, 2)
	})

	t.Run("Success - returns empty list when no personas", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		personaSvc := NewPersonaService(mockDB, logger)

		all, err := personaSvc.GetAll()
		assert.NoError(t, err)
		assert.Empty(t, all)
	})
}

func TestUserService_GetByID(t *testing.T) {
	t.Run("Success - retrieves user", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		// Create a user
		createdUser, err := userSvc.CreateUser()
		require.NoError(t, err)

		// Get the user by ID
		foundUser, err := userSvc.GetByID(createdUser.ID)
		assert.NoError(t, err)
		assert.NotNil(t, foundUser)
		assert.Equal(t, createdUser.ID, foundUser.ID)
		assert.Equal(t, constants.UserStatusActive, foundUser.Status)
	})

	t.Run("Success - returns nil for non-existent user", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		foundUser, err := userSvc.GetByID("non-existent")
		assert.NoError(t, err)
		assert.Nil(t, foundUser)
	})
}

func TestUserService_GetBySub(t *testing.T) {
	t.Run("Success - retrieves user by sub", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		// Create a user
		createdUser, err := userSvc.CreateUser()
		require.NoError(t, err)

		// Get the user by sub (same as ID)
		foundUser, err := userSvc.GetBySub(createdUser.ID)
		assert.NoError(t, err)
		assert.NotNil(t, foundUser)
		assert.Equal(t, createdUser.ID, foundUser.ID)
	})

	t.Run("Error - empty sub", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		_, err := userSvc.GetBySub("")
		assert.Error(t, err)
		assert.Equal(t, constants.ErrMissingRequiredField, err)
	})
}

func TestUserService_DeleteUser(t *testing.T) {
	t.Run("Success - deletes user", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		// Create a user
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		// Delete the user
		err = userSvc.DeleteUser(user.ID)
		assert.NoError(t, err)

		// Verify user is deleted
		_, err = userSvc.GetByID(user.ID)
		assert.NoError(t, err)
		assert.Nil(t, nil) // Should return nil, not found
	})

	t.Run("Error - user not found", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		err := userSvc.DeleteUser("non-existent")
		assert.Error(t, err)
		assert.Equal(t, constants.ErrUserNotFound, err)
	})
}

func TestUserService_UpdatePasskeyCredentials(t *testing.T) {
	t.Run("Success - updates credentials", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		// Create a user
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		// Update passkey credentials
		credentials := []models.PasskeyCredential{
			{
				ID:              []byte("cred-1"),
				PublicKey:       []byte("test-public-key"),
				AttestationType: "none",
				Authenticator: models.Authenticator{
					AAGUID:       []byte("test-aaguid"),
					SignCount:    0,
					CloneWarning: false,
				},
				CreatedAtUnixMs: time.Now().UnixMilli(),
			},
		}
		err = userSvc.UpdatePasskeyCredentials(user.ID, credentials)
		assert.NoError(t, err)

		// Verify credentials were updated
		updatedUser, err := userSvc.GetByID(user.ID)
		assert.NoError(t, err)
		assert.Len(t, updatedUser.PasskeyCredentials, 1)
	})
}

func TestPersonaService(t *testing.T) {
	t.Run("NewPersonaService", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()

		personaSvc := NewPersonaService(mockDB, logger)

		assert.NotNil(t, personaSvc)
		assert.NotNil(t, personaSvc.db)
		assert.Equal(t, logger, personaSvc.logger)
	})

	t.Run("CreatePersona", func(t *testing.T) {
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		personaSvc := NewPersonaService(mockDB, logger)

		persona := &models.Persona{
			ID:          "test-persona",
			Name:        "Test Persona",
			Description: "A test persona",
			Roles:       []string{"admin"},
		}

		err := personaSvc.CreatePersona(persona)
		assert.NoError(t, err)

		// Verify persona was created
		found, err := personaSvc.GetByID("test-persona")
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, "test-persona", found.ID)
		assert.Equal(t, "Test Persona", found.Name)
	})
}
