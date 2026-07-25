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

// gatewayNotary provides L3 verification for gateway mode. It requires passkey
// authorization for all proofs (browser and CLI). CLI callers additionally present
// mTLS fields for transport-layer authentication.
// This is a layered model: passkey = authorization (human presence), mTLS = transport auth.
type gatewayNotary struct {
	cliVerifier     CLISessionVerifier
	passkeyVerifier L3Notary
	logger          *slog.Logger
}

// outboundNotary provides L3 verification for outbound mode (no CLI session
// verification, no passkey). It performs suspended transaction lookup and
// Ed25519 signature verification only.
type outboundNotary struct {
	suspendedStore storage.SuspendedTransactionStore
	logger         *slog.Logger
}

// cliNotary provides L3 verification for gateway CLI mode. It performs CLI
// session verification (user active, session validity, cert revocation) before
// the shared suspended transaction and signature verification.
type cliNotary struct {
	suspendedStore storage.SuspendedTransactionStore
	cliVerifier    CLISessionVerifier
	logger         *slog.Logger
}

// NewOutboundL3Notary creates a new L3 notary for outbound mode (no CLI session verification).
func NewOutboundL3Notary(suspendedStore storage.SuspendedTransactionStore, logger *slog.Logger) L3Notary {
	return &outboundNotary{
		suspendedStore: suspendedStore,
		logger:         logger,
	}
}

// NewCLIL3Notary creates a new L3 notary with CLI session verification for gateway mode.
// The cliVerifier performs user active, CLI session, and certificate revocation checks
// before the shared suspended transaction and signature verification.
func NewCLIL3Notary(suspendedStore storage.SuspendedTransactionStore, cliVerifier CLISessionVerifier, logger *slog.Logger) L3Notary {
	return &cliNotary{
		suspendedStore: suspendedStore,
		cliVerifier:    cliVerifier,
		logger:         logger,
	}
}

// NewGatewayL3Notary creates a unified L3 notary that requires passkey authorization
// for all proofs (browser and CLI). CLI callers additionally present mTLS fields for
// transport-layer authentication.
func NewGatewayL3Notary(cliVerifier CLISessionVerifier, passkeyVerifier L3Notary, logger *slog.Logger) L3Notary {
	return &gatewayNotary{
		cliVerifier:     cliVerifier,
		passkeyVerifier: passkeyVerifier,
		logger:          logger,
	}
}

// demoL3Notary provides L3 verification for demo environments where WebAuthn
// passkey enrollment is not available (e.g., headless Docker containers).
// It accepts any non-nil proof, allowing the harness mock L3 mode (principal
// Ed25519 signature) to satisfy notary posture without a browser.
// This must NEVER be used in production — it is gated by the G8E_L3_MOCK env var.
type demoL3Notary struct {
	logger *slog.Logger
}

// NewDemoL3Notary creates an L3 notary that auto-approves any non-nil proof.
// For demo/test environments only — never use in production.
func NewDemoL3Notary(logger *slog.Logger) L3Notary {
	return &demoL3Notary{logger: logger}
}

func (d *demoL3Notary) VerifyL3Proof(_ context.Context, userID, transactionHash, _ string, proof *commonv1.L3Proof) (bool, error) {
	if proof == nil {
		return false, constants.ErrGatewayL3ProofRequired
	}
	hashPrefix := transactionHash
	if len(hashPrefix) > 8 {
		hashPrefix = hashPrefix[:8]
	}
	d.logger.Info("L3 demo mode: auto-approving proof", "user_id", userID, "transaction_hash", hashPrefix)
	return true, nil
}

