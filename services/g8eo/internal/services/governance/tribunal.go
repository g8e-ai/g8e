package governance

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sentinel"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
)

// Tribunal is the internal consensus engine's evaluator.
// It receives UAP envelopes from agents and appends a cryptographic vote.
type Tribunal struct {
	NodeID     string
	Sentinel   *sentinel.Sentinel
	PrivateKey ed25519.PrivateKey
}

// EvaluatePayload represents the Tribunal's core loop.
func (t *Tribunal) EvaluatePayload(env *uap.UAPEnvelope) error {
	// 1. Verify Sender Hash
	expectedHash, err := uap.GenerateMessageID(env)
	if err != nil {
		return fmt.Errorf("failed to generate message ID: %w", err)
	}
	if env.Id != expectedHash {
		return errors.New("FATAL: Payload hash mismatch. Dropping request.")
	}

	// 2. Run Deterministic SRE Rules (e.g., MITRE checks)
	// For proto-first, we use IntentData or Payload
	var cmdData string
	var intent constants.CloudIntent
	if env.IntentData != nil && len(env.IntentData.Fields) > 0 {
		jsonBytes, err := env.IntentData.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal intent data: %w", err)
		}
		cmdData = string(jsonBytes)

		// If this is an intent request, extract and validate the specific intent
		actionType := constants.ActionType(env.ActionType)
		if actionType == constants.ActionTypeGrantIntent || actionType == constants.ActionTypeRevokeIntent {
			if v, ok := env.IntentData.Fields["intent"]; ok {
				intent = constants.CloudIntent(v.GetStringValue())
			}
		}
	} else {
		cmdData = string(env.Payload)
	}

	isSafe := t.RunMITREChecks(env.TargetResource, cmdData)

	// L1 Intent Validation: ensure the requested intent is in the allowlist
	if intent != "" && t.Sentinel != nil {
		if !t.Sentinel.ValidateIntent(intent) {
			isSafe = false
		}
	}

	// 3. Append Vote
	// Note: We are using GovernanceMetadata instead of ConsensusState
	if env.Governance == nil {
		env.Governance = &commonv1.GovernanceMetadata{
			L1: &commonv1.L1Metadata{},
			L2: &commonv1.L2Metadata{},
			L3: &commonv1.L3Metadata{},
		}
	}

	env.Governance.L2.AgentIds = append(env.Governance.L2.AgentIds, t.NodeID)
	sig, err := t.SignDecision(env.Id, isSafe)
	if err != nil {
		return fmt.Errorf("failed to sign decision: %w", err)
	}
	env.Governance.L2.TribunalSignature = sig

	if !isSafe {
		env.Governance.L1.Validated = false
		env.Governance.L1.Violations = append(env.Governance.L1.Violations, "MITRE_CHECK_FAILED")
	}

	return nil
}

// RunMITREChecks leverages Sentinel to identify malicious activity patterns.
func (t *Tribunal) RunMITREChecks(resource string, data string) bool {
	if t.Sentinel == nil {
		return false // Fail-closed: if Sentinel is missing, the payload is NOT safe.
	}
	analysis := t.Sentinel.AnalyzeCommand(data)
	return analysis.Safe
}

// SignDecision creates a cryptographic signature of the decision.
func (t *Tribunal) SignDecision(messageID string, isSafe bool) (string, error) {
	if t.PrivateKey == nil {
		return "", errors.New("Tribunal private key missing - cannot sign governance votes")
	}
	// Sign the message ID and the decision
	payload := fmt.Sprintf("%s|%v", messageID, isSafe)
	sig := ed25519.Sign(t.PrivateKey, []byte(payload))
	return hex.EncodeToString(sig), nil
}
