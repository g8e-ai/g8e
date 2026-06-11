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

//go:build integration || e2e

package scenario

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

// Builder provides a fluent API for constructing GovernanceEnvelope structures
// with dynamic cryptography. No pre-baked fixtures, no hardcoded keys.
type Builder struct {
	envelope *commonv1.GovernanceEnvelope
	privKey  ed25519.PrivateKey
}

// New creates a new envelope builder with sensible defaults.
func New() *Builder {
	now := time.Now()
	return &Builder{
		envelope: &commonv1.GovernanceEnvelope{
			ProtocolVersion:   "1.0",
			Timestamp:         timestamppb.New(now),
			ExpiresAt:         timestamppb.New(now.Add(time.Hour)),
			SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
			OperatorId:        "test-operator",
			OperatorSessionId: "test-session",
			ActionType:        "EXECUTE_BASH",
			TargetResource:    "localhost",
			StateMerkleRoot:   "test-state-root",
			Nonce:             fmt.Sprintf("nonce-%d", now.UnixNano()),
			Governance:        &commonv1.GovernanceMetadata{},
		},
	}
}

// WithCommand sets the command payload for EXECUTE_BASH actions.
func (b *Builder) WithCommand(cmd string) *Builder {
	cmdPayload := &operatorv1.CommandRequested{
		Command:        cmd,
		ExecutionId:    fmt.Sprintf("exec-%d", time.Now().UnixNano()),
		Justification:  "test command",
		VaultMode:      "strict",
		TimeoutSeconds: 30,
	}
	payloadBytes, _ := proto.Marshal(cmdPayload)
	b.envelope.Payload = payloadBytes
	b.envelope.ActionType = "EXECUTE_BASH"
	return b
}

// WithOperatorID sets the Operator ID.
func (b *Builder) WithOperatorID(id string) *Builder {
	b.envelope.OperatorId = id
	return b
}

// WithOperatorSessionID sets the Operator session ID.
func (b *Builder) WithOperatorSessionID(id string) *Builder {
	b.envelope.OperatorSessionId = id
	return b
}

// WithStateRoot sets the state Merkle root.
func (b *Builder) WithStateRoot(root string) *Builder {
	b.envelope.StateMerkleRoot = root
	return b
}

// WithNonce sets the nonce for replay protection.
func (b *Builder) WithNonce(nonce string) *Builder {
	b.envelope.Nonce = nonce
	return b
}

// WithL2 adds L2 consensus metadata with a signature.
func (b *Builder) WithL2(privKey ed25519.PrivateKey, vote bool) *Builder {
	if b.envelope.Governance == nil {
		b.envelope.Governance = &commonv1.GovernanceMetadata{}
	}

	b.envelope.Governance.L2 = &commonv1.L2Metadata{
		KeyId: hex.EncodeToString(privKey.Public().(ed25519.PublicKey)),
	}

	// Signature will be computed during Build() after hash is known
	b.privKey = privKey
	return b
}

// WithBadID sets an intentionally incorrect envelope ID for testing rejection.
func (b *Builder) WithBadID() *Builder {
	b.envelope.Id = "wrongidwrongidwrongidwrongidwrongidwrongidwrongidwrongidwrongid"
	return b
}

// WithBadHash sets an intentionally incorrect transaction hash for testing rejection.
func (b *Builder) WithBadHash() *Builder {
	b.envelope.TransactionHash = "wronghashwronghashwronghashwronghashwronghashwronghashwronghash"
	return b
}

// WithBadSignature sets an intentionally incorrect L2 signature for testing rejection.
func (b *Builder) WithBadSignature() *Builder {
	if b.envelope.Governance != nil && b.envelope.Governance.L2 != nil {
		b.envelope.Governance.L2.ConsensusSignature = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	}
	return b
}

// Build finalizes the envelope, computes hashes and signatures, and returns protojson bytes.
func (b *Builder) Build() ([]byte, error) {
	// Compute transaction hash
	hash, err := governance.GenerateMessageID(b.envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to generate message ID: %w", err)
	}

	// Set id and transaction_hash if not already set to bad values
	if b.envelope.Id == "" || b.envelope.Id != "wrongidwrongidwrongidwrongidwrongidwrongidwrongidwrongidwrongid" {
		b.envelope.Id = hash
	}
	if b.envelope.TransactionHash == "" || b.envelope.TransactionHash != "wronghashwronghashwronghashwronghashwronghashwronghashwronghash" {
		b.envelope.TransactionHash = hash
	}

	// Compute L2 signature if private key is provided
	if b.privKey != nil && b.envelope.Governance != nil && b.envelope.Governance.L2 != nil {
		if b.envelope.Governance.L2.ConsensusSignature == "" || b.envelope.Governance.L2.ConsensusSignature != "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef" {
			sig := ed25519.Sign(b.privKey, []byte(hash+"|true"))
			b.envelope.Governance.L2.ConsensusSignature = hex.EncodeToString(sig)
		}
	}

	// Marshal to protojson
	marshaler := &protojson.MarshalOptions{}
	jsonBytes, err := marshaler.Marshal(b.envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal envelope: %w", err)
	}

	return jsonBytes, nil
}
