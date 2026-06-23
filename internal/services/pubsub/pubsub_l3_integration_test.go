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
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/execution"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/internal/testutil"
	govpkg "github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestOperatorPubSubService_L3Rejection_FailClosed verifies that OperatorPubSubService
// rejects mutation envelopes when L3 verification fails, ensuring fail-closed behavior
// through the full ProcessEnvelope → L4Warden → Actuator chain.
func TestOperatorPubSubService_L3Rejection_FailClosed(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Create a rejecting L3 notary
	rejectingL3 := &testutil.ConfigurableMockL3Notary{ShouldPass: false}

	// Create a stateful replay store
	replayStore := testutil.NewStatefulMockReplayStore()
	stateRootProvider := testutil.NewMockStateRootProvider("test-root")

	// Create signer store with test key
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate signer: %v", err)
	}
	signerStore := &governance.SimpleSignerStore{
		Signers: map[string]ed25519.PublicKey{"test-key": pubKey},
	}

	// Create execution service
	execSvc := execution.NewExecutionService(cfg, logger)
	fileSvc := execution.NewFileEditService(cfg, logger)

	// Create command service with rejecting L3 notary
	cmdSvc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:             cfg,
		Logger:             logger,
		Execution:          execSvc,
		FileEdit:           fileSvc,
		PubSubClient:       NewInProcessPubSubClient(nil),
		ResultsService:     nil, // ResultsPublisher is optional for L3 verification test
		L3Notary:           rejectingL3,
		ReplayStore:        replayStore,
		StateRootProvider:  stateRootProvider,
		TransactionAudit:   &testutil.MockTransactionAudit{},
		SignerStore:        signerStore,
		TribunalStore:      testTribunalStore(),
		ActuatorSigningKey: privKey,
		ActuatorKeyID:      "test-key",
		Scrubbing:          scrubbing.NewScrubbingService(scrubbing.DefaultConfig(), logger, nil),
	})
	if err != nil {
		t.Fatalf("failed to create command service: %v", err)
	}

	// Build a mutation envelope requiring L3 proof
	cmdPayload := &operatorv1.CommandRequested{
		Command:       "echo test",
		ExecutionId:   "exec-1",
		Justification: "test",
	}
	payloadBytes, err := proto.Marshal(cmdPayload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	envelope := &govpkg.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "operator-1",
		OperatorSessionId: "session-1",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
		Payload:           payloadBytes,
		StateMerkleRoot:   "test-root",
		Nonce:             "nonce-test-1",
	}

	// Generate transaction hash
	txHash, err := govpkg.GenerateMessageID(envelope)
	if err != nil {
		t.Fatalf("failed to generate transaction hash: %v", err)
	}
	envelope.Id = txHash
	envelope.TransactionHash = txHash

	// Add L2 signature
	sigPayload := txHash + "|true"
	sig := ed25519.Sign(privKey, []byte(sigPayload))
	envelope.Governance = &commonv1.GovernanceMetadata{
		L1: &commonv1.L1Metadata{Validated: true},
		L2: &commonv1.L2Metadata{
			TribunalId: "test-tribunal",
			Votes: []*commonv1.L2Vote{
				{
					SignerKeyId:        "test-key",
					ConsensusSignature: hex.EncodeToString(sig),
					Decision:           true,
				},
			},
		},
		L3: &commonv1.L3Metadata{
			Proof: &commonv1.L3Proof{
				Signature: "test-proof",
			},
		},
	}

	// Marshal envelope to protojson (canonical wire format)
	envelopeBytes, err := (protojson.MarshalOptions{}).Marshal(envelope)
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	// Process envelope through command service
	receipt, err := cmdSvc.ProcessEnvelope(context.Background(), envelopeBytes)

	// Verify fail-closed behavior: L3 rejection should return error
	if err == nil {
		t.Error("Expected L3 rejection error, got nil")
	}
	if receipt != nil {
		t.Error("Expected nil receipt on L3 rejection, got receipt")
	}

	// Verify the error is an L3-related error
	if !isL3Error(err) {
		t.Errorf("Expected L3-related error, got: %v", err)
	}
}

