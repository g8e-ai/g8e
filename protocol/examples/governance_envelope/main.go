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
	"encoding/json"
	"fmt"
	"log"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

func main() {
	// Create a governance envelope for a command execution
	envelope := &commonv1.GovernanceEnvelope{
		Id:                "txn-abc-123",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(5 * time.Minute)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "op-456",
		OperatorSessionId: "session-789",
		WebSessionId:      "web-xyz",
		CliSessionId:      "cli-123",
		EventType:         "g8e.v1.operator.command.requested",
		ActionType:        "EXECUTE_BASH",
		TargetResource:    "/home/user",
		StateMerkleRoot:   "root-hash-abc",
		Nonce:             "nonce-xyz",
		TransactionHash:   "hash-123",
		ProtocolVersion:   "1.0",
		CaseId:            "case-456",
		InvestigationId:   "inv-789",
		TaskId:            "task-abc",
		SystemFingerprint: "fp-123",
		TenantId:          "tenant-xyz",
		BindingPersona:    "default",
		Governance: &commonv1.GovernanceMetadata{
			L1: &commonv1.L1Metadata{
				Validated:  true,
				Violations: []string{},
			},
			L2: &commonv1.L2Metadata{
				ConsensusSignature: "sig-abc",
				AgentIds:           []string{"agent-1", "agent-2", "agent-3"},
				KeyId:              "key-123",
			},
			L3: &commonv1.L3Metadata{
				Proof: &commonv1.L3Proof{
					ClientDataJson:      `{"challenge":"abc"}`,
					AuthenticatorData:   "auth-data",
					Signature:           "sig-xyz",
					CredentialId:        "cred-123",
					MtlsCertFingerprint: "fp-456",
					CliSignature:        "cli-sig",
				},
				AutoApproved: false,
			},
			GatewaySigned: false,
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

	// Convert envelope to protojson (canonical wire format)
	marshaler := protojson.MarshalOptions{UseProtoNames: false}
	envelopeJSON, err := marshaler.Marshal(envelope)
	if err != nil {
		log.Fatalf("Failed to marshal envelope to JSON: %v", err)
	}

	fmt.Println("Governance Envelope (protojson):")
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal(envelopeJSON, &prettyJSON); err != nil {
		log.Fatalf("Failed to unmarshal for pretty print: %v", err)
	}
	formatted, _ := json.MarshalIndent(prettyJSON, "", "  ")
	fmt.Println(string(formatted))

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
}
