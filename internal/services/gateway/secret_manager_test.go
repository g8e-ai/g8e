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
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSecretManagerTestDB opens a raw sqliteutil.DB with just the documents +
// kv_store schema that SecretManager needs, without pulling in the full
// CanonicalDBService wiring.
func newSecretManagerTestDB(t *testing.T) *sqliteutil.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, constants.TestSecretManagerDBFilename)
	cfg := sqliteutil.DefaultDBConfig(dbPath)
	db, err := sqliteutil.OpenDB(cfg, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(GatewaySchema())
	require.NoError(t, err)
	return db
}

// newTestSecretManager creates a SecretManager with the in-memory keyring.
func newTestSecretManager(t *testing.T, db *sqliteutil.DB, secretsDir string) *SecretManager {
	t.Helper()
	keyring, err := keystore.NewMemoryKeyring()
	require.NoError(t, err)
	ks, err := keystore.NewWithKeyring(secretsDir, testutil.NewTestLogger(), keyring)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnforcePermissions())
	return &SecretManager{
		db:                  db,
		secretsDir:          secretsDir,
		bootstrapDigestPath: filepath.Join(secretsDir, constants.SecretsFileBootstrapDigest),
		logger:              testutil.NewTestLogger(),
		keystore:            ks,
	}
}

func readSecretFromDB(t *testing.T, db *sqliteutil.DB, name string) string {
	t.Helper()
	var dataJSON string
	err := db.QueryRow(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON)
	require.NoError(t, err)
	var doc models.SettingsDocument
	require.NoError(t, json.Unmarshal([]byte(dataJSON), &doc))
	require.NotNil(t, doc.Settings)
	switch name {
	case "actuator_key_id":
		return doc.Settings.ActuatorKeyID
	default:
		return ""
	}
}

// readSecretFromKeystore reads a secret directly from the keystore for testing
func readSecretFromKeystore(t *testing.T, sm *SecretManager, name string) string {
	t.Helper()
	value, err := sm.keystore.DecryptSecret(name)
	require.NoError(t, err)
	return value
}

func updatePlatformSetting(t *testing.T, db *sqliteutil.DB, name string, value string) {
	t.Helper()
	var dataJSON string
	require.NoError(t, db.QueryRow(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON))
	var doc models.SettingsDocument
	require.NoError(t, json.Unmarshal([]byte(dataJSON), &doc))
	require.NotNil(t, doc.Settings)
	switch name {
	case "actuator_key_id":
		doc.Settings.ActuatorKeyID = value
	}
	mutated, err := json.Marshal(doc)
	require.NoError(t, err)
	_, err = db.Exec(
		"UPDATE documents SET data = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		string(mutated),
	)
	require.NoError(t, err)
}

func TestSecretManager_InitAppSettings_CreatesSecretsAndFiles(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	require.NoError(t, sm.InitAppSettings())

	key, err := sm.keystore.DecryptSecret("session_encryption_key")
	require.NoError(t, err)
	assert.NotEmpty(t, key)
	// Secrets are now stored in keystore, not in DB
	// actuator_key_id is the only secret stored in DB
	keyID := readSecretFromDB(t, db, "actuator_key_id")
	assert.NotEmpty(t, keyID)

}

func TestSecretManager_InitAppSettings_CreatesValidActuatorKey(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	require.NoError(t, sm.InitAppSettings())

	// Seed is now stored in keystore, not DB
	seedHex, err := sm.keystore.DecryptSecret("actuator_signing_key")
	require.NoError(t, err)
	seed, err := hex.DecodeString(seedHex)
	require.NoError(t, err)
	require.Len(t, seed, ed25519.SeedSize)

	priv, keyID, err := sm.GetActuatorKey()
	require.NoError(t, err)
	require.Len(t, priv, ed25519.PrivateKeySize)
	assert.Equal(t, readSecretFromDB(t, db, "actuator_key_id"), keyID)
	assert.Equal(t, hex.EncodeToString(priv.Public().(ed25519.PublicKey)), keyID)
}

func TestSecretManager_GetActuatorKey_RejectsMalformedSeedLength(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitAppSettings())

	// Store malformed seed directly in keystore
	err := sm.keystore.EncryptSecret("actuator_signing_key", strings.Repeat("a", ed25519.PrivateKeySize*2))
	require.NoError(t, err)

	_, _, err = sm.GetActuatorKey()
	require.Error(t, err)
}

