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
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
)

func TestVerifyL2Governance(t *testing.T) {
	hmacKey := "test-hmac-key"
	eventType := "g8e.v1.operator.command.requested"
	payload := []byte(`{"command":"echo hello"}`)

	// Generate ED25519 keypair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	trustedKeys := map[string]ed25519.PublicKey{
		"agent-1": pub,
	}

	t.Run("HMAC - Success", func(t *testing.T) {
		sig := computeL2HMAC(eventType, payload, hmacKey)
		env := &commonv1.UniversalEnvelope{
			EventType: eventType,
			Payload:   payload,
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					TribunalSignature: sig,
				},
			},
		}

		err := VerifyL2Governance(env, hmacKey, nil)
		assert.NoError(t, err)
	})

	t.Run("HMAC - Failure (Wrong Key)", func(t *testing.T) {
		sig := computeL2HMAC(eventType, payload, "wrong-key")
		env := &commonv1.UniversalEnvelope{
			EventType: eventType,
			Payload:   payload,
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					TribunalSignature: sig,
				},
			},
		}

		err := VerifyL2Governance(env, hmacKey, nil)
		assert.ErrorIs(t, err, ErrL2SignatureInvalid)
	})

	t.Run("ED25519 - Success with KeyId", func(t *testing.T) {
		signingPayload := computeL2SigningPayload(eventType, payload)
		sigBytes := ed25519.Sign(priv, signingPayload)
		sig := hex.EncodeToString(sigBytes)

		env := &commonv1.UniversalEnvelope{
			EventType: eventType,
			Payload:   payload,
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					TribunalSignature: sig,
					KeyId:             "agent-1",
				},
			},
		}

		err := VerifyL2Governance(env, hmacKey, trustedKeys)
		assert.NoError(t, err)
	})

	t.Run("ED25519 - Failure (Wrong KeyId)", func(t *testing.T) {
		signingPayload := computeL2SigningPayload(eventType, payload)
		sigBytes := ed25519.Sign(priv, signingPayload)
		sig := hex.EncodeToString(sigBytes)

		env := &commonv1.UniversalEnvelope{
			EventType: eventType,
			Payload:   payload,
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					TribunalSignature: sig,
					KeyId:             "non-existent-agent",
				},
			},
		}

		// Since KeyId is provided but not found, it should fall back to legacy loop
		// which also fails because the signature is for agent-1's key.
		err := VerifyL2Governance(env, hmacKey, trustedKeys)
		assert.ErrorIs(t, err, ErrL2AsymmetricInvalid)
	})

	t.Run("ED25519 - Success (Legacy Fallback - No KeyId)", func(t *testing.T) {
		signingPayload := computeL2SigningPayload(eventType, payload)
		sigBytes := ed25519.Sign(priv, signingPayload)
		sig := hex.EncodeToString(sigBytes)

		env := &commonv1.UniversalEnvelope{
			EventType: eventType,
			Payload:   payload,
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					TribunalSignature: sig,
				},
			},
		}

		err := VerifyL2Governance(env, hmacKey, trustedKeys)
		assert.NoError(t, err)
	})

	t.Run("ED25519 - Failure (Wrong Key)", func(t *testing.T) {
		_, priv2, _ := ed25519.GenerateKey(rand.Reader)
		signingPayload := computeL2SigningPayload(eventType, payload)
		sigBytes := ed25519.Sign(priv2, signingPayload)
		sig := hex.EncodeToString(sigBytes)

		env := &commonv1.UniversalEnvelope{
			EventType: eventType,
			Payload:   payload,
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					TribunalSignature: sig,
				},
			},
		}

		err := VerifyL2Governance(env, hmacKey, trustedKeys)
		assert.ErrorIs(t, err, ErrL2AsymmetricInvalid)
	})

	t.Run("Missing Governance", func(t *testing.T) {
		env := &commonv1.UniversalEnvelope{
			EventType: eventType,
			Payload:   payload,
		}

		err := VerifyL2Governance(env, hmacKey, trustedKeys)
		assert.ErrorIs(t, err, ErrL2SignatureMissing)
	})

	t.Run("Missing L2", func(t *testing.T) {
		env := &commonv1.UniversalEnvelope{
			EventType:  eventType,
			Payload:    payload,
			Governance: &commonv1.GovernanceMetadata{},
		}

		err := VerifyL2Governance(env, hmacKey, trustedKeys)
		assert.ErrorIs(t, err, ErrL2SignatureMissing)
	})

	t.Run("Empty Signature", func(t *testing.T) {
		env := &commonv1.UniversalEnvelope{
			EventType: eventType,
			Payload:   payload,
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					TribunalSignature: "",
				},
			},
		}

		err := VerifyL2Governance(env, hmacKey, trustedKeys)
		assert.ErrorIs(t, err, ErrL2SignatureMissing)
	})
}
