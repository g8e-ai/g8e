package uap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// UAPEnvelope is the unforgiving root structure for all g8e communication.
type UAPEnvelope struct {
	ProtocolVersion string    `json:"protocol_version"`
	MessageID       string    `json:"message_id"` // SHA-256 hash of critical intent/context fields
	Timestamp       time.Time `json:"timestamp"`
	ExpiresAt       time.Time `json:"expires_at"`

	Metadata Metadata `json:"metadata"`
	Intent   Intent   `json:"intent"`
	Context  Context  `json:"context"`

	// Governance Metadata (L1/L2/L3)
	Governance GovernanceMetadata `json:"governance"`

	// Consensus is the legacy UAP field, kept for internal tracking but Governance is the source of truth
	Consensus ConsensusState `json:"consensus_state"`

	// State root binding
	StateMerkleRoot string `json:"state_merkle_root"`

	// Replay protection
	Nonce string `json:"nonce"`

	// Structured intent data for JSON-first protocol
	IntentData map[string]interface{} `json:"intent_data,omitempty"`

	// Application-layer identifiers
	CaseID          string  `json:"case_id,omitempty"`
	InvestigationID string  `json:"investigation_id,omitempty"`
	TaskID          *string `json:"task_id,omitempty"`

	// Receipt and Audit identifiers
	ReceiptID string `json:"receipt_id,omitempty"`
	AuditID   string `json:"audit_id,omitempty"`

	// Payload is DEPRECATED in favor of IntentData
	Payload []byte `json:"payload,omitempty"`
}

type Metadata struct {
	SenderID  string    `json:"sender_id"`
	Timestamp time.Time `json:"timestamp"` // DEPRECATED: use UAPEnvelope.Timestamp
	Signature string    `json:"signature"` // Sender's mTLS cert signature
}

type Intent struct {
	ActionType     string `json:"action_type"`     // e.g., "EXECUTE_BASH", "QUERY_DB"
	TargetResource string `json:"target_resource"` // e.g., "prod-db-01", "k8s-namespace-default"
}

type Context struct {
	DataFormat string                 `json:"data_format"` // Must be strictly defined, e.g., "markdown", "raw", "json"
	IntentData map[string]interface{} `json:"intent_data"` // Structured intent parameters (replaces DataBlob)
	DataBlob   string                 `json:"data_blob"`   // DEPRECATED: Use IntentData
}

type GovernanceMetadata struct {
	L1 L1Metadata `json:"l1"`
	L2 L2Metadata `json:"l2"`
	L3 L3Metadata `json:"l3"`
}

type L1Metadata struct {
	Validated  bool     `json:"validated"`
	Violations []string `json:"violations,omitempty"`
}

type L2Metadata struct {
	TribunalSignature string   `json:"tribunal_signature"`
	AgentIDs          []string `json:"agent_ids,omitempty"`
	KeyID             string   `json:"key_id,omitempty"`
}

type L3Metadata struct {
	HumanSignature string `json:"human_signature"`
	PublicKey      string `json:"public_key"`
	AutoApproved   bool   `json:"auto_approved"`
}

type ConsensusState struct {
	RequiredVotes int    `json:"required_votes"`
	CurrentVotes  []Vote `json:"current_votes"`
	Status        string `json:"status"` // "PENDING", "APPROVED", "REJECTED"
}

type Vote struct {
	NodeID    string `json:"node_id"`
	Signature string `json:"signature"`
	Decision  bool   `json:"decision"` // True = Approve, False = Reject
}

// GenerateMessageID creates a deterministic hash of the critical payload.
func (env *UAPEnvelope) GenerateMessageID() (string, error) {
	// Include all critical fields that bind the intent to the state and time
	payload, err := json.Marshal(struct {
		Intent          Intent                 `json:"intent"`
		IntentData      map[string]interface{} `json:"intent_data"`
		StateMerkleRoot string                 `json:"state_merkle_root"`
		ExpiresAt       time.Time              `json:"expires_at"`
		Nonce           string                 `json:"nonce"`
	}{
		Intent:          env.Intent,
		IntentData:      env.IntentData,
		StateMerkleRoot: env.StateMerkleRoot,
		ExpiresAt:       env.ExpiresAt,
		Nonce:           env.Nonce,
	})

	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}
