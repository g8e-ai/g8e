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
	"fmt"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance/governancetest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var errConsensusPolicyStoreDB = errors.New("consensus policy store database unavailable")

// errorConsensusPolicyStore simulates an L2ConsensusPolicyStore whose backing database is unavailable.
type errorConsensusPolicyStore struct{}

func (m *errorConsensusPolicyStore) GetConsensusPolicy(id string) (*L2ConsensusPolicy, error) {
	return nil, errConsensusPolicyStoreDB
}

// TestL4Warden_ConsensusPolicyStoreError_FailClosed verifies that when the consensus
// policy store returns an error from GetConsensusPolicy, the warden fails closed
// under consensus posture (where L2 is enforced) rather than silently accepting
// the envelope.
func TestL4Warden_ConsensusPolicyStoreError_FailClosed(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	warden := NewL4Warden(
		testutil.NewTestLogger(),
		testutil.NewStatefulMockReplayStore(),
		testutil.NewMockStateRootProvider("root-1"),
		&FailClosedSignerStore{Signers: map[string]ed25519.PublicKey{"member-1": pub}},
		&errorConsensusPolicyStore{},
		testutil.NewConfigurableMockL3Notary(true),
		NewL1Doctrine(),
		constants.AllActionTypes,
		nil,
	)

	payload := typedPayload(t, constants.ActionTypeFsList)
	env := signedEnvelope(t, constants.ActionTypeFsList, payload, priv, "consensus")
	// Override the L2 vote to use our member key
	hash := env.TransactionHash
	env.Governance.L2.Votes[0] = signL2Vote(priv, "member-1", hash, true)

	_, err = warden.VerifyEnvelope(context.Background(), env)
	if err == nil {
		t.Fatal("expected error when consensus policy store fails, got nil")
	}
	if !errors.Is(err, errConsensusPolicyStoreDB) {
		t.Fatalf("expected error wrapping errConsensusPolicyStoreDB, got %v", err)
	}
}

// TestL4Warden_L2SplitVote_QuorumNotMet verifies that a split vote (some members
// approve, some veto) does not meet quorum when the affirmative count is below
// the quorum threshold. This simulates the real-world scenario where consensus
// members disagree on safety.
func TestL4Warden_L2SplitVote_QuorumNotMet(t *testing.T) {
	t.Parallel()
	pub1, priv1, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pub2, priv2, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pub3, priv3, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	signers := map[string]ed25519.PublicKey{
		"member-1": pub1,
		"member-2": pub2,
		"member-3": pub3,
	}
	consensus3of3 := &models.ConsensusPolicy{
		ID:              "split-consensus",
		MemberAppIDs:    []string{"member-1", "member-2", "member-3"},
		Quorum:          3,
		RequireDistinct: true,
		Enabled:         true,
	}

	warden := NewL4Warden(
		testutil.NewTestLogger(),
		testutil.NewStatefulMockReplayStore(),
		testutil.NewMockStateRootProvider("root-1"),
		&FailClosedSignerStore{Signers: signers},
		&consensusStoreTestAdapter{Inner: &governancetest.SimpleConsensusStore{Consensus: map[string]*models.ConsensusPolicy{"split-consensus": consensus3of3}}},
		testutil.NewConfigurableMockL3Notary(true),
		NewL1Doctrine(),
		constants.AllActionTypes,
		nil,
	)

	payload := typedPayload(t, constants.ActionTypeFsList)
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
		Nonce:             "nonce-splitvote-" + nonceSuffix,
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
			ConsensusSetId: "split-consensus",
			Votes: []*commonv1.L2Vote{
				signL2Vote(priv1, "member-1", hash, true),
				signL2Vote(priv2, "member-2", hash, false),
				signL2Vote(priv3, "member-3", hash, false),
			},
		},
	}

	_, err = warden.VerifyEnvelope(context.Background(), env)
	if !errors.Is(err, ErrL2QuorumNotMet) {
		t.Fatalf("expected ErrL2QuorumNotMet for split vote (1 affirmative, quorum 3), got %v", err)
	}
}

