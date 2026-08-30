// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// TestL4Warden_Doctrine_ValidNonMutationPasses verifies that a valid
// non-mutation envelope passes under doctrine posture.
func TestL4Warden_Doctrine_ValidNonMutationPasses(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "doctrine")

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
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey, "doctrine")

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
			verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
			payload := typedPayload(t, actionType)
			env := signedEnvelope(t, actionType, payload, privKey, "doctrine")

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
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "doctrine")

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
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey, "doctrine")

	env.Governance.L3 = nil

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
		verifier, privKey := createStrictVerifier(t, replayStore, testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "doctrine")
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
		verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("other-root"), testutil.NewConfigurableMockL3Notary(true))
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "doctrine")
		rejected, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrStateRootMismatch) {
			t.Fatalf("expected state root mismatch, got %v", err)
		}
		require.NotNil(t, rejected)
		require.Len(t, rejected.DeterministicStageEvidence, 2)
		assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, rejected.DeterministicStageEvidence[0].Kind)
		assert.Equal(t, operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, rejected.DeterministicStageEvidence[0].Outcome)
		assert.Equal(t, operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, rejected.DeterministicStageEvidence[1].Kind)
		assert.Equal(t, operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED, rejected.DeterministicStageEvidence[1].Outcome)
	})
}

// TestL4Warden_Doctrine_MissingVerifierDependenciesReject verifies that
// missing critical verifier dependencies are rejected under doctrine posture.
func TestL4Warden_Doctrine_MissingVerifierDependenciesReject(t *testing.T) {
	t.Parallel()
	t.Run("missing replay store", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, nil, testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "doctrine")
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrReplayStoreMissing) {
			t.Fatalf("expected replay store rejection, got %v", err)
		}
	})

	t.Run("missing state root provider", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), nil, testutil.NewConfigurableMockL3Notary(true))
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "doctrine")
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrStateRootMissing) {
			t.Fatalf("expected state root provider rejection, got %v", err)
		}
	})
}

func TestL4WardenVerifyEnvelopeProducesAuthoritativeDeterministicStageEvidence(t *testing.T) {
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	envelope := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey, "doctrine")
	envelope.Governance.L2 = nil
	envelope.Governance.L3 = nil
	envelope.CaseId = "case-1"
	envelope.InvestigationId = "investigation-1"
	envelope.TaskId = "task-1"
	rehash(t, envelope)

	verified, err := verifier.VerifyEnvelope(context.Background(), envelope)
	require.NoError(t, err)
	require.Len(t, verified.DeterministicStageEvidence, 4)

	expected := []struct {
		kind    operatorv1.DeterministicStageKind
		outcome operatorv1.DeterministicStageOutcome
	}{
		{operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED},
		{operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2, operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED},
		{operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY, operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED},
		{operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED},
	}
	l1Stage := verified.DeterministicStageEvidence[0]
	assert.NotEmpty(t, l1Stage.DoctrineBundleHash)
	assert.NotEmpty(t, l1Stage.DoctrineBundleVersion)

	l4StageID := verified.DeterministicStageEvidence[3].StageId
	for index, want := range expected {
		stage := verified.DeterministicStageEvidence[index]
		assert.Equal(t, want.kind, stage.Kind)
		assert.Equal(t, want.outcome, stage.Outcome)
		assert.Equal(t, envelope.Id, stage.TransactionId)
		assert.Equal(t, envelope.TransactionHash, stage.TransactionHash)
		assert.Equal(t, envelope.CaseId, stage.CaseId)
		assert.Equal(t, envelope.InvestigationId, stage.InvestigationId)
		assert.Equal(t, envelope.TaskId, stage.TaskId)
		assert.Equal(t, "g8e-operator-process", stage.ClockDomain)
		assert.Equal(t, "go_monotonic", stage.TimingSource)
		assert.GreaterOrEqual(t, stage.MonotonicEndNs, stage.MonotonicStartNs)
		if want.kind != operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION {
			assert.Equal(t, l4StageID, stage.ParentStageId)
		}
	}
}
