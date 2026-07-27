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
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	govtypes "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance/governancetest"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
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
		&SimpleSignerStore{Signers: map[string]ed25519.PublicKey{"member-1": pub}},
		&errorConsensusPolicyStore{},
		nil,
		testutil.NewConfigurableMockL3Notary(true),
		NewL1Doctrine(),
		constants.AllActionTypes,
		"consensus",
		nil,
	)

	payload := typedPayload(t, constants.ActionTypeFsList)
	env := signedEnvelope(t, constants.ActionTypeFsList, payload, priv)
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
		ID:              "split-trib",
		MemberAppIDs:    []string{"member-1", "member-2", "member-3"},
		Quorum:          3,
		RequireDistinct: true,
		Enabled:         true,
	}

	warden := NewL4Warden(
		testutil.NewTestLogger(),
		testutil.NewStatefulMockReplayStore(),
		testutil.NewMockStateRootProvider("root-1"),
		&SimpleSignerStore{Signers: signers},
		&consensusStoreTestAdapter{Inner: &governancetest.SimpleConsensusStore{Consensus: map[string]*models.ConsensusPolicy{"split-trib": consensus3of3}}},
		nil,
		testutil.NewConfigurableMockL3Notary(true),
		NewL1Doctrine(),
		constants.AllActionTypes,
		"consensus",
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
	}
	hash, err := govtypes.GenerateMessageID(env)
	if err != nil {
		t.Fatalf("failed to generate hash: %v", err)
	}
	env.Id = hash
	env.TransactionHash = hash
	env.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			ConsensusSetId: "split-trib",
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
		ID:              "order-trib",
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
			&SimpleSignerStore{Signers: signers},
			&consensusStoreTestAdapter{Inner: &governancetest.SimpleConsensusStore{Consensus: map[string]*models.ConsensusPolicy{"order-trib": consensus2of2}}},
			nil,
			testutil.NewConfigurableMockL3Notary(true),
			NewL1Doctrine(),
			constants.AllActionTypes,
			"consensus",
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
		}
		hash, err := govtypes.GenerateMessageID(env)
		if err != nil {
			t.Fatalf("failed to generate hash: %v", err)
		}
		env.Id = hash
		env.TransactionHash = hash
		env.Governance = &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				ConsensusSetId: "order-trib",
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