// TestL4Warden_L2VoteOrderingIndependence verifies that the warden's L2
// quorum verification produces the same result regardless of the order of
// votes in the L2 metadata. This guards against order-dependent quorum bugs.
func TestL4Warden_L2VoteOrderingIndependence(t *testing.T) {
	t.Parallel()
	pub1, priv1, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pub2, priv2, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	signers := map[string]ed25519.PublicKey{
		"member-1": pub1,
		"member-2": pub2,
	}
	consensus2of2 := &models.ConsensusPolicy{
		ID:              "order-consensus",
		MemberAppIDs:    []string{"member-1", "member-2"},
		Quorum:          2,
		RequireDistinct: true,
		Enabled:         true,
	}

	makeWarden := func() *L4Warden {
		return NewL4Warden(
			testutil.NewTestLogger(),
			testutil.NewStatefulMockReplayStore(),
			testutil.NewMockStateRootProvider("root-1"),
			&FailClosedSignerStore{Signers: signers},
			&consensusStoreTestAdapter{Inner: &governancetest.SimpleConsensusStore{Consensus: map[string]*models.ConsensusPolicy{"order-consensus": consensus2of2}}},
			testutil.NewConfigurableMockL3Notary(true),
			NewL1Doctrine(),
			constants.AllActionTypes,
			nil,
		)
	}

	buildEnv := func(nonceTag string, voteOrder []*commonv1.L2Vote) *govtypes.GovernanceEnvelope {
		payload := typedPayload(t, constants.ActionTypeFsList)
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
			Nonce:             fmt.Sprintf("nonce-order-%s-%s", nonceTag, nonceSuffix),
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
				ConsensusSetId: "order-consensus",
				Votes:          voteOrder,
			},
		}
		return env
	}

	hash1 := func(env *govtypes.GovernanceEnvelope) string { return env.TransactionHash }

	// Order A: member-1 first, member-2 second
	envA := buildEnv("a", nil)
	hA := hash1(envA)
	envA.Governance.L2.Votes = []*commonv1.L2Vote{
		signL2Vote(priv1, "member-1", hA, true),
		signL2Vote(priv2, "member-2", hA, true),
	}

	// Order B: member-2 first, member-1 second
	envB := buildEnv("b", nil)
	hB := hash1(envB)
	envB.Governance.L2.Votes = []*commonv1.L2Vote{
		signL2Vote(priv2, "member-2", hB, true),
		signL2Vote(priv1, "member-1", hB, true),
	}

	verifiedA, errA := makeWarden().VerifyEnvelope(context.Background(), envA)
	verifiedB, errB := makeWarden().VerifyEnvelope(context.Background(), envB)

	if errA != nil {
		t.Fatalf("order A failed: %v", errA)
	}
	if errB != nil {
		t.Fatalf("order B failed: %v", errB)
	}
	if !verifiedA.L2Valid {
		t.Fatal("order A: expected L2Valid=true")
	}
	if !verifiedB.L2Valid {
		t.Fatal("order B: expected L2Valid=true")
	}
}

