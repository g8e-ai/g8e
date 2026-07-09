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
	"encoding/hex"
	"log/slog"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	govtypes "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestEvalAnswerVerification tests that EVAL_ANSWER envelopes are accepted by the verifier
// and can be executed by the Actuator with a signed receipt.
func TestEvalAnswerVerification(t *testing.T) {
	// Generate test key
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err, "failed to generate signer")

	// Create verifier with EVAL_ANSWER in known action types
	verifier := NewL4Warden(
		nil,
		testutil.NewStatefulMockReplayStore(),
		testutil.NewMockStateRootProvider("test-state-root-v1"),
		&SimpleSignerStore{Signers: map[string]ed25519.PublicKey{"test-key-id": pubKey}},
		nil, // TribunalStore not used in tests
		nil, // AppPolicyStore not used in tests
		nil, // L3 verifier not needed for EVAL_ANSWER (non-mutation)
		nil, // doctrine defaults to L1Doctrine
		[]constants.ActionType{constants.ActionTypeEvalAnswer},
		"doctrine",
		nil, // Clock defaults to RealClock
	)

	// Create an EVAL_ANSWER payload
	payload := &operatorv1.EvalAnswerRequested{
		PromptId:  "test-prompt-001",
		Benchmark: "ifeval",
		Answer:    "This is a test answer.",
		Model:     "openai:gpt-4",
	}

	payloadBytes, err := proto.Marshal(payload)
	require.NoError(t, err, "Failed to marshal payload")

	// Create envelope with proper structure
	envelope := &govtypes.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().UTC().Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "operator-1",
		OperatorSessionId: "operator-session-1",
		ActionType:        string(constants.ActionTypeEvalAnswer),
		TargetResource:    "localhost",
		Payload:           payloadBytes,
		StateMerkleRoot:   "test-state-root-v1",
		Nonce:             "test-nonce-001",
	}

	// Compute transaction hash
	computedHash, err := govtypes.GenerateMessageID(envelope)
	require.NoError(t, err, "Failed to compute transaction hash")
	envelope.Id = computedHash
	envelope.TransactionHash = computedHash

	// Add L2 governance signature
	envelope.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			TribunalId: "test-tribunal",
			Votes: []*commonv1.L2Vote{
				{
					SignerKeyId:        "test-key-id",
					ConsensusSignature: hex.EncodeToString(ed25519.Sign(privKey, []byte(computedHash+"|true"))),
					Decision:           true,
				},
			},
		},
	}

	// Verify the envelope
	verified, err := verifier.VerifyEnvelope(context.Background(), envelope)
	require.NoError(t, err, "VerifyEnvelope failed")

	assert.Equal(t, constants.ActionTypeEvalAnswer, verified.ActionType, "Expected action type EVAL_ANSWER")

	// Check that the decoded payload is correct
	evalPayload, ok := verified.DecodedPayload.(*operatorv1.EvalAnswerRequested)
	require.True(t, ok, "Decoded payload is not EvalAnswerRequested, got %T", verified.DecodedPayload)

	assert.Equal(t, "test-prompt-001", evalPayload.PromptId, "Expected prompt_id test-prompt-001")
	assert.Equal(t, "ifeval", evalPayload.Benchmark, "Expected benchmark ifeval")
	assert.Equal(t, "This is a test answer.", evalPayload.Answer, "Expected answer 'This is a test answer.'")
	assert.Equal(t, "openai:gpt-4", evalPayload.Model, "Expected model openai:gpt-4")

	// 4. Execute through L5Actuator
	keyID := "test-key-id"
	actuator := &L5Actuator{
		Logger:            slog.Default(),
		StateRootProvider: testutil.NewMockStateRootProvider("test-state-root-v1"),
		ExecutionHandler: &mockExecutionHandler{
			ExecuteVerifiedTransactionFunc: func(ctx context.Context, eventType constants.EventType, cmdMsg interface{}) (string, error) {
				return payload.Answer, nil
			},
		},
		SigningKey: privKey,
		KeyID:      keyID,
	}

	receipt, err := actuator.Execute(context.Background(), verified, nil)
	require.NoError(t, err, "L5Actuator execution failed")

	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status, "Expected status COMPLETED")
	assert.Equal(t, payload.Answer, receipt.ResultSummary, "Expected result summary '%s'", payload.Answer)
	assert.Equal(t, keyID, receipt.SignerKeyId, "Expected signer key ID %s", keyID)
	assert.NotEmpty(t, receipt.Signature, "Expected non-empty signature in receipt")
}
