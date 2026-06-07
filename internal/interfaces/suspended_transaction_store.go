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

// SuspendedTransactionStore defines the interface for L3 approval workflow storage.
// This service stores transactions awaiting human approval.
//
// All methods that return errors must wrap errors with context using
// fmt.Errorf("suspended_transaction_store: action: %w", err) to provide clear error attribution.
type SuspendedTransactionStore interface {
	// StoreSuspendedTransaction stores a transaction awaiting L3 approval.
	// Returns an error if storage fails, wrapping the underlying error with context.
	StoreSuspendedTransaction(ctx context.Context, tx *models.SuspendedTransaction) error

	// GetSuspendedTransaction retrieves a suspended transaction by hash.
	// Returns (nil, false) if not found or expired.
	// Returns an error if retrieval fails, wrapping the underlying error with context.
	GetSuspendedTransaction(ctx context.Context, txHash string) (*models.SuspendedTransaction, bool, error)

	// ListSuspendedTransactions retrieves all non-expired suspended transactions.
	// Optionally filters by user_id if provided.
	// Returns an error if retrieval fails, wrapping the underlying error with context.
	ListSuspendedTransactions(ctx context.Context, userID string) ([]*models.SuspendedTransaction, error)

	// ApproveSuspendedTransaction marks a suspended transaction as approved with cryptographic signature.
	// Returns an error if approval fails, wrapping the underlying error with context.
	ApproveSuspendedTransaction(ctx context.Context, txHash, approvedBy, approvalSignature, expectedCertFingerprint string) error

	// DeleteSuspendedTransaction removes a suspended transaction after approval/rejection.
	// Returns an error if deletion fails, wrapping the underlying error with context.
	DeleteSuspendedTransaction(ctx context.Context, txHash string) error

	// CleanupExpiredSuspendedTransactions removes expired suspended transactions.
	// Returns the count of deleted transactions.
	// Returns an error if cleanup fails, wrapping the underlying error with context.
	CleanupExpiredSuspendedTransactions(ctx context.Context) (int64, error)
}
