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
	"errors"
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// L2Consensus is the internal consensus engine's evaluator.
// It receives GovernanceEnvelope envelopes from agents and appends a cryptographic vote.
type L2Consensus struct {
	NodeID     string
	Doctrine   *L1Doctrine
	PrivateKey ed25519.PrivateKey
}

// NewL2Consensus creates a new L2 consensus engine.
func NewL2Consensus(nodeID string, d *L1Doctrine, pk ed25519.PrivateKey) *L2Consensus {
	return &L2Consensus{
		NodeID:     nodeID,
		Doctrine:   d,
		PrivateKey: pk,
	}
}

// EvaluatePayload represents the L2Consensus's core loop.
func (l *L2Consensus) EvaluatePayload(env *governance.GovernanceEnvelope) error {
	if err := l.verifyPayloadHash(env); err != nil {
		return err
	}

	cmdData, intent, err := l.extractCommandData(env)
	if err != nil {
		return err
	}

	isSafe := l.evaluateSafety(env.TargetResource, cmdData, intent)

	return l.appendVote(env, isSafe)
}

// verifyPayloadHash verifies the sender hash matches the expected hash.
func (l *L2Consensus) verifyPayloadHash(env *governance.GovernanceEnvelope) error {
	expectedHash, err := governance.GenerateMessageID(env)
	if err != nil {
		return fmt.Errorf("l2consensus: verify payload hash: %w", err)
	}
	if env.Id != expectedHash {
		return errors.New("l2consensus: verify payload hash: payload hash mismatch")
	}
	return nil
}

// extractCommandData extracts command data and intent from the envelope.
func (l *L2Consensus) extractCommandData(env *governance.GovernanceEnvelope) (string, constants.CloudIntent, error) {
	var cmdData string
	var intent constants.CloudIntent

	if env.IntentData != nil && len(env.IntentData.Fields) > 0 {
		jsonBytes, err := env.IntentData.MarshalJSON()
		if err != nil {
			return "", "", fmt.Errorf("l2consensus: extract command data: %w", err)
		}
		cmdData = string(jsonBytes)

		actionType := constants.ActionType(env.ActionType)
		if actionType == constants.ActionTypeGrantIntent || actionType == constants.ActionTypeRevokeIntent {
			if v, ok := env.IntentData.Fields[string(constants.ApprovalTypeIntent)]; ok {
				intent = constants.CloudIntent(v.GetStringValue())
			}
		}
	} else {
		cmdData = string(env.Payload)
	}

	return cmdData, intent, nil
}

// evaluateSafety runs MITRE checks and doctrine intent validation.
func (l *L2Consensus) evaluateSafety(resource string, cmdData string, intent constants.CloudIntent) bool {
	isSafe := l.RunMITREChecks(resource, cmdData)

	if intent != "" && l.Doctrine != nil {
		if !l.Doctrine.ValidateIntent(intent) {
			isSafe = false
		}
	}

	return isSafe
}

// appendVote appends the consensus vote to the envelope.
func (l *L2Consensus) appendVote(env *governance.GovernanceEnvelope, isSafe bool) error {
	if env.Governance == nil {
		env.Governance = &commonv1.GovernanceMetadata{
			L1: &commonv1.L1Metadata{},
			L2: &commonv1.L2Metadata{},
			L3: &commonv1.L3Metadata{},
		}
	}

	env.Governance.L2.AgentIds = append(env.Governance.L2.AgentIds, l.NodeID)
	sig, err := l.SignDecision(env.Id, isSafe)
	if err != nil {
		return fmt.Errorf("l2consensus: append vote: %w", err)
	}
	env.Governance.L2.ConsensusSignature = sig

	if !isSafe {
		env.Governance.L1.Validated = false
		env.Governance.L1.Violations = append(env.Governance.L1.Violations, "MITRE_CHECK_FAILED")
	}

	return nil
}

// RunMITREChecks leverages L1Doctrine to identify malicious activity patterns.
func (l *L2Consensus) RunMITREChecks(resource string, data string) bool {
	if l.Doctrine == nil {
		return false // Fail-closed: if Doctrine is missing, the payload is NOT safe.
	}
	signals := l.Doctrine.AnalyzeCommand(data)
	// If any signal recommends blocking, the payload is not safe
	for _, sig := range signals {
		if sig.BlockRecommended {
			return false
		}
	}
	return true
}

// SignDecision creates a cryptographic signature of the decision.
func (l *L2Consensus) SignDecision(messageID string, isSafe bool) (string, error) {
	if l.PrivateKey == nil {
		return "", fmt.Errorf("l2consensus: sign decision: private key missing")
	}
	// Sign the message ID and the decision
	payload := fmt.Sprintf("%s|%v", messageID, isSafe)
	sig := ed25519.Sign(l.PrivateKey, []byte(payload))
	return hex.EncodeToString(sig), nil
}
