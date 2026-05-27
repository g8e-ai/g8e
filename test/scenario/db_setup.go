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

package scenario

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/gateway"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/storage"
)

// SetupTestDB initializes an in-memory SQLite database for scenario tests.
// It applies the gateway schema and returns the database connection.
func SetupTestDB() (*sqliteutil.DB, error) {
	cfg := sqliteutil.DefaultDBConfig(":memory:")
	db, err := sqliteutil.OpenDB(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		return nil, fmt.Errorf("failed to open test database: %w", err)
	}

	// Apply the embedded gateway schema (single source of truth)
	_, err = db.ExecWithRetry(gateway.GatewaySchema())
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to apply gateway schema: %w", err)
	}

	return db, nil
}

// TeardownTestDB closes the database connection.
func TeardownTestDB(db *sqliteutil.DB) error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// QueryReceipt queries a receipt from the console_audit collection in the documents table.
// Returns nil if the receipt is not found.
func QueryReceipt(db *sqliteutil.DB, transactionID string) (*models.Document, error) {
	var dataJSON, createdAtStr, updatedAtStr string
	err := db.QueryRowWithRetry(
		"SELECT data, created_at, updated_at FROM documents WHERE collection = ? AND id = ?",
		"console_audit", transactionID,
	).Scan(&dataJSON, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query receipt: %w", err)
	}

	// Parse timestamps
	createdAt, err := sqliteutil.ParseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("invalid created_at timestamp: %w", err)
	}
	updatedAt, err := sqliteutil.ParseTimestamp(updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("invalid updated_at timestamp: %w", err)
	}

	// Parse JSON data into map[string]json.RawMessage to match models.Document
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal receipt data: %w", err)
	}
	if data == nil {
		data = make(map[string]json.RawMessage)
	}

	return &models.Document{
		Collection: "console_audit",
		ID:         transactionID,
		Data:       data,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

// QueryAuditVault queries a receipt from the audit vault receipts table.
// Returns nil if the receipt is not found.
func QueryAuditVault(vault *storage.AuditVaultService, transactionID string) (*models.ActionReceiptRecord, error) {
	if vault == nil {
		return nil, fmt.Errorf("audit vault is nil")
	}

	record, err := vault.GetActionReceipt(transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit vault: %w", err)
	}

	return record, nil
}

// CreateTestSession creates a test session in the audit vault for receipt recording.
// This is required because the receipts table has a foreign key constraint on sessions.
func CreateTestSession(vault *storage.AuditVaultService, sessionID, sessionType string) error {
	if vault == nil {
		return fmt.Errorf("audit vault is nil")
	}

	return vault.CreateSession(sessionID, sessionType, "test session", "test-user")
}
