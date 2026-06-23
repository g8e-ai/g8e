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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// MockTribunal is a basic mock for the Tribunal (L2 consensus) layer.
// It allows tests to control consensus evaluation behavior.
type MockTribunal struct {
	NodeID            string
	ShouldPass        bool
	ReturnError       error
	EvaluateCallCount int
}

// EvaluatePayload mimics Tribunal evaluation for testing.
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

	env.Governance.L2.TribunalId = "mock-tribunal"
	env.Governance.L2.Votes = append(env.Governance.L2.Votes, &commonv1.L2Vote{
		SignerKeyId:       m.NodeID,
		ConsensusSignature: "mock-signature",
		Decision:          true,
	})

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
func (m *MockHumanSigner) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	m.VerifyCallCount++
	if m.ReturnError != nil {
		return false, m.ReturnError
	}
	return m.ShouldApprove, nil
}

// mockExecutionHandler is a test-only implementation of ExecutionHandler.
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