// VerifyL3Proof verifies an L3 proof in gateway mode.
//
//  1. Passkey authorization is required — proofs without a credential_id are rejected
//     with ErrPasskeyProofRequired. The passkey verifier validates the WebAuthn assertion.
//  2. CLI mTLS session authentication — if the proof includes mtls_cert_fingerprint
//     (CLI caller), the CLI session is verified as an additional transport-auth layer.
func (v *gatewayNotary) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	if proof == nil {
		return false, constants.ErrGatewayL3ProofRequired
	}

	if proof.CredentialId == "" {
		return false, constants.ErrPasskeyProofRequired
	}
	ok, err := v.passkeyVerifier.VerifyL3Proof(ctx, userID, transactionHash, cliSessionID, proof)
	if err != nil || !ok {
		return false, err
	}

	// Layer 2: CLI mTLS session authentication (additional check for CLI callers)
	if proof.MtlsCertFingerprint != "" && v.cliVerifier != nil {
		if err := v.cliVerifier.VerifyCLISession(userID, cliSessionID, proof.MtlsCertFingerprint); err != nil {
			if errors.Is(err, ErrCLISessionDenied) {
				return false, nil
			}
			return false, err
		}
	}

	return true, nil
}

// VerifyL3Proof verifies an L3 proof in outbound mode (suspended transaction + signature only).
func (v *outboundNotary) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return verifyOutboundProof(ctx, v.suspendedStore, nil, v.logger, userID, transactionHash, cliSessionID, proof)
}

// VerifyL3Proof verifies an L3 proof in CLI mode (CLI session check + suspended transaction + signature).
func (v *cliNotary) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return verifyOutboundProof(ctx, v.suspendedStore, v.cliVerifier, v.logger, userID, transactionHash, cliSessionID, proof)
}

