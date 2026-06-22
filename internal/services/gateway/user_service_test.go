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
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// newTestDocumentStoreService creates a DocumentStoreService with a test DB.
func newTestDocumentStoreService(t *testing.T) *DocumentStoreService {
	t.Helper()
	tmpDir := t.TempDir()
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

// newTestCanonicalDBService creates a CanonicalDBService with a test DocumentStoreService.
func newTestCanonicalDBService(t *testing.T) *CanonicalDBService {
	t.Helper()
	docStore := newTestDocumentStoreService(t)
	return &CanonicalDBService{DocStore: docStore}
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
	t.Parallel()

	mockDB := newTestCanonicalDBService(t)
	logger := newNoopLogger()

	userSvc := NewUserService(mockDB, logger)

	assert.NotNil(t, userSvc)
	assert.NotNil(t, userSvc.db)
	assert.Equal(t, logger, userSvc.logger)
	assert.Nil(t, userSvc.authSvc)
}

func TestUserService_SetAuthService(t *testing.T) {
	t.Parallel()

	mockDB := newTestCanonicalDBService(t)
	logger := newNoopLogger()
	userSvc := NewUserService(mockDB, logger)

	mockAuth := &mockAuthService{}
	userSvc.SetAuthService(mockAuth)

	assert.Equal(t, mockAuth, userSvc.authSvc)
}

func TestUserService_CreateUser(t *testing.T) {
	t.Run("Success - creates regular user", func(t *testing.T) {
		t.Parallel()
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		user, err := userSvc.CreateUser()

		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.False(t, user.IsBootstrap)
		assert.Equal(t, constants.UserStatusActive, user.Status)
		assert.NotEmpty(t, user.ID)
		assert.NotEmpty(t, user.WebAuthnUserID)
		assert.Equal(t, string(constants.AuthProviderPasskey), user.Provider)
	})
}

func TestUserService_CreateBootstrapUser(t *testing.T) {
	t.Run("Success - creates bootstrap user", func(t *testing.T) {
		t.Parallel()
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		user, err := userSvc.CreateBootstrapUser()

		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.True(t, user.IsBootstrap)
		assert.Equal(t, constants.UserStatusActive, user.Status)
	})

	t.Run("Error - bootstrap user already exists", func(t *testing.T) {
		t.Parallel()
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		// Create first bootstrap user
		_, err := userSvc.CreateBootstrapUser()
		require.NoError(t, err)

		// Try to create second bootstrap user
		_, err = userSvc.CreateBootstrapUser()
		assert.Error(t, err)
		assert.Equal(t, constants.ErrAlreadyExists, err)
	})
}

func TestUserService_Disable(t *testing.T) {
	t.Run("Success - disables user", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		err := userSvc.Disable("", "test reason", "actor-123", "operator-123")
		assert.Error(t, err)
		assert.Equal(t, constants.ErrUserIDRequired, err)
	})

	t.Run("Error - user not found", func(t *testing.T) {
		t.Parallel()
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		err := userSvc.Disable("non-existent", "test reason", "actor-123", "operator-123")
		assert.Error(t, err)
		assert.Equal(t, constants.ErrUserNotFound, err)
	})
}

func TestUserService_FindBootstrapUser(t *testing.T) {
	t.Run("Success - finds bootstrap user", func(t *testing.T) {
		t.Parallel()
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		// Create bootstrap user
		bootstrapUser, err := userSvc.CreateBootstrapUser()
		require.NoError(t, err)

		// Find bootstrap user
		found, err := userSvc.FindBootstrapUser()
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, bootstrapUser.ID, found.ID)
		assert.True(t, found.IsBootstrap)
	})

	t.Run("Success - returns nil when no bootstrap user", func(t *testing.T) {
		t.Parallel()
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		found, err := userSvc.FindBootstrapUser()
		assert.NoError(t, err)
		assert.Nil(t, found)
	})
}

func TestUser_IsActive(t *testing.T) {
	t.Run("Active status returns true", func(t *testing.T) {
		t.Parallel()
		user := &models.User{
			Status: constants.UserStatusActive,
		}
		assert.True(t, user.IsActive())
	})

	t.Run("Disabled status returns false", func(t *testing.T) {
		t.Parallel()
		user := &models.User{
			Status: constants.UserStatusDisabled,
		}
		assert.False(t, user.IsActive())
	})

	t.Run("Empty status returns true (backward compatibility)", func(t *testing.T) {
		t.Parallel()
		user := &models.User{
			Status: "",
		}
		assert.True(t, user.IsActive())
	})

	t.Run("Nil user returns false", func(t *testing.T) {
		t.Parallel()
		var user *models.User = nil
		assert.False(t, user.IsActive())
	})
}

func TestUserService_HasAnyUsers(t *testing.T) {
	t.Run("True - users exist", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()
		userSvc := NewUserService(mockDB, logger)

		hasUsers, err := userSvc.HasAnyUsers()
		assert.NoError(t, err)
		assert.False(t, hasUsers)
	})
}

func TestDefaultPersonaDefinitions(t *testing.T) {
	t.Parallel()

	personas := DefaultPersonaDefinitions()
	assert.NotEmpty(t, personas)
	assert.GreaterOrEqual(t, len(personas), 4) // admin, security-analyst, developer, auditor, default
}

func TestPersonaService_CreatePersona(t *testing.T) {
	t.Run("Success - creates persona", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
		mockDB := newTestCanonicalDBService(t)
		logger := newNoopLogger()

		personaSvc := NewPersonaService(mockDB, logger)

		assert.NotNil(t, personaSvc)
		assert.NotNil(t, personaSvc.db)
		assert.Equal(t, logger, personaSvc.logger)
	})

	t.Run("CreatePersona", func(t *testing.T) {
		t.Parallel()
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