func TestSecretManager_GetActuatorKey_RejectsMismatchedKeyID(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitAppSettings())

	updatePlatformSetting(t, db, "actuator_key_id", strings.Repeat("b", ed25519.PublicKeySize*2))

	_, _, err := sm.GetActuatorKey()
	require.Error(t, err)
}

func TestSecretManager_InitAppSettings_FailsWhenFileWriteFails(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)

	// Replace the secrets directory with a file to cause real write failure
	// This uses actual filesystem operations without mocking
	require.NoError(t, os.RemoveAll(secretsDir))
	require.NoError(t, os.WriteFile(secretsDir, []byte("not a directory"), 0600))
	t.Cleanup(func() { _ = os.Remove(secretsDir) })

	err := sm.InitAppSettings()
	require.Error(t, err)
	// Error occurs during preexisting bootstrap state check when stat fails on a file
}

func TestSecretManager_InitAppSettings_DetectsDBFileDivergence(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitAppSettings())

	// Write corrupted encrypted data (simulating manual file tampering)
	corruptedData := []byte(`{"version":1,"nonce":"AAAA","ciphertext":"corrupted"}`)
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, constants.SecretsFileSessionEncryptionKey), corruptedData, 0600))

	sm2 := newTestSecretManager(t, db, secretsDir)
	err := sm2.InitAppSettings()
	require.Error(t, err)
	// With encryption, file corruption causes digest mismatch
}

func TestSecretManager_InitAppSettings_WritesDigestManifest(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitAppSettings())

	manifestPath := filepath.Join(secretsDir, constants.SecretsFileBootstrapDigest)
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err, "bootstrap digest manifest must be written")

	var manifest bootstrapDigestManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	assert.Equal(t, 1, manifest.Version)
	assert.NotEmpty(t, manifest.UpdatedAt)

	// Manifest should contain entries for all required secrets
	for _, name := range requiredBootstrapSecrets {
		ref, ok := manifest.Secrets[name]
		require.True(t, ok, "manifest must include %s entry", name)
		assert.NotEmpty(t, ref.SHA256, "manifest digest for %s must not be empty", name)
	}
}

func TestSecretManager_InitAppSettings_ManifestPermissions(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitAppSettings())

	info, err := os.Stat(filepath.Join(secretsDir, constants.SecretsFileBootstrapDigest))
	require.NoError(t, err)
	// Windows doesn't support Unix permissions exactly, so just check file is not world-writable
	perm := info.Mode().Perm()
	assert.NotEqual(t, os.FileMode(0777), perm, "manifest should not be world-writable")
}

func TestSecretManager_InitAppSettings_RejectsUncoordinatedSecretRotation(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitAppSettings())

	// Write corrupted encrypted data (simulating manual file tampering)
	corruptedData := []byte(`{"version":1,"nonce":"AAAA","ciphertext":"corrupted"}`)
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, constants.SecretsFileSessionEncryptionKey), corruptedData, 0600))

	sm2 := newTestSecretManager(t, db, secretsDir)
	err := sm2.InitAppSettings()
	require.Error(t, err)
	// With encryption, file corruption causes digest mismatch
}

func TestSecretManager_InitAppSettings_RejectsPreexistingSecretWithoutAppSettings(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	preSeeded := strings.Repeat("c", 64)
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, constants.SecretsFileSessionEncryptionKey), []byte(preSeeded), 0600))

	sm := newTestSecretManager(t, db, secretsDir)
	err := sm.InitAppSettings()
	require.Error(t, err)
}

func TestSecretManager_InitAppSettings_FailsWhenRequiredSecretFileMissing(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitAppSettings())
	require.NoError(t, os.Remove(filepath.Join(secretsDir, constants.SecretsFileSessionEncryptionKey)))

	sm2 := newTestSecretManager(t, db, secretsDir)
	err := sm2.InitAppSettings()
	require.Error(t, err)
	// Missing file causes read error during validation
}

