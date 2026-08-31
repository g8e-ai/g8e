// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
)

// L2ConsensusPolicy is the generic consensus policy consumed by the L4 Warden.
// It is not tied to any specific consensus implementation (e.g., Consensus).
type L2ConsensusPolicy struct {
	MemberKeyIDs    []string
	Quorum          int
	RequireDistinct bool
	Enabled         bool
}

// L2ConsensusPolicyStore defines the generic interface for loading an L2
// consensus policy by ID. The L4 Warden depends on this interface rather than
// ConsensusStore, allowing alternative consensus implementations to be plugged
// in without modifying the warden.
type L2ConsensusPolicyStore interface {
	GetConsensusPolicy(id string) (*L2ConsensusPolicy, error)
}

// NoopConsensusPolicyStore is a no-op implementation of L2ConsensusPolicyStore.
// It returns nil (no policy found) with no error. Used in tests and chaos
// engineering; production outbound mode uses nil with explicit nil-checks.
type NoopConsensusPolicyStore struct{}

func (NoopConsensusPolicyStore) GetConsensusPolicy(string) (*L2ConsensusPolicy, error) {
	return nil, nil
}

// verifyL2Posture verifies L2 (machine consensus) votes against the consensus
// policy. Returns true if L2 consensus was achieved, false if no L2 votes were
// present (and not required by posture). Returns an error if L2 is required by
// posture but verification fails. The posture is read per-envelope from
// GovernanceEnvelope.Posture by the caller and passed in here.
func (tv *L4Warden) verifyL2Posture(envelope *govtypes.GovernanceEnvelope, computedHash string, posture GovernancePosture) (bool, error) {
	if envelope.Governance == nil || envelope.Governance.L2 == nil || len(envelope.Governance.L2.Votes) == 0 {
		if posture.RequiresL2Signature() {
			tv.logger.Error("L2 votes missing but required by posture", "posture", posture.Name())
			return false, constants.ErrTxL2SignatureMissing
		}
		return false, nil
	}

	l2 := envelope.Governance.L2

	if tv.signerStore == nil {
		if posture.RequiresL2Signature() {
			tv.logger.Error("Signer store not configured but required by posture", "posture", posture.Name())
			return false, constants.ErrTxL2SignerStoreNotConfigured
		}
		return false, nil
	}

	if tv.consensusPolicyStore == nil {
		if posture.RequiresL2Signature() {
			tv.logger.Error("Consensus policy store not configured but required by posture", "posture", posture.Name())
			return false, constants.ErrTxL2ConsensusNotConfigured
		}
		return false, nil
	}

	policy, err := tv.consensusPolicyStore.GetConsensusPolicy(l2.ConsensusSetId)
	if err != nil {
		if posture.RequiresL2Signature() {
			tv.logger.Error("Failed to load L2 consensus policy", "consensus_set_id", l2.ConsensusSetId, string(constants.ConnectionStateError), err)
			return false, fmt.Errorf("l4 warden: verify L2 posture: %w", err)
		}
		return false, nil
	}
	if policy == nil || !policy.Enabled {
		if posture.RequiresL2Signature() {
			tv.logger.Error("L2 consensus policy not found or disabled", "consensus_set_id", l2.ConsensusSetId)
			return false, constants.ErrTxL2ConsensusNotConfigured
		}
		return false, nil
	}

	members := make(map[string]bool, len(policy.MemberKeyIDs))
	for _, m := range policy.MemberKeyIDs {
		members[m] = true
	}

	seen := make(map[string]bool)
	affirmative := 0

	for _, vote := range l2.Votes {
		if !members[vote.SignerKeyId] {
			continue
		}
		if seen[vote.SignerKeyId] {
			if policy.RequireDistinct {
				tv.logger.Error("Duplicate signer in vote set with require_distinct", "key_id", vote.SignerKeyId)
				if posture.RequiresL2Signature() {
					return false, constants.ErrTxL2DuplicateSigner
				}
				return false, nil
			}
			continue
		}
		pubKey, err := tv.signerStore.GetTrustedSigner(vote.SignerKeyId)
		if err != nil {
			tv.logger.Error("Failed to load trusted signer", "key_id", vote.SignerKeyId, string(constants.ConnectionStateError), err)
			continue
		}
		if pubKey == nil {
			tv.logger.Error("Consensus (L2) signer key not found in trusted signers", "key_id", vote.SignerKeyId)
			continue
		}
		if !tv.verifyL2Signature(pubKey, vote.ConsensusSignature, computedHash, vote.Decision) {
			tv.logger.Error("L2 signature verification failed", "key_id", vote.SignerKeyId)
			continue
		}
		seen[vote.SignerKeyId] = true
		if vote.Decision {
			affirmative++
		}
	}

	if affirmative < policy.Quorum {
		if posture.RequiresL2Signature() {
			tv.logger.Error("L2 quorum not met", "affirmative", affirmative, "quorum", policy.Quorum, "posture", posture.Name())
			return false, constants.ErrTxL2QuorumNotMet
		}
		return false, nil
	}

	return true, nil
}

// verifyL2Signature verifies an L2 ED25519 signature.
func (tv *L4Warden) verifyL2Signature(pubKey ed25519.PublicKey, signature, messageID string, decision bool) bool {
	if signature == "" || signature == "UNSIGNED" {
		return false
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	payload := fmt.Sprintf("%s|%v", messageID, decision)
	return ed25519.Verify(pubKey, []byte(payload), sigBytes)
}
