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

package governance

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"os"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockExecutionHandler struct {
	executed                       bool
	err                            error
	ExecuteVerifiedTransactionFunc func(ctx context.Context, eventType constants.EventType, cmdMsg interface{}) (string, error)
}

func (m *mockExecutionHandler) ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg interface{}) (string, error) {
	m.executed = true
	if m.ExecuteVerifiedTransactionFunc != nil {
		return m.ExecuteVerifiedTransactionFunc(ctx, eventType, cmdMsg)
	}
	return "", m.err
}

// MockTribunal is a basic mock for the L2 Consensus layer (Tribunal).
// It allows tests to control consensus evaluation behavior.
type MockTribunal struct {
	NodeID            string
	ShouldPass        bool
	ReturnError       error
	EvaluateCallCount int
}

// EvaluatePayload mimics L2Consensus.EvaluatePayload for testing.
func (m *MockTribunal) EvaluatePayload(env *governance.GovernanceEnvelope) error {
	m.EvaluateCallCount++
	if m.ReturnError != nil {
		return m.ReturnError
	}

	if env.Governance == nil {
		env.Governance = &commonv1.GovernanceMetadata{
			L1: &commonv1.L1Metadata{},
			L2: &commonv1.L2Metadata{},
			L3: &commonv1.L3Metadata{},
		}
	}

	env.Governance.L2.AgentIds = append(env.Governance.L2.AgentIds, m.NodeID)
	env.Governance.L2.ConsensusSignature = "mock-signature"

	if !m.ShouldPass {
		env.Governance.L1.Validated = false
		env.Governance.L1.Violations = append(env.Governance.L1.Violations, "MOCK_TRIBUNAL_REJECTED")
	}

	return nil
}

// MockHumanSigner is a basic mock for the L3 Authorization layer (Human signer).
// It allows tests to control L3 verification behavior.
type MockHumanSigner struct {
	ShouldApprove   bool
	ReturnError     error
	VerifyCallCount int
}

// VerifyL3Proof mimics L3Notary.VerifyL3Proof for testing.
func (m *MockHumanSigner) VerifyL3Proof(userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	m.VerifyCallCount++
	if m.ReturnError != nil {
		return false, m.ReturnError
	}
	return m.ShouldApprove, nil
}

func TestGovernanceFlow(t *testing.T) {
	t.Parallel()
	pub, priv, _ := ed25519.GenerateKey(nil)
	nodeID := "test-node-1"

	consensus := &L2Consensus{
		NodeID:     nodeID,
		PrivateKey: priv,
	}

	actuator := &L5Actuator{
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
		SignerStore: &SimpleSignerStore{
			Signers: map[string]ed25519.PublicKey{
				nodeID: pub,
			},
		},
	}

	env := &governance.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "agent-1",
		Timestamp:       timestamppb.Now(),
		ActionType:      string(constants.ActionTypeFetchLogs),
		TargetResource:  "localhost",
		Payload:         []byte("fetch logs"),
	}

	// 1. Generate Message ID
	id, _ := governance.GenerateMessageID(env)
	env.Id = id

	// 2. Consensus Evaluation
	err := consensus.EvaluatePayload(env)
	if err != nil {
		t.Fatalf("L2Consensus evaluation failed: %v", err)
	}

	if env.Governance == nil || len(env.Governance.L2.AgentIds) != 1 {
		t.Errorf("Expected 1 agent ID in L2, got %v", env.Governance)
	}

	// Ensure status is validated for L5Actuator
	env.Governance.L1.Validated = true
	sig, _ := consensus.SignDecision(env.Id, true)
	env.Governance.L2.ConsensusSignature = sig

	handler := &mockExecutionHandler{}
	actuator.ExecutionHandler = handler
	actuator.SigningKey = priv
	actuator.KeyID = nodeID
	actuator.Ctx = context.Background()

	vt := &VerifiedTransaction{
		Envelope:   env,
		ActionType: constants.ActionTypeFetchLogs,
	}

	// 3. L5Actuator Execution
	receipt, err := actuator.Execute(context.Background(), vt, nil)
	if err != nil {
		t.Fatalf("L5Actuator execution failed: %v", err)
	}

	if !handler.executed {
		t.Error("Expected handler to be executed")
	}

	if receipt.TransactionId != env.Id {
		t.Errorf("Expected receipt tx id %s, got %s", env.Id, receipt.TransactionId)
	}
}

func TestGovernanceFailClosed(t *testing.T) {
	t.Parallel()
	_, priv, _ := ed25519.GenerateKey(nil)
	nodeID := "test-node-1"

	t.Run("DoctrineNil_FailClosed", func(t *testing.T) {
		t.Parallel()
		consensus := &L2Consensus{
			NodeID:     nodeID,
			PrivateKey: priv,
			Doctrine:   nil, // explicitly nil
		}
		isSafe := consensus.RunMITREChecks("test", "echo 'hello'")
		if isSafe {
			t.Error("Expected fail-closed (Safe=false) when Doctrine is nil")
		}
	})

	t.Run("MissingPrivateKey_Error", func(t *testing.T) {
		t.Parallel()
		consensus := &L2Consensus{NodeID: nodeID, PrivateKey: nil}
		_, err := consensus.SignDecision("test-id", true)
		if err == nil {
			t.Errorf("Expected error when PrivateKey is nil during SignDecision")
		}
	})
}
