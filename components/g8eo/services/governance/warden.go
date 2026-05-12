package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/components/g8eo/internal/mappings"
	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/storage"
)

type L3Verifier interface {
	VerifyL3Proof(userID, messageID, signatureHex, pubKeyHex string) (bool, error)
}

// ExecutionHandler is the interface for executing verified transactions.
// This avoids import cycles between governance and pubsub packages.
type ExecutionHandler interface {
	ExecuteVerifiedTransaction(ctx context.Context, eventType string, cmdMsg interface{}) error
}

// Warden is the execution gateway. It is the final stop for all UAP envelopes.
// It ensures that consensus has been reached and quorum is met before execution.
type Warden struct {
	Logger           *slog.Logger
	TrustedNodes     map[string]ed25519.PublicKey
	Execution        *execution.ExecutionService
	AuditVault       *storage.AuditVaultService
	L3Verifier       L3Verifier
	Ctx              context.Context
	ExecutionHandler ExecutionHandler
}

// AuthorizeExecution is the Warden's gatekeeper function.
func (w *Warden) AuthorizeExecution(env *uap.UAPEnvelope) error {
	// 1. Check L1 Technical Bedrock
	if env.Governance != nil && env.Governance.L1 != nil && !env.Governance.L1.Validated {
		w.LogToSQLite(env, "BLOCKED_BY_L1")
		return fmt.Errorf("execution blocked: L1 validation failed: %v", env.Governance.L1.Violations)
	}

	// 2. Count Valid Signatures (L2 Consensus)
	validApproveVotes := 0
	if env.Governance != nil && env.Governance.L2 != nil {
		// In proto-first, we use TribunalSignature as proof of consensus
		// For transition, we check if ANY agent ID signed it
		if env.Governance.L2.TribunalSignature != "" {
			for _, agentID := range env.Governance.L2.AgentIds {
				if w.VerifySignature(agentID, env.Governance.L2.TribunalSignature, env.Id, true) {
					validApproveVotes++
				}
			}
		}
	}

	// 3. Enforce Quorum (Placeholder: 1 vote required during transition)
	requiredVotes := 1
	if validApproveVotes < requiredVotes {
		w.LogToSQLite(env, "BLOCKED_INSUFFICIENT_QUORUM")
		return errors.New("execution blocked: required consensus votes not met")
	}

	// 4. Verify L3 Human Proof (Sovereign Authority)
	if w.IsMutation(env.ActionType) {
		if env.Governance == nil || env.Governance.L3 == nil || env.Governance.L3.HumanSignature == "" {
			w.LogToSQLite(env, "BLOCKED_L3_MISSING")
			return errors.New("execution blocked: L3 human signature missing for mutation")
		}
		if w.L3Verifier != nil {
			ok, err := w.L3Verifier.VerifyL3Proof(env.OperatorId, env.Id, env.Governance.L3.HumanSignature, env.Governance.L3.PublicKey)
			if err != nil || !ok {
				w.LogToSQLite(env, "BLOCKED_L3_INVALID")
				return fmt.Errorf("execution blocked: L3 human proof failed: %v", err)
			}
		}
	}

	// 5. Execute and Commit Receipt
	w.LogToSQLite(env, "EXECUTED")

	return nil
}

// VerifySignature checks a node's signature on the message ID and decision.
func (w *Warden) VerifySignature(nodeID, signature, messageID string, decision bool) bool {
	pub, ok := w.TrustedNodes[nodeID]
	if !ok {
		return false
	}
	if signature == "" || signature == "UNSIGNED" {
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

// IsMutation returns true if the action type modifies the system state.
func (w *Warden) IsMutation(actionType string) bool {
	switch actionType {
	case "EXECUTE_BASH", "FILE_EDIT", "RESTORE_FILE", "SHUTDOWN":
		return true
	default:
		return false
	}
}

// LogToSQLite records the UAP transaction result in the audit vault.
func (w *Warden) LogToSQLite(env *uap.UAPEnvelope, status string) {
	if w.AuditVault == nil {
		return
	}

	// For proto-first, we use fields directly
	var cmdData string
	if env.IntentData != nil && len(env.IntentData.Fields) > 0 {
		jsonBytes, _ := env.IntentData.MarshalJSON()
		cmdData = string(jsonBytes)
	} else {
		cmdData = string(env.Payload)
	}

	event := &storage.Event{
		OperatorSessionID: env.OperatorId,
		Timestamp:         env.Timestamp.AsTime(),
		Type:              storage.EventType("uap_transaction"),
		ContentText:       fmt.Sprintf("UAP Action: %s on %s (Status: %s)", env.ActionType, env.TargetResource, status),
		CommandRaw:        cmdData,
	}

	if _, err := w.AuditVault.RecordEvent(event); err != nil {
		if w.Logger != nil {
			w.Logger.Error("Failed to record UAP transaction in audit vault", "error", err)
		}
	}
}

// ExecuteVerifiedTransaction executes a verified transaction through the execution handler.
// This makes Warden the execution boundary - mutations only execute through this method.
func (w *Warden) ExecuteVerifiedTransaction(vt *VerifiedTransaction, cmdMsg interface{}) error {
	if w.ExecutionHandler == nil {
		return errors.New("Warden ExecutionHandler not set")
	}

	// Map action type to event type for handler lookup
	eventType := mappings.MapActionTypeToEventType(vt.ActionType)

	// Invoke the handler through Warden (execution boundary)
	w.Logger.Info("Warden executing transaction", "event_type", eventType, "message_id", vt.Envelope.Id)
	return w.ExecutionHandler.ExecuteVerifiedTransaction(w.Ctx, eventType, cmdMsg)
}
