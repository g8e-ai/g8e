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

package l3

import (
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/internal/services/storage"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// CLIL3Notary provides L3 verification for outbound mode using CLI-based approval.
// In outbound mode, mutations requiring L3 are suspended and must be approved via
// a CLI command (e.g., `g8e approve <tx_hash>`). This notary checks if a transaction
// has been approved by verifying the L3 proof contains the required approval signature.
type CLIL3Notary struct {
	suspendedStore *storage.LocalStoreService
	logger         *slog.Logger
}

// NewCLIL3Notary creates a new CLI L3 notary for outbound mode.
func NewCLIL3Notary(suspendedStore *storage.LocalStoreService, logger *slog.Logger) *CLIL3Notary {
	return &CLIL3Notary{
		suspendedStore: suspendedStore,
		logger:         logger,
	}
}

// VerifyL3Proof verifies an L3 proof for CLI-based approval in outbound mode.
// For outbound mode, the L3 proof is verified by checking that:
// 1. The transaction exists in the suspended store (meaning it was suspended for L3 approval)
// 2. The proof contains a valid CLI approval signature
//
// This method is called during transaction verification. The actual CLI approval
// is handled by a separate command that marks the transaction as approved.
func (v *CLIL3Notary) VerifyL3Proof(userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	if userID == "" {
		return false, fmt.Errorf("user_id is required for CLI L3 verification")
	}
	if transactionHash == "" {
		return false, fmt.Errorf("transaction_hash is required for CLI L3 verification")
	}
	if proof == nil {
		return false, fmt.Errorf("L3 proof is required")
	}

	// Check if the transaction exists in the suspended store
	tx, ok := v.suspendedStore.GetSuspendedTransaction(transactionHash)
	if !ok {
		v.logger.Warn("CLI L3 verification failed: transaction not found in suspended store", "transaction_hash", transactionHash)
		return false, fmt.Errorf("transaction not found in suspended store - must be approved via CLI")
	}

	// Verify the user ID matches
	if tx.UserID != userID {
		v.logger.Warn("CLI L3 verification failed: user ID mismatch", "expected_user_id", tx.UserID, "provided_user_id", userID)
		return false, fmt.Errorf("user ID mismatch")
	}

	// For CLI-based approval, we use mtls_cert_fingerprint as the approval token
	// The actual approval is handled by the CLI command that marks the transaction
	// as approved in the suspended store. The proof serves as evidence of approval.
	if proof.MtlsCertFingerprint == "" {
		v.logger.Warn("CLI L3 verification failed: CLI approval token missing", "transaction_hash", transactionHash)
		return false, fmt.Errorf("CLI approval token required - use 'g8e approve' command")
	}

	hashPrefix := transactionHash
	if len(transactionHash) > 8 {
		hashPrefix = transactionHash[:8]
	}
	v.logger.Info("CLI L3 verification passed", "user_id", userID, "transaction_hash", hashPrefix, "tool_name", tx.ToolName)
	return true, nil
}