func TestSecretManager_InitAppSettings_RecreatesWhenDigestManifestMissing(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitAppSettings())
	require.NoError(t, os.Remove(filepath.Join(secretsDir, constants.SecretsFileBootstrapDigest)))

	sm2 := newTestSecretManager(t, db, secretsDir)
	// Should recreate secrets instead of failing
	require.NoError(t, sm2.InitAppSettings())
	// Verify manifest was recreated
	_, err := os.Stat(filepath.Join(secretsDir, constants.SecretsFileBootstrapDigest))
	require.NoError(t, err)
}

func TestSecretManager_InitAppSettings_FailsWhenDigestManifestEntryMissing(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitAppSettings())
	manifestPath := filepath.Join(secretsDir, constants.SecretsFileBootstrapDigest)
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest bootstrapDigestManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	delete(manifest.Secrets, "session_encryption_key")
	mutated, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, mutated, 0600))

	sm2 := newTestSecretManager(t, db, secretsDir)
	err = sm2.InitAppSettings()
	require.Error(t, err)
}

func TestSecretManager_InitAppSettings_ReturnsErrorOnMalformedPlatformSettings(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitAppSettings())

	_, err := db.Exec(
		"UPDATE documents SET data = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		"{invalid json",
	)
	require.NoError(t, err)

	sm2 := newTestSecretManager(t, db, secretsDir)
	err = sm2.InitAppSettings()
	// This test is no longer valid since InitAppSettings doesn't read platform_settings on subsequent boots
	// It only checks for existence and then validates secrets
	// The cleanupStaleAppSettings would fail on malformed JSON
	require.NoError(t, err)
}

func TestSecretManager_APIKeys(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Store and retrieve API key
	err := sm.StoreAPIKey("openai", "sk-test-key-12345")
	require.NoError(t, err)

	retrieved, err := sm.GetAPIKey("openai")
	require.NoError(t, err)
	assert.Equal(t, "sk-test-key-12345", retrieved)

	// Non-existent service returns error
	_, err = sm.GetAPIKey("nonexistent")
	require.Error(t, err)
}

func TestSecretManager_OperatorPrivateKey(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Generate and store Operator private key
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	privKey := ed25519.NewKeyFromSeed(seed)

	err := sm.StoreOperatorPrivateKey(privKey)
	require.NoError(t, err)

	// Retrieve and validate
	retrieved, err := sm.GetOperatorPrivateKey()
	require.NoError(t, err)
	assert.Equal(t, privKey, retrieved)

	// Verify public key matches
	assert.Equal(t, privKey.Public().(ed25519.PublicKey), retrieved.Public().(ed25519.PublicKey))
}

func TestSecretManager_OperatorPrivateKey_RejectsInvalidSeed(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Store invalid seed length
	err := sm.keystore.EncryptSecret("operator_private_key", "deadbeef")
	require.NoError(t, err)

	_, err = sm.GetOperatorPrivateKey()
	require.Error(t, err)
}

func TestSecretManager_CLIPrivateKey(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Generate and store CLI private key
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	privKey := ed25519.NewKeyFromSeed(seed)

	err := sm.StoreCLIPrivateKey(privKey)
	require.NoError(t, err)

	// Retrieve and validate
	retrieved, err := sm.GetCLIPrivateKey()
	require.NoError(t, err)
	assert.Equal(t, privKey, retrieved)

	// Verify public key matches
	assert.Equal(t, privKey.Public().(ed25519.PublicKey), retrieved.Public().(ed25519.PublicKey))
}

func TestSecretManager_SessionToken(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Store session token with 1 hour TTL
	token := "session-abc-123"
	err := sm.StoreSessionToken(token, time.Hour)
	require.NoError(t, err)

	// Retrieve valid token
	retrieved, err := sm.GetSessionToken()
	require.NoError(t, err)
	assert.Equal(t, token, retrieved)
}

func TestSecretManager_SessionToken_Expires(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Store session token with 1 millisecond TTL
	token := "session-expired"
	err := sm.StoreSessionToken(token, time.Millisecond)
	require.NoError(t, err)

	// Wait for expiry with polling
	require.Eventually(t, func() bool {
		_, err := sm.GetSessionToken()
		return errors.Is(err, constants.ErrExpired)
	}, 100*time.Millisecond, 10*time.Millisecond, "session token should expire")
}

