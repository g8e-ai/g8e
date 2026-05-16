package governance

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"os"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockExecutionHandler struct {
	executed bool
	err      error
}

func (m *mockExecutionHandler) ExecuteVerifiedTransaction(ctx context.Context, eventType string, cmdMsg interface{}) error {
	m.executed = true
	return m.err
}

func TestGovernanceFlow(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	nodeID := "test-node-1"

	tribunal := &Tribunal{
		NodeID:     nodeID,
		PrivateKey: priv,
	}

	warden := &Warden{
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
		SignerStore: &SimpleSignerStore{
			Signers: map[string]ed25519.PublicKey{
				nodeID: pub,
			},
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

	handler := &mockExecutionHandler{}
	warden.ExecutionHandler = handler
	warden.SigningKey = priv
	warden.KeyID = nodeID
	warden.Ctx = context.Background()

	vt := &VerifiedTransaction{
		Envelope:   env,
		ActionType: env.ActionType,
	}

	// 3. Warden Execution
	receipt, err := warden.Execute(context.Background(), vt, nil)
	if err != nil {
		t.Fatalf("Warden execution failed: %v", err)
	}

	if !handler.executed {
		t.Error("Expected handler to be executed")
	}

	if receipt.TransactionId != env.Id {
		t.Errorf("Expected receipt tx id %s, got %s", env.Id, receipt.TransactionId)
	}
}

func TestGovernanceFailClosed(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
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
