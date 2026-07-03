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

package tribunal

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
	govsvc "github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/governance"
)

// TribunalMember represents a single member identity in the Tribunal.
// Each member is an enrolled agentic app with its own Ed25519 signing key.
// The member's public key is registered as a TrustedSigner (keyID = AppID).
type TribunalMember struct {
	AppID      string
	PrivateKey ed25519.PrivateKey
}

// evaluateSafety runs MITRE checks via L1Doctrine.
// This is the deterministic L1-doctrine check relocated from
// l2_consensus.go. The pluggable heterogeneous-reasoner backend is a
// later extension point and is not stubbed in (devs.md: do not build
// things that should not yet exist).
func (s *TribunalService) evaluateSafety(doctrine *govsvc.L1Doctrine, resource string, cmdData string, intent constants.CloudIntent) bool {
	return s.runMITREChecks(doctrine, resource, cmdData)
}

// runMITREChecks leverages L1Doctrine to identify malicious activity patterns.
// Fail-closed: if Doctrine is nil, the payload is NOT safe.
func (s *TribunalService) runMITREChecks(doctrine *govsvc.L1Doctrine, resource string, data string) bool {
	if doctrine == nil {
		return false
	}
	signals := doctrine.AnalyzeCommand(data)
	for _, sig := range signals {
		if sig.BlockRecommended {
			return false
		}
	}
	return true
}

// extractCommandData extracts command data and intent from the envelope.
// Relocated from l2_consensus.go.
func extractCommandData(env *governance.GovernanceEnvelope) (string, constants.CloudIntent, error) {
	var cmdData string
	var intent constants.CloudIntent

	if env.IntentData != nil && len(env.IntentData.Fields) > 0 {
		jsonBytes, err := env.IntentData.MarshalJSON()
		if err != nil {
			return "", "", fmt.Errorf("tribunal: extract command data: %w", err)
		}
		cmdData = string(jsonBytes)

	} else {
		cmdData = string(env.Payload)
	}

	return cmdData, intent, nil
}

// signDecision creates a cryptographic signature of the decision.
// The signature is over "<transaction_hash>|<decision>", matching the
// existing verifyL2Signature primitive in l4_warden.go.
func signDecision(privateKey ed25519.PrivateKey, messageID string, isSafe bool) (string, error) {
	if privateKey == nil {
		return "", fmt.Errorf("tribunal: sign decision: private key missing")
	}
	payload := fmt.Sprintf("%s|%v", messageID, isSafe)
	sig := ed25519.Sign(privateKey, []byte(payload))
	return hex.EncodeToString(sig), nil
}
