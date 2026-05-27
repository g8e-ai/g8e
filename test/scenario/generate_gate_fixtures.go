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

//go:build ignore

package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// Test placeholder constants for intentionally corrupted test fixtures
const (
	fixtureBadID               = "wrongidwrongidwrongidwrongidwrongidwrongidwrongidwrongidwrongid"
	fixtureBadSignature        = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	fixtureInvalidL3ClientData = "invalidclientdata"
	fixtureInvalidL3AuthData   = "invalidauthdata"
	fixtureInvalidL3Signature  = "invalidsignature"
)

func main() {
	// Use the same private key as the test fixtures
	privKeyHex := "c847d8625a1d1be737b8c86012ef1ceb7cfe1c2f5bed5115b90b490c55600502797c07dc7211981020b7fea8c31ed993d30576e0e14523a76678672a0d18b8cd"
	privKeyBytes, _ := hex.DecodeString(privKeyHex)
	privKey := ed25519.PrivateKey(privKeyBytes)
	pubKey := privKey.Public().(ed25519.PublicKey)
	keyID := hex.EncodeToString(pubKey)

	fixedTime := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	marshaler := &protojson.MarshalOptions{Indent: "  "}

	// Helper to create base envelope
	createBase := func(nonce string) *commonv1.GovernanceEnvelope {
		env := &commonv1.GovernanceEnvelope{
			ProtocolVersion:   "1.0",
			Timestamp:         timestamppb.New(fixedTime),
			ExpiresAt:         timestamppb.New(fixedTime.Add(time.Hour)),
			SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
			OperatorId:        "operator-1",
			OperatorSessionId: "operator-session-1",
			ActionType:        "EXECUTE_BASH",
			TargetResource:    "localhost",
			StateMerkleRoot:   "abc123def456",
			Nonce:             nonce,
		}
		cmdPayload := &operatorv1.CommandRequested{
			Command:        "ls -la",
			ExecutionId:    "exec-123",
			Justification:  "test command",
			SentinelMode:   "strict",
			TimeoutSeconds: 30,
		}
		payloadBytes, _ := proto.Marshal(cmdPayload)
		env.Payload = payloadBytes
		return env
	}

	// Helper to sign and hash
	signAndHash := func(env *commonv1.GovernanceEnvelope) {
		hash, err := governance.GenerateMessageID(env)
		if err != nil {
			panic(err)
		}
		env.Id = hash
		env.TransactionHash = hash
		env.Governance = &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				KeyId:              keyID,
				ConsensusSignature: hex.EncodeToString(ed25519.Sign(privKey, []byte(hash+"|true"))),
			},
		}
	}

	// 1. all_valid - valid envelope with L3
	allValid := createBase("nonce-all-valid-123")
	signAndHash(allValid)
	allValid.Governance.L3 = &commonv1.L3Metadata{
		Proof: &commonv1.L3Proof{
			AuthenticatorData: "mockauthdata",
			ClientDataJson:    "mockclientdata",
			Signature:         "mocksignature",
		},
	}
	allValidJSON, _ := marshaler.Marshal(allValid)
	fmt.Println("ALL_VALID_INTENT:")
	fmt.Println(string(allValidJSON))
	fmt.Println()

	// 2. bad_integrity - id != computed hash
	badIntegrity := createBase("nonce-bad-integrity-123")
	signAndHash(badIntegrity)
	badIntegrity.Id = fixtureBadID
	badIntegrityJSON, _ := marshaler.Marshal(badIntegrity)
	fmt.Println("BAD_INTEGRITY_INTENT:")
	fmt.Println(string(badIntegrityJSON))
	fmt.Println()

	// 3. l2_missing - no L2 signature
	l2Missing := createBase("nonce-l2-missing-123")
	hash, _ := governance.GenerateMessageID(l2Missing)
	l2Missing.Id = hash
	l2Missing.TransactionHash = hash
	l2Missing.Governance = &commonv1.GovernanceMetadata{}
	l2MissingJSON, _ := marshaler.Marshal(l2Missing)
	fmt.Println("L2_MISSING_INTENT:")
	fmt.Println(string(l2MissingJSON))
	fmt.Println()

	// 4. l2_invalid - forged L2 signature
	l2Invalid := createBase("nonce-l2-invalid-123")
	signAndHash(l2Invalid)
	l2Invalid.Governance.L2.ConsensusSignature = fixtureBadSignature
	l2InvalidJSON, _ := marshaler.Marshal(l2Invalid)
	fmt.Println("L2_INVALID_INTENT:")
	fmt.Println(string(l2InvalidJSON))
	fmt.Println()

	// 5. l3_missing - no L3 proof
	l3Missing := createBase("nonce-l3-missing-123")
	signAndHash(l3Missing)
	l3MissingJSON, _ := marshaler.Marshal(l3Missing)
	fmt.Println("L3_MISSING_INTENT:")
	fmt.Println(string(l3MissingJSON))
	fmt.Println()

	// 6. l3_invalid - invalid L3 proof
	l3Invalid := createBase("nonce-l3-invalid-123")
	signAndHash(l3Invalid)
	l3Invalid.Governance.L3 = &commonv1.L3Metadata{
		Proof: &commonv1.L3Proof{
			AuthenticatorData: fixtureInvalidL3AuthData,
			ClientDataJson:    fixtureInvalidL3ClientData,
			Signature:         fixtureInvalidL3Signature,
		},
	}
	l3InvalidJSON, _ := marshaler.Marshal(l3Invalid)
	fmt.Println("L3_INVALID_INTENT:")
	fmt.Println(string(l3InvalidJSON))
	fmt.Println()

	// 7. stale_state_root - wrong state root
	staleState := createBase("nonce-stale-state-123")
	staleState.StateMerkleRoot = "stale123stale456"
	signAndHash(staleState)
	staleStateJSON, _ := marshaler.Marshal(staleState)
	fmt.Println("STALE_STATE_ROOT_INTENT:")
	fmt.Println(string(staleStateJSON))
	fmt.Println()

	// 8. unknown_signer - envelope signed by untrusted key
	// Generate a different keypair for this test
	unknownPrivKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	unknownPubKey := unknownPrivKey.Public().(ed25519.PublicKey)
	unknownKeyID := hex.EncodeToString(unknownPubKey)
	unknownSigner := createBase("nonce-unknown-signer-123")
	// Compute hash WITHOUT governance (verifier doesn't include governance in hash)
	unknownHash, err := governance.GenerateMessageID(unknownSigner)
	if err != nil {
		panic(err)
	}
	unknownSigner.Id = unknownHash
	unknownSigner.TransactionHash = unknownHash
	// Now set governance with unknown key_id and sign with unknown private key
	unknownSigner.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			KeyId:              unknownKeyID,
			ConsensusSignature: hex.EncodeToString(ed25519.Sign(unknownPrivKey, []byte(unknownHash+"|true"))),
		},
	}
	unknownSignerJSON, _ := marshaler.Marshal(unknownSigner)
	fmt.Println("UNKNOWN_SIGNER_INTENT:")
	fmt.Println(string(unknownSignerJSON))
	fmt.Println()

	// 9. malformed_payload - invalid base64 payload
	malformedPayload := createBase("nonce-malformed-payload-123")
	signAndHash(malformedPayload)
	// Set payload to invalid base64 (not properly encoded)
	malformedPayload.Payload = []byte("!!!invalid!!!base64!!!")
	// Recompute hash after changing payload
	malformedHash, err1 := governance.GenerateMessageID(malformedPayload)
	if err1 != nil {
		panic(err1)
	}
	malformedPayload.Id = malformedHash
	malformedPayload.TransactionHash = malformedHash
	// Re-sign with the new hash
	malformedPayload.Governance.L2.ConsensusSignature = hex.EncodeToString(ed25519.Sign(privKey, []byte(malformedHash+"|true")))
	malformedPayloadJSON, _ := marshaler.Marshal(malformedPayload)
	fmt.Println("MALFORMED_PAYLOAD_INTENT:")
	fmt.Println(string(malformedPayloadJSON))
	fmt.Println()

	// 10. empty_payload - missing payload field
	emptyPayload := createBase("nonce-empty-payload-123")
	emptyPayload.Payload = []byte{}
	emptyHash, err2 := governance.GenerateMessageID(emptyPayload)
	if err2 != nil {
		panic(err2)
	}
	emptyPayload.Id = emptyHash
	emptyPayload.TransactionHash = emptyHash
	emptyPayload.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			KeyId:              keyID,
			ConsensusSignature: hex.EncodeToString(ed25519.Sign(privKey, []byte(emptyHash+"|true"))),
		},
	}
	emptyPayloadJSON, _ := marshaler.Marshal(emptyPayload)
	fmt.Println("EMPTY_PAYLOAD_INTENT:")
	fmt.Println(string(emptyPayloadJSON))
	fmt.Println()
}
