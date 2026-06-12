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

package interfaces

import (
	"context"

	"github.com/g8e-ai/g8e/internal/models"
)

// ExecutionVault defines the interface for execution log and file diff storage.
// This service stores command execution results and file diffs with optional encryption.
//
// All methods that return errors must wrap errors with context using
// fmt.Errorf("execution_vault: action: %w", err) to provide clear error attribution.
type ExecutionVault interface {
	// StoreExecution stores a command execution result locally.
	// Content is encrypted at rest if an encryption vault is configured.
	// Returns an error if storage fails, wrapping the underlying error with context.
	StoreExecution(ctx context.Context, record *models.ExecutionRecord) error

	// GetExecution retrieves a stored execution by ID.
	// Returns (nil, nil) if not found.
	// Returns an error if retrieval fails, wrapping the underlying error with context.
	GetExecution(ctx context.Context, executionID string) (*models.ExecutionRecord, error)

	// StoreFileDiff stores a file diff in the execution vault.
	// Content is encrypted at rest if an encryption vault is configured.
	// Returns an error if storage fails, wrapping the underlying error with context.
	StoreFileDiff(ctx context.Context, record *models.FileDiffRecord) error

	// GetFileDiff retrieves a file diff by ID.
	// Returns (nil, nil) if not found.
	// Returns an error if retrieval fails, wrapping the underlying error with context.
	GetFileDiff(ctx context.Context, diffID string) (*models.FileDiffRecord, error)

	// GetFileDiffsBySession retrieves all file diffs for a session.
	// Returns an error if retrieval fails, wrapping the underlying error with context.
	GetFileDiffsBySession(ctx context.Context, operatorSessionID string, limit int) ([]*models.FileDiffRecord, error)

	// Close shuts down the execution vault service.
	// Returns an error if shutdown fails, wrapping the underlying error with context.
	Close() error

	// Wait blocks until all background workers and writes have finished.
	Wait()
}
