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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/pkg/governance"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestL4Warden_Notary_ValidNonMutationPasses verifies that a valid
// non-mutation envelope passes under notary posture without L3 proof
// (L3 is only required for mutations).
func TestL4Warden_Notary_ValidNonMutationPasses(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "notary")
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("expected non-mutation under notary to pass without L3 proof, got %v", err)
	}
}

// TestL4Warden_Notary_ValidMutationPassesWithL3 verifies that a valid
// mutation envelope with L2 and L3 passes under notary posture.
func TestL4Warden_Notary_ValidMutationPassesWithL3(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "notary")
	env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey)

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("expected verification to pass, got %v", err)
	}
}

// TestL4Warden_Notary_AllActionTypesFromSSOT verifies that every action type
// from the SSOT can be decoded and verified under notary posture.
func TestL4Warden_Notary_AllActionTypesFromSSOT(t *testing.T) {
	t.Parallel()
	allActionTypes := constants.AllActionTypes
	if len(allActionTypes) == 0 {
		t.Fatal("AllActionTypes() returned empty list")
	}

	for _, actionType := range allActionTypes {
		t.Run(string(actionType), func(t *testing.T) {
			t.Parallel()
			verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "notary")
			payload := typedPayload(t, actionType)
			env := signedEnvelope(t, actionType, payload, privKey)

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

// TestL4Warden_Notary_FailClosedProofs verifies that structural and L2/L3
// proof failures are rejected under notary posture (all layers enforced).
func TestL4Warden_Notary_FailClosedProofs(t *testing.T) {
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

// TestNotary_MutationRequiresRealL3Proof guards the AutoApproved removal:
// a mutation under notary with no L3 proof must fail closed. There is no
// bypass field to set. This pairs with TestNoSelfSign_ConsensusWithoutDeliberator
// as a trust-boundary regression guard.
func TestNotary_MutationRequiresRealL3Proof(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "notary")
	env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey)

	env.Governance.L3 = nil

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if !errors.Is(err, constants.ErrTxL3ProofMissing) {
		t.Fatalf("expected ErrTxL3ProofMissing for mutation under notary with no L3 proof, got: %v", err)
	}
}

// TestL4Warden_Notary_ReplayAndStateRootReject verifies that replay
// attacks and state root mismatches are rejected under notary posture.
func TestL4Warden_Notary_ReplayAndStateRootReject(t *testing.T) {
	t.Parallel()
	t.Run("replayed nonce", func(t *testing.T) {
		t.Parallel()
		replayStore := testutil.NewStatefulMockReplayStore()
		verifier, privKey := createStrictVerifier(t, replayStore, testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "notary")
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
		verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("other-root"), testutil.NewConfigurableMockL3Notary(true), "notary")
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrStateRootMismatch) {
			t.Fatalf("expected state root mismatch, got %v", err)
		}
	})
}

// TestL4Warden_Notary_MissingVerifierDependenciesReject verifies that
// missing critical verifier dependencies are rejected under notary posture.
func TestL4Warden_Notary_MissingVerifierDependenciesReject(t *testing.T) {
	t.Parallel()
	t.Run("missing replay store", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, nil, testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "notary")
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

// TestL4Warden_Notary_NonceRaceCondition verifies that concurrent submissions
// of the same nonce are serialized — exactly one succeeds, the rest are
// rejected as replays.
func TestL4Warden_Notary_NonceRaceCondition(t *testing.T) {
	t.Parallel()
	replayStore := testutil.NewStatefulMockReplayStore()
	stateRootProvider := testutil.NewMockStateRootProvider("root-1")
	l3Notary := testutil.NewSlowMockL3Notary(50 * time.Millisecond)

	verifier, privKey := createStrictVerifier(t, replayStore, stateRootProvider, l3Notary, "notary")

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
	if replayCount != numConcurrent-1 {
		t.Errorf("expected exactly %d replays, got %d", numConcurrent-1, replayCount)
	}
}

// TestL4Warden_Notary_AppPolicyStore_L3Required_Mutation verifies that mutating
// intents NOT in AutoApproveIntents require L3 human presence verification.
func TestL4Warden_Notary_AppPolicyStore_L3Required_Mutation(t *testing.T) {
	t.Parallel()

	appPolicyStore := &SimpleAppPolicyStore{
		Policies: map[string]*models.AppPolicy{
			"spiffe://g8e.local/app/test-app-id": {
				AppID: "spiffe://g8e.local/app/test-app-id",
			},
		},
	}

	l3Notary := testutil.NewConfigurableMockL3Notary(false)
	verifier, privKey := createVerifierWithAppPolicyStore(t, appPolicyStore, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), l3Notary)

	actionType := constants.ActionTypeExecuteBash
	payload := typedPayload(t, actionType)
	env := signedEnvelopeWithAppID(t, actionType, payload, privKey, "spiffe://g8e.local/app/test-app-id")

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if err == nil {
		t.Fatalf("mutating action %s should require L3 proof when not in AutoApproveIntents", actionType)
	}
	if !errors.Is(err, ErrL3ProofMissing) && !errors.Is(err, ErrL3ProofInvalid) {
		t.Fatalf("expected L3 proof error for mutating action, got: %v", err)
	}
}

// TestL4Warden_Notary_AppPolicyStore_NoPolicy_Fallback verifies that when
// no policy is found for an app, the system falls back to requiring standard L3.
func TestL4Warden_Notary_AppPolicyStore_NoPolicy_Fallback(t *testing.T) {
	t.Parallel()

	appPolicyStore := &SimpleAppPolicyStore{
		Policies: map[string]*models.AppPolicy{},
	}

	l3Notary := testutil.NewConfigurableMockL3Notary(false)
	verifier, privKey := createVerifierWithAppPolicyStore(t, appPolicyStore, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), l3Notary)

	actionType := constants.ActionTypeExecuteBash
	payload := typedPayload(t, actionType)
	env := signedEnvelopeWithAppID(t, actionType, payload, privKey, "spiffe://g8e.local/app/test-app-id")

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if err == nil {
		t.Fatalf("action should require L3 when no app policy exists")
	}
	if !errors.Is(err, ErrL3ProofMissing) && !errors.Is(err, ErrL3ProofInvalid) {
		t.Fatalf("expected L3 proof error when no policy exists, got: %v", err)
	}
}
