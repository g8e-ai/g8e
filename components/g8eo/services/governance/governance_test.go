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
	tribunal.EvaluatePayload(env)                              // Only 1 vote
	err = warden.AuthorizeExecution(env)
	if err == nil {
		t.Error("Expected error on insufficient quorum, got nil")
	}
}
