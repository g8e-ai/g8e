// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance/governancetest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stretchr/testify/assert"
)

// TestL4Warden_Consensus_ValidNonMutationPasses verifies that a valid
// non-mutation envelope with L2 passes under consensus posture.
func TestL4Warden_Consensus_ValidNonMutationPasses(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "consensus")

	verified, err := verifier.VerifyEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("expected verification to pass, got %v", err)
	}
	if !verified.L2Valid {
		t.Fatalf("expected L2Valid=true under consensus with valid L2")
	}
}

// TestL4Warden_Consensus_ValidMutationPassesWithoutL3 verifies that a
// mutation envelope passes under consensus without L3 proof, since
// consensus enforces L1+L2 but audits L3 only.
func TestL4Warden_Consensus_ValidMutationPassesWithoutL3(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey, "consensus")

	env.Governance.L3 = nil

	verified, err := verifier.VerifyEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("expected mutation to pass under consensus without L3, got %v", err)
	}
	if verified.L3Valid {
		t.Fatalf("expected L3Valid=false with no L3 proof")
	}
}

// TestL4Warden_Consensus_AllActionTypesFromSSOT verifies that every action type
// from the SSOT can be decoded and verified under consensus posture.
func TestL4Warden_Consensus_AllActionTypesFromSSOT(t *testing.T) {
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
			env := signedEnvelope(t, actionType, payload, privKey, "consensus")

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

// TestL4Warden_Consensus_MissingL2Rejects verifies that missing L2 votes
// reject an envelope under consensus (L2 is enforced).
func TestL4Warden_Consensus_MissingL2Rejects(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "consensus")

	env.Governance.L2 = nil

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if !errors.Is(err, ErrL2SignatureMissing) {
		t.Fatalf("expected ErrL2SignatureMissing under consensus with no L2, got %v", err)
	}
}

// TestL4Warden_Consensus_MissingL3DoesNotReject verifies that missing L3
// proof does not reject a mutation under consensus (L3 is audited, not enforced).
func TestL4Warden_Consensus_MissingL3DoesNotReject(t *testing.T) {
	t.Parallel()
	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey, "consensus")

	env.Governance.L3 = nil

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("expected mutation to pass under consensus without L3, got %v", err)
	}
}

// TestL4Warden_Consensus_ReplayAndStateRootReject verifies that replay
// attacks and state root mismatches are rejected under consensus posture.
func TestL4Warden_Consensus_ReplayAndStateRootReject(t *testing.T) {
	t.Parallel()
	t.Run("replayed nonce", func(t *testing.T) {
		t.Parallel()
		replayStore := testutil.NewStatefulMockReplayStore()
		verifier, privKey := createStrictVerifier(t, replayStore, testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "consensus")
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
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "consensus")
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrStateRootMismatch) {
			t.Fatalf("expected state root mismatch, got %v", err)
		}
	})
}

// TestL4Warden_Consensus_MissingVerifierDependenciesReject verifies that
// missing critical verifier dependencies are rejected under consensus posture.
func TestL4Warden_Consensus_MissingVerifierDependenciesReject(t *testing.T) {
	t.Parallel()
	t.Run("missing replay store", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, nil, testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "consensus")
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrReplayStoreMissing) {
			t.Fatalf("expected replay store rejection, got %v", err)
		}
	})

	t.Run("missing state root provider", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), nil, testutil.NewConfigurableMockL3Notary(true))
		env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "consensus")
		_, err := verifier.VerifyEnvelope(context.Background(), env)
		if !errors.Is(err, ErrStateRootMissing) {
			t.Fatalf("expected state root provider rejection, got %v", err)
		}
	})
}

