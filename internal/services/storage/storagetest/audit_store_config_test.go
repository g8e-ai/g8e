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

package storagetest

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditVaultConfig_Default(t *testing.T) {
	t.Parallel()
	// Ensure no environment variable override is set
	os.Unsetenv("G8E_DATA_DIR")

	config := DefaultTestSQLAuditStoreConfig()

	// Default DataDir is now a relative path - caller must resolve based on workDir
	assert.Equal(t, ".g8e/data", config.DataDir)
	assert.Equal(t, "g8e.db", config.DBPath)
	assert.Equal(t, constants.TestLedgerDirname, config.LedgerDir)
	assert.Equal(t, int64(2048), config.MaxDBSizeMB)
	assert.Equal(t, 90, config.RetentionDays)
	assert.Equal(t, 102400, config.OutputTruncationThreshold)
	assert.Equal(t, 51200, config.HeadTailSize)
}

func TestSQLAuditStore_BootstrapWithURL(t *testing.T) {
	t.Parallel()
	gitPath := testGitPath(t)

	// Create temporary directory for test
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 constants.TestLedgerDirname,
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		GitPath:                   gitPath,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NotNil(t, avs)
	defer avs.Close()

	// Verify directory structure was created
	assert.DirExists(t, tempDir)
	assert.DirExists(t, filepath.Join(tempDir, constants.TestLedgerDirname))
	assert.DirExists(t, filepath.Join(tempDir, constants.TestLedgerDirname, "files"))

	// Verify database was created
	assert.FileExists(t, filepath.Join(tempDir, "test.db"))

	// Verify git was initialized
	assert.DirExists(t, filepath.Join(tempDir, constants.TestLedgerDirname, constants.TestGitDirname))

	assert.Equal(t, tempDir, avs.GetDataDir())
}

func TestSQLAuditStore_GetDataDir(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 constants.TestLedgerDirname,
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	assert.Equal(t, tempDir, avs.GetDataDir())

	// Nil service
	var nilService *TestSQLAuditStore
	assert.Empty(t, nilService.GetDataDir())
}

func TestSQLAuditStore_GetLedgerPath(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 constants.TestLedgerDirname,
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	ledgerPath := avs.GetLedgerPath()
	assert.Contains(t, ledgerPath, constants.TestLedgerDirname)
	assert.DirExists(t, ledgerPath)

	// Nil service
	var nilService *TestSQLAuditStore
	assert.Empty(t, nilService.GetLedgerPath())
}

func TestSQLAuditStore_NilServiceMethods(t *testing.T) {
	t.Parallel()
	var avs *TestSQLAuditStore

	// These should not panic and return gracefully
	err := avs.CreateSession("id", "operator", "title", "user")
	require.NoError(t, err)

	eventID, err := avs.RecordEvent(&storage.Event{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), eventID)

	err = avs.RecordFileMutation(&storage.FileMutationLog{})
	require.NoError(t, err)

	err = avs.Close()
	require.NoError(t, err)
}

func TestSQLAuditStore_DefaultConfig(t *testing.T) {
	t.Parallel()
	// Verify default config uses relative paths (caller resolves them)
	// g8eo uses CLI flags only, not environment variables for configuration
	config := DefaultTestSQLAuditStoreConfig()
	assert.Equal(t, ".g8e/data", config.DataDir)
	assert.Equal(t, "g8e.db", config.DBPath)
	assert.Equal(t, constants.TestLedgerDirname, config.LedgerDir)
	assert.Equal(t, int64(2048), config.MaxDBSizeMB)
	assert.Equal(t, 90, config.RetentionDays)
}

func TestSQLAuditStore_GetEncryptionVault(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	logger := testutil.NewTestLogger()

	// 1. Constructor should return error when vault is nil
	config1 := &TestSQLAuditStoreConfig{
		DataDir: tempDir,
		DBPath:  "test1.db",
	}
	avs1, err := NewTestSQLAuditStore(config1, logger)
	require.Error(t, err)
	require.Nil(t, avs1)
	assert.Contains(t, err.Error(), "EncryptionVault is required")

	// 2. With encryption vault
	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: filepath.Join(tempDir, "vault"),
		Logger:  logger,
	})
	require.NoError(t, err)
	defer v.Close()

	config2 := &TestSQLAuditStoreConfig{
		DataDir:         tempDir,
		DBPath:          "test2.db",
		EncryptionVault: v,
	}
	avs2, err := NewTestSQLAuditStore(config2, logger)
	require.NoError(t, err)
	defer avs2.Close()
	assert.Equal(t, v, avs2.GetEncryptionVault())

	// 3. Nil service
	var nilService *TestSQLAuditStore
	assert.Nil(t, nilService.GetEncryptionVault())
}

func TestSQLAuditStore_CloseIdempotent(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 constants.TestLedgerDirname,
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)

	// Close multiple times should not panic
	err = avs.Close()
	require.NoError(t, err)

	// Second close might error (db already closed) but shouldn't panic
	_ = avs.Close()
}

func TestSQLAuditStore_WALMode(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 constants.TestLedgerDirname,
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// WAL mode should create additional files
	dbPath := filepath.Join(tempDir, "test.db")
	assert.FileExists(t, dbPath)

	// WAL file should be created after some activity
	err = avs.CreateSession("wal-test", "operator", "WAL Test", "user@test.com")
	require.NoError(t, err)

	// The -wal and -shm files may or may not exist depending on activity
	// Just verify the main db exists and works
	assert.FileExists(t, dbPath)
}
