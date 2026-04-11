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
	"crypto/rand"
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

	var internalAuthToken string
	if data, err := os.ReadFile(tokenPath); err == nil {
		internalAuthToken = strings.TrimSpace(string(data))
	}

	var sessionEncryptionKey string
	if data, err := os.ReadFile(sessionKeyPath); err == nil {
		sessionEncryptionKey = strings.TrimSpace(string(data))
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

	if internalAuthToken != "" {
		if err := os.WriteFile(tokenPath, []byte(internalAuthToken), 0600); err != nil {
			m.logger.Error("[SecretManager] Failed to write internal_auth_token to file", "path", tokenPath, "error", err)
		}
	}
	if sessionEncryptionKey != "" {
		if err := os.WriteFile(sessionKeyPath, []byte(sessionEncryptionKey), 0600); err != nil {
			m.logger.Error("[SecretManager] Failed to write session_encryption_key to file", "path", sessionKeyPath, "error", err)
		}
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
