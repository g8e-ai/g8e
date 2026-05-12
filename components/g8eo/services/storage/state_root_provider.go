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
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/services/sqliteutil"
)

// StateRootProvider provides the current state merkle root for transaction verification.
type StateRootProvider struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewStateRootProvider creates a new state root provider.
func NewStateRootProvider(db *sql.DB, logger *slog.Logger) (*StateRootProvider, error) {
	srp := &StateRootProvider{
		db:     db,
		logger: logger,
	}

	if err := srp.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize state root schema: %w", err)
	}

	return srp, nil
}

// initSchema creates the state_root table if it doesn't exist.
func (srp *StateRootProvider) initSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS state_root (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		root TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	`

	_, err := srp.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create state_root table: %w", err)
	}

	// Initialize with a default root if not present
	var count int
	err = srp.db.QueryRow("SELECT COUNT(*) FROM state_root").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check state_root table: %w", err)
	}

	if count == 0 {
		// Insert initial state root
		_, err = srp.db.Exec(
			"INSERT INTO state_root (id, root, updated_at) VALUES (1, '', ?)",
			sqliteutil.FormatTimestamp(time.Now().UTC()),
		)
		if err != nil {
			return fmt.Errorf("failed to initialize state root: %w", err)
		}
	}

	return nil
}

// GetCurrentStateRoot returns the current state merkle root.
// Returns empty string if state root verification is not configured.
func (srp *StateRootProvider) GetCurrentStateRoot() (string, error) {
	var root string
	err := srp.db.QueryRow("SELECT root FROM state_root WHERE id = 1").Scan(&root)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("failed to get current state root: %w", err)
	}
	return root, nil
}

// SetCurrentStateRoot updates the current state merkle root.
func (srp *StateRootProvider) SetCurrentStateRoot(root string) error {
	_, err := srp.db.Exec(
		"UPDATE state_root SET root = ?, updated_at = ? WHERE id = 1",
		root,
		sqliteutil.FormatTimestamp(time.Now().UTC()),
	)
	if err != nil {
		return fmt.Errorf("failed to update state root: %w", err)
	}
	return nil
}
