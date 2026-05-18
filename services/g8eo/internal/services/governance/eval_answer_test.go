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
	"encoding/hex"
	"log/slog"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestEvalAnswerVerification tests that EVAL_ANSWER envelopes are accepted by the verifier
// and can be executed by the Warden with a signed receipt.
func TestEvalAnswerVerification(t *testing.T) {
	// Generate test key
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate signer: %v", err)
	}

	// Create verifier with EVAL_ANSWER in known action types
	verifier := NewTransactionVerifier(
		nil,
		newMockReplayStore(),
		&mockStateRootProvider{root: "test-state-root-v1"},
		&SimpleSignerStore{Signers: map[string]ed25519.PublicKey{"test-key-id": pubKey}},
		nil, // L3 verifier not needed for EVAL_ANSWER (non-mutation)
		[]constants.ActionType{constants.ActionTypeEvalAnswer},
	)

	// Create an EVAL_ANSWER payload
	payload := &operatorv1.EvalAnswerRequested{
		PromptId:  "test-prompt-001",
		Benchmark: "ifeval",
		Answer:    "This is a test answer.",
		Model:     "openai:gpt-4",
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	// Create envelope with proper structure
	envelope := &uap.UAPEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().UTC().Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_G8EE,
		OperatorId:        "operator-1",
		OperatorSessionId: "session-1",
		ActionType:        string(constants.ActionTypeEvalAnswer),
		TargetResource:    "localhost",
		Payload:           payloadBytes,
		StateMerkleRoot:   "test-state-root-v1",
		Nonce:             "test-nonce-001",
	}

	// Compute transaction hash
	computedHash, err := uap.GenerateMessageID(envelope)
	if err != nil {
		t.Fatalf("Failed to compute transaction hash: %v", err)
	}
	envelope.Id = computedHash
	envelope.TransactionHash = computedHash

	// Add L2 governance signature
	envelope.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			KeyId:             "test-key-id",
			TribunalSignature: hex.EncodeToString(ed25519.Sign(privKey, []byte(computedHash+"|true"))),
		},
	}

	// Verify the envelope
	verified, err := verifier.VerifyEnvelope(envelope)
	if err != nil {
		t.Fatalf("VerifyEnvelope failed: %v", err)
	}

	if verified.ActionType != constants.ActionTypeEvalAnswer {
		t.Errorf("Expected action type EVAL_ANSWER, got %s", verified.ActionType)
	}

	// Check that the decoded payload is correct
	evalPayload, ok := verified.DecodedPayload.(*operatorv1.EvalAnswerRequested)
	if !ok {
		t.Fatalf("Decoded payload is not EvalAnswerRequested, got %T", verified.DecodedPayload)
	}

	if evalPayload.PromptId != "test-prompt-001" {
		t.Errorf("Expected prompt_id test-prompt-001, got %s", evalPayload.PromptId)
	}

	if evalPayload.Benchmark != "ifeval" {
		t.Errorf("Expected benchmark ifeval, got %s", evalPayload.Benchmark)
	}

	if evalPayload.Answer != "This is a test answer." {
		t.Errorf("Expected answer 'This is a test answer.', got %s", evalPayload.Answer)
	}

	if evalPayload.Model != "openai:gpt-4" {
		t.Errorf("Expected model openai:gpt-4, got %s", evalPayload.Model)
	}

	// 4. Execute through Warden
	keyID := "test-key-id"
	warden := &Warden{
		Logger:            slog.Default(),
		SignerStore:       &SimpleSignerStore{Signers: map[string]ed25519.PublicKey{keyID: pubKey}},
		StateRootProvider: &mockStateRootProvider{root: "test-state-root-v1"},
		ExecutionHandler: &mockExecutionHandler{
			ExecuteVerifiedTransactionFunc: func(ctx context.Context, eventType constants.EventType, cmdMsg interface{}) (string, error) {
				return payload.Answer, nil
			},
		},
		SigningKey: privKey,
		KeyID:      keyID,
	}

	receipt, err := warden.Execute(context.Background(), verified, nil)
	if err != nil {
		t.Fatalf("Warden execution failed: %v", err)
	}

	if receipt.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		t.Errorf("Expected status COMPLETED, got %v", receipt.Status)
	}

	if receipt.ResultSummary != payload.Answer {
		t.Errorf("Expected result summary '%s', got '%s'", payload.Answer, receipt.ResultSummary)
	}

	if receipt.SignerKeyId != keyID {
		t.Errorf("Expected signer key ID %s, got %s", keyID, receipt.SignerKeyId)
	}

	if receipt.Signature == "" {
		t.Error("Expected non-empty signature in receipt")
	}
}

// TestEvalAnswerIsNotMutation verifies that EVAL_ANSWER is not treated as a mutation
// and does not require L3 verification.
func TestEvalAnswerIsNotMutation(t *testing.T) {
	verifier := &TransactionVerifier{}

	if verifier.isMutation(constants.ActionTypeEvalAnswer) {
		t.Error("EVAL_ANSWER should not be treated as a mutation")
	}

	// Verify that actual mutations are still detected
	if !verifier.isMutation(constants.ActionTypeExecuteBash) {
		t.Error("EXECUTE_BASH should be treated as a mutation")
	}

	if !verifier.isMutation(constants.ActionTypeFileEdit) {
		t.Error("FILE_EDIT should be treated as a mutation")
	}
}
