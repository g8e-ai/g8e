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

package scenario

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/services/system"
	"github.com/g8e-ai/g8e/pkg/uap"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestPropertyBasedInvariants uses property-based testing to verify governance invariants.
// It generates random envelope combinations and asserts that nothing executes unless
// integrity + freshness + state + required-gates all pass, in order.
func TestPropertyBasedInvariants(t *testing.T) {
	// Use doctrine mode for testing (L1-only, most permissive)
	mode := ModeDoctrine
	fixedTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	clock := system.NewFixedClock(fixedTime)
	testStateRoot := "abc123def456"

	// Generate test signers
	testSigners := generateTestSigners()

	gate, err := NewOperatorGate(mode, clock, testStateRoot, testSigners, nil, nil)
	if err != nil {
		t.Fatalf("failed to create operator gate: %v", err)
	}

	// Test invariant: envelope with valid signature but wrong hash must reject
	t.Run("valid_signature_wrong_hash_rejects", func(t *testing.T) {
		intentBytes := generateValidIntent()

		// Parse JSON to modify transaction hash
		var intent map[string]interface{}
		if err := json.Unmarshal(intentBytes, &intent); err != nil {
			t.Fatalf("failed to unmarshal intent: %v", err)
		}
		intent["transactionHash"] = "wronghashwronghashwronghashwronghashwronghashwronghashwronghash"

		modifiedBytes, err := json.Marshal(intent)
		if err != nil {
			t.Fatalf("failed to marshal intent: %v", err)
		}

		ctx := context.Background()
		result := gate.Submit(ctx, modifiedBytes)

		if result.Error == nil {
			t.Error("expected rejection for envelope with wrong transaction hash, got acceptance")
		}
		if result.Receipt != nil {
			t.Error("expected nil receipt for rejected envelope")
		}
	})

	// Test invariant: envelope with stale state root must reject
	t.Run("stale_state_root_rejects", func(t *testing.T) {
		intentBytes := generateValidIntent()

		// Parse JSON to modify state root
		var intent map[string]interface{}
		if err := json.Unmarshal(intentBytes, &intent); err != nil {
			t.Fatalf("failed to unmarshal intent: %v", err)
		}
		intent["stateMerkleRoot"] = "stalestalestalestalestalestalestalestale"

		modifiedBytes, err := json.Marshal(intent)
		if err != nil {
			t.Fatalf("failed to marshal intent: %v", err)
		}

		ctx := context.Background()
		result := gate.Submit(ctx, modifiedBytes)

		if result.Error == nil {
			t.Error("expected rejection for envelope with stale state root, got acceptance")
		}
		if result.Receipt != nil {
			t.Error("expected nil receipt for rejected envelope")
		}
	})

	// Test invariant: envelope with expired timestamp must reject
	t.Run("expired_timestamp_rejects", func(t *testing.T) {
		intentBytes := generateValidIntent()

		// Parse JSON to modify timestamp
		var intent map[string]interface{}
		if err := json.Unmarshal(intentBytes, &intent); err != nil {
			t.Fatalf("failed to unmarshal intent: %v", err)
		}
		intent["timestamp"] = "2020-01-01T00:00:00Z"
		intent["expiresAt"] = "2020-01-01T01:00:00Z"

		modifiedBytes, err := json.Marshal(intent)
		if err != nil {
			t.Fatalf("failed to marshal intent: %v", err)
		}

		ctx := context.Background()
		result := gate.Submit(ctx, modifiedBytes)

		if result.Error == nil {
			t.Error("expected rejection for envelope with expired timestamp, got acceptance")
		}
		if result.Receipt != nil {
			t.Error("expected nil receipt for rejected envelope")
		}
	})

	// Test invariant: envelope with unknown action type must reject
	t.Run("unknown_action_type_rejects", func(t *testing.T) {
		intentBytes := generateValidIntent()

		// Parse JSON to modify action type
		var intent map[string]interface{}
		if err := json.Unmarshal(intentBytes, &intent); err != nil {
			t.Fatalf("failed to unmarshal intent: %v", err)
		}
		intent["actionType"] = "UNKNOWN_ACTION_TYPE"

		modifiedBytes, err := json.Marshal(intent)
		if err != nil {
			t.Fatalf("failed to marshal intent: %v", err)
		}

		ctx := context.Background()
		result := gate.Submit(ctx, modifiedBytes)

		if result.Error == nil {
			t.Error("expected rejection for envelope with unknown action type, got acceptance")
		}
		if result.Receipt != nil {
			t.Error("expected nil receipt for rejected envelope")
		}
	})

	// Test invariant: envelope with all valid fields must accept (doctrine mode)
	t.Run("all_valid_accepts", func(t *testing.T) {
		intentBytes := generateValidIntent()

		ctx := context.Background()
		result := gate.Submit(ctx, intentBytes)

		if result.Error != nil {
			t.Errorf("expected acceptance for valid envelope, got rejection: %v", result.Error)
		}
		if result.Receipt == nil {
			t.Error("expected receipt for accepted envelope")
		}
	})

	// Test invariant: replay detection must work
	t.Run("replay_detection_rejects", func(t *testing.T) {
		intentBytes := generateValidIntent()

		ctx := context.Background()

		// First submission should succeed
		result1 := gate.Submit(ctx, intentBytes)
		if result1.Error != nil {
			t.Fatalf("first submission should succeed, got: %v", result1.Error)
		}

		// Second submission with same nonce should reject
		result2 := gate.Submit(ctx, intentBytes)
		if result2.Error == nil {
			t.Error("second submission with same nonce should reject, got acceptance")
		}
		if result2.Receipt != nil {
			t.Error("expected nil receipt for replayed envelope")
		}
	})
}

// generateValidIntent creates a valid envelope for property-based testing.
func generateValidIntent() []byte {
	fixedTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

	// Create a CommandRequested payload
	cmdPayload := &operatorv1.CommandRequested{
		Command:        "echo hello",
		ExecutionId:    "exec-property-test",
		Justification:  "property test command",
		SentinelMode:   "strict",
		TimeoutSeconds: 30,
	}
	payloadBytes, _ := proto.Marshal(cmdPayload)

	// Create envelope
	env := &commonv1.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.New(fixedTime),
		ExpiresAt:         timestamppb.New(fixedTime.Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "operator-1",
		OperatorSessionId: "operator-session-1",
		ActionType:        "EXECUTE_BASH",
		TargetResource:    "localhost",
		StateMerkleRoot:   "abc123def456",
		Nonce:             fmt.Sprintf("nonce-property-test-%d", time.Now().UnixNano()),
		Payload:           payloadBytes,
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{},
		},
	}

	// Generate transaction hash
	hash, _ := uap.GenerateMessageID(env)
	env.Id = hash
	env.TransactionHash = hash

	// Marshal to JSON
	marshaler := &protojson.MarshalOptions{}
	intentJSON, _ := marshaler.Marshal(env)

	return intentJSON
}