// TestOperatorPubSubService_L3Acceptance_Success verifies that OperatorPubSubService
// accepts mutation envelopes when L3 verification passes, ensuring the full
// ProcessEnvelope → L4Warden → Actuator chain works correctly.
func TestOperatorPubSubService_L3Acceptance_Success(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Create an accepting L3 notary
	acceptingL3 := &testutil.ConfigurableMockL3Notary{ShouldPass: true}

	// Create a stateful replay store
	replayStore := testutil.NewStatefulMockReplayStore()
	stateRootProvider := testutil.NewMockStateRootProvider("test-root")

	// Create signer store with test key
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate signer: %v", err)
	}
	signerStore := &governance.SimpleSignerStore{
		Signers: map[string]ed25519.PublicKey{"test-key": pubKey},
	}

	// Create execution service
	execSvc := execution.NewExecutionService(cfg, logger)
	fileSvc := execution.NewFileEditService(cfg, logger)

	// Create command service with accepting L3 notary
	cmdSvc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:             cfg,
		Logger:             logger,
		Execution:          execSvc,
		FileEdit:           fileSvc,
		PubSubClient:       NewInProcessPubSubClient(nil),
		ResultsService:     nil, // ResultsPublisher is optional for L3 verification test
		L3Notary:           acceptingL3,
		ReplayStore:        replayStore,
		StateRootProvider:  stateRootProvider,
		TransactionAudit:   &testutil.MockTransactionAudit{},
		SignerStore:        signerStore,
		TribunalStore:      testTribunalStore(),
		ActuatorSigningKey: privKey,
		ActuatorKeyID:      "test-key",
		Scrubbing:          scrubbing.NewScrubbingService(scrubbing.DefaultConfig(), logger, nil),
	})
	if err != nil {
		t.Fatalf("failed to create command service: %v", err)
	}

	// Build a mutation envelope requiring L3 proof
	cmdPayload := &operatorv1.CommandRequested{
		Command:       "echo test",
		ExecutionId:   "exec-1",
		Justification: "test",
	}
	payloadBytes, err := proto.Marshal(cmdPayload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	envelope := &govpkg.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "operator-1",
		OperatorSessionId: "session-1",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
		Payload:           payloadBytes,
		StateMerkleRoot:   "test-root",
		Nonce:             "nonce-test-2",
	}

	// Generate transaction hash
	txHash, err := govpkg.GenerateMessageID(envelope)
	if err != nil {
		t.Fatalf("failed to generate transaction hash: %v", err)
	}
	envelope.Id = txHash
	envelope.TransactionHash = txHash

	// Add L2 signature
	sigPayload := txHash + "|true"
	sig := ed25519.Sign(privKey, []byte(sigPayload))
	envelope.Governance = &commonv1.GovernanceMetadata{
		L1: &commonv1.L1Metadata{Validated: true},
		L2: &commonv1.L2Metadata{
			TribunalId: "test-tribunal",
			Votes: []*commonv1.L2Vote{
				{
					SignerKeyId:        "test-key",
					ConsensusSignature: hex.EncodeToString(sig),
					Decision:           true,
				},
			},
		},
		L3: &commonv1.L3Metadata{
			Proof: &commonv1.L3Proof{
				Signature: "test-proof",
			},
		},
	}

	// Marshal envelope to protojson (canonical wire format)
	envelopeBytes, err := (protojson.MarshalOptions{}).Marshal(envelope)
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	// Process envelope through command service
	receipt, err := cmdSvc.ProcessEnvelope(context.Background(), envelopeBytes)

	// Verify success: L3 acceptance should return receipt
	if err != nil {
		t.Errorf("Expected success with L3 acceptance, got error: %v", err)
	}
	if receipt == nil {
		t.Error("Expected receipt on L3 acceptance, got nil")
	}

	// Verify receipt fields
	if receipt != nil {
		if receipt.TransactionId != txHash {
			t.Errorf("Expected transaction ID %s, got %s", txHash, receipt.TransactionId)
		}
		if receipt.TransactionHash != txHash {
			t.Errorf("Expected transaction hash %s, got %s", txHash, receipt.TransactionHash)
		}
	}
}

