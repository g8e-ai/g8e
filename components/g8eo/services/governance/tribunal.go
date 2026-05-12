package governance

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	"github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
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
	expectedHash, err := env.GenerateMessageID()
	if err != nil {
		return fmt.Errorf("failed to generate message ID: %w", err)
	}
	if env.MessageID != expectedHash {
		return errors.New("FATAL: Payload hash mismatch. Dropping request.")
	}

	// 2. Run Deterministic SRE Rules (e.g., MITRE checks)
	isSafe := t.RunMITREChecks(env.Intent.TargetResource, env.Context.DataBlob)

	// 3. Append Vote
	vote := uap.Vote{
		NodeID:    t.NodeID,
		Signature: t.SignDecision(env.MessageID, isSafe), // Cryptographic signature
		Decision:  isSafe,
	}

	env.Consensus.CurrentVotes = append(env.Consensus.CurrentVotes, vote)

	if !isSafe {
		env.Consensus.Status = "REJECTED"
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
func (t *Tribunal) SignDecision(messageID string, isSafe bool) string {
	if t.PrivateKey == nil {
		panic("FATAL: Tribunal PrivateKey is nil. Cannot sign governance votes.")
	}
	// Sign the message ID and the decision
	payload := fmt.Sprintf("%s|%v", messageID, isSafe)
	sig := ed25519.Sign(t.PrivateKey, []byte(payload))
	return hex.EncodeToString(sig)
}