// verifyOutboundProof performs the shared outbound verification logic used by both
// outboundNotary and cliNotary. If cliVerifier is non-nil, CLI session checks are
// performed before the suspended transaction and signature verification.
//
// 1. The transaction exists in the suspended store and is marked as approved
// 2. The proof contains a valid Ed25519 signature over the transaction hash
// 3. The signature was created by the expected certificate (fingerprint match)
// 4. The approval has not expired (30 minute window)
func verifyOutboundProof(
	ctx context.Context,
	suspendedStore storage.SuspendedTransactionStore,
	cliVerifier CLISessionVerifier,
	logger *slog.Logger,
	userID, transactionHash, cliSessionID string,
	proof *commonv1.L3Proof,
) (bool, error) {
	if proof == nil {
		return false, constants.ErrGatewayL3ProofRequired
	}

	// Validate required inputs
	if userID == "" {
		return false, constants.ErrUserIDRequired
	}
	if transactionHash == "" {
		return false, constants.ErrCLIL3TransactionHashRequired
	}

	// CLI mode: perform CLI session, user active, and cert revocation checks
	if cliVerifier != nil {
		if proof.MtlsCertFingerprint == "" {
			return false, constants.ErrCLIL3CertFingerprintRequired
		}
		if proof.CliSignature == "" {
			return false, constants.ErrCLIL3SignatureRequired
		}
		if _, err := hex.DecodeString(proof.MtlsCertFingerprint); err != nil {
			return false, fmt.Errorf("%w: %w", constants.ErrCLIL3InvalidFingerprintFormat, err)
		}
		if err := cliVerifier.VerifyCLISession(userID, cliSessionID, proof.MtlsCertFingerprint); err != nil {
			if errors.Is(err, ErrCLISessionDenied) {
				return false, nil
			}
			return false, err
		}
	}

	// Load the suspended transaction
	if suspendedStore == nil {
		return false, constants.ErrCLIL3SuspendedStoreNotConfigured
	}
	tx, ok, err := suspendedStore.GetSuspendedTransaction(ctx, transactionHash)
	if err != nil {
		logger.Warn("CLI L3 verification failed: error getting suspended transaction", "transaction_hash", transactionHash, "error", err)
		return false, fmt.Errorf("%w: %w", constants.ErrCLIL3GetSuspendedTransactionFailed, err)
	}
	if !ok {
		logger.Warn("CLI L3 verification failed: transaction not found in suspended store", "transaction_hash", transactionHash)
		return false, constants.ErrNotFound
	}

	// Verify the user ID matches
	if tx.UserID != userID {
		logger.Warn("CLI L3 verification failed: user ID mismatch", "expected_user_id", tx.UserID, "provided_user_id", userID)
		return false, constants.ErrCLIL3SessionUserMismatch
	}

	// Require explicit approval decision
	if !tx.Approved {
		logger.Warn("CLI L3 verification failed: transaction not approved", "transaction_hash", transactionHash)
		return false, constants.ErrCLIL3TransactionNotApproved
	}

	// Verify approval has not expired (30 minute approval window)
	if tx.ApprovedAt != nil {
		approvalExpiry := tx.ApprovedAt.Add(30 * time.Minute)
		if time.Now().UTC().After(approvalExpiry) {
			logger.Warn("CLI L3 verification failed: approval expired", "transaction_hash", transactionHash, "approved_at", tx.ApprovedAt)
			return false, constants.ErrCLIL3ApprovalExpired
		}
	}

	// Require cryptographic signature over transaction hash
	if proof.CliSignature == "" {
		logger.Warn("CLI L3 verification failed: CLI signature missing", "transaction_hash", transactionHash)
		return false, constants.ErrCLIL3SignatureRequired
	}

	// Verify signature format (hex-encoded Ed25519 signature)
	sigBytes, err := hex.DecodeString(proof.CliSignature)
	if err != nil {
		logger.Warn("CLI L3 verification failed: invalid signature encoding", "transaction_hash", transactionHash, "error", err)
		return false, fmt.Errorf("%w: %w", constants.ErrCLIL3SignatureEncodingFailed, err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		logger.Warn("CLI L3 verification failed: invalid signature length", "transaction_hash", transactionHash, "length", len(sigBytes))
		return false, constants.ErrCLIL3SignatureEncodingFailed
	}

	// Verify the certificate fingerprint matches the expected fingerprint (constant-time)
	if tx.ExpectedCertFingerprint != "" {
		if subtle.ConstantTimeCompare([]byte(tx.ExpectedCertFingerprint), []byte(proof.MtlsCertFingerprint)) != 1 {
			logger.Warn("CLI L3 verification failed: certificate fingerprint mismatch",
				"transaction_hash", transactionHash,
				"expected", tx.ExpectedCertFingerprint,
				"provided", proof.MtlsCertFingerprint)
			return false, constants.ErrCLIL3FingerprintMismatch
		}
	}

	// Cryptographic verification: verify the signature against the stored public key
	if tx.ApprovalPublicKey == "" {
		logger.Warn("CLI L3 verification failed: approval public key missing", "transaction_hash", transactionHash)
		return false, constants.ErrCLIL3PublicKeyMissing
	}

	pubKeyBytes, err := hex.DecodeString(tx.ApprovalPublicKey)
	if err != nil {
		logger.Warn("CLI L3 verification failed: invalid public key encoding", "transaction_hash", transactionHash, "error", err)
		return false, fmt.Errorf("%w: %w", constants.ErrCLIL3PublicKeyInvalid, err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		logger.Warn("CLI L3 verification failed: invalid public key size", "transaction_hash", transactionHash, "length", len(pubKeyBytes))
		return false, constants.ErrCLIL3PublicKeyInvalid
	}

	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), []byte(transactionHash), sigBytes) {
		logger.Warn("CLI L3 verification failed: cryptographic signature verification failed", "transaction_hash", transactionHash)
		return false, constants.ErrCLIL3SignatureVerificationFailed
	}

	hashPrefix := transactionHash
	if len(transactionHash) > 8 {
		hashPrefix = transactionHash[:8]
	}
	logger.Info("CLI L3 verification passed",
		"user_id", userID,
		"transaction_hash", hashPrefix,
		"tool_name", tx.ToolName,
		"approved_by", tx.ApprovedBy,
		"approved_at", tx.ApprovedAt)
	return true, nil
}
