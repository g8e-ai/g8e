package governance

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	"github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/storage"
)

// Warden is the execution gateway. It is the final stop for all UAP envelopes.
// It ensures that consensus has been reached and quorum is met before execution.
type Warden struct {
	Logger       *slog.Logger
	TrustedNodes map[string]ed25519.PublicKey
	Execution    *execution.ExecutionService
	AuditVault   *storage.AuditVaultService
}

// AuthorizeExecution is the Warden's gatekeeper function.
func (w *Warden) AuthorizeExecution(env *uap.UAPEnvelope) error {
	// 1. Check Status
	if env.Consensus.Status == "REJECTED" {
		w.LogToSQLite(env, "BLOCKED_BY_TRIBUNAL")
		return errors.New("execution blocked: consensus rejected")
	}

	// 2. Count Valid Signatures (Byzantine Check)
	validApproveVotes := 0
	for _, vote := range env.Consensus.CurrentVotes {
		if vote.Decision == true && w.VerifySignature(vote.NodeID, vote.Signature, env.MessageID, vote.Decision) {
			validApproveVotes++
		}
	}

	// 3. Enforce Quorum
	if validApproveVotes < env.Consensus.RequiredVotes {
		w.LogToSQLite(env, "BLOCKED_INSUFFICIENT_QUORUM")
		return errors.New("execution blocked: required votes not met")
	}

	// 4. Execute and Commit Receipt
	// In a full implementation, we would proceed to:
	// w.ExecuteSystemCommand(env.Intent.ActionType, env.Intent.TargetResource)
	// w.CommitToGitSubstrate(env)
	w.LogToSQLite(env, "EXECUTED")

	return nil
}

// VerifySignature checks a node's signature on the message ID and decision.
func (w *Warden) VerifySignature(nodeID, signature, messageID string, decision bool) bool {
	pub, ok := w.TrustedNodes[nodeID]
	if !ok {
		return false
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	// Verify that the node signed (messageID | decision)
	payload := fmt.Sprintf("%s|%v", messageID, decision)
	return ed25519.Verify(pub, []byte(payload), sigBytes)
}

// LogToSQLite records the UAP transaction result in the audit vault.
func (w *Warden) LogToSQLite(env *uap.UAPEnvelope, status string) {
	if w.AuditVault == nil {
		return
	}

	event := &storage.Event{
		OperatorSessionID: env.Metadata.SenderID,
		Timestamp:         env.Metadata.Timestamp,
		Type:              storage.EventType("uap_transaction"),
		ContentText:       fmt.Sprintf("UAP Action: %s on %s (Status: %s)", env.Intent.ActionType, env.Intent.TargetResource, status),
		CommandRaw:        env.Context.DataBlob,
	}

	if _, err := w.AuditVault.RecordEvent(event); err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to record UAP transaction in audit vault", "error", err)
		}
	}
}
