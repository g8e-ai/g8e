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
	"fmt"
)

// Migration represents a single schema migration step.
type Migration struct {
	// Version is the monotonically increasing migration version number.
	Version int
	// Description is a human-readable description of what this migration does.
	Description string
	// SQL is the SQL statement(s) to execute for this migration.
	SQL string
}

// schemaVersionDDL creates the schema_version table if it doesn't exist.
const schemaVersionDDL = `
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER PRIMARY KEY,
	description TEXT,
	applied_at TEXT NOT NULL
);
`

// RunMigrations applies any unapplied migrations in order.
// It creates the schema_version tracking table if it doesn't exist,
// then applies each migration whose version is greater than the current max.
// Each migration is run in its own implicit transaction.
func (db *DB) RunMigrations(migrations []Migration) error {
	// Ensure the schema_version table exists
	if _, err := db.Exec(schemaVersionDDL); err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	// Get current schema version
	var currentVersion int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		currentVersion = 0
	}

	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		db.logger.Info("Applying schema migration",
			"version", m.Version,
			"description", m.Description)

		// Each migration and its version recording must be atomic
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction for migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s) failed: %w", m.Version, m.Description, err)
		}

		// Record the migration
		_, err = tx.Exec(
			"INSERT INTO schema_version (version, description, applied_at) VALUES (?, ?, ?)",
			m.Version, m.Description, NowTimestamp(),
		)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction for migration %d: %w", m.Version, err)
		}

		db.logger.Info("Schema migration applied",
			"version", m.Version,
			"description", m.Description)
	}

	return nil
}

// ColumnExists checks whether a column exists on a table.
// Useful for conditional ALTER TABLE migrations.
func (db *DB) ColumnExists(table, column string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?",
		table, column,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check column %s.%s: %w", table, column, err)
	}
	return count > 0, nil
}
