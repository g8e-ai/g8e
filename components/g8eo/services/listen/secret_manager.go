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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/services/sqliteutil"
)

// SecretManager handles generation and synchronization of platform security secrets.
type SecretManager struct {
	db     *sqliteutil.DB
	logger *slog.Logger
	sslDir string
}

func NewSecretManager(db *sqliteutil.DB, sslDir string, logger *slog.Logger) *SecretManager {
	return &SecretManager{
		db:     db,
		sslDir: sslDir,
		logger: logger,
	}
}

// InitPlatformSettings ensures platform secrets exist in both the DB and on disk.
func (m *SecretManager) InitPlatformSettings() error {
	tokenPath := filepath.Join(m.sslDir, "internal_auth_token")
	sessionKeyPath := filepath.Join(m.sslDir, "session_encryption_key")

	// Get encryption key from environment (if set)
	secretsKey := os.Getenv("G8E_SECRETS_KEY")

	var internalAuthToken string
	if data, err := os.ReadFile(tokenPath); err == nil {
		encrypted := strings.TrimSpace(string(data))
		// Try to decrypt if key is available
		if secretsKey != "" {
			if decrypted, err := m.decryptSecret(encrypted, secretsKey); err == nil {
				internalAuthToken = decrypted
			} else {
				// If decryption fails, assume it's plain text (backward compatibility)
				internalAuthToken = encrypted
			}
		} else {
			internalAuthToken = encrypted
		}
	}

	var sessionEncryptionKey string
	if data, err := os.ReadFile(sessionKeyPath); err == nil {
		encrypted := strings.TrimSpace(string(data))
		// Try to decrypt if key is available
		if secretsKey != "" {
			if decrypted, err := m.decryptSecret(encrypted, secretsKey); err == nil {
				sessionEncryptionKey = decrypted
			} else {
				// If decryption fails, assume it's plain text (backward compatibility)
				sessionEncryptionKey = encrypted
			}
		} else {
			sessionEncryptionKey = encrypted
		}
	}

	var exists bool
	err := m.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM documents WHERE collection = 'settings' AND id = 'platform_settings')",
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check platform_settings existence: %w", err)
	}

	now := time.Now().UTC()

	if !exists {
		if internalAuthToken == "" {
			internalAuthToken = m.generateSecureToken(32)
		}
		if sessionEncryptionKey == "" {
			sessionEncryptionKey = m.generateSecureToken(32)
		}

		platformSettings := models.SettingsDocument{}
		platformSettings.Settings = map[string]interface{}{
			"internal_auth_token":    internalAuthToken,
			"session_encryption_key": sessionEncryptionKey,
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

		cacheKey := "g8e:cache:doc:settings:platform_settings"
		cacheTTL := 3600
		_, err = m.db.Exec(
			`INSERT INTO kv_store (key, value, created_at, expires_at)
			 VALUES (?, ?, ?, ?)`,
			cacheKey, string(dataJSON), nowStr, sqliteutil.FormatTimestamp(now.Add(time.Duration(cacheTTL)*time.Second)),
		)
		if err != nil {
			m.logger.Warn("[SecretManager] Failed to warm cache for platform_settings", "error", err)
		} else {
			m.logger.Info("[SecretManager] platform_settings cache warmed", "key", cacheKey, "ttl", cacheTTL)
		}
	} else {
		var dataJSON string
		err := m.db.QueryRow(
			"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
		).Scan(&dataJSON)

		if err == nil {
			var settings models.SettingsDocument
			if err := json.Unmarshal([]byte(dataJSON), &settings); err == nil {
				changed := false
				if internalAuthToken != "" {
					if val, ok := settings.Settings["internal_auth_token"].(string); !ok || val != internalAuthToken {
						settings.Settings["internal_auth_token"] = internalAuthToken
						changed = true
						m.logger.Info("[SecretManager] Synchronized internal_auth_token from file to DB")
					}
				}
				if sessionEncryptionKey != "" {
					if val, ok := settings.Settings["session_encryption_key"].(string); !ok || val != sessionEncryptionKey {
						settings.Settings["session_encryption_key"] = sessionEncryptionKey
						changed = true
						m.logger.Info("[SecretManager] Synchronized session_encryption_key from file to DB")
					}
				}

				if changed {
					settings.UpdatedAt = now
					updatedJSON, _ := json.Marshal(settings)
					nowStr := sqliteutil.FormatTimestamp(now)
					m.db.Exec("UPDATE documents SET data = ?, updated_at = ? WHERE collection = ? AND id = ?",
						string(updatedJSON), nowStr, "settings", "platform_settings")

					cacheKey := "g8e:cache:doc:settings:platform_settings"
					cacheTTL := 3600
					_, err = m.db.Exec(
						`INSERT INTO kv_store (key, value, created_at, expires_at)
						 VALUES (?, ?, ?, ?)
						 ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
						cacheKey, string(updatedJSON), nowStr, sqliteutil.FormatTimestamp(now.Add(time.Duration(cacheTTL)*time.Second)),
					)
					if err != nil {
						m.logger.Warn("[SecretManager] Failed to warm cache for platform_settings after update", "error", err)
					} else {
						m.logger.Info("[SecretManager] platform_settings cache warmed after update", "key", cacheKey, "ttl", cacheTTL)
					}
				}

				if internalAuthToken == "" {
					if val, ok := settings.Settings["internal_auth_token"].(string); ok {
						internalAuthToken = val
					}
				}
				if sessionEncryptionKey == "" {
					if val, ok := settings.Settings["session_encryption_key"].(string); ok {
						sessionEncryptionKey = val
					}
				}
			}
		}
	}

	// Encrypt secrets before writing to disk if key is available
	var tokenToWrite, keyToWrite string
	if secretsKey != "" {
		if encrypted, err := m.encryptSecret(internalAuthToken, secretsKey); err == nil {
			tokenToWrite = encrypted
		} else {
			m.logger.Warn("[SecretManager] Failed to encrypt internal_auth_token, writing plain text", "error", err)
			tokenToWrite = internalAuthToken
		}
		if encrypted, err := m.encryptSecret(sessionEncryptionKey, secretsKey); err == nil {
			keyToWrite = encrypted
		} else {
			m.logger.Warn("[SecretManager] Failed to encrypt session_encryption_key, writing plain text", "error", err)
			keyToWrite = sessionEncryptionKey
		}
	} else {
		tokenToWrite = internalAuthToken
		keyToWrite = sessionEncryptionKey
	}

	if tokenToWrite != "" {
		if err := m.writeSecretFile(tokenPath, tokenToWrite, "internal_auth_token"); err != nil {
			return err
		}
	}
	if keyToWrite != "" {
		if err := m.writeSecretFile(sessionKeyPath, keyToWrite, "session_encryption_key"); err != nil {
			return err
		}
	}

	if err := m.verifyDBMatchesFile(tokenPath, "internal_auth_token"); err != nil {
		return err
	}
	if err := m.verifyDBMatchesFile(sessionKeyPath, "session_encryption_key"); err != nil {
		return err
	}

	if err := m.writeDigestManifest(map[string]string{
		"internal_auth_token":    internalAuthToken,
		"session_encryption_key": sessionEncryptionKey,
	}, now); err != nil {
		return err
	}

	return nil
}

// BootstrapDigestManifestFile is the filename of the tamper-evidence manifest
// written alongside bootstrap secrets on the SSL volume. Consumers
// (g8ed/g8ee BootstrapService) verify the SHA-256 of each secret they read
// from the volume matches the digest recorded here by g8eo at write time.
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

	finalPath := filepath.Join(m.sslDir, BootstrapDigestManifestFile)
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

// writeSecretFile atomically persists a bootstrap secret to the SSL volume and fails
// hard on any I/O error. The file is the source of truth read by g8ed and g8ee
// BootstrapService on startup; a silent write failure would leave the DB and volume
// out of sync and cause confusing auth failures on the next restart.
func (m *SecretManager) writeSecretFile(path, value, name string) error {
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		m.logger.Error("[SecretManager] Failed to write bootstrap secret to volume",
			"name", name, "path", path, "error", err)
		return fmt.Errorf("write bootstrap secret %s to %s: %w", name, path, err)
	}
	return nil
}

// verifyDBMatchesFile re-reads the on-disk secret and compares it (via SHA-256 digest
// for log safety) to the value stored in the platform_settings document. Divergence
// indicates a coding error in InitPlatformSettings or a concurrent writer and must
// abort startup rather than let the two authorities drift.
func (m *SecretManager) verifyDBMatchesFile(path, name string) error {
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read bootstrap secret %s from %s for verification: %w", name, path, err)
	}
	fileValue := strings.TrimSpace(string(fileBytes))

	var dataJSON string
	err = m.db.QueryRow(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON)
	if err != nil {
		return fmt.Errorf("read platform_settings for %s verification: %w", name, err)
	}
	var settings models.SettingsDocument
	if err := json.Unmarshal([]byte(dataJSON), &settings); err != nil {
		return fmt.Errorf("unmarshal platform_settings for %s verification: %w", name, err)
	}
	dbValue, _ := settings.Settings[name].(string)

	if fileValue != dbValue {
		fileDigest := sha256.Sum256([]byte(fileValue))
		dbDigest := sha256.Sum256([]byte(dbValue))
		m.logger.Error("[SecretManager] Bootstrap secret divergence between volume and DB",
			"name", name,
			"file_sha256", hex.EncodeToString(fileDigest[:]),
			"db_sha256", hex.EncodeToString(dbDigest[:]))
		return fmt.Errorf("bootstrap secret %s differs between volume file and platform_settings DB", name)
	}
	return nil
}

func (m *SecretManager) generateSecureToken(bytes int) string {
	tokenBytes := make([]byte, bytes)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return fmt.Sprintf("%0*x", bytes*2, time.Now().UnixNano())
	}
	return hex.EncodeToString(tokenBytes)
}

// encryptSecret encrypts a plaintext value using AES-256-GCM
// Returns base64-encoded ciphertext with format: iv:ciphertext
func (m *SecretManager) encryptSecret(plaintext, keyHex string) (string, error) {
	if keyHex == "" {
		return plaintext, nil
	}

	// Convert hex key to bytes
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", fmt.Errorf("invalid encryption key: %w", err)
	}
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes (64 hex chars)")
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce (12 bytes for GCM)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and seal
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Format: base64(nonce + ciphertext)
	combined := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

// decryptSecret decrypts a ciphertext value using AES-256-GCM
// Expects base64-encoded ciphertext with format: iv:ciphertext
func (m *SecretManager) decryptSecret(ciphertext, keyHex string) (string, error) {
	if keyHex == "" {
		return ciphertext, nil
	}

	// Convert hex key to bytes
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", fmt.Errorf("invalid encryption key: %w", err)
	}
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes (64 hex chars)")
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decode base64
	combined, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	// Extract nonce and ciphertext
	nonceSize := gcm.NonceSize()
	if len(combined) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := combined[:nonceSize], combined[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
