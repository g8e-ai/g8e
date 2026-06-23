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
	"errors"
	"fmt"
	"path/filepath"
	"sync"
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

	"github.com/stretchr/testify/assert"
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
	case constants.ActionTypeGrantIntent:
		msg = &operatorv1.GrantIntentRequested{IntentName: "test", ExecutionId: "exec-1"}
	case constants.ActionTypeRevokeIntent:
		msg = &operatorv1.RevokeIntentRequested{IntentName: "test", ExecutionId: "exec-1"}
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
	hash, err := governance.GenerateMessageID(env)
	if err != nil {
		t.Fatalf("failed to generate transaction hash: %v", err)
	}
	env.Id = hash
	env.TransactionHash = hash
	env.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			TribunalId: "test-tribunal",
			Votes: []*commonv1.L2Vote{
				signL2Vote(privKey, "test-key", hash, true),
			},
		},
	}
	// Add L3 proof for mutation actions
	if actionType.IsMutation() {
		env.Governance.L3 = &commonv1.L3Metadata{
			Proof: &commonv1.L3Proof{
				Signature: "human-proof",
			},
		}
	}
	return env
}

func TestL4Warden_AcceptsValidNonMutationGovernanceEnvelope(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "doctrine")
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)

	verified, err := verifier.VerifyEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("expected verification to pass, got %v", err)
	}
	if verified.DecodedPayload == nil || verified.ActionType != constants.ActionTypeFsList {
		t.Fatalf("verified transaction missing decoded payload or action: %#v", verified)
	}
}

func TestL4Warden_AcceptsValidMutationGovernanceEnvelopeWithL3(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "notary")
	env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey)

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("expected verification to pass, got %v", err)
	}
}

func TestL4Warden_FailClosedProofs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*governance.GovernanceEnvelope)
		want   error
	}{
		{name: "missing id", mutate: func(env *governance.GovernanceEnvelope) { env.Id = "" }, want: ErrTransactionIDMissing},
		{name: "unknown action", mutate: func(env *governance.GovernanceEnvelope) { env.ActionType = "UNKNOWN" }, want: ErrUnknownActionType},
		{name: "missing payload", mutate: func(env *governance.GovernanceEnvelope) { env.Payload = nil }, want: ErrPayloadMissing},
		{name: "invalid typed payload", mutate: func(env *governance.GovernanceEnvelope) { env.Payload = []byte("not protobuf") }, want: ErrPayloadDecodeFailed},
		{name: "missing transaction hash", mutate: func(env *governance.GovernanceEnvelope) { env.TransactionHash = "" }, want: ErrTransactionHashMissing},
		{name: "hash mismatch", mutate: func(env *governance.GovernanceEnvelope) { env.TransactionHash = "wrong" }, want: ErrTransactionHashMismatch},
		{name: "expired", mutate: func(env *governance.GovernanceEnvelope) {
			env.ExpiresAt = timestamppb.New(time.Now().UTC().Add(-time.Minute))
			rehash(t, env)
		}, want: ErrTransactionExpired},
		{name: "missing nonce", mutate: func(env *governance.GovernanceEnvelope) { env.Nonce = ""; rehash(t, env) }, want: ErrNonceMissing},
		{name: "missing state root", mutate: func(env *governance.GovernanceEnvelope) { env.StateMerkleRoot = ""; rehash(t, env) }, want: ErrStateRootRequired},
		{name: "missing l2", mutate: func(env *governance.GovernanceEnvelope) { env.Governance.L2 = nil }, want: ErrL2SignatureMissing},
		{name: "non-member signer", mutate: func(env *governance.GovernanceEnvelope) { env.Governance.L2.Votes[0].SignerKeyId = "" }, want: ErrL2QuorumNotMet},
		{name: "invalid l2 signature", mutate: func(env *governance.GovernanceEnvelope) { env.Governance.L2.Votes[0].ConsensusSignature = "deadbeef" }, want: ErrL2QuorumNotMet},
		{name: "missing l3", mutate: func(env *governance.GovernanceEnvelope) { env.Governance.L3 = nil }, want: ErrL3ProofMissing},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "notary")
			env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey)
			tc.mutate(env)

			_, err := verifier.VerifyEnvelope(context.Background(), env)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestL4Warden_ReplayAndStateRootReject(t *testing.T) {
	t.Parallel()
	t.Run("replayed nonce", func(t *testing.T) {
		t.Parallel()
		replayStore := testutil.NewStatefulMockReplayStore()
		verifier, privKey := createStrictVerifier(t, replayStore, testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "doctrine")
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)
		if _, err := verifier.VerifyEnvelope(context.Background(), env); err != nil {
			t.Fatalf("first verification failed: %v", err)
		}
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrTransactionReplay) {
			t.Fatalf("expected replay rejection, got %v", err)
		}
	})

	t.Run("state root mismatch", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("other-root"), testutil.NewConfigurableMockL3Notary(true), "doctrine")
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrStateRootMismatch) {
			t.Fatalf("expected state root mismatch, got %v", err)
		}
	})
}

