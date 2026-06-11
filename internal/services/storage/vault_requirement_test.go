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

package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVaultRequirement_ExecutionVaultService verifies that NewExecutionVaultService
// requires a vault parameter and returns an error when vault is nil.
func TestVaultRequirement_ExecutionVaultService(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	config := DefaultExecutionVaultConfig()

	// Test that service fails to initialize with nil vault
	evs, err := NewExecutionVaultService(config, logger, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encryption vault is required")
	assert.Nil(t, evs)
}

// TestVaultRequirement_TokenStoreService verifies that NewTokenStoreService
// requires a vault parameter and returns an error when vault is nil.
func TestVaultRequirement_TokenStoreService(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	config := DefaultTokenStoreConfig()

	// Test that service fails to initialize with nil vault
	tss, err := NewTokenStoreService(config, logger, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encryption vault is required")
	assert.Nil(t, tss)
}

// TestVaultRequirement_SQLAuditStore verifies that NewSQLAuditStore
// requires EncryptionVault in config and returns an error when vault is nil.
func TestVaultRequirement_SQLAuditStore(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	config := DefaultAuditStoreConfig()

	// Test that service fails to initialize with nil EncryptionVault
	ass, err := NewSQLAuditStore(config, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EncryptionVault is required")
	assert.Nil(t, ass)
}

// TestLockedVaultHandling verifies that encryption operations fail-closed
// when the vault is locked (not unlocked).
func TestLockedVaultHandling(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	tempDir := t.TempDir()
	config := DefaultExecutionVaultConfig()
	config.DBPath = filepath.Join(tempDir, "test_locked_vault.db")

	// Create a vault but do NOT unlock it
	vaultDataDir := filepath.Join(tempDir, "vault")
	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: vaultDataDir,
		Logger:  logger,
	})
	require.NoError(t, err)
	t.Cleanup(func() { v.Close() })

	// Verify vault is locked
	assert.False(t, v.IsUnlocked(), "Vault should be locked after creation")

	// Service should initialize with locked vault (constructor doesn't check IsUnlocked)
	evs, err := NewExecutionVaultService(config, logger, v)
	require.NoError(t, err)
	require.NotNil(t, evs)
	defer evs.Close()

	// Encryption operations should fail when vault is locked
	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "test-locked-123",
		TimestampUTC:     time.Now().UTC(),
		Command:          "echo 'test'",
		ExitCode:         &exitCode,
		DurationMs:       100,
		StdoutCompressed: []byte("test output"),
		StderrCompressed: []byte(""),
		StdoutSize:       11,
		StderrSize:       0,
		UserID:           "user-123",
		CaseID:           "case-456",
	}

	err = evs.StoreExecution(context.Background(), record)
	assert.Error(t, err, "StoreExecution should fail when vault is locked")
	assert.Contains(t, err.Error(), "vault is locked", "Error should indicate vault is locked")
}
