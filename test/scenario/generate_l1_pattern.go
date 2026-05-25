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
	// Use the same private key as the test fixtures
	privKeyHex := "c847d8625a1d1be737b8c86012ef1ceb7cfe1c2f5bed5115b90b490c55600502797c07dc7211981020b7fea8c31ed993d30576e0e14523a76678672a0d18b8cd"
	privKeyBytes, _ := hex.DecodeString(privKeyHex)
	privKey := ed25519.PrivateKey(privKeyBytes)
	pubKey := privKey.Public().(ed25519.PublicKey)
	keyID := hex.EncodeToString(pubKey)

	// Create a valid base envelope with fixed time matching test clock
	fixedTime := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
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
		Nonce:             "nonce-l1-pattern-123",
	}

	// Create a CommandRequested payload with forbidden pattern
	// The protobuf command field has forbidden_patterns = "sudo,su,rm -rf /"
	cmdPayload := &operatorv1.CommandRequested{
		Command:        "sudo rm -rf /",
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
			KeyId:              keyID,
			ConsensusSignature: hex.EncodeToString(ed25519.Sign(privKey, []byte(hash+"|true"))),
		},
	}

	// Marshal to JSON
	marshaler := &protojson.MarshalOptions{Indent: "  "}
	l1PatternJSON, _ := marshaler.Marshal(baseEnv)

	// Output the JSON string for fixture
	fmt.Println("L1_PATTERN_INTENT:")
	fmt.Println(string(l1PatternJSON))
}