// TestL4Warden_L2QuorumVerification verifies L2 quorum mechanics under
// consensus posture (L2 is enforced).
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

	enabledConsensus2of2 := &models.ConsensusPolicy{
		ID:              "consensus-1",
		MemberAppIDs:    []string{"member-1", "member-2"},
		Quorum:          2,
		RequireDistinct: true,
		Enabled:         true,
	}
	enabledConsensus1of2 := &models.ConsensusPolicy{
		ID:              "consensus-2",
		MemberAppIDs:    []string{"member-1", "member-2"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	disabledConsensus := &models.ConsensusPolicy{
		ID:              "consensus-disabled",
		MemberAppIDs:    []string{"member-1", "member-2"},
		Quorum:          2,
		RequireDistinct: true,
		Enabled:         false,
	}

	payload := typedPayload(t, constants.ActionTypeFsList)

	buildEnv := func(nonceTag, consensusID string, votes []*commonv1.L2Vote) *govtypes.GovernanceEnvelope {
		nonceSuffix := hex.EncodeToString(payload)
		if len(nonceSuffix) > 8 {
			nonceSuffix = nonceSuffix[:8]
		}
		env := &govtypes.GovernanceEnvelope{
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
			Posture:           "consensus",
		}
		hash, err := govtypes.GenerateMessageID(env)
		if err != nil {
			t.Fatalf("failed to generate hash: %v", err)
		}
		env.Id = hash
		env.TransactionHash = hash
		env.Governance = &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				ConsensusSetId: consensusID,
				Votes:          votes,
			},
		}
		return env
	}

	makeVerifier := func(signers map[string]ed25519.PublicKey, consensus map[string]*models.ConsensusPolicy) *L4Warden {
		return NewL4Warden(
			testutil.NewTestLogger(),
			testutil.NewStatefulMockReplayStore(),
			testutil.NewMockStateRootProvider("root-1"),
			&FailClosedSignerStore{Signers: signers},
			&consensusStoreTestAdapter{Inner: &governancetest.SimpleConsensusStore{Consensus: consensus}},
			testutil.NewConfigurableMockL3Notary(true),
			NewL1Doctrine(),
			constants.AllActionTypes,
			nil,
		)
	}

	tests := []struct {
		name     string
		verifier *L4Warden
		env      *govtypes.GovernanceEnvelope
		wantErr  error
		wantL2   bool
	}{
		{
			name:     "2-of-2 quorum pass",
			verifier: makeVerifier(allSigners, map[string]*models.ConsensusPolicy{"consensus-1": enabledConsensus2of2}),
			env: func() *govtypes.GovernanceEnvelope {
				env := buildEnv("2of2pass", "consensus-1", nil)
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
			verifier: makeVerifier(allSigners, map[string]*models.ConsensusPolicy{"consensus-1": enabledConsensus2of2}),
			env: func() *govtypes.GovernanceEnvelope {
				env := buildEnv("1of2fail", "consensus-1", nil)
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
			verifier: makeVerifier(allSigners, map[string]*models.ConsensusPolicy{"consensus-1": enabledConsensus2of2}),
			env: func() *govtypes.GovernanceEnvelope {
				env := buildEnv("dupsign", "consensus-1", nil)
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
			verifier: makeVerifier(allSigners, map[string]*models.ConsensusPolicy{"consensus-1": enabledConsensus2of2}),
			env: func() *govtypes.GovernanceEnvelope {
				env := buildEnv("falsevote", "consensus-1", nil)
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
			verifier: makeVerifier(partialSigners, map[string]*models.ConsensusPolicy{"consensus-2": enabledConsensus1of2}),
			env: func() *govtypes.GovernanceEnvelope {
				env := buildEnv("unknownsigner", "consensus-2", nil)
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
			verifier: makeVerifier(allSigners, map[string]*models.ConsensusPolicy{"consensus-1": enabledConsensus2of2}),
			env:      buildEnv("emptyvotes", "consensus-1", []*commonv1.L2Vote{}),
			wantErr:  ErrL2SignatureMissing,
			wantL2:   false,
		},
		{
			name:     "disabled consensus policy",
			verifier: makeVerifier(allSigners, map[string]*models.ConsensusPolicy{"consensus-disabled": disabledConsensus}),
			env: func() *govtypes.GovernanceEnvelope {
				env := buildEnv("disabled-consensus", "consensus-disabled", nil)
				hash := env.TransactionHash
				env.Governance.L2.Votes = []*commonv1.L2Vote{
					signL2Vote(priv1, "member-1", hash, true),
					signL2Vote(priv2, "member-2", hash, true),
				}
				return env
			}(),
			wantErr: ErrL2ConsensusNotConfigured,
			wantL2:  false,
		},
		{
			name:     "unknown consensus ID",
			verifier: makeVerifier(allSigners, map[string]*models.ConsensusPolicy{"consensus-1": enabledConsensus2of2}),
			env: func() *govtypes.GovernanceEnvelope {
				env := buildEnv("unknown-consensus", "nonexistent-consensus", nil)
				hash := env.TransactionHash
				env.Governance.L2.Votes = []*commonv1.L2Vote{
					signL2Vote(priv1, "member-1", hash, true),
					signL2Vote(priv2, "member-2", hash, true),
				}
				return env
			}(),
			wantErr: ErrL2ConsensusNotConfigured,
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
