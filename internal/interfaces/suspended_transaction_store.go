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

// SuspendedTransactionStore defines the interface for L3 approval workflow storage.
// This service stores transactions awaiting human approval.
type SuspendedTransactionStore interface {
	// StoreSuspendedTransaction stores a transaction awaiting L3 approval.
	StoreSuspendedTransaction(tx *models.SuspendedTransaction) error

	// GetSuspendedTransaction retrieves a suspended transaction by hash.
	// Returns (nil, false) if not found or expired.
	GetSuspendedTransaction(txHash string) (*models.SuspendedTransaction, bool)

	// ListSuspendedTransactions retrieves all non-expired suspended transactions.
	// Optionally filters by user_id if provided.
	ListSuspendedTransactions(userID string) ([]*models.SuspendedTransaction, error)

	// ApproveSuspendedTransaction marks a suspended transaction as approved with cryptographic signature.
	ApproveSuspendedTransaction(txHash, approvedBy, approvalSignature, expectedCertFingerprint string) error

	// DeleteSuspendedTransaction removes a suspended transaction after approval/rejection.
	DeleteSuspendedTransaction(txHash string) error

	// CleanupExpiredSuspendedTransactions removes expired suspended transactions.
	// Returns the count of deleted transactions.
	CleanupExpiredSuspendedTransactions() (int64, error)
}
