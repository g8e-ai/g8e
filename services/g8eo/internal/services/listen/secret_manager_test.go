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

package listen

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/keystore"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	keystore.ResetTestStorage()
	os.Exit(m.Run())
}

// newSecretManagerTestDB opens a raw sqliteutil.DB with just the documents +
// kv_store schema that SecretManager needs, without pulling in the full
// ListenDBService wiring.
func newSecretManagerTestDB(t *testing.T) *sqliteutil.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "secret_manager_test.db")
	cfg := sqliteutil.DefaultDBConfig(dbPath)
	db, err := sqliteutil.OpenDB(cfg, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(listenSchema)
	require.NoError(t, err)
	return db
}

// newTestSecretManager creates a SecretManager with the in-memory test backend.
func newTestSecretManager(t *testing.T, db *sqliteutil.DB, secretsDir string) *SecretManager {
	t.Helper()
	backend, err := keystore.NewTestBackend()
	require.NoError(t, err)
	ks, err := keystore.NewWithBackend(secretsDir, testutil.NewTestLogger(), backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnsurePermissions())
	return &SecretManager{
		db:         db,
		secretsDir: secretsDir,
		logger:     testutil.NewTestLogger(),
		keystore:   ks,
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
	value, _ := doc.Settings[name].(string)
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
	doc.Settings[name] = value
	mutated, err := json.Marshal(doc)
	require.NoError(t, err)
	_, err = db.Exec(
		"UPDATE documents SET data = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		string(mutated),
	)
	require.NoError(t, err)
}

func TestSecretManager_InitPlatformSettings_CreatesSecretsAndFiles(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	require.NoError(t, sm.InitPlatformSettings())

	key, err := sm.keystore.DecryptSecret("session_encryption_key")
	require.NoError(t, err)
	assert.NotEmpty(t, key)
	assert.Equal(t, key, readSecretFromDB(t, db, "session_encryption_key"))

}

func TestSecretManager_InitPlatformSettings_CreatesValidActuatorKey(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	require.NoError(t, sm.InitPlatformSettings())

	seedHex := readSecretFromDB(t, db, "Actuator_signing_key")
	seed, err := hex.DecodeString(seedHex)
	require.NoError(t, err)
	require.Len(t, seed, ed25519.SeedSize)

	seedFromFile, err := sm.keystore.DecryptSecret("Actuator_signing_key")
	require.NoError(t, err)
	assert.Equal(t, seedHex, seedFromFile)

	priv, keyID, err := sm.GetActuatorKey()
	require.NoError(t, err)
	require.Len(t, priv, ed25519.PrivateKeySize)
	assert.Equal(t, readSecretFromDB(t, db, "Actuator_key_id"), keyID)
	assert.Equal(t, hex.EncodeToString(priv.Public().(ed25519.PublicKey)), keyID)
}

func TestSecretManager_GetActuatorKey_RejectsMalformedSeedLength(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitPlatformSettings())

	updatePlatformSetting(t, db, "Actuator_signing_key", strings.Repeat("a", ed25519.PrivateKeySize*2))

	_, _, err := sm.GetActuatorKey()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Actuator_signing_key decoded to 64 bytes; expected 32")
}

func TestSecretManager_GetActuatorKey_RejectsMismatchedKeyID(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitPlatformSettings())

	updatePlatformSetting(t, db, "Actuator_key_id", strings.Repeat("b", ed25519.PublicKeySize*2))

	_, _, err := sm.GetActuatorKey()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Actuator_key_id does not match Actuator_signing_key")
}

func TestSecretManager_InitPlatformSettings_FailsWhenFileWriteFails(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)

	// Replace the secrets directory with a file to cause real write failure
	// This uses actual filesystem operations without mocking
	require.NoError(t, os.RemoveAll(secretsDir))
	require.NoError(t, os.WriteFile(secretsDir, []byte("not a directory"), 0600))
	t.Cleanup(func() { _ = os.Remove(secretsDir) })

	err := sm.InitPlatformSettings()
	require.Error(t, err)
	// Error occurs during preexisting bootstrap state check when stat fails on a file
	assert.Contains(t, err.Error(), "not a directory")
}

func TestSecretManager_InitPlatformSettings_DetectsDBFileDivergence(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitPlatformSettings())

	// Write corrupted encrypted data (simulating manual file tampering)
	corruptedData := []byte(`{"version":1,"nonce":"AAAA","ciphertext":"corrupted"}`)
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), corruptedData, 0600))

	var dataJSON string
	require.NoError(t, db.QueryRow(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON))
	var doc models.SettingsDocument
	require.NoError(t, json.Unmarshal([]byte(dataJSON), &doc))
	doc.Settings["session_encryption_key"] = "divergent-db-only-value"
	mutated, err := json.Marshal(doc)
	require.NoError(t, err)
	_, err = db.Exec(
		"UPDATE documents SET data = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		string(mutated),
	)
	require.NoError(t, err)

	sm2 := newTestSecretManager(t, db, secretsDir)
	err = sm2.InitPlatformSettings()
	require.Error(t, err)
	// With encryption, file corruption causes decryption failure
	assert.Contains(t, err.Error(), "decryption failed")
}