func TestL4Warden_MissingVerifierDependenciesReject(t *testing.T) {
	t.Parallel()
	t.Run("missing replay store", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, nil, testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "doctrine")
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrReplayStoreMissing) {
			t.Fatalf("expected replay store rejection, got %v", err)
		}
	})

	t.Run("missing state root provider", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), nil, testutil.NewConfigurableMockL3Notary(true), "notary")
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrStateRootMissing) {
			t.Fatalf("expected state root provider rejection, got %v", err)
		}
	})

	t.Run("missing l3 notary", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), nil, "notary")
		env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey)
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrL3NotaryNotConfigured) {
			t.Fatalf("expected l3 notary rejection, got %v", err)
		}
	})
}

func TestL4Warden_NonceRaceCondition(t *testing.T) {
	t.Parallel()
	replayStore := testutil.NewStatefulMockReplayStore()
	stateRootProvider := testutil.NewMockStateRootProvider("root-1")

	// Slow mock notary to hold transactions in-flight
	l3Notary := testutil.NewSlowMockL3Notary(50 * time.Millisecond)

	verifier, privKey := createStrictVerifier(t, replayStore, stateRootProvider, l3Notary, "notary")

	// Prepare an envelope
	payload := typedPayload(t, constants.ActionTypeExecuteBash)
	env := signedEnvelope(t, constants.ActionTypeExecuteBash, payload, privKey)

	const numConcurrent = 5
	var wg sync.WaitGroup
	errs := make(chan error, numConcurrent)
	successes := make(chan bool, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := verifier.VerifyEnvelope(context.Background(), env)
			if err != nil {
				errs <- err
			} else {
				successes <- true
			}
		}()
	}

	wg.Wait()
	close(errs)
	close(successes)

	successCount := 0
	for range successes {
		successCount++
	}

	replayCount := 0
	for err := range errs {
		if errors.Is(err, ErrTransactionReplay) || errors.Is(err, ErrTxInFlight) {
			replayCount++
		}
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 success, got %d", successCount)
	}
	// The mutex + SQLite replay store should prevent all concurrent identical requests
	// except one. All remaining should be rejected as replays (either in-flight or SQLite).
	if replayCount != numConcurrent-1 {
		t.Errorf("expected exactly %d replays, got %d", numConcurrent-1, replayCount)
	}
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

