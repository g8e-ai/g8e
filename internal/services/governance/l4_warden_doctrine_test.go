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
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// TestL4Warden_Doctrine_ValidNonMutationPasses verifies that a valid
// non-mutation envelope passes under doctrine posture.
func TestL4Warden_Doctrine_ValidNonMutationPasses(t *testing.T) {
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

// TestL4Warden_Doctrine_ValidMutationPassesWithoutL2L3 verifies that a
// mutation envelope passes under doctrine even without L2 and L3 proofs,
// since doctrine only enforces L1 (L2/L3 are audited but do not gate).
func TestL4Warden_Doctrine_ValidMutationPassesWithoutL2L3(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "doctrine")
	env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey)

	env.Governance.L3 = nil
	env.Governance.L2 = nil
	rehash(t, env)

	verified, err := verifier.VerifyEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("expected mutation to pass under doctrine without L2/L3, got %v", err)
	}
	if verified.L2Valid {
		t.Fatalf("expected L2Valid=false under doctrine with no L2 proof")
	}
	if verified.L3Valid {
		t.Fatalf("expected L3Valid=false under doctrine with no L3 proof")
	}
}

// TestL4Warden_Doctrine_AllActionTypesFromSSOT verifies that every action type
// from the SSOT can be decoded and verified under doctrine posture.
func TestL4Warden_Doctrine_AllActionTypesFromSSOT(t *testing.T) {
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

// TestL4Warden_Doctrine_MissingL2DoesNotReject verifies that missing L2
// votes do not reject an envelope under doctrine (L2 is audited, not enforced).
func TestL4Warden_Doctrine_MissingL2DoesNotReject(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "doctrine")
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)

	env.Governance.L2 = nil

	verified, err := verifier.VerifyEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("expected non-mutation to pass under doctrine without L2, got %v", err)
	}
	if verified.L2Valid {
		t.Fatalf("expected L2Valid=false with no L2 proof")
	}
}

// TestL4Warden_Doctrine_MissingL3DoesNotReject verifies that missing L3
// proof does not reject a mutation under doctrine (L3 is audited, not enforced).
func TestL4Warden_Doctrine_MissingL3DoesNotReject(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true), "doctrine")
	env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey)

	env.Governance.L3 = nil
	rehashAndResign(t, env, privKey)

	verified, err := verifier.VerifyEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("expected mutation to pass under doctrine without L3, got %v", err)
	}
	if verified.L3Valid {
		t.Fatalf("expected L3Valid=false with no L3 proof")
	}
}

// TestL4Warden_Doctrine_ReplayAndStateRootReject verifies that replay
// attacks and state root mismatches are rejected under doctrine posture.
func TestL4Warden_Doctrine_ReplayAndStateRootReject(t *testing.T) {
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

// TestL4Warden_Doctrine_MissingVerifierDependenciesReject verifies that
// missing critical verifier dependencies are rejected under doctrine posture.
func TestL4Warden_Doctrine_MissingVerifierDependenciesReject(t *testing.T) {
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
		verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), nil, testutil.NewConfigurableMockL3Notary(true), "doctrine")
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey)
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrStateRootMissing) {
			t.Fatalf("expected state root provider rejection, got %v", err)
		}
	})
}
