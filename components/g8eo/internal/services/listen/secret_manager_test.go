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

	"github.com/g8e-ai/g8e/components/g8eo/internal/models"
	"github.com/g8e-ai/g8e/components/g8eo/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/components/g8eo/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	sm := NewSecretManager(db, secretsDir, testutil.NewTestLogger())

	require.NoError(t, sm.InitPlatformSettings())

	keyBytes, err := os.ReadFile(filepath.Join(secretsDir, "session_encryption_key"))
	require.NoError(t, err)
	key := strings.TrimSpace(string(keyBytes))
	assert.NotEmpty(t, key)
	assert.Equal(t, key, readSecretFromDB(t, db, "session_encryption_key"))

}

func TestSecretManager_InitPlatformSettings_CreatesValidWardenKey(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := NewSecretManager(db, secretsDir, testutil.NewTestLogger())

	require.NoError(t, sm.InitPlatformSettings())

	seedHex := readSecretFromDB(t, db, "warden_signing_key")
	seed, err := hex.DecodeString(seedHex)
	require.NoError(t, err)
	require.Len(t, seed, ed25519.SeedSize)

	seedBytes, err := os.ReadFile(filepath.Join(secretsDir, "warden_signing_key"))
	require.NoError(t, err)
	assert.Equal(t, seedHex, strings.TrimSpace(string(seedBytes)))

	priv, keyID, err := sm.GetWardenKey()
	require.NoError(t, err)
	require.Len(t, priv, ed25519.PrivateKeySize)
	assert.Equal(t, readSecretFromDB(t, db, "warden_key_id"), keyID)
	assert.Equal(t, hex.EncodeToString(priv.Public().(ed25519.PublicKey)), keyID)
}

func TestSecretManager_GetWardenKey_RejectsMalformedSeedLength(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := NewSecretManager(db, secretsDir, testutil.NewTestLogger())
	require.NoError(t, sm.InitPlatformSettings())

	updatePlatformSetting(t, db, "warden_signing_key", strings.Repeat("a", ed25519.PrivateKeySize*2))

	_, _, err := sm.GetWardenKey()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "warden_signing_key decoded to 64 bytes; expected 32")
}

func TestSecretManager_GetWardenKey_RejectsMismatchedKeyID(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := NewSecretManager(db, secretsDir, testutil.NewTestLogger())
	require.NoError(t, sm.InitPlatformSettings())

	updatePlatformSetting(t, db, "warden_key_id", strings.Repeat("b", ed25519.PublicKeySize*2))

	_, _, err := sm.GetWardenKey()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "warden_key_id does not match warden_signing_key")
}

func TestSecretManager_InitPlatformSettings_FailsWhenFileWriteFails(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	require.NoError(t, os.Chmod(secretsDir, 0500))
	t.Cleanup(func() { _ = os.Chmod(secretsDir, 0700) })

	sm := NewSecretManager(db, secretsDir, testutil.NewTestLogger())
	err := sm.InitPlatformSettings()
	require.Error(t, err, "InitPlatformSettings must surface file write failures as hard errors")
	assert.Contains(t, err.Error(), "write bootstrap secret")
}

func TestSecretManager_InitPlatformSettings_DetectsDBFileDivergence(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	require.NoError(t, NewSecretManager(db, secretsDir, logger).InitPlatformSettings())

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

	sm := NewSecretManager(db, secretsDir, logger)
	err = sm.InitPlatformSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "differs between secrets directory and platform_settings DB")
}

func TestSecretManager_InitPlatformSettings_WritesDigestManifest(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	sm := NewSecretManager(db, secretsDir, testutil.NewTestLogger())
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
	sm := NewSecretManager(db, secretsDir, testutil.NewTestLogger())
	require.NoError(t, sm.InitPlatformSettings())

	info, err := os.Stat(filepath.Join(secretsDir, BootstrapDigestManifestFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestSecretManager_InitPlatformSettings_RejectsUncoordinatedSecretRotation(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	require.NoError(t, NewSecretManager(db, secretsDir, logger).InitPlatformSettings())

	rotated := strings.Repeat("a", 64)
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), []byte(rotated), 0600))

	err := NewSecretManager(db, secretsDir, logger).InitPlatformSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "differs between secrets directory and platform_settings DB")
}

func TestSecretManager_InitPlatformSettings_RejectsPreexistingSecretWithoutPlatformSettings(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	preSeeded := strings.Repeat("c", 64)
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), []byte(preSeeded), 0600))

	err := NewSecretManager(db, secretsDir, logger).InitPlatformSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found preexisting bootstrap secret session_encryption_key without platform_settings")
}

func TestSecretManager_InitPlatformSettings_FailsWhenRequiredSecretFileMissing(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	require.NoError(t, NewSecretManager(db, secretsDir, logger).InitPlatformSettings())
	require.NoError(t, os.Remove(filepath.Join(secretsDir, "session_encryption_key")))

	err := NewSecretManager(db, secretsDir, logger).InitPlatformSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap secret session_encryption_key is required")
}

func TestSecretManager_InitPlatformSettings_FailsWhenDigestManifestMissing(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	require.NoError(t, NewSecretManager(db, secretsDir, logger).InitPlatformSettings())
	require.NoError(t, os.Remove(filepath.Join(secretsDir, BootstrapDigestManifestFile)))

	err := NewSecretManager(db, secretsDir, logger).InitPlatformSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap digest manifest")
	assert.Contains(t, err.Error(), "is required")
}

func TestSecretManager_InitPlatformSettings_FailsWhenDigestManifestEntryMissing(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	require.NoError(t, NewSecretManager(db, secretsDir, logger).InitPlatformSettings())
	manifestPath := filepath.Join(secretsDir, BootstrapDigestManifestFile)
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest bootstrapDigestManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	delete(manifest.Secrets, "session_encryption_key")
	mutated, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, mutated, 0600))

	err = NewSecretManager(db, secretsDir, logger).InitPlatformSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap digest manifest missing required entry session_encryption_key")
}

func TestSecretManager_InitPlatformSettings_ReturnsErrorOnMalformedPlatformSettings(t *testing.T) {
	db := newSecretManagerTestDB(t)
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	require.NoError(t, NewSecretManager(db, secretsDir, logger).InitPlatformSettings())

	_, err := db.Exec(
		"UPDATE documents SET data = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		"{invalid json",
	)
	require.NoError(t, err)

	sm := NewSecretManager(db, secretsDir, logger)
	err = sm.InitPlatformSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal platform_settings document")
}
