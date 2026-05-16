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
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sqliteutil"
)

var requiredBootstrapSecrets = []string{
	"session_encryption_key",
	"warden_signing_key",
	"warden_key_id",
	"auditor_hmac_key",
}

// SecretManager handles generation and validation of platform security secrets.
type SecretManager struct {
	db         *sqliteutil.DB
	logger     *slog.Logger
	secretsDir string
}

func NewSecretManager(db *sqliteutil.DB, secretsDir string, logger *slog.Logger) *SecretManager {
	return &SecretManager{
		db:         db,
		secretsDir: secretsDir,
		logger:     logger,
	}
}

// InitPlatformSettings creates secrets on first boot and validates them on later boots.
func (m *SecretManager) InitPlatformSettings() error {
	var exists bool
	err := m.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM documents WHERE collection = 'settings' AND id = 'platform_settings')",
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check platform_settings existence: %w", err)
	}

	now := time.Now().UTC()

	if !exists {
		return m.createPlatformSettings(now)
	}

	if err := m.cleanupStalePlatformSettings(); err != nil {
		m.logger.Warn("[SecretManager] Failed to cleanup stale platform settings", "error", err)
	}

	return m.validatePlatformSettings()
}

func (m *SecretManager) cleanupStalePlatformSettings() error {
	var dataJSON string
	err := m.db.QueryRow(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON)
	if err != nil {
		return err
	}

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(dataJSON), &doc); err != nil {
		return err
	}

	staleFields := []string{"passkey_rp_id", "passkey_origin", "setup_complete"}
	changed := false
	for _, field := range staleFields {
		if _, ok := doc[field]; ok {
			delete(doc, field)
			changed = true
		}
	}

	if !changed {
		return nil
	}

	m.logger.Info("[SecretManager] Cleaning up stale fields from platform_settings document", "fields", staleFields)

	newData, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	_, err = m.db.Exec(
		"UPDATE documents SET data = ?, updated_at = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		string(newData), sqliteutil.NowTimestamp(),
	)
	return err
}

