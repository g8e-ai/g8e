// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/governance"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func main() {
	// Operator Ed25519 key used for the CLI/mTLS L3 notary proof.
	_, opPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate operator key: %v", err)
	}

	// L2 consensus signer key. signer_key_id in production maps to a
	// TrustedSigner record; here we use a descriptive test ID.
	_, l2Priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate L2 signer key: %v", err)
	}

	// mTLS certificate fingerprint: raw SHA-256 hex, no prefixes or colons.
	mtlsHash := sha256.Sum256([]byte("mock-cli-cert"))
	mtlsFingerprint := hex.EncodeToString(mtlsHash[:])

	// Create a governance envelope for a command execution.
	envelope := &commonv1.GovernanceEnvelope{
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(5 * time.Minute)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "op-456",
		OperatorSessionId: "session-789",
		CliSessionId:      "cli-123",
		RequestorUserId:   "user-abc",
		ActingAppId:       "app-cursor",
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
		Posture:           "notary",
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
						SignerKeyId:        "l2-signer-1",
						ConsensusSignature: "", // filled in after transaction hash is computed
						Decision:           true,
					},
				},
			},
			L3: &commonv1.L3Metadata{
				Proof: &commonv1.L3Proof{
					MtlsCertFingerprint: mtlsFingerprint,
					CliSignature:        "", // filled in after transaction hash is computed
				},
			},
		},
	}

	// Create the command payload.
	cmd := &operatorv1.CommandRequested{
		Command:          "ls -la",
		ExecutionId:      "exec-123",
		Justification:    "List directory contents",
		VaultMode:        "scrubbed",
		TimeoutSeconds:   30,
		Intent:           "Inspect filesystem",
		Environment:      map[string]string{"PATH": "/usr/bin"},
		WorkingDirectory: "/home/user",
	}

	// Serialize command to bytes for the envelope payload.
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

	// L2 consensus signature is over "<transaction_hash>|<decision>".
	l2Payload := fmt.Sprintf("%s|true", txHash)
	l2Sig := ed25519.Sign(l2Priv, []byte(l2Payload))
	envelope.Governance.L2.Votes[0].ConsensusSignature = hex.EncodeToString(l2Sig)

	// L3 CLI signature is the operator's Ed25519 signature over the transaction hash.
	cliSig := ed25519.Sign(opPriv, []byte(txHash))
	envelope.Governance.L3.Proof.CliSignature = hex.EncodeToString(cliSig)

	// Id and TransactionHash must match the canonical hash.
	envelope.Id = txHash
	envelope.TransactionHash = txHash

	// Convert envelope to protojson (canonical wire format).
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

	// Parse back from JSON.
	parsedEnvelope := &commonv1.GovernanceEnvelope{}
	if err := protojson.Unmarshal(envelopeJSON, parsedEnvelope); err != nil {
		log.Fatalf("Failed to unmarshal envelope from JSON: %v", err)
	}

	// Parse command payload.
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
	fmt.Printf("Posture: %s\n", parsedEnvelope.Posture)
}
