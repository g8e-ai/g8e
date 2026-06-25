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

package governance

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

//go:generate mockery --name L3Notary --output ./mocks --dir .

// L3Notary provides L3 (Authorization) verification for human-in-the-loop approval.
// L3 is the final gate that requires human presence before mutations execute.
type L3Notary interface {
	// VerifyL3Proof verifies an L3 proof for a transaction.
	// Returns true if the proof is valid and the transaction should be allowed.
	VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error)
}

// ErrCLISessionDenied signals that the CLI session was denied (e.g., revoked certificate)
// rather than encountering a system error. VerifyL3Proof translates this into (false, nil).
var ErrCLISessionDenied = errors.New("CLI session denied")

// CLISessionVerifier performs CLI session-specific verification including user active
// status, session validity, and certificate revocation. Returns nil if verification passes.
// Returns ErrCLISessionDenied for denials (revoked certs, inactive sessions) and other
// errors for system failures.
type CLISessionVerifier interface {
	VerifyCLISession(userID, cliSessionID, certFingerprint string) error
}

// outboundL3Notary provides L3 verification for CLI-based approval. It supports three modes:
// - Outbound mode: suspended transaction + signature verification only
// - Gateway CLI mode: additional CLI session, user active, and certificate revocation checks
// - Gateway passkey mode: WebAuthn passkey verification for web sessions
//
// When a passkeyVerifier is configured, VerifyL3Proof dispatches based on proof type:
// proofs with mtls_cert_fingerprint use the CLI path; all others use the passkey verifier.
type outboundL3Notary struct {
	suspendedStore storage.SuspendedTransactionStore
	cliVerifier    CLISessionVerifier
	passkeyVerifier L3Notary
	logger         *slog.Logger
}

// NewOutboundL3Notary creates a new L3 notary for outbound mode (no CLI session verification).
func NewOutboundL3Notary(suspendedStore storage.SuspendedTransactionStore, logger *slog.Logger) L3Notary {
	return &outboundL3Notary{
		suspendedStore: suspendedStore,
		logger:         logger,
	}
}

// NewCLIL3Notary creates a new L3 notary with CLI session verification for gateway mode.
// The cliVerifier performs user active, CLI session, and certificate revocation checks
// before the shared suspended transaction and signature verification.
func NewCLIL3Notary(suspendedStore storage.SuspendedTransactionStore, cliVerifier CLISessionVerifier, logger *slog.Logger) L3Notary {
	return &outboundL3Notary{
		suspendedStore: suspendedStore,
		cliVerifier:    cliVerifier,
		logger:         logger,
	}
}

// NewGatewayL3Notary creates a unified L3 notary that handles both CLI (mTLS) and
// passkey (WebAuthn) proofs. Proofs with mtls_cert_fingerprint use the CLI verification
// path; all others delegate to the passkey verifier.
func NewGatewayL3Notary(suspendedStore storage.SuspendedTransactionStore, cliVerifier CLISessionVerifier, passkeyVerifier L3Notary, logger *slog.Logger) L3Notary {
	return &outboundL3Notary{
		suspendedStore:  suspendedStore,
		cliVerifier:     cliVerifier,
		passkeyVerifier: passkeyVerifier,
		logger:          logger,
	}
}