func TestSecretManager_InitPlatformSettings_WritesDigestManifest(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitPlatformSettings())

	manifestPath := filepath.Join(secretsDir, BootstrapDigestManifestFile)
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err, "bootstrap digest manifest must be written")

	var manifest bootstrapDigestManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	assert.Equal(t, 1, manifest.Version)
	assert.NotEmpty(t, manifest.UpdatedAt)

	for _, name := range []string{"session_encryption_key"} {
		secret := readSecretFromDB(t, db, name)
		require.NotEmpty(t, secret)
		sum := sha256.Sum256([]byte(secret))
		ref, ok := manifest.Secrets[name]
		require.True(t, ok, "manifest must include %s entry", name)
		assert.Equal(t, hex.EncodeToString(sum[:]), ref.SHA256,
			"manifest digest for %s must match SHA-256 of DB/volume value", name)
	}
}

func TestSecretManager_InitPlatformSettings_ManifestPermissions(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitPlatformSettings())

	info, err := os.Stat(filepath.Join(secretsDir, BootstrapDigestManifestFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestSecretManager_InitPlatformSettings_RejectsUncoordinatedSecretRotation(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitPlatformSettings())

	// Write corrupted encrypted data (simulating manual file tampering)
	corruptedData := []byte(`{"version":1,"nonce":"AAAA","ciphertext":"corrupted"}`)
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), corruptedData, 0600))

	sm2 := newTestSecretManager(t, db, secretsDir)
	var err error
	err = sm2.InitPlatformSettings()
	require.Error(t, err)
	// With encryption, file corruption causes decryption failure
	assert.Contains(t, err.Error(), "decryption failed")
}

func TestSecretManager_InitPlatformSettings_RejectsPreexistingSecretWithoutPlatformSettings(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	preSeeded := strings.Repeat("c", 64)
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), []byte(preSeeded), 0600))

	sm := newTestSecretManager(t, db, secretsDir)
	err := sm.InitPlatformSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found preexisting bootstrap secret session_encryption_key without platform_settings")
}

func TestSecretManager_InitPlatformSettings_FailsWhenRequiredSecretFileMissing(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitPlatformSettings())
	require.NoError(t, os.Remove(filepath.Join(secretsDir, "session_encryption_key")))

	sm2 := newTestSecretManager(t, db, secretsDir)
	var err error
	err = sm2.InitPlatformSettings()
	require.Error(t, err)
	// With encryption, missing file causes decryption error
	assert.Contains(t, err.Error(), "decryption failed")
}

func TestSecretManager_InitPlatformSettings_FailsWhenDigestManifestMissing(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitPlatformSettings())
	require.NoError(t, os.Remove(filepath.Join(secretsDir, BootstrapDigestManifestFile)))

	sm2 := newTestSecretManager(t, db, secretsDir)
	var err error
	err = sm2.InitPlatformSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap digest manifest")
	assert.Contains(t, err.Error(), "is required")
}

func TestSecretManager_InitPlatformSettings_FailsWhenDigestManifestEntryMissing(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitPlatformSettings())
	manifestPath := filepath.Join(secretsDir, BootstrapDigestManifestFile)
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest bootstrapDigestManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	delete(manifest.Secrets, "session_encryption_key")
	mutated, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, mutated, 0600))

	sm2 := newTestSecretManager(t, db, secretsDir)
	err = sm2.InitPlatformSettings()
	require.Error(t, err)
	// With shared test backend, decryption succeeds and manifest validation fails
	assert.Contains(t, err.Error(), "bootstrap digest manifest missing required entry session_encryption_key")
}

func TestSecretManager_InitPlatformSettings_ReturnsErrorOnMalformedPlatformSettings(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()

	sm := newTestSecretManager(t, db, secretsDir)
	require.NoError(t, sm.InitPlatformSettings())

	_, err := db.Exec(
		"UPDATE documents SET data = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		"{invalid json",
	)
	require.NoError(t, err)

	sm2 := newTestSecretManager(t, db, secretsDir)
	err = sm2.InitPlatformSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal platform_settings document")
}

func TestSecretManager_APIKeys(t *testing.T) {
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
	assert.Error(t, err)
}

func TestSecretManager_OperatorPrivateKey(t *testing.T) {
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
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Store invalid seed length
	err := sm.keystore.EncryptSecret("operator_private_key", "deadbeef")
	require.NoError(t, err)

	_, err = sm.GetOperatorPrivateKey()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid length")
}

func TestSecretManager_CLIPrivateKey(t *testing.T) {
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
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Store session token with 1 millisecond TTL
	token := "session-expired"
	err := sm.StoreSessionToken(token, time.Millisecond)
	require.NoError(t, err)

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Retrieve should fail
	_, err = sm.GetSessionToken()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestSecretManager_SessionToken_InvalidFormat(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := newTestSecretManager(t, db, secretsDir)

	// Store malformed token data
	err := sm.keystore.EncryptSecret("session_token", "invalid-format-no-pipe")
	require.NoError(t, err)

	_, err = sm.GetSessionToken()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid session token format")
}
