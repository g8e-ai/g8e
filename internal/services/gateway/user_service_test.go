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
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestNewUserService(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false)
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
		dbDir := t.TempDir()
		secretsDir := t.TempDir()
		db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)
		require.NotNil(t, user)
		require.False(t, user.IsBootstrap)
		require.Equal(t, constants.UserStatusActive, user.Status)
	})
}

func TestUserService_CreateBootstrapUser(t *testing.T) {
	t.Run("Success - creates bootstrap user", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		dbDir := t.TempDir()
		secretsDir := t.TempDir()
		db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false)
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
		dbDir := t.TempDir()
		secretsDir := t.TempDir()
		db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)
		// Create first bootstrap user
		_, err = userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		// Second bootstrap user should fail
		_, err = userSvc.CreateBootstrapUser()
		require.Error(t, err)
		require.Contains(t, err.Error(), "bootstrap user already exists")
	})
}

func TestUserService_Disable(t *testing.T) {
	t.Parallel()
	t.Run("Success - disables user", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		dbDir := t.TempDir()
		secretsDir := t.TempDir()
		db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false)
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
		results, err := db.DocQuery(marshaler.CollectionName(constants.CollectionAuthAdminAudit), filters, "", 0)
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

	t.Run("Error - user not found", func(t *testing.T) {
		t.Parallel()
		logger := testutil.NewTestLogger()
		dataDir := t.TempDir()
		secretsDir := t.TempDir()
		vaultDir := t.TempDir()
		db, err := OpenCanonicalDBService(dataDir, secretsDir, vaultDir, logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		userSvc := NewUserService(db, logger)

		err = userSvc.Disable("non-existent-id", "test_reason", "actor_user_id", "operator_id")
		require.Error(t, err)
		require.Contains(t, err.Error(), "user not found")
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
		db, err := OpenCanonicalDBService(t.TempDir(), t.TempDir(), t.TempDir(), logger, true, "", false)
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
		db, err := OpenCanonicalDBService(t.TempDir(), t.TempDir(), t.TempDir(), logger, true, "", false)
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
		dbDir := t.TempDir()
		secretsDir := t.TempDir()
		db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false)
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
		dbDir := t.TempDir()
		secretsDir := t.TempDir()
		db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		userSvc := NewUserService(db, logger)
		_, err = userSvc.CreateUser()
		require.NoError(t, err)

		hasUsers, err := userSvc.HasAnyUsers()
		require.NoError(t, err)
		require.True(t, hasUsers)
	})
}
