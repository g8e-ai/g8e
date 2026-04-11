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

package sqliteutil

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DBConfig holds common configuration for opening a SQLite database.
type DBConfig struct {
	// Path is the filesystem path to the SQLite database file.
	Path string

	// CacheSizeMB is the SQLite page cache size in megabytes.
	// Default: 64
	CacheSizeMB int

	// BusyTimeoutMs is the SQLite busy timeout in milliseconds.
	// Default: 5000
	BusyTimeoutMs int

	// SetFilePermissions controls whether to chmod the DB file to 0600 after creation.
	// Default: true
	SetFilePermissions bool
}

// DefaultDBConfig returns a DBConfig with sensible defaults.
// The caller must set Path.
func DefaultDBConfig(path string) DBConfig {
	return DBConfig{
		Path:               path,
		CacheSizeMB:        64,
		BusyTimeoutMs:      5000,
		SetFilePermissions: true,
	}
}

// DB represents a wrapper around *sql.DB that provides common VSA data operations.
type DB struct {
	*sql.DB
	logger *slog.Logger
	path   string
}

// OpenDB opens (or creates) a SQLite database with best-practice settings.
func OpenDB(cfg DBConfig, logger *slog.Logger) (*DB, error) {
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", dir, err)
	}

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=%d",
		cfg.Path, cfg.BusyTimeoutMs)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database %s: %w", cfg.Path, err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database %s: %w", cfg.Path, err)
	}

	cacheSizeKB := cfg.CacheSizeMB * 1024
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		fmt.Sprintf("PRAGMA cache_size = -%d", cacheSizeKB),
		"PRAGMA auto_vacuum = INCREMENTAL",
		"PRAGMA temp_store = MEMORY",
	}
	for _, pragma := range pragmas {
		if _, err := sqlDB.Exec(pragma); err != nil {
			logger.Warn("Failed to set pragma", "pragma", pragma, "error", err)
		}
	}

	if cfg.SetFilePermissions {
		if err := os.Chmod(cfg.Path, 0600); err != nil {
			logger.Warn("Failed to set database file permissions", "path", cfg.Path, "error", err)
		}
	}

	logger.Info("SQLite database opened", "path", cfg.Path)
	return &DB{
		DB:     sqlDB,
		logger: logger,
		path:   cfg.Path,
	}, nil
}

// GetPath returns the filesystem path to the database file.
func (db *DB) GetPath() string {
	return db.path
}

// RunIncrementalVacuum runs an incremental vacuum to reclaim free pages.
func (db *DB) RunIncrementalVacuum(pages int) error {
	_, err := db.Exec(fmt.Sprintf("PRAGMA incremental_vacuum(%d)", pages))
	if err != nil {
		return fmt.Errorf("incremental vacuum failed: %w", err)
	}
	return nil
}

// GetSizeBytes returns the database size in bytes using PRAGMA page_count * page_size.
func (db *DB) GetSizeBytes() (int64, error) {
	var pageCount, pageSize int64
	if err := db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, fmt.Errorf("failed to query page_count: %w", err)
	}
	if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("failed to query page_size: %w", err)
	}
	return pageCount * pageSize, nil
}
