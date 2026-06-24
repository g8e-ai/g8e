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
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func createStrictVerifier(t *testing.T, replayStore ReplayStore, stateRootProvider StateRootProvider, l3Notary L3Notary, posture string) (*L4Warden, ed25519.PrivateKey) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate signer: %v", err)
	}
	return NewL4Warden(
		testutil.NewTestLogger(),
		replayStore,
		stateRootProvider,
		&SimpleSignerStore{Signers: map[string]ed25519.PublicKey{"test-key": pubKey}},
		&SimpleTribunalStore{Tribunals: map[string]*models.TribunalPolicy{
			"test-tribunal": {
				ID:              "test-tribunal",
				MemberAppIDs:    []string{"test-key"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
		}},
		nil, // AppPolicyStore not used in tests
		l3Notary,
		nil,                      // doctrine defaults to L1Doctrine
		constants.AllActionTypes, // Use SSOT for action types
		posture,
		nil, // Clock defaults to RealClock
	), privKey
}

func typedPayload(t *testing.T, actionType constants.ActionType) []byte {
	t.Helper()
	tmpDir := t.TempDir()
	var msg proto.Message
	switch actionType {
	case constants.ActionTypeExecuteBash:
		msg = &operatorv1.CommandRequested{Command: "uptime", ExecutionId: "exec-1", Justification: "test"}
	case constants.ActionTypeFileEdit:
		msg = &operatorv1.FileEditRequested{FilePath: filepath.Join(tmpDir, "test"), Content: "test", ExecutionId: "exec-1"}
	case constants.ActionTypeFsList:
		msg = &operatorv1.FsListRequested{Path: ".", ExecutionId: "exec-1"}
	case constants.ActionTypeFsRead:
		msg = &operatorv1.FsReadRequested{Path: filepath.Join(tmpDir, "test"), ExecutionId: "exec-1"}
	case constants.ActionTypeFsGrep:
		msg = &operatorv1.FsGrepRequested{Path: ".", Pattern: "test", ExecutionId: "exec-1"}
	case constants.ActionTypePortCheck:
		msg = &operatorv1.CheckPortRequested{Port: 8080, ExecutionId: "exec-1"}
	case constants.ActionTypeFetchLogs:
		msg = &operatorv1.FetchLogsRequested{ExecutionId: "exec-1"}
	case constants.ActionTypeFetchHistory:
		msg = &operatorv1.FetchHistoryRequested{ExecutionId: "exec-1"}
	case constants.ActionTypeFetchFileHistory:
		msg = &operatorv1.FetchFileHistoryRequested{FilePath: filepath.Join(tmpDir, "test"), ExecutionId: "exec-1"}
	case constants.ActionTypeRestoreFile:
		msg = &operatorv1.RestoreFileRequested{FilePath: filepath.Join(tmpDir, "test"), ExecutionId: "exec-1"}
	case constants.ActionTypeFetchFileDiff:
		msg = &operatorv1.FetchFileDiffRequested{FilePath: filepath.Join(tmpDir, "test"), ExecutionId: "exec-1"}
	case constants.ActionTypeShutdown:
		msg = &operatorv1.ShutdownRequested{Reason: "test"}
	case constants.ActionTypeHeartbeat:
		msg = &operatorv1.HeartbeatRequested{}
	case constants.ActionTypeEvalAnswer:
		msg = &operatorv1.EvalAnswerRequested{PromptId: "test", Benchmark: "test", Model: "test", Answer: "test"}
	case constants.ActionTypeMcpCall:
		msg = &operatorv1.McpCallRequested{ToolName: "test", ArgumentsJson: "{}", ExecutionId: "exec-1"}
	case constants.ActionTypeA2aCall:
		msg = &operatorv1.A2ACallRequested{SkillName: "test", PayloadJson: "{}", ExecutionId: "exec-1"}
	case constants.ActionTypeMcpResourceList:
		msg = &operatorv1.McpResourceListRequested{ExecutionId: "exec-1"}
	case constants.ActionTypeMcpResourceRead:
		msg = &operatorv1.McpResourceReadRequested{Uri: "test://resource", ExecutionId: "exec-1"}
	case constants.ActionTypeMcpPromptList:
		msg = &operatorv1.McpPromptListRequested{ExecutionId: "exec-1"}
	case constants.ActionTypeMcpPromptGet:
		msg = &operatorv1.McpPromptGetRequested{Name: "test", ExecutionId: "exec-1"}
	case constants.ActionTypeInvestigationCreate:
		// INVESTIGATION_CREATE has no typed payload, uses raw bytes
		return []byte(`{"test": "data"}`)
	case constants.ActionTypeCancel:
		msg = &operatorv1.CommandCancelRequested{ExecutionId: "exec-1"}
	default:
		t.Fatalf("unsupported action type: %v", actionType)
	}
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	return payload
}

// signL2Vote creates an L2Vote with a signature derived from the Decision field,
// preventing decision/signature mismatch bugs where the signed string and the
// Decision field diverge.
func signL2Vote(privKey ed25519.PrivateKey, keyID, hash string, decision bool) *commonv1.L2Vote {
	sig := ed25519.Sign(privKey, []byte(fmt.Sprintf("%s|%v", hash, decision)))
	return &commonv1.L2Vote{
		SignerKeyId:        keyID,
		ConsensusSignature: hex.EncodeToString(sig),
		Decision:           decision,
	}
}

func signedEnvelope(t *testing.T, actionType constants.ActionType, payload []byte, privKey ed25519.PrivateKey) *governance.GovernanceEnvelope {
	t.Helper()
	// Generate a safe nonce from action type and payload (handle empty payloads)
	nonceSuffix := hex.EncodeToString(payload)
	if len(nonceSuffix) > 8 {
		nonceSuffix = nonceSuffix[:8]
	}
	if nonceSuffix == "" {
		nonceSuffix = "empty"
	}

	env := &governance.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().UTC().Add(time.Hour)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        "operator-1",
		OperatorSessionId: "operator-session-1",
		ActionType:        string(actionType),
		TargetResource:    "localhost",
		Payload:           payload,
		StateMerkleRoot:   "root-1",
		Nonce:             "nonce-" + string(actionType) + "-" + nonceSuffix,
	}
	// Compute transaction hash before any governance metadata.
	// Protocol ordering: L1 → L2 → L3 → L4. L2 signs the hash, then L3 is added.
	hash, err := governance.GenerateMessageID(env)
	if err != nil {
		t.Fatalf("failed to generate transaction hash: %v", err)
	}
	env.Id = hash
	env.TransactionHash = hash

	// L2 signs the hash (machine consensus before human notary)
	env.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			TribunalId: "test-tribunal",
			Votes: []*commonv1.L2Vote{
				signL2Vote(privKey, "test-key", hash, true),
			},
		},
	}

	// L3 proof added after L2 signs (human notary after machine consensus)
	if actionType.IsMutation() {
		env.Governance.L3 = &commonv1.L3Metadata{
			Proof: &commonv1.L3Proof{
				Signature: "human-proof",
			},
		}
	}
	return env
}

