package governance

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
)

func TestGovernanceFlow(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	nodeID := "test-node-1"

	tribunal := &Tribunal{
		NodeID:     nodeID,
		PrivateKey: priv,
	}

	warden := &Warden{
		TrustedNodes: map[string]ed25519.PublicKey{
			nodeID: pub,
		},
	}

	env := &uap.UAPEnvelope{
		ProtocolVersion: "1.0",
		Metadata: uap.Metadata{
			SenderID:  "agent-1",
			Timestamp: time.Now(),
		},
		Intent: uap.Intent{
			ActionType:     "EXECUTE_BASH",
			TargetResource: "localhost",
		},
		Context: uap.Context{
			DataFormat: "raw",
			DataBlob:   "echo 'hello'",
		},
		Consensus: uap.ConsensusState{
			RequiredVotes: 1,
			Status:        "PENDING",
		},
	}

	// 1. Generate Message ID
	id, _ := env.GenerateMessageID()
	env.MessageID = id

	// 2. Tribunal Evaluation
	err := tribunal.EvaluatePayload(env)
	if err != nil {
		t.Fatalf("Tribunal evaluation failed: %v", err)
	}

	if len(env.Consensus.CurrentVotes) != 1 {
		t.Errorf("Expected 1 vote, got %d", len(env.Consensus.CurrentVotes))
	}

	// Ensure status is PENDING or APPROVED for Warden (tribunal sets to REJECTED if unsafe)
	// In this test, Sentinel is nil, so tribunal.RunMITREChecks returns false (fail-closed).
	// We need to bypass this for the happy path test of Warden.
	env.Consensus.Status = "PENDING"
	for i := range env.Consensus.CurrentVotes {
		env.Consensus.CurrentVotes[i].Decision = true
		// Re-sign with Decision=true
		env.Consensus.CurrentVotes[i].Signature = tribunal.SignDecision(env.MessageID, true)
	}

	// 3. Warden Authorization
	err = warden.AuthorizeExecution(env)
	if err != nil {
		t.Fatalf("Warden authorization failed: %v", err)
	}

	// 4. Test Hash Mismatch
	env.MessageID = "wrong-hash"
	err = tribunal.EvaluatePayload(env)
	if err == nil {
		t.Error("Expected error on hash mismatch, got nil")
	}

	// 5. Test Insufficient Quorum
	env.MessageID = id // Reset
	env.Consensus.RequiredVotes = 2
	env.Consensus.CurrentVotes = env.Consensus.CurrentVotes[:0] // Clear votes
	tribunal.EvaluatePayload(env)                               // Only 1 vote
	err = warden.AuthorizeExecution(env)
	if err == nil {
		t.Error("Expected error on insufficient quorum, got nil")
	}
}

func TestGovernanceFailClosed(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	nodeID := "test-node-1"

	t.Run("SentinelNil_FailClosed", func(t *testing.T) {
		tribunal := &Tribunal{
			NodeID:     nodeID,
			PrivateKey: priv,
			Sentinel:   nil, // explicitly nil
		}
		isSafe := tribunal.RunMITREChecks("test", "echo 'hello'")
		if isSafe {
			t.Error("Expected fail-closed (Safe=false) when Sentinel is nil")
		}
	})

	t.Run("UnsignedVote_Blocked", func(t *testing.T) {
		warden := &Warden{
			TrustedNodes: map[string]ed25519.PublicKey{nodeID: pub},
		}
		env := &uap.UAPEnvelope{
			MessageID: "test-id",
			Consensus: uap.ConsensusState{
				RequiredVotes: 1,
				CurrentVotes: []uap.Vote{
					{NodeID: nodeID, Signature: "UNSIGNED", Decision: true},
				},
			},
		}
		err := warden.AuthorizeExecution(env)
		if err == nil || err.Error() != "execution blocked: required votes not met" {
			t.Errorf("Expected UNSIGNED vote to be blocked, got: %v", err)
		}
	})

	t.Run("InvalidSignature_Blocked", func(t *testing.T) {
		warden := &Warden{
			TrustedNodes: map[string]ed25519.PublicKey{nodeID: pub},
		}
		env := &uap.UAPEnvelope{
			MessageID: "test-id",
			Consensus: uap.ConsensusState{
				RequiredVotes: 1,
				CurrentVotes: []uap.Vote{
					{NodeID: nodeID, Signature: "deadbeef", Decision: true},
				},
			},
		}
		err := warden.AuthorizeExecution(env)
		if err == nil || err.Error() != "execution blocked: required votes not met" {
			t.Errorf("Expected invalid signature to be blocked, got: %v", err)
		}
	})

	t.Run("MissingPrivateKey_Panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic when PrivateKey is nil during SignDecision")
			}
		}()
		tribunal := &Tribunal{NodeID: nodeID, PrivateKey: nil}
		tribunal.SignDecision("test-id", true)
	})
}
