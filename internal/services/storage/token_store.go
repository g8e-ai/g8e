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
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/interfaces"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/vault"
)

// TokenStoreConfig holds configuration for the token store service.
type TokenStoreConfig struct {
	DBPath               string
	MaxDBSizeMB          int64
	RetentionDays        int
	PruneIntervalMinutes int
	Enabled              bool
}

// DefaultTokenStoreConfig returns the default configuration.
func DefaultTokenStoreConfig() *TokenStoreConfig {
	return &TokenStoreConfig{
		DBPath:               ".g8e/token_store.db",
		MaxDBSizeMB:          512,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}
}

// TokenStoreService provides KV storage for Sentinel token persistence.
// This service implements the TokenStore interface with optional encryption.
type TokenStoreService struct {
	db     *sqliteutil.DB
	config *TokenStoreConfig
	logger *slog.Logger
	pruner *sqliteutil.Pruner
	vault  *vault.Vault

	wg sync.WaitGroup
}

// Ensure TokenStoreService implements interfaces.TokenStore.
var _ interfaces.TokenStore = (*TokenStoreService)(nil)

// NewTokenStoreService creates a new token store service.
func NewTokenStoreService(config *TokenStoreConfig, logger *slog.Logger, v *vault.Vault) (*TokenStoreService, error) {
	if config == nil {
		config = DefaultTokenStoreConfig()
	}

	if !config.Enabled {
		logger.Info("Token store is disabled")
		return nil, nil
	}

	if v == nil {
		return nil, fmt.Errorf("encryption vault is required")
	}

	cfg := sqliteutil.DefaultDBConfig(config.DBPath)
	db, err := sqliteutil.OpenDB(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	if _, err := db.Exec(tokenStoreSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	ts := &TokenStoreService{
		config: config,
		logger: logger,
		db:     db,
		vault:  v,
	}

	interval := time.Duration(config.PruneIntervalMinutes) * time.Minute
	ts.pruner = sqliteutil.NewPruner(db, logger, interval, tokenStorePrune(config))
	ts.pruner.Start()

	encryptionEnabled := ts.vault != nil && ts.vault.IsUnlocked()
	ts.logger.Info("Token store initialized",
		"db_path", config.DBPath,
		"encryption_enabled", encryptionEnabled)
	return ts, nil
}

// tokenStoreSchema defines the initial schema for the token store database.
const tokenStoreSchema = `
CREATE TABLE IF NOT EXISTS kv (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	expires_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_kv_expiry ON kv(expires_at);
`

// KVSet sets a key-value pair with an optional TTL (in seconds).
// If a vault is configured, the value is encrypted at rest using AES-256-GCM.
func (ts *TokenStoreService) KVSet(ctx context.Context, key, value string, ttlSeconds int) error {
	if ts == nil || ts.db == nil {
		return fmt.Errorf("token store is disabled")
	}
	ts.wg.Add(1)
	defer ts.wg.Done()

	var expiresAt *string
	if ttlSeconds > 0 {
		tsTime := sqliteutil.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &tsTime
	}

	if !ts.vault.IsUnlocked() {
		return fmt.Errorf("vault is locked, cannot encrypt value for key %s", key)
	}

	encrypted, err := ts.vault.Encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("failed to encrypt value for key %s: %w", key, err)
	}
	valueToStore := string(encrypted)

	query := `
	INSERT INTO kv (key, value, expires_at) VALUES (?, ?, ?)
	ON CONFLICT(key) DO UPDATE SET
		value = excluded.value,
		expires_at = excluded.expires_at
	`
	_, err = ts.db.ExecWithRetry(query, key, valueToStore, expiresAt)
	return err
}

// KVGet retrieves a value by key, honoring TTL.
// If a vault is configured, the value is decrypted using AES-256-GCM.
func (ts *TokenStoreService) KVGet(ctx context.Context, key string) (string, error) {
	if ts == nil || ts.db == nil {
		return "", fmt.Errorf("token store is disabled")
	}

	query := `
	SELECT value FROM kv
	WHERE key = ? AND (expires_at IS NULL OR expires_at > ?)
	`
	now := sqliteutil.FormatTimestamp(time.Now())
	var value string
	err := ts.db.QueryRowWithRetry(query, key, now).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("key not found: %s", key)
		}
		return "", fmt.Errorf("failed to query key %s: %w", key, err)
	}

	if !ts.vault.IsUnlocked() {
		return "", fmt.Errorf("vault is locked, cannot decrypt value for key %s", key)
	}

	decrypted, err := ts.vault.Decrypt([]byte(value))
	if err != nil {
		return "", fmt.Errorf("failed to decrypt value for key %s: %w", key, err)
	}
	return string(decrypted), nil
}