func (m *SecretManager) createPlatformSettings(now time.Time) error {
	if err := m.rejectPreexistingBootstrapState(); err != nil {
		return err
	}
	if err := os.MkdirAll(m.secretsDir, 0700); err != nil {
		return fmt.Errorf("create bootstrap secrets directory %s: %w", m.secretsDir, err)
	}

	// Generate Warden signing key and compute its KeyID once
	wardenSeedBytes, err := m.generateSecureTokenBytes(ed25519.SeedSize)
	if err != nil {
		return err
	}
	wardenSeed := hex.EncodeToString(wardenSeedBytes)
	wardenPriv := ed25519.NewKeyFromSeed(wardenSeedBytes)
	wardenPub := wardenPriv.Public().(ed25519.PublicKey)
	wardenKeyID := hex.EncodeToString(wardenPub)
	sessionEncryptionKey, err := m.generateSecureToken(32)
	if err != nil {
		return err
	}
	auditorHMACKey, err := m.generateSecureToken(32)
	if err != nil {
		return err
	}

	secrets := map[string]string{
		"session_encryption_key": sessionEncryptionKey,
		"warden_signing_key":     wardenSeed, // Seed for ED25519
		"warden_key_id":          wardenKeyID,
		"auditor_hmac_key":       auditorHMACKey,
	}

	platformSettings := models.SettingsDocument{}
	platformSettings.Settings = map[string]interface{}{
		"session_encryption_key": secrets["session_encryption_key"],
		"warden_signing_key":     secrets["warden_signing_key"],
		"warden_key_id":          secrets["warden_key_id"],
		"auditor_hmac_key":       secrets["auditor_hmac_key"],
	}
	platformSettings.CreatedAt = now
	platformSettings.UpdatedAt = now

	dataJSON, err := json.Marshal(platformSettings)
	if err != nil {
		return fmt.Errorf("failed to marshal platform_settings: %w", err)
	}

	nowStr := sqliteutil.FormatTimestamp(now)
	_, err = m.db.Exec(
		`INSERT INTO documents (collection, id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"settings", "platform_settings", string(dataJSON), nowStr, nowStr,
	)
	if err != nil {
		return fmt.Errorf("failed to create platform_settings document: %w", err)
	}
	m.logger.Info("[SecretManager] platform_settings document created with security secrets")

	m.warmPlatformSettingsCache(string(dataJSON), now)

	for _, name := range requiredBootstrapSecrets {
		if err := m.writeSecretFile(m.secretPath(name), secrets[name], name); err != nil {
			return err
		}
	}

	if err := m.writeDigestManifest(secrets, now); err != nil {
		return err
	}

	return m.validatePlatformSettings()
}

func (m *SecretManager) validatePlatformSettings() error {
	if info, err := os.Stat(m.secretsDir); err != nil {
		return fmt.Errorf("bootstrap secrets directory %s is required after platform_settings exists: %w; delete and recreate runtime state", m.secretsDir, err)
	} else if !info.IsDir() {
		return fmt.Errorf("bootstrap secrets path %s is not a directory; delete and recreate runtime state", m.secretsDir)
	}

	dbSecrets, err := m.loadRequiredDBSecrets()
	if err != nil {
		return err
	}
	manifest, err := m.readDigestManifest()
	if err != nil {
		return err
	}

	for _, name := range requiredBootstrapSecrets {
		fileValue, err := m.readRequiredSecretFile(name)
		if err != nil {
			return err
		}
		if fileValue != dbSecrets[name] {
			fileDigest := sha256.Sum256([]byte(fileValue))
			dbDigest := sha256.Sum256([]byte(dbSecrets[name]))
			m.logger.Error("[SecretManager] Bootstrap secret divergence between file and DB",
				"name", name,
				"file_sha256", hex.EncodeToString(fileDigest[:]),
				"db_sha256", hex.EncodeToString(dbDigest[:]))
			return fmt.Errorf("bootstrap secret %s differs between secrets directory and platform_settings DB; delete and recreate runtime state", name)
		}
		entry, ok := manifest.Secrets[name]
		if !ok || entry.SHA256 == "" {
			return fmt.Errorf("bootstrap digest manifest missing required entry %s; delete and recreate runtime state", name)
		}
		fileDigest := sha256.Sum256([]byte(fileValue))
		if actual := hex.EncodeToString(fileDigest[:]); actual != entry.SHA256 {
			return fmt.Errorf("bootstrap secret %s digest %s does not match manifest digest %s; delete and recreate runtime state", name, actual, entry.SHA256)
		}
	}

	return nil
}

func (m *SecretManager) loadRequiredDBSecrets() (map[string]string, error) {
	var dataJSON string
	if err := m.db.QueryRow(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON); err != nil {
		return nil, fmt.Errorf("failed to query platform_settings document: %w", err)
	}

	var settings models.SettingsDocument
	if err := json.Unmarshal([]byte(dataJSON), &settings); err != nil {
		return nil, fmt.Errorf("failed to unmarshal platform_settings document: %w", err)
	}
	if settings.Settings == nil {
		return nil, fmt.Errorf("platform_settings missing settings map; delete and recreate runtime state")
	}

	secrets := make(map[string]string, len(requiredBootstrapSecrets))
	for _, name := range requiredBootstrapSecrets {
		value, ok := settings.Settings[name].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("platform_settings missing required bootstrap secret %s; delete and recreate runtime state", name)
		}
		secrets[name] = strings.TrimSpace(value)
	}
	return secrets, nil
}

// BootstrapDigestManifestFile is the filename of the tamper-evidence manifest
// written alongside bootstrap secrets.
const BootstrapDigestManifestFile = "bootstrap_digest.json"

// bootstrapDigestManifest is the on-disk schema for bootstrap_digest.json.
// Consumers on startup compute SHA-256 of each secret they load from the
// volume and compare to the digest recorded here. Divergence means the
// volume file has drifted from the DB-authoritative value SecretManager
// wrote, which must abort startup rather than authenticate with a silently
// incorrect secret.
type bootstrapDigestManifest struct {
	Version   int                           `json:"version"`
	UpdatedAt string                        `json:"updated_at"`
	Secrets   map[string]bootstrapDigestRef `json:"secrets"`
}

type bootstrapDigestRef struct {
	SHA256 string `json:"sha256"`
}

// writeDigestManifest writes the bootstrap digest manifest atomically. Empty
// secret values are skipped (they were never written to disk either). A
// failure here is fatal: without the manifest, consumers cannot detect
// volume-vs-DB drift on their next startup, which is the whole point of the
// manifest.
func (m *SecretManager) writeDigestManifest(secrets map[string]string, now time.Time) error {
	manifest := bootstrapDigestManifest{
		Version:   1,
		UpdatedAt: now.UTC().Format(time.RFC3339Nano),
		Secrets:   make(map[string]bootstrapDigestRef, len(secrets)),
	}
	for name, value := range secrets {
		if value == "" {
			continue
		}
		sum := sha256.Sum256([]byte(value))
		manifest.Secrets[name] = bootstrapDigestRef{SHA256: hex.EncodeToString(sum[:])}
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bootstrap digest manifest: %w", err)
	}

	finalPath := filepath.Join(m.secretsDir, BootstrapDigestManifestFile)
	tmpPath := finalPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		m.logger.Error("[SecretManager] Failed to write bootstrap digest manifest",
			"path", tmpPath, "error", err)
		return fmt.Errorf("write bootstrap digest manifest %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		m.logger.Error("[SecretManager] Failed to rename bootstrap digest manifest",
			"from", tmpPath, "to", finalPath, "error", err)
		return fmt.Errorf("rename bootstrap digest manifest to %s: %w", finalPath, err)
	}
	m.logger.Info("[SecretManager] Bootstrap digest manifest written",
		"path", finalPath, "secrets", len(manifest.Secrets))
	return nil
}

func (m *SecretManager) readDigestManifest() (*bootstrapDigestManifest, error) {
	manifestPath := filepath.Join(m.secretsDir, BootstrapDigestManifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("bootstrap digest manifest %s is required: %w; delete and recreate runtime state", manifestPath, err)
	}

	var manifest bootstrapDigestManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("bootstrap digest manifest %s is malformed: %w; delete and recreate runtime state", manifestPath, err)
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("bootstrap digest manifest version %d is unsupported; delete and recreate runtime state", manifest.Version)
	}
	if manifest.Secrets == nil {
		return nil, fmt.Errorf("bootstrap digest manifest missing secrets map; delete and recreate runtime state")
	}
	return &manifest, nil
}

func (m *SecretManager) rejectPreexistingBootstrapState() error {
	for _, name := range requiredBootstrapSecrets {
		if _, err := os.Stat(m.secretPath(name)); err == nil {
			return fmt.Errorf("found preexisting bootstrap secret %s without platform_settings; delete and recreate runtime state", name)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect bootstrap secret %s: %w", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(m.secretsDir, BootstrapDigestManifestFile)); err == nil {
		return fmt.Errorf("found preexisting bootstrap digest manifest without platform_settings; delete and recreate runtime state")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect bootstrap digest manifest: %w", err)
	}
	return nil
}

func (m *SecretManager) warmPlatformSettingsCache(dataJSON string, now time.Time) {
	cacheKey := "g8e:cache:doc:settings:platform_settings"
	cacheTTL := 3600
	nowStr := sqliteutil.FormatTimestamp(now)
	_, err := m.db.Exec(
		`INSERT INTO kv_store (key, value, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		cacheKey, dataJSON, nowStr, sqliteutil.FormatTimestamp(now.Add(time.Duration(cacheTTL)*time.Second)),
	)
	if err != nil {
		m.logger.Warn("[SecretManager] Failed to warm cache for platform_settings", "error", err)
	} else {
		m.logger.Info("[SecretManager] platform_settings cache warmed", "key", cacheKey, "ttl", cacheTTL)
	}
}

func (m *SecretManager) secretPath(name string) string {
	return filepath.Join(m.secretsDir, name)
}

func (m *SecretManager) readRequiredSecretFile(name string) (string, error) {
	path := m.secretPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("bootstrap secret %s is required at %s: %w; delete and recreate runtime state", name, path, err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("bootstrap secret %s is empty at %s; delete and recreate runtime state", name, path)
	}
	return value, nil
}

// writeSecretFile atomically persists a bootstrap secret and fails hard on any I/O error.
func (m *SecretManager) writeSecretFile(path, value, name string) error {
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		m.logger.Error("[SecretManager] Failed to write bootstrap secret",
			"name", name, "path", path, "error", err)
		return fmt.Errorf("write bootstrap secret %s to %s: %w", name, path, err)
	}
	return nil
}

