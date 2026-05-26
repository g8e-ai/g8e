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
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/services/system"
	"github.com/g8e-ai/g8e/pkg/uap"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// TestConcurrencyReplayDetection tests actual replay detection with concurrent submissions.
// It submits the same valid envelope twice concurrently using goroutines and asserts that
// exactly one succeeds and one rejects with TX_REPLAY.
func TestConcurrencyReplayDetection(t *testing.T) {
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

	// Create a valid envelope with a fixed nonce
	intentBytes := generateValidIntentWithNonce("nonce-concurrency-test-123")

	ctx := context.Background()

	// Submit the same envelope twice concurrently
	var wg sync.WaitGroup
	results := make(chan Result, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := gate.Submit(ctx, intentBytes)
			results <- result
		}()
	}

	// Wait for both submissions to complete
	wg.Wait()
	close(results)

	// Collect results
	var result1, result2 Result
	for result := range results {
		if result1.Error == nil && result1.Receipt == nil {
			result1 = result
		} else {
			result2 = result
		}
	}

	// Assert that exactly one succeeded and one failed
	successCount := 0
	rejectCount := 0

	if result1.Error == nil && result1.Receipt != nil {
		successCount++
	} else {
		rejectCount++
	}

	if result2.Error == nil && result2.Receipt != nil {
		successCount++
	} else {
		rejectCount++
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 success, got %d", successCount)
	}
	if rejectCount != 1 {
		t.Errorf("expected exactly 1 rejection, got %d", rejectCount)
	}

	// Assert that the rejection was due to replay
	var rejectedResult Result
	if result1.Error != nil {
		rejectedResult = result1
	} else {
		rejectedResult = result2
	}

	if rejectedResult.Error == nil {
		t.Error("expected one result to have an error")
	} else {
		errMsg := rejectedResult.Error.Error()
		if !strings.HasPrefix(errMsg, "TX_REPLAY") {
			t.Errorf("expected rejection reason to start with 'TX_REPLAY', got %q", errMsg)
		}
	}
}

// generateValidIntentWithNonce creates a valid envelope with a specific nonce for concurrency testing.
func generateValidIntentWithNonce(nonce string) []byte {
	fixedTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

	// Create a CommandRequested payload
	cmdPayload := &operatorv1.CommandRequested{
		Command:        "echo hello",
		ExecutionId:    "exec-concurrency-test",
		Justification:  "concurrency test command",
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
		Nonce:             nonce,
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
