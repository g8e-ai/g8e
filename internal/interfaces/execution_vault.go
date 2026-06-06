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

import "github.com/g8e-ai/g8e/internal/models"

// ExecutionVault defines the interface for execution log and file diff storage.
// This service stores command execution results and file diffs with optional encryption.
type ExecutionVault interface {
	// StoreExecution stores a command execution result locally.
	// Content is encrypted at rest if an encryption vault is configured.
	StoreExecution(record *models.ExecutionRecord) error

	// GetExecution retrieves a stored execution by ID.
	// Returns (nil, nil) if not found.
	GetExecution(executionID string) (*models.ExecutionRecord, error)

	// StoreFileDiff stores a file diff in the execution vault.
	// Content is encrypted at rest if an encryption vault is configured.
	StoreFileDiff(record *models.FileDiffRecord) error

	// GetFileDiff retrieves a file diff by ID.
	// Returns (nil, nil) if not found.
	GetFileDiff(diffID string) (*models.FileDiffRecord, error)

	// GetFileDiffsBySession retrieves all file diffs for a session.
	GetFileDiffsBySession(operatorSessionID string, limit int) ([]*models.FileDiffRecord, error)

	// IsEnabled returns whether the execution vault is enabled.
	IsEnabled() bool

	// IsEncryptionEnabled returns whether content encryption is enabled.
	IsEncryptionEnabled() bool

	// Close shuts down the execution vault service.
	Close() error

	// Wait blocks until all background workers and writes have finished.
	Wait()
}