// TestL4Warden_AllActionTypesFromSSOT verifies that every action type
// defined in the SSOT (constants.AllActionTypes) can be successfully decoded
// and verified. This prevents action type drift where new action types are added
// to constants but not to the decodePayloadForAction switch.
func TestL4Warden_AllActionTypesFromSSOT(t *testing.T) {
	t.Parallel()
	allActionTypes := constants.AllActionTypes
	if len(allActionTypes) == 0 {
		t.Fatal("AllActionTypes() returned empty list")
	}

	for _, actionType := range allActionTypes {
		t.Run(string(actionType), func(t *testing.T) {
			t.Parallel()
			verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "doctrine")
			payload := typedPayload(t, actionType)
			env := signedEnvelope(t, actionType, payload, privKey)

			// signedEnvelope now adds L3 for mutation actions, so no manual adjustment needed

			verified, err := verifier.VerifyEnvelope(context.Background(), env)
			if err != nil {
				t.Fatalf("verification failed for action type %s: %v", actionType, err)
			}
			if verified == nil {
				t.Fatalf("verified transaction is nil for action type %s", actionType)
				return
			}
			if verified.ActionType != actionType {
				t.Fatalf("action type mismatch: expected %s, got %s", actionType, verified.ActionType)
			}
		})
	}
}

// TestL4Warden_AppPolicyStore_L3Required_Mutation verifies that mutating
// intents NOT in AutoApproveIntents require L3 human presence verification.
func TestL4Warden_AppPolicyStore_L3Required_Mutation(t *testing.T) {
	t.Parallel()

	// Create an AppPolicyStore
	appPolicyStore := &SimpleAppPolicyStore{
		Policies: map[string]*models.AppPolicy{
			"spiffe://g8e.local/app/test-app-id": {
				AppID: "spiffe://g8e.local/app/test-app-id",
			},
		},
	}

	// Provide a mock L3 notary that rejects all proofs
	l3Notary := testutil.NewConfigurableMockL3Notary(false)
	verifier, privKey := createVerifierWithAppPolicyStore(t, appPolicyStore, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), l3Notary)

	// Test a mutating action not in AutoApproveIntents
	actionType := constants.ActionTypeExecuteBash
	payload := typedPayload(t, actionType)
	env := signedEnvelopeWithAppID(t, actionType, payload, privKey, "spiffe://g8e.local/app/test-app-id")

	// Mutating action should require L3 proof
	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if err == nil {
		t.Fatalf("mutating action %s should require L3 proof when not in AutoApproveIntents", actionType)
	}
	if !errors.Is(err, ErrL3ProofMissing) && !errors.Is(err, ErrL3ProofInvalid) {
		t.Fatalf("expected L3 proof error for mutating action, got: %v", err)
	}
}