func rehash(t *testing.T, env *governance.GovernanceEnvelope) {
	t.Helper()
	hash, err := governance.GenerateMessageID(env)
	if err != nil {
		t.Fatalf("failed to regenerate transaction hash: %v", err)
	}
	env.Id = hash
	env.TransactionHash = hash
}

// TestNewGovernancePosture_PanicsOnInvalidPosture verifies that invalid posture
// strings cause a panic at startup rather than silently defaulting.
func TestNewGovernancePosture_PanicsOnInvalidPosture(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for invalid posture, but did not panic")
		}
	}()
	NewGovernancePosture("invalid-posture")
}

// TestNewGovernancePosture_AcceptsValidPostures verifies that all valid posture
// strings are accepted without panicking.
func TestNewGovernancePosture_AcceptsValidPostures(t *testing.T) {
	t.Parallel()
	validPostures := []string{"doctrine", "consensus", "notary"}
	for _, posture := range validPostures {
		t.Run(posture, func(t *testing.T) {
			t.Parallel()
			p := NewGovernancePosture(posture)
			if p == nil {
				t.Errorf("expected non-nil posture for %s", posture)
			}
			if p.Name() != posture {
				t.Errorf("expected posture name %s, got %s", posture, p.Name())
			}
		})
	}
}

// createVerifierWithAppPolicyStore creates a L4Warden with a custom AppPolicyStore.
func createVerifierWithAppPolicyStore(t *testing.T, appPolicyStore AppPolicyStore, replayStore ReplayStore, stateRootProvider StateRootProvider, l3Notary L3Notary) (*L4Warden, ed25519.PrivateKey) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate signer: %v", err)
	}
	return NewL4Warden(
		testutil.NewTestLogger(),
		replayStore,
		stateRootProvider,
		&SimpleSignerStore{Signers: map[string]ed25519.PublicKey{"spiffe://g8e.local/app/test-app-id": pubKey}},
		&SimpleTribunalStore{Tribunals: map[string]*models.TribunalPolicy{
			"test-tribunal": {
				ID:              "test-tribunal",
				MemberAppIDs:    []string{"spiffe://g8e.local/app/test-app-id"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
		}},
		appPolicyStore,
		l3Notary,
		nil, // doctrine defaults to L1Doctrine
		constants.AllActionTypes,
		"notary",
		nil, // Clock defaults to RealClock
	), privKey
}

// signedEnvelopeWithAppID creates a signed envelope with a specific L2 KeyId.
func signedEnvelopeWithAppID(t *testing.T, actionType constants.ActionType, payload []byte, privKey ed25519.PrivateKey, appID string) *governance.GovernanceEnvelope {
	t.Helper()
	env := signedEnvelope(t, actionType, payload, privKey)
	if env.Governance != nil && env.Governance.L2 != nil && len(env.Governance.L2.Votes) > 0 {
		env.Governance.L2.Votes[0].SignerKeyId = appID
	}
	rehash(t, env)
	return env
}