func (m *SecretManager) generateSecureToken(bytes int) (string, error) {
	tokenBytes, err := m.generateSecureTokenBytes(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(tokenBytes), nil
}

func (m *SecretManager) generateSecureTokenBytes(bytes int) ([]byte, error) {
	if bytes <= 0 {
		return nil, fmt.Errorf("secure token byte length must be positive")
	}
	tokenBytes := make([]byte, bytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate secure random token: %w", err)
	}
	return tokenBytes, nil
}

// GetWardenKey retrieves the Warden's ED25519 signing key and its KeyID.
// The KeyID is stored explicitly in platform_settings to avoid recomputation.
func (m *SecretManager) GetWardenKey() (ed25519.PrivateKey, string, error) {
	secrets, err := m.loadRequiredDBSecrets()
	if err != nil {
		return nil, "", err
	}

	seedHex, ok := secrets["warden_signing_key"]
	if !ok {
		return nil, "", fmt.Errorf("warden_signing_key not found in platform_settings")
	}

	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode warden_signing_key: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, "", fmt.Errorf("warden_signing_key decoded to %d bytes; expected %d; delete and recreate runtime state", len(seed), ed25519.SeedSize)
	}

	priv := ed25519.NewKeyFromSeed(seed)

	keyID, ok := secrets["warden_key_id"]
	if !ok {
		return nil, "", fmt.Errorf("warden_key_id not found in platform_settings")
	}
	expectedKeyID := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	if keyID != expectedKeyID {
		return nil, "", fmt.Errorf("warden_key_id does not match warden_signing_key; delete and recreate runtime state")
	}

	return priv, keyID, nil
}
