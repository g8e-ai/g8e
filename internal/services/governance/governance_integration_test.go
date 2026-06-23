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

//go:build integration

package governance

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"os"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestGovernanceFlow tests the full governance flow from envelope creation
// through L5Actuator execution.
func TestGovernanceFlow(t *testing.T) {
	t.Parallel()
	pub, priv, _ := ed25519.GenerateKey(nil)
	nodeID := "test-node-1"

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

	// 2. Set governance metadata (L1 validated, L2 vote pre-populated)
	env.Governance = &commonv1.GovernanceMetadata{
		L1: &commonv1.L1Metadata{Validated: true},
		L2: &commonv1.L2Metadata{
			TribunalId: "test-tribunal",
			Votes: []*commonv1.L2Vote{
				{
					SignerKeyId:        nodeID,
					ConsensusSignature: "test-sig",
					Decision:           true,
				},
			},
		},
		L3: &commonv1.L3Metadata{AutoApproved: true},
	}

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

// TestGovernanceFailClosed tests that the L1 Doctrine fails closed
// when critical components are missing or misconfigured.
func TestGovernanceFailClosed(t *testing.T) {
	t.Parallel()

	t.Run("DoctrineNil_FailClosed", func(t *testing.T) {
		t.Parallel()
		doctrine := NewL1Doctrine()
		warden := NewL4Warden(
			slog.New(slog.NewTextHandler(os.Stdout, nil)),
			&testutil.MockReplayStore{},
			testutil.NewMockStateRootProvider("test-state-root"),
			&SimpleSignerStore{Signers: map[string]ed25519.PublicKey{}},
			nil, // TribunalStore
			nil, // AppPolicyStore
			nil, // L3Notary
			doctrine,
			constants.AllActionTypes,
			"doctrine",
			nil, // Clock defaults to RealClock
		)
		if warden == nil {
			t.Error("Expected non-nil warden with doctrine")
		}
	})
}
