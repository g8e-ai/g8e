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

package gateway

import (
	"fmt"
	"log/slog"

	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
)

// CompositeL3Verifier provides L3Notary verification for both web sessions (WebAuthn)
// and CLI sessions (mTLS certificates). It delegates to the appropriate notary
// based on the proof type.
type CompositeL3Verifier struct {
	passkeyL3 *PasskeyService
	cliL3     *CLIL3Notary
	logger    *slog.Logger
}

// NewCompositeL3Verifier creates a new composite L3Notary verifier.
func NewCompositeL3Verifier(passkeyL3 *PasskeyService, cliL3 *CLIL3Notary, logger *slog.Logger) *CompositeL3Verifier {
	return &CompositeL3Verifier{
		passkeyL3: passkeyL3,
		cliL3:     cliL3,
		logger:    logger,
	}
}

// VerifyL3Proof verifies an L3Notary proof, delegating to the appropriate verifier
// based on the proof type.
// - If the proof contains mtls_cert_fingerprint, it uses the CLI mTLS verifier
// - Otherwise, it uses the WebAuthn passkey verifier
func (v *CompositeL3Verifier) VerifyL3Proof(userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	if proof == nil {
		return false, fmt.Errorf("L3Notary proof is required")
	}

	// Check if this is a CLI mTLS proof
	if proof.MtlsCertFingerprint != "" {
		if v.cliL3 == nil {
			return false, fmt.Errorf("CLI L3Notary verifier not configured")
		}
		hashPrefix := transactionHash
		if len(transactionHash) > 8 {
			hashPrefix = transactionHash[:8]
		}
		v.logger.Debug("Delegating to CLI L3Notary verifier", "user_id", userID, "transaction_hash", hashPrefix, "cli_session_id", cliSessionID)
		return v.cliL3.VerifyL3Proof(userID, transactionHash, cliSessionID, proof)
	}

	// Otherwise, use WebAuthn passkey verifier (web sessions don't use cli_session_id)
	if v.passkeyL3 == nil {
		return false, fmt.Errorf("Passkey L3Notary verifier not configured")
	}
	hashPrefix := transactionHash
	if len(transactionHash) > 8 {
		hashPrefix = transactionHash[:8]
	}
	v.logger.Debug("Delegating to Passkey L3Notary verifier", "user_id", userID, "transaction_hash", hashPrefix)
	return v.passkeyL3.VerifyL3Proof(userID, transactionHash, cliSessionID, proof)
}