// VerifyL3Proof verifies an L3 proof for CLI-based approval.
// The L3 proof is verified by checking that:
// 1. (Gateway mode) The user is active, CLI session is valid, and certificate is not revoked
// 2. The transaction exists in the suspended store and is marked as approved
// 3. The proof contains a valid Ed25519 signature over the transaction hash
// 4. The signature was created by the expected certificate (fingerprint match)
// 5. The approval has not expired (30 minute window)
//
// In outbound mode (no cliVerifier), only checks 2-5 are performed.
// In gateway mode (with cliVerifier), all checks are performed.
func (v *outboundL3Notary) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	if proof == nil {
		return false, constants.ErrGatewayL3ProofRequired
	}

	// Dispatch to passkey verifier for WebAuthn proofs (no mtls_cert_fingerprint)
	if v.passkeyVerifier != nil && proof.MtlsCertFingerprint == "" {
		return v.passkeyVerifier.VerifyL3Proof(ctx, userID, transactionHash, cliSessionID, proof)
	}

	// CLI path: validate required inputs
	if userID == "" {
		return false, constants.ErrUserIDRequired
	}
	if transactionHash == "" {
		return false, constants.ErrCLIL3TransactionHashRequired
	}

	// Gateway mode: perform CLI session, user active, and cert revocation checks
	if v.cliVerifier != nil {
		if proof.MtlsCertFingerprint == "" {
			return false, constants.ErrCLIL3CertFingerprintRequired
		}
		if proof.CliSignature == "" {
			return false, constants.ErrCLIL3SignatureRequired
		}
		if _, err := hex.DecodeString(proof.MtlsCertFingerprint); err != nil {
			return false, fmt.Errorf("%w: %w", constants.ErrCLIL3InvalidFingerprintFormat, err)
		}
		if err := v.cliVerifier.VerifyCLISession(userID, cliSessionID, proof.MtlsCertFingerprint); err != nil {
			if errors.Is(err, ErrCLISessionDenied) {
				return false, nil
			}
			return false, err
		}
	}

	// Load the suspended transaction
	if v.suspendedStore == nil {
		return false, constants.ErrCLIL3SuspendedStoreNotConfigured
	}
	tx, ok, err := v.suspendedStore.GetSuspendedTransaction(ctx, transactionHash)
	if err != nil {
		v.logger.Warn("CLI L3 verification failed: error getting suspended transaction", "transaction_hash", transactionHash, "error", err)
		return false, fmt.Errorf("%w: %w", constants.ErrCLIL3GetSuspendedTransactionFailed, err)
	}
	if !ok {
		v.logger.Warn("CLI L3 verification failed: transaction not found in suspended store", "transaction_hash", transactionHash)
		return false, constants.ErrNotFound
	}

	// Verify the user ID matches
	if tx.UserID != userID {
		v.logger.Warn("CLI L3 verification failed: user ID mismatch", "expected_user_id", tx.UserID, "provided_user_id", userID)
		return false, constants.ErrCLIL3SessionUserMismatch
	}

	// Require explicit approval decision
	if !tx.Approved {
		v.logger.Warn("CLI L3 verification failed: transaction not approved", "transaction_hash", transactionHash)
		return false, constants.ErrCLIL3TransactionNotApproved
	}

	// Verify approval has not expired (30 minute approval window)
	if tx.ApprovedAt != nil {
		approvalExpiry := tx.ApprovedAt.Add(30 * time.Minute)
		if time.Now().UTC().After(approvalExpiry) {
			v.logger.Warn("CLI L3 verification failed: approval expired", "transaction_hash", transactionHash, "approved_at", tx.ApprovedAt)
			return false, constants.ErrCLIL3ApprovalExpired
		}
	}

	// Require cryptographic signature over transaction hash
	if proof.CliSignature == "" {
		v.logger.Warn("CLI L3 verification failed: CLI signature missing", "transaction_hash", transactionHash)
		return false, constants.ErrCLIL3SignatureRequired
	}

	// Verify signature format (hex-encoded Ed25519 signature)
	sigBytes, err := hex.DecodeString(proof.CliSignature)
	if err != nil {
		v.logger.Warn("CLI L3 verification failed: invalid signature encoding", "transaction_hash", transactionHash, "error", err)
		return false, fmt.Errorf("%w: %w", constants.ErrCLIL3SignatureEncodingFailed, err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		v.logger.Warn("CLI L3 verification failed: invalid signature length", "transaction_hash", transactionHash, "length", len(sigBytes))
		return false, constants.ErrCLIL3SignatureEncodingFailed
	}

	// Verify the certificate fingerprint matches the expected fingerprint (constant-time)
	if tx.ExpectedCertFingerprint != "" {
		if subtle.ConstantTimeCompare([]byte(tx.ExpectedCertFingerprint), []byte(proof.MtlsCertFingerprint)) != 1 {
			v.logger.Warn("CLI L3 verification failed: certificate fingerprint mismatch",
				"transaction_hash", transactionHash,
				"expected", tx.ExpectedCertFingerprint,
				"provided", proof.MtlsCertFingerprint)
			return false, constants.ErrCLIL3FingerprintMismatch
		}
	}

	// Cryptographic verification: verify the signature against the stored public key
	if tx.ApprovalPublicKey == "" {
		v.logger.Warn("CLI L3 verification failed: approval public key missing", "transaction_hash", transactionHash)
		return false, constants.ErrCLIL3PublicKeyMissing
	}

	pubKeyBytes, err := hex.DecodeString(tx.ApprovalPublicKey)
	if err != nil {
		v.logger.Warn("CLI L3 verification failed: invalid public key encoding", "transaction_hash", transactionHash, "error", err)
		return false, fmt.Errorf("%w: %w", constants.ErrCLIL3PublicKeyInvalid, err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		v.logger.Warn("CLI L3 verification failed: invalid public key size", "transaction_hash", transactionHash, "length", len(pubKeyBytes))
		return false, constants.ErrCLIL3PublicKeyInvalid
	}

	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), []byte(transactionHash), sigBytes) {
		v.logger.Warn("CLI L3 verification failed: cryptographic signature verification failed", "transaction_hash", transactionHash)
		return false, constants.ErrCLIL3SignatureVerificationFailed
	}

	hashPrefix := transactionHash
	if len(transactionHash) > 8 {
		hashPrefix = transactionHash[:8]
	}
	v.logger.Info("CLI L3 verification passed",
		"user_id", userID,
		"transaction_hash", hashPrefix,
		"tool_name", tx.ToolName,
		"approved_by", tx.ApprovedBy,
		"approved_at", tx.ApprovedAt)
	return true, nil
}
