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

	"github.com/g8e-ai/g8e/pkg/uap"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

func main() {
	// Generate test signers
	pub1, priv1, _ := ed25519.GenerateKey(nil)
	keyID1 := hex.EncodeToString(pub1)
	priv1Hex := hex.EncodeToString(priv1)

	// Output the private key for use in tests
	fmt.Println("PRIVATE_KEY_HEX:")
	fmt.Println(priv1Hex)
	fmt.Println("KEY_ID:")
	fmt.Println(keyID1)
	fmt.Println()

	// Create a valid base envelope with fixed time matching test clock
	fixedTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	baseEnv := &commonv1.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.New(fixedTime),
		ExpiresAt:         timestamppb.New(fixedTime.Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "operator-1",
		OperatorSessionId: "operator-session-1",
		ActionType:        "EXECUTE_BASH",
		TargetResource:    "localhost",
		StateMerkleRoot:   "abc123def456",
		Nonce:             "nonce-forged-123",
	}

	// Create a CommandRequested payload
	cmdPayload := &operatorv1.CommandRequested{
		Command:        "ls -la",
		ExecutionId:    "exec-123",
		Justification:  "test command",
		SentinelMode:   "strict",
		TimeoutSeconds: 30,
	}
	payloadBytes, _ := proto.Marshal(cmdPayload)
	baseEnv.Payload = payloadBytes

	// Generate transaction hash
	hash, _ := uap.GenerateMessageID(baseEnv)
	baseEnv.Id = hash
	baseEnv.TransactionHash = hash

	// Sign with L2
	baseEnv.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			KeyId:             keyID1,
			TribunalSignature: hex.EncodeToString(ed25519.Sign(priv1, []byte(hash+"|true"))),
		},
	}

	// Marshal to JSON
	marshaler := &protojson.MarshalOptions{Indent: "  "}

	// 1. Forged signature - use wrong signature
	forgedEnv := proto.Clone(baseEnv).(*commonv1.GovernanceEnvelope)
	forgedEnv.Governance.L2.TribunalSignature = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	forgedJSON, _ := marshaler.Marshal(forgedEnv)

	// 2. Replay - use a different nonce but same envelope structure
	replayEnv := proto.Clone(baseEnv).(*commonv1.GovernanceEnvelope)
	replayEnv.Nonce = "nonce-replay-123"
	// Rehash with new nonce
	replayHash, _ := uap.GenerateMessageID(replayEnv)
	replayEnv.Id = replayHash
	replayEnv.TransactionHash = replayHash
	// Re-sign with new hash
	replayEnv.Governance.L2.TribunalSignature = hex.EncodeToString(ed25519.Sign(priv1, []byte(replayHash+"|true")))
	replayJSON, _ := marshaler.Marshal(replayEnv)

	// 3. Stale state root
	staleEnv := proto.Clone(baseEnv).(*commonv1.GovernanceEnvelope)
	staleEnv.StateMerkleRoot = "stale-old-root-999"
	staleEnv.Nonce = "nonce-stale-123"
	// Rehash with new state root
	newHash, _ := uap.GenerateMessageID(staleEnv)
	staleEnv.Id = newHash
	staleEnv.TransactionHash = newHash
	// Re-sign with new hash
	staleEnv.Governance.L2.TribunalSignature = hex.EncodeToString(ed25519.Sign(priv1, []byte(newHash+"|true")))
	staleJSON, _ := marshaler.Marshal(staleEnv)

	// 4. Tampered receipt - this is tested by the receipt verification, not envelope
	// For now, use a valid envelope that will be accepted
	tamperedEnv := proto.Clone(baseEnv).(*commonv1.GovernanceEnvelope)
	tamperedEnv.Nonce = "nonce-tampered-123"
	// Rehash with new nonce
	tamperedHash, _ := uap.GenerateMessageID(tamperedEnv)
	tamperedEnv.Id = tamperedHash
	tamperedEnv.TransactionHash = tamperedHash
	// Re-sign with new hash
	tamperedEnv.Governance.L2.TribunalSignature = hex.EncodeToString(ed25519.Sign(priv1, []byte(tamperedHash+"|true")))
	tamperedJSON, _ := marshaler.Marshal(tamperedEnv)

	// Output the JSON strings for fixtures
	fmt.Println("FORGED_SIG_INTENT:")
	fmt.Println(string(forgedJSON))
	fmt.Println("\nREPLAY_NONCE_INTENT:")
	fmt.Println(string(replayJSON))
	fmt.Println("\nSTALE_STATE_ROOT_INTENT:")
	fmt.Println(string(staleJSON))
	fmt.Println("\nTAMPERED_RECEIPT_INTENT:")
	fmt.Println(string(tamperedJSON))
}