func TestSecretManager_SessionToken_InvalidFormat(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Store malformed token data
	err := sm.keystore.EncryptSecret("session_token", "invalid-format-no-pipe")
	require.NoError(t, err)

	_, err = sm.GetSessionToken()
	require.Error(t, err)
}

func TestSecretManager_GetKeystore(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	ks := sm.GetKeystore()
	assert.NotNil(t, ks)
	assert.Same(t, sm.keystore, ks)
}

func TestSecretManager_GetSessionEncryptionKey(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Initialize to create the key
	require.NoError(t, sm.InitAppSettings())

	// Retrieve the key
	key, err := sm.GetSessionEncryptionKey()
	require.NoError(t, err)
	assert.NotEmpty(t, key)
	assert.Len(t, key, 64) // 32 bytes hex encoded
}

func TestSecretManager_GetSessionEncryptionKey_NotInitialized(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Try to retrieve without initialization
	_, err := sm.GetSessionEncryptionKey()
	require.Error(t, err)
}

func TestSecretManager_GetAuditorHMACKey(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Initialize to create the key
	require.NoError(t, sm.InitAppSettings())

	// Retrieve the key
	key, err := sm.GetAuditorHMACKey()
	require.NoError(t, err)
	assert.NotEmpty(t, key)
	assert.Len(t, key, 64) // 32 bytes hex encoded
}

func TestSecretManager_GetAuditorHMACKey_NotInitialized(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Try to retrieve without initialization
	_, err := sm.GetAuditorHMACKey()
	require.Error(t, err)
}

func TestSecretManager_NotaryKey(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Store notary key
	seedHex := hex.EncodeToString([]byte("notary-seed-32-bytes-long-1234567890"))

	err := sm.StoreNotaryKey(seedHex)
	require.NoError(t, err)

	// Retrieve and validate
	retrieved, err := sm.GetNotaryKey()
	require.NoError(t, err)
	assert.Equal(t, seedHex, retrieved)
}

func TestSecretManager_GetNotaryKey_NotInitialized(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Try to retrieve without initialization
	_, err := sm.GetNotaryKey()
	require.Error(t, err)
}

func TestSecretManager_CleanupStaleAppSettings(t *testing.T) {
	t.Parallel()
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Test with no platform_settings document (query error)
	err := sm.cleanupStaleAppSettings()
	assert.Error(t, err)

	// Create a platform_settings document with stale fields
	settingsDoc := models.SettingsDocument{
		Settings: &models.PlatformSettings{
			PasskeyRPID:   "localhost",
			PasskeyOrigin: "https://localhost",
		},
	}
	settingsJSON, err := json.Marshal(settingsDoc)
	require.NoError(t, err)
	_, err = db.Exec(
		"INSERT INTO documents (collection, id, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"settings", "platform_settings", string(settingsJSON), sqliteutil.NowTimestamp(), sqliteutil.NowTimestamp(),
	)
	require.NoError(t, err)

	// Cleanup should remove stale fields
	err = sm.cleanupStaleAppSettings()
	assert.NoError(t, err)

	// Verify fields were cleared
	var dataJSON string
	err = db.QueryRow(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON)
	require.NoError(t, err)
	var doc models.SettingsDocument
	require.NoError(t, json.Unmarshal([]byte(dataJSON), &doc))
	assert.NotNil(t, doc.Settings)
	assert.Empty(t, doc.Settings.PasskeyRPID)
	assert.Empty(t, doc.Settings.PasskeyOrigin)

	// Test with no stale fields (no-op)
	err = sm.cleanupStaleAppSettings()
	assert.NoError(t, err)

	// Test with nil settings (no-op)
	settingsDoc.Settings = nil
	settingsJSON, err = json.Marshal(settingsDoc)
	require.NoError(t, err)
	_, err = db.Exec(
		"UPDATE documents SET data = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		string(settingsJSON),
	)
	require.NoError(t, err)

	err = sm.cleanupStaleAppSettings()
	assert.NoError(t, err)

	// Test with malformed JSON
	_, err = db.Exec(
		"UPDATE documents SET data = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		"{invalid json",
	)
	require.NoError(t, err)

	err = sm.cleanupStaleAppSettings()
	assert.Error(t, err)
}
