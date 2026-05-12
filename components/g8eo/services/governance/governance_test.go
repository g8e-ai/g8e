package governance

import (
	"crypto/ed25519"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	"google.golang.org/protobuf/types/known/timestamppb"
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
		OperatorId:      "agent-1",
		Timestamp:       timestamppb.Now(),
		ActionType:      "FETCH_LOGS",
		TargetResource:  "localhost",
		Payload:         []byte("fetch logs"),
	}

	// 1. Generate Message ID
	id, _ := uap.GenerateMessageID(env)
	env.Id = id

	// 2. Tribunal Evaluation
	err := tribunal.EvaluatePayload(env)
	if err != nil {
		t.Fatalf("Tribunal evaluation failed: %v", err)
	}

	if env.Governance == nil || len(env.Governance.L2.AgentIds) != 1 {
		t.Errorf("Expected 1 agent ID in L2, got %v", env.Governance)
	}

	// Ensure status is validated for Warden
	env.Governance.L1.Validated = true
	env.Governance.L2.TribunalSignature = tribunal.SignDecision(env.Id, true)

	// 3. Warden Authorization
	err = warden.AuthorizeExecution(env)
	if err != nil {
		t.Fatalf("Warden authorization failed: %v", err)
	}

	// 4. Test Hash Mismatch
	env.Id = "wrong-hash"
	err = tribunal.EvaluatePayload(env)
	if err == nil {
		t.Error("Expected error on hash mismatch, got nil")
	}

	// 5. Test Insufficient Quorum
	env.Id = id // Reset
	env.Governance.L2.AgentIds = nil
	env.Governance.L2.TribunalSignature = ""
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
			Id: "test-id",
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					AgentIds:          []string{nodeID},
					TribunalSignature: "", // Missing signature
				},
			},
		}
		err := warden.AuthorizeExecution(env)
		if err == nil || err.Error() != "execution blocked: required consensus votes not met" {
			t.Errorf("Expected unsigned vote to be blocked, got: %v", err)
		}
	})

	t.Run("InvalidSignature_Blocked", func(t *testing.T) {
		warden := &Warden{
			TrustedNodes: map[string]ed25519.PublicKey{nodeID: pub},
		}
		env := &uap.UAPEnvelope{
			Id: "test-id",
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					AgentIds:          []string{nodeID},
					TribunalSignature: "deadbeef",
				},
			},
		}
		err := warden.AuthorizeExecution(env)
		if err == nil || err.Error() != "execution blocked: required consensus votes not met" {
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
