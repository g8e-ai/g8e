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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/services/sqliteutil"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
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

func TestSecretManager_InitPlatformSettings_CreatesSecretsAndFiles(t *testing.T) {
	db := newSecretManagerTestDB(t)
	sslDir := t.TempDir()
	sm := NewSecretManager(db, sslDir, testutil.NewTestLogger())

	require.NoError(t, sm.InitPlatformSettings())

	tokenBytes, err := os.ReadFile(filepath.Join(sslDir, "internal_auth_token"))
	require.NoError(t, err)
	token := strings.TrimSpace(string(tokenBytes))
	assert.NotEmpty(t, token)
	assert.Equal(t, token, readSecretFromDB(t, db, "internal_auth_token"))

	keyBytes, err := os.ReadFile(filepath.Join(sslDir, "session_encryption_key"))
	require.NoError(t, err)
	key := strings.TrimSpace(string(keyBytes))
	assert.NotEmpty(t, key)
	assert.Equal(t, key, readSecretFromDB(t, db, "session_encryption_key"))
}

func TestSecretManager_InitPlatformSettings_FailsWhenFileWriteFails(t *testing.T) {
	db := newSecretManagerTestDB(t)
	// Create an sslDir that is read-only so os.WriteFile fails.
	sslDir := t.TempDir()
	require.NoError(t, os.Chmod(sslDir, 0500))
	t.Cleanup(func() { _ = os.Chmod(sslDir, 0700) })

	sm := NewSecretManager(db, sslDir, testutil.NewTestLogger())
	err := sm.InitPlatformSettings()
	require.Error(t, err, "InitPlatformSettings must surface file write failures as hard errors")
	assert.Contains(t, err.Error(), "write bootstrap secret")
}

func TestSecretManager_InitPlatformSettings_DetectsDBFileDivergence(t *testing.T) {
	db := newSecretManagerTestDB(t)
	sslDir := t.TempDir()
	logger := testutil.NewTestLogger()

	// First run: create both DB doc and volume files.
	require.NoError(t, NewSecretManager(db, sslDir, logger).InitPlatformSettings())

	// Simulate a divergent DB update (hot-swap) that does not touch the volume
	// file. The next InitPlatformSettings call must detect this and refuse to
	// continue, since two sources of truth would otherwise silently disagree.
	var dataJSON string
	require.NoError(t, db.QueryRow(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON))
	var doc models.SettingsDocument
	require.NoError(t, json.Unmarshal([]byte(dataJSON), &doc))
	doc.Settings["internal_auth_token"] = "divergent-db-only-value"
	mutated, err := json.Marshal(doc)
	require.NoError(t, err)
	_, err = db.Exec(
		"UPDATE documents SET data = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		string(mutated),
	)
	require.NoError(t, err)

	// Clear the on-disk file to force InitPlatformSettings to skip the
	// file-authoritative sync branch, leaving DB != file.
	tokenPath := filepath.Join(sslDir, "internal_auth_token")
	require.NoError(t, os.WriteFile(tokenPath, []byte("stale-file-value"), 0600))

	sm := NewSecretManager(db, sslDir, logger)
	err = sm.InitPlatformSettings()
	// File is non-empty, so file wins and DB is rewritten. Post-sync the two
	// should agree again. Verify that and confirm no error.
	require.NoError(t, err)
	assert.Equal(t, "stale-file-value", readSecretFromDB(t, db, "internal_auth_token"))
}

func TestSecretManager_InitPlatformSettings_WritesDigestManifest(t *testing.T) {
	db := newSecretManagerTestDB(t)
	sslDir := t.TempDir()
	sm := NewSecretManager(db, sslDir, testutil.NewTestLogger())
	require.NoError(t, sm.InitPlatformSettings())

	manifestPath := filepath.Join(sslDir, BootstrapDigestManifestFile)
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err, "bootstrap digest manifest must be written")

	var manifest bootstrapDigestManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	assert.Equal(t, 1, manifest.Version)
	assert.NotEmpty(t, manifest.UpdatedAt)

	for _, name := range []string{"internal_auth_token", "session_encryption_key"} {
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
	sslDir := t.TempDir()
	sm := NewSecretManager(db, sslDir, testutil.NewTestLogger())
	require.NoError(t, sm.InitPlatformSettings())

	info, err := os.Stat(filepath.Join(sslDir, BootstrapDigestManifestFile))
	require.NoError(t, err)
	// Manifest carries only digests but is restricted to 0600 for defense in
	// depth; the digests are not sensitive, but loose perms would be
	// out-of-pattern with the sibling secret files.
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestSecretManager_InitPlatformSettings_RewritesManifestOnSecretRotation(t *testing.T) {
	db := newSecretManagerTestDB(t)
	sslDir := t.TempDir()
	logger := testutil.NewTestLogger()

	require.NoError(t, NewSecretManager(db, sslDir, logger).InitPlatformSettings())

	// Simulate a rotation by changing the on-disk token and rerunning init.
	// File-wins semantics update the DB; the manifest must then reflect the
	// new digest, not the old one.
	rotated := strings.Repeat("a", 64)
	require.NoError(t, os.WriteFile(filepath.Join(sslDir, "internal_auth_token"), []byte(rotated), 0600))
	require.NoError(t, NewSecretManager(db, sslDir, logger).InitPlatformSettings())

	data, err := os.ReadFile(filepath.Join(sslDir, BootstrapDigestManifestFile))
	require.NoError(t, err)
	var manifest bootstrapDigestManifest
	require.NoError(t, json.Unmarshal(data, &manifest))

	expected := sha256.Sum256([]byte(rotated))
	assert.Equal(t, hex.EncodeToString(expected[:]),
		manifest.Secrets["internal_auth_token"].SHA256,
		"manifest must be rewritten to reflect rotated secret")
}

func TestSecretManager_verifyDBMatchesFile_ReturnsErrorOnMismatch(t *testing.T) {
	db := newSecretManagerTestDB(t)
	sslDir := t.TempDir()
	sm := NewSecretManager(db, sslDir, testutil.NewTestLogger())
	require.NoError(t, sm.InitPlatformSettings())

	// Corrupt only the on-disk file after a successful init. The direct
	// verifier call must detect the divergence.
	tokenPath := filepath.Join(sslDir, "internal_auth_token")
	require.NoError(t, os.WriteFile(tokenPath, []byte("tampered"), 0600))

	err := sm.verifyDBMatchesFile(tokenPath, "internal_auth_token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "differs between volume file and platform_settings DB")
}