// TestL4Warden_AppPolicyStore_NoPolicy_Fallback verifies that when
// no policy is found for an app, the system falls back to requiring standard L3.
func TestL4Warden_AppPolicyStore_NoPolicy_Fallback(t *testing.T) {
	t.Parallel()

	// Create an AppPolicyStore with no policy for the app
	appPolicyStore := &SimpleAppPolicyStore{
		Policies: map[string]*models.AppPolicy{},
	}

	// Provide a mock L3 notary that rejects all proofs
	l3Notary := testutil.NewConfigurableMockL3Notary(false)
	verifier, privKey := createVerifierWithAppPolicyStore(t, appPolicyStore, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), l3Notary)

	// Test a mutating action that would normally require L3
	actionType := constants.ActionTypeExecuteBash
	payload := typedPayload(t, actionType)
	env := signedEnvelopeWithAppID(t, actionType, payload, privKey, "spiffe://g8e.local/app/test-app-id")

	// Should require L3 when no policy exists
	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if err == nil {
		t.Fatalf("action should require L3 when no app policy exists")
	}
	if !errors.Is(err, ErrL3ProofMissing) && !errors.Is(err, ErrL3ProofInvalid) {
		t.Fatalf("expected L3 proof error when no policy exists, got: %v", err)
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

func TestL4Warden_L2QuorumVerification(t *testing.T) {
	t.Parallel()

	pub1, priv1, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pub2, priv2, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	allSigners := map[string]ed25519.PublicKey{
		"member-1": pub1,
		"member-2": pub2,
	}
	partialSigners := map[string]ed25519.PublicKey{
		"member-1": pub1,
	}

	enabledTribunal2of2 := &models.TribunalPolicy{
		ID:              "trib-1",
		MemberAppIDs:    []string{"member-1", "member-2"},
		Quorum:          2,
		RequireDistinct: true,
		Enabled:         true,
	}
	enabledTribunal1of2 := &models.TribunalPolicy{
		ID:              "trib-2",
		MemberAppIDs:    []string{"member-1", "member-2"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	disabledTribunal := &models.TribunalPolicy{
		ID:              "trib-disabled",
		MemberAppIDs:    []string{"member-1", "member-2"},
		Quorum:          2,
		RequireDistinct: true,
		Enabled:         false,
	}

	payload := typedPayload(t, constants.ActionTypeFsList)

	buildEnv := func(nonceTag, tribunalID string, votes []*commonv1.L2Vote) *governance.GovernanceEnvelope {
		nonceSuffix := hex.EncodeToString(payload)
		if len(nonceSuffix) > 8 {
			nonceSuffix = nonceSuffix[:8]
		}
		env := &governance.GovernanceEnvelope{
			ProtocolVersion:   "1.0",
			Timestamp:         timestamppb.Now(),
			ExpiresAt:         timestamppb.New(time.Now().UTC().Add(time.Hour)),
			SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
			OperatorId:        "operator-1",
			OperatorSessionId: "operator-session-1",
			ActionType:        string(constants.ActionTypeFsList),
			TargetResource:    "localhost",
			Payload:           payload,
			StateMerkleRoot:   "root-1",
			Nonce:             "nonce-quorum-" + nonceTag + "-" + nonceSuffix,
		}
		hash, err := governance.GenerateMessageID(env)
		if err != nil {
			t.Fatalf("failed to generate hash: %v", err)
		}
		env.Id = hash
		env.TransactionHash = hash
		env.Governance = &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				TribunalId: tribunalID,
				Votes:      votes,
			},
		}
		return env
	}

	makeVerifier := func(signers map[string]ed25519.PublicKey, tribunals map[string]*models.TribunalPolicy) *L4Warden {
		return NewL4Warden(
			testutil.NewTestLogger(),
			testutil.NewStatefulMockReplayStore(),
			testutil.NewMockStateRootProvider("root-1"),
			&SimpleSignerStore{Signers: signers},
			&SimpleTribunalStore{Tribunals: tribunals},
			nil,
			testutil.NewConfigurableMockL3Notary(true),
			nil,
			constants.AllActionTypes,
			"consensus",
			nil,
		)
	}

	tests := []struct {
		name     string
		verifier *L4Warden
		env      *governance.GovernanceEnvelope
		wantErr  error
		wantL2   bool
	}{
		{
			name:     "2-of-2 quorum pass",
			verifier: makeVerifier(allSigners, map[string]*models.TribunalPolicy{"trib-1": enabledTribunal2of2}),
			env: func() *governance.GovernanceEnvelope {
				env := buildEnv("2of2pass", "trib-1", nil)
				hash := env.TransactionHash
				env.Governance.L2.Votes = []*commonv1.L2Vote{
					signL2Vote(priv1, "member-1", hash, true),
					signL2Vote(priv2, "member-2", hash, true),
				}
				return env
			}(),
			wantErr: nil,
			wantL2:  true,
		},
		{
			name:     "1 valid of quorum-2 fails",
			verifier: makeVerifier(allSigners, map[string]*models.TribunalPolicy{"trib-1": enabledTribunal2of2}),
			env: func() *governance.GovernanceEnvelope {
				env := buildEnv("1of2fail", "trib-1", nil)
				hash := env.TransactionHash
				env.Governance.L2.Votes = []*commonv1.L2Vote{
					signL2Vote(priv1, "member-1", hash, true),
				}
				return env
			}(),
			wantErr: ErrL2QuorumNotMet,
			wantL2:  false,
		},
		{
			name:     "duplicate signer with require_distinct",
			verifier: makeVerifier(allSigners, map[string]*models.TribunalPolicy{"trib-1": enabledTribunal2of2}),
			env: func() *governance.GovernanceEnvelope {
				env := buildEnv("dupsign", "trib-1", nil)
				hash := env.TransactionHash
				env.Governance.L2.Votes = []*commonv1.L2Vote{
					signL2Vote(priv1, "member-1", hash, true),
					signL2Vote(priv1, "member-1", hash, true),
				}
				return env
			}(),
			wantErr: ErrL2DuplicateSigner,
			wantL2:  false,
		},
		{
			name:     "false vote does not count toward quorum",
			verifier: makeVerifier(allSigners, map[string]*models.TribunalPolicy{"trib-1": enabledTribunal2of2}),
			env: func() *governance.GovernanceEnvelope {
				env := buildEnv("falsevote", "trib-1", nil)
				hash := env.TransactionHash
				env.Governance.L2.Votes = []*commonv1.L2Vote{
					signL2Vote(priv1, "member-1", hash, true),
					signL2Vote(priv2, "member-2", hash, false),
				}
				return env
			}(),
			wantErr: ErrL2QuorumNotMet,
			wantL2:  false,
		},
		{
			name:     "unknown signer ignored, quorum-1 passes",
			verifier: makeVerifier(partialSigners, map[string]*models.TribunalPolicy{"trib-2": enabledTribunal1of2}),
			env: func() *governance.GovernanceEnvelope {
				env := buildEnv("unknownsigner", "trib-2", nil)
				hash := env.TransactionHash
				env.Governance.L2.Votes = []*commonv1.L2Vote{
					signL2Vote(priv1, "member-1", hash, true),
					signL2Vote(priv2, "member-2", hash, true),
				}
				return env
			}(),
			wantErr: nil,
			wantL2:  true,
		},
		{
			name:     "empty votes under consensus posture",
			verifier: makeVerifier(allSigners, map[string]*models.TribunalPolicy{"trib-1": enabledTribunal2of2}),
			env:      buildEnv("emptyvotes", "trib-1", []*commonv1.L2Vote{}),
			wantErr:  ErrL2SignatureMissing,
			wantL2:   false,
		},
		{
			name:     "disabled tribunal policy",
			verifier: makeVerifier(allSigners, map[string]*models.TribunalPolicy{"trib-disabled": disabledTribunal}),
			env: func() *governance.GovernanceEnvelope {
				env := buildEnv("disabledtrib", "trib-disabled", nil)
				hash := env.TransactionHash
				env.Governance.L2.Votes = []*commonv1.L2Vote{
					signL2Vote(priv1, "member-1", hash, true),
					signL2Vote(priv2, "member-2", hash, true),
				}
				return env
			}(),
			wantErr: ErrL2TribunalNotConfigured,
			wantL2:  false,
		},
		{
			name:     "unknown tribunal ID",
			verifier: makeVerifier(allSigners, map[string]*models.TribunalPolicy{"trib-1": enabledTribunal2of2}),
			env: func() *governance.GovernanceEnvelope {
				env := buildEnv("unknowntrib", "nonexistent-trib", nil)
				hash := env.TransactionHash
				env.Governance.L2.Votes = []*commonv1.L2Vote{
					signL2Vote(priv1, "member-1", hash, true),
					signL2Vote(priv2, "member-2", hash, true),
				}
				return env
			}(),
			wantErr: ErrL2TribunalNotConfigured,
			wantL2:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verified, err := tc.verifier.VerifyEnvelope(context.Background(), tc.env)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
			if verified != nil {
				assert.Equal(t, tc.wantL2, verified.L2Valid)
			}
		})
	}
}