// KVScanPrefix retrieves all key-value pairs with a given prefix, honoring TTL.
func (ts *TokenStoreService) KVScanPrefix(ctx context.Context, prefix string) (map[string]string, error) {
	if ts == nil || ts.db == nil {
		return nil, fmt.Errorf("token store is disabled")
	}

	query := `
	SELECT key, value FROM kv
	WHERE key LIKE ? AND (expires_at IS NULL OR expires_at > ?)
	`
	now := sqliteutil.FormatTimestamp(time.Now())

	type kvPair struct {
		key   string
		value string
	}

	pairs, err := sqliteutil.MaterializeRows(ts.db, query, []interface{}{prefix + "%", now}, func(rows *sql.Rows) (kvPair, error) {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return kvPair{}, fmt.Errorf("failed to scan row: %w", err)
		}
		return kvPair{key: key, value: value}, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan keys: %w", err)
	}

	result := make(map[string]string)
	for _, pair := range pairs {
		value := pair.value
		if !ts.vault.IsUnlocked() {
			ts.logger.Error("Vault is locked, cannot decrypt value for key", "key", pair.key)
			continue
		}
		decrypted, err := ts.vault.Decrypt([]byte(value))
		if err != nil {
			ts.logger.Error("Failed to decrypt value for key", "key", pair.key, "error", err)
			continue
		}
		result[pair.key] = string(decrypted)
	}

	return result, nil
}

// KVDelete deletes a key-value pair.
func (ts *TokenStoreService) KVDelete(key string) error {
	if ts == nil || ts.db == nil {
		return nil
	}
	ts.wg.Add(1)
	defer ts.wg.Done()

	_, err := ts.db.ExecWithRetry("DELETE FROM kv WHERE key = ?", key)
	return err
}

// tokenStorePrune returns a PruneFunc for retention and size-based pruning.
func tokenStorePrune(config *TokenStoreConfig) sqliteutil.PruneFunc {
	return func(ctx context.Context, db *sqliteutil.DB, logger *slog.Logger) error {
		_, err := db.ExecWithRetry("DELETE FROM kv WHERE expires_at < ?", sqliteutil.FormatTimestamp(time.Now()))
		if err != nil {
			logger.Error("Failed to prune expired kv records", "error", err)
			return err
		}

		dbSizeBytes, err := db.GetSizeBytes()
		if err != nil {
			logger.Warn("Failed to get database size", "error", err)
		}
		maxSizeBytes := config.MaxDBSizeMB * 1024 * 1024

		if err == nil && dbSizeBytes > maxSizeBytes {
			_, err := db.ExecWithRetry(`
				DELETE FROM kv
				WHERE key IN (
					SELECT key FROM kv
					ORDER BY expires_at ASC
					LIMIT (SELECT COUNT(*) / 10 FROM kv)
				)
			`)
			if err != nil {
				logger.Error("Failed to prune kv for size limit", "error", err)
			}

			logger.Info("Pruned for size limit", "db_size_mb", dbSizeBytes/(1024*1024))
		}

		if err := db.RunIncrementalVacuum(1000); err != nil {
			logger.Info("Failed to run incremental vacuum", "error", err)
		}
		return nil
	}
}

// Close shuts down the token store service.
func (ts *TokenStoreService) Close() error {
	if ts == nil {
		return nil
	}

	if ts.pruner != nil {
		ts.pruner.Stop()
	}

	if ts.db != nil {
		return ts.db.Close()
	}

	return nil
}

// IsEnabled returns whether the token store is enabled.
func (ts *TokenStoreService) IsEnabled() bool {
	return ts != nil && ts.db != nil
}

// Wait blocks until all background workers and writes have finished.
func (ts *TokenStoreService) Wait() {
	ts.wg.Wait()
}
