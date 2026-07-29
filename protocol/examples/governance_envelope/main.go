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

package main

import (
	"fmt"
	"log"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

func main() {
	// Create a governance envelope for a command execution
	envelope := &commonv1.GovernanceEnvelope{
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(5 * time.Minute)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "op-456",
		OperatorSessionId: "session-789",
		CliSessionId:      "cli-123",
		EventType:         "g8e.v1.operator.command.requested",
		ActionType:        "EXECUTE_BASH",
		TargetResource:    "/home/user",
		StateMerkleRoot:   "root-hash-abc",
		Nonce:             "nonce-xyz",
		ProtocolVersion:   "1.0",
		CaseId:            "case-456",
		InvestigationId:   "inv-789",
		TaskId:            "task-abc",
		SystemFingerprint: "fp-123",
		TenantId:          "tenant-xyz",
		BindingPersona:    "default",
		RequestorUserId:   "user-abc",
		ActingAppId:       "app-cursor",
		IntentData: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"goal":         structpb.NewStringValue("Inspect filesystem"),
				"risk_level":   structpb.NewStringValue("low"),
				"auto_approve": structpb.NewBoolValue(false),
			},
		},
		Governance: &commonv1.GovernanceMetadata{
			L1: &commonv1.L1Metadata{
				Validated:  true,
				Violations: []string{},
			},
			L2: &commonv1.L2Metadata{
				ConsensusSetId: "consensus-1",
				Votes: []*commonv1.L2Vote{
					{
						SignerKeyId:        "key-123",
						ConsensusSignature: "sig-abc",
						Decision:           true,
					},
				},
			},
			L3: &commonv1.L3Metadata{
				Proof: &commonv1.L3Proof{
					MtlsCertFingerprint: "sha256:ab:cd:ef:01:23:45:67:89:ab:cd:ef:01:23:45:67:89:ab:cd:ef:01:23:45:67:89:ab:cd:ef:01:23:45:67:89",
					CliSignature:        "ed25519:9f3a2b1c8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1a",
				},
			},
		},
	}

	// Create the command payload
	cmd := &operatorv1.CommandRequested{
		Command:          "ls -la",
		ExecutionId:      "exec-123",
		Justification:    "List directory contents",
		VaultMode:        "scrub",
		TimeoutSeconds:   30,
		Intent:           "Inspect filesystem",
		Environment:      map[string]string{"PATH": "/usr/bin"},
		WorkingDirectory: "/home/user",
	}

	// Serialize command to bytes for the envelope payload
	cmdBytes, err := proto.Marshal(cmd)
	if err != nil {
		log.Fatalf("Failed to marshal command: %v", err)
	}
	envelope.Payload = cmdBytes

	// Compute the canonical transaction hash from the envelope's critical fields.
	// The Id and TransactionHash must both equal this computed value.
	txHash, err := governance.GenerateMessageID(envelope)
	if err != nil {
		log.Fatalf("Failed to compute transaction hash: %v", err)
	}
	envelope.Id = txHash
	envelope.TransactionHash = txHash

	// Convert envelope to protojson (canonical wire format)
	envelopeJSON, err := (protojson.MarshalOptions{Multiline: false}).Marshal(envelope)
	if err != nil {
		log.Fatalf("Failed to marshal envelope to JSON: %v", err)
	}

	fmt.Println("Governance Envelope (protojson):")
	pretty, err := (protojson.MarshalOptions{Multiline: true, Indent: "  "}).Marshal(envelope)
	if err != nil {
		log.Fatalf("Failed to marshal envelope for pretty print: %v", err)
	}
	fmt.Println(string(pretty))

	// Parse back from JSON
	parsedEnvelope := &commonv1.GovernanceEnvelope{}
	if err := protojson.Unmarshal(envelopeJSON, parsedEnvelope); err != nil {
		log.Fatalf("Failed to unmarshal envelope from JSON: %v", err)
	}

	// Parse command payload
	parsedCmd := &operatorv1.CommandRequested{}
	if err := proto.Unmarshal(parsedEnvelope.Payload, parsedCmd); err != nil {
		log.Fatalf("Failed to unmarshal command payload: %v", err)
	}

	fmt.Printf("\nParsed Command: %s\n", parsedCmd.Command)
	fmt.Printf("Execution ID: %s\n", parsedCmd.ExecutionId)
	fmt.Printf("Justification: %s\n", parsedCmd.Justification)
	fmt.Printf("Transaction Hash: %s\n", parsedEnvelope.TransactionHash)
	fmt.Printf("Requestor User ID: %s\n", parsedEnvelope.RequestorUserId)
	fmt.Printf("Acting App ID: %s\n", parsedEnvelope.ActingAppId)
}