// TestOperatorPubSubService_L3NilNotary_FailClosed verifies that OperatorPubSubService
// rejects mutation envelopes when L3Notary is nil, ensuring fail-closed behavior.
func TestOperatorPubSubService_L3NilNotary_FailClosed(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Create a stateful replay store
	replayStore := testutil.NewStatefulMockReplayStore()
	stateRootProvider := testutil.NewMockStateRootProvider("test-root")

	// Create signer store with test key
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate signer: %v", err)
	}
	signerStore := &governance.SimpleSignerStore{
		Signers: map[string]ed25519.PublicKey{"test-key": pubKey},
	}

	// Create execution service
	execSvc := execution.NewExecutionService(cfg, logger)
	fileSvc := execution.NewFileEditService(cfg, logger)

	// Create command service with nil L3 notary (should fail-closed)
	cmdSvc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:             cfg,
		Logger:             logger,
		Execution:          execSvc,
		FileEdit:           fileSvc,
		PubSubClient:       NewInProcessPubSubClient(nil),
		ResultsService:     nil, // ResultsPublisher is optional for L3 verification test
		L3Notary:           nil, // Explicitly nil to test fail-closed
		ReplayStore:        replayStore,
		StateRootProvider:  stateRootProvider,
		TransactionAudit:   &testutil.MockTransactionAudit{},
		SignerStore:        signerStore,
		TribunalStore:      testTribunalStore(),
		ActuatorSigningKey: privKey,
		ActuatorKeyID:      "test-key",
		Scrubbing:          scrubbing.NewScrubbingService(scrubbing.DefaultConfig(), logger, nil),
	})
	if err != nil {
		t.Fatalf("failed to create command service: %v", err)
	}

	// Build a mutation envelope requiring L3 proof
	cmdPayload := &operatorv1.CommandRequested{
		Command:       "echo test",
		ExecutionId:   "exec-1",
		Justification: "test",
	}
	payloadBytes, err := proto.Marshal(cmdPayload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	envelope := &govpkg.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "operator-1",
		OperatorSessionId: "session-1",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
		Payload:           payloadBytes,
		StateMerkleRoot:   "test-root",
		Nonce:             "nonce-test-3",
	}

	// Generate transaction hash
	txHash, err := govpkg.GenerateMessageID(envelope)
	if err != nil {
		t.Fatalf("failed to generate transaction hash: %v", err)
	}
	envelope.Id = txHash
	envelope.TransactionHash = txHash

	// Add L2 signature
	sigPayload := txHash + "|true"
	sig := ed25519.Sign(privKey, []byte(sigPayload))
	envelope.Governance = &commonv1.GovernanceMetadata{
		L1: &commonv1.L1Metadata{Validated: true},
		L2: &commonv1.L2Metadata{
			TribunalId: "test-tribunal",
			Votes: []*commonv1.L2Vote{
				{
					SignerKeyId:        "test-key",
					ConsensusSignature: hex.EncodeToString(sig),
					Decision:           true,
				},
			},
		},
		L3: &commonv1.L3Metadata{
			Proof: &commonv1.L3Proof{
				Signature: "test-proof",
			},
		},
	}

	// Marshal envelope to protojson (canonical wire format)
	envelopeBytes, err := (protojson.MarshalOptions{}).Marshal(envelope)
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	// Process envelope through command service
	receipt, err := cmdSvc.ProcessEnvelope(context.Background(), envelopeBytes)

	// Verify fail-closed behavior: nil L3 notary should return error
	if err == nil {
		t.Error("Expected error with nil L3 notary, got nil")
	}
	if receipt != nil {
		t.Error("Expected nil receipt with nil L3 notary, got receipt")
	}
}

// isL3Error checks if an error is L3-related
func isL3Error(err error) bool {
	if err == nil {
		return false
	}
	return err == governance.ErrL3ProofInvalid ||
		err == governance.ErrL3ProofMissing ||
		err == governance.ErrL3NotaryNotConfigured
}