// TestL4Warden_SingleKeyCannotSatisfyQuorum is the regression test for the
// vacuous quorum bug: with the old single-key ensemble pattern, one Ed25519
// key was registered as the trusted signer for every member, so a single
// compromised key could sign all votes and satisfy a multi-member quorum.
//
// Phase 1 (per-member consensus keys) fixes this at the configuration level by
// registering a distinct public key per member. This test proves the fix is
// cryptographically meaningful at the warden: when each member has its own
// key, a single private key cannot forge the other members' votes because the
// signatures fail verification against the other members' registered public
// keys, leaving affirmative < quorum.
//
// This test would have caught the vacuous quorum bug: under the old single-key
// config (same pub registered for all members), the forged-votes case below
// would have passed instead of returning ErrL2QuorumNotMet.
func TestL4Warden_SingleKeyCannotSatisfyQuorum(t *testing.T) {
	t.Parallel()
	pub1, priv1, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pub2, priv2, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pub3, priv3, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Per-member key registration: each member has its own distinct public key.
	signers := map[string]ed25519.PublicKey{
		"member-1": pub1,
		"member-2": pub2,
		"member-3": pub3,
	}
	consensus3of3 := &models.ConsensusPolicy{
		ID:              "per-member-consensus",
		MemberAppIDs:    []string{"member-1", "member-2", "member-3"},
		Quorum:          3,
		RequireDistinct: true,
		Enabled:         true,
	}

	makeWarden := func() *L4Warden {
		return NewL4Warden(
			testutil.NewTestLogger(),
			testutil.NewStatefulMockReplayStore(),
			testutil.NewMockStateRootProvider("root-1"),
			&FailClosedSignerStore{Signers: signers},
			&consensusStoreTestAdapter{Inner: &governancetest.SimpleConsensusStore{Consensus: map[string]*models.ConsensusPolicy{"per-member-consensus": consensus3of3}}},
			testutil.NewConfigurableMockL3Notary(true),
			NewL1Doctrine(),
			constants.AllActionTypes,
			nil,
		)
	}

	buildEnv := func(nonceTag string, votes []*commonv1.L2Vote) *govtypes.GovernanceEnvelope {
		payload := typedPayload(t, constants.ActionTypeFsList)
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
			Nonce:             fmt.Sprintf("nonce-singlekey-%s-%s", nonceTag, nonceSuffix),
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
				ConsensusSetId: "per-member-consensus",
				Votes:          votes,
			},
		}
		return env
	}

	// Negative case: forge all 3 votes with member-1's private key. Each vote
	// carries the correct SignerKeyId, but the signatures for member-2 and
	// member-3 will not verify against their distinct registered public keys,
	// so only member-1's affirmative vote counts -> quorum 3 not met.
	forgedEnv := buildEnv("forged", nil)
	fHash := forgedEnv.TransactionHash
	forgedEnv.Governance.L2.Votes = []*commonv1.L2Vote{
		signL2Vote(priv1, "member-1", fHash, true),
		signL2Vote(priv1, "member-2", fHash, true),
		signL2Vote(priv1, "member-3", fHash, true),
	}
	_, err = makeWarden().VerifyEnvelope(context.Background(), forgedEnv)
	if !errors.Is(err, ErrL2QuorumNotMet) {
		t.Fatalf("expected ErrL2QuorumNotMet when one key forges all votes with per-member keys, got %v", err)
	}

	// Positive control: sign each vote with the correct member's distinct key.
	// All 3 signatures verify and quorum 3 is met.
	distinctEnv := buildEnv("distinct", nil)
	dHash := distinctEnv.TransactionHash
	distinctEnv.Governance.L2.Votes = []*commonv1.L2Vote{
		signL2Vote(priv1, "member-1", dHash, true),
		signL2Vote(priv2, "member-2", dHash, true),
		signL2Vote(priv3, "member-3", dHash, true),
	}
	verified, err := makeWarden().VerifyEnvelope(context.Background(), distinctEnv)
	if err != nil {
		t.Fatalf("positive control: expected success with 3 distinct keys, got %v", err)
	}
	if !verified.L2Valid {
		t.Fatal("positive control: expected L2Valid=true with 3 distinct keys")
	}

	// Quorum-2 positive control: 2 distinct keys satisfy a 2-of-3 quorum.
	consensus2of3 := &models.ConsensusPolicy{
		ID:              "per-member-2of3",
		MemberAppIDs:    []string{"member-1", "member-2", "member-3"},
		Quorum:          2,
		RequireDistinct: true,
		Enabled:         true,
	}
	makeWarden2of3 := func() *L4Warden {
		return NewL4Warden(
			testutil.NewTestLogger(),
			testutil.NewStatefulMockReplayStore(),
			testutil.NewMockStateRootProvider("root-1"),
			&FailClosedSignerStore{Signers: signers},
			&consensusStoreTestAdapter{Inner: &governancetest.SimpleConsensusStore{Consensus: map[string]*models.ConsensusPolicy{"per-member-2of3": consensus2of3}}},
			testutil.NewConfigurableMockL3Notary(true),
			NewL1Doctrine(),
			constants.AllActionTypes,
			nil,
		)
	}
	twoOfThreeEnv := buildEnv("2of3", nil)
	tHash := twoOfThreeEnv.TransactionHash
	twoOfThreeEnv.Governance.L2.ConsensusSetId = "per-member-2of3"
	twoOfThreeEnv.Governance.L2.Votes = []*commonv1.L2Vote{
		signL2Vote(priv1, "member-1", tHash, true),
		signL2Vote(priv2, "member-2", tHash, true),
	}
	verified2, err := makeWarden2of3().VerifyEnvelope(context.Background(), twoOfThreeEnv)
	if err != nil {
		t.Fatalf("2-of-3 positive control: expected success, got %v", err)
	}
	if !verified2.L2Valid {
		t.Fatal("2-of-3 positive control: expected L2Valid=true")
	}
}
