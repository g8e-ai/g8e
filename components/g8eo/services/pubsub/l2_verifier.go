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

package pubsub

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
)

// L2 Tribunal verification errors. Each error is distinct so dispatcher
// logs can distinguish between "no signature present" and "signature
// present but invalid" — operationally important during rollout.
var (
	ErrL2KeyNotConfigured  = errors.New("L2: auditor HMAC key not configured")
	ErrL2SignatureMissing  = errors.New("L2: tribunal_signature missing from envelope")
	ErrL2SignatureInvalid  = errors.New("L2: tribunal_signature does not match computed HMAC")
	ErrL2AsymmetricInvalid = errors.New("L2: tribunal_signature failed ED25519 verification")
	ErrL3SignatureMissing  = errors.New("L3: human_signature missing from envelope")
	ErrL3SignatureInvalid  = errors.New("L3: human_signature failed verification")
	ErrTransactionExpired  = errors.New("Protocol: transaction has expired")
	ErrTransactionReplay   = errors.New("Protocol: transaction replay detected")
)

// computeL2HMAC produces the canonical HMAC-SHA256 over
// (event_type || '\n' || payload_bytes).
func computeL2HMAC(eventType string, payload []byte, stringKey string) string {
	key := []byte(stringKey)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(eventType))
	mac.Write([]byte{'\n'})
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// computeL2SigningPayload produces the canonical bytes for asymmetric signing.
// The payload is structured as:
// ID | Timestamp | EventType | OperatorID | SessionID | StateRoot | Expiry | Nonce | Payload
func computeL2SigningPayload(env *commonv1.GovernanceEnvelope) []byte {
	var b []byte
	b = append(b, []byte(env.Id)...)
	b = append(b, '|')
	if env.Timestamp != nil {
		b = append(b, []byte(fmt.Sprintf("%d", env.Timestamp.Seconds))...)
	}
	b = append(b, '|')
	b = append(b, []byte(env.EventType)...)
	b = append(b, '|')
	b = append(b, []byte(env.OperatorId)...)
	b = append(b, '|')
	b = append(b, []byte(env.OperatorSessionId)...)
	b = append(b, '|')
	b = append(b, []byte(env.StateMerkleRoot)...)
	b = append(b, '|')
	if env.ExpiresAt != nil {
		b = append(b, []byte(fmt.Sprintf("%d", env.ExpiresAt.Seconds))...)
	}
	b = append(b, '|')
	b = append(b, []byte(env.Nonce)...)
	b = append(b, '|')
	b = append(b, env.Payload...)
	return b
}

// VerifyL2Governance enforces the L2 Tribunal signature on an inbound
// GovernanceEnvelope. Returns nil when the signature is present and
// matches. The caller is expected to reject the command on any
// non-nil return.
func VerifyL2Governance(env *commonv1.GovernanceEnvelope, stringKey string, trustedKeys map[string]ed25519.PublicKey) error {
	if env == nil || env.Governance == nil || env.Governance.L2 == nil {
		return ErrL2SignatureMissing
	}
	got := env.Governance.L2.TribunalSignature
	if got == "" {
		return ErrL2SignatureMissing
	}

	// 1. Try Asymmetric (ED25519) if KeyId is present
	if len(trustedKeys) > 0 {
		keyID := env.Governance.L2.KeyId
		if keyID != "" {
			if pub, ok := trustedKeys[keyID]; ok {
				sigBytes, err := hex.DecodeString(got)
				if err != nil || len(sigBytes) != ed25519.SignatureSize {
					return ErrL2AsymmetricInvalid
				}

				signingPayload := computeL2SigningPayload(env)
				if ed25519.Verify(pub, signingPayload, sigBytes) {
					return nil
				}
				return ErrL2AsymmetricInvalid
			}
			// If KeyId was provided but not found in trustedKeys, we should fail fast
			// instead of falling back to the legacy loop.
			return ErrL2AsymmetricInvalid
		}

		// Fallback: If KeyId is missing but we have an ED25519-length signature,
		// try all trusted keys (legacy behavior for transition).
		if len(got) == 128 {
			sigBytes, err := hex.DecodeString(got)
			if err == nil && len(sigBytes) == ed25519.SignatureSize {
				signingPayload := computeL2SigningPayload(env)
				for _, pub := range trustedKeys {
					if ed25519.Verify(pub, signingPayload, sigBytes) {
						return nil
					}
				}
				return ErrL2AsymmetricInvalid
			}
		}
	}

	// 2. Fallback to Legacy HMAC
	if stringKey == "" {
		return ErrL2KeyNotConfigured
	}
	want := computeL2HMAC(env.EventType, env.Payload, stringKey)
	if !hmac.Equal([]byte(got), []byte(want)) {
		return fmt.Errorf("%w: event_type=%s", ErrL2SignatureInvalid, env.EventType)
	}
	return nil
}

// VerifyL3Governance enforces the L3 (Human Approval) proof on an inbound
// GovernanceEnvelope for mutation requests.
func VerifyL3Governance(env *commonv1.GovernanceEnvelope, trustedL3Keys map[string]ed25519.PublicKey) error {
	if env == nil || env.Governance == nil || env.Governance.L3 == nil {
		return ErrL3SignatureMissing
	}

	// If auto-approved, we still require local policy check (handled by caller/dispatcher)
	if env.Governance.L3.AutoApproved {
		return nil
	}

	got := env.Governance.L3.HumanSignature
	if got == "" {
		return ErrL3SignatureMissing
	}

	// Verification logic for L3 (hardware-bound signatures / passkeys)
	// For now, we expect an ED25519 signature if a public key is provided or trusted.
	pubKey := env.Governance.L3.PublicKey
	var pub ed25519.PublicKey

	if pubKey != "" {
		pubBytes, err := hex.DecodeString(pubKey)
		if err == nil && len(pubBytes) == ed25519.PublicKeySize {
			pub = ed25519.PublicKey(pubBytes)
		}
	}

	// Fallback to trusted keys if no public key provided in envelope
	if pub == nil && len(trustedL3Keys) > 0 {
		// In a real system, we'd lookup by web_session_id or operator_session_id
		// For Phase 3, we'll require the public key to be in the envelope or known.
	}

	if pub == nil {
		return ErrL3SignatureInvalid
	}

	sigBytes, err := hex.DecodeString(got)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return ErrL3SignatureInvalid
	}

	signingPayload := computeL2SigningPayload(env)
	if ed25519.Verify(pub, signingPayload, sigBytes) {
		return nil
	}

	return ErrL3SignatureInvalid
}
