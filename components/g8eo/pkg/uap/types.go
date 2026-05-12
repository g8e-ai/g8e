package uap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// UAPEnvelope is the unforgiving root structure for all g8e communication.
type UAPEnvelope struct {
	ProtocolVersion string         `json:"protocol_version"`
	MessageID       string         `json:"message_id"` // SHA-256 hash of Intent + Context
	Metadata        Metadata       `json:"metadata"`
	Intent          Intent         `json:"intent"`
	Context         Context        `json:"context"`
	Consensus       ConsensusState `json:"consensus_state"`

	// Structured intent data for JSON-first protocol
	IntentData map[string]interface{} `json:"intent_data,omitempty"`

	// Application-layer identifiers
	CaseID          string  `json:"case_id,omitempty"`
	InvestigationID string  `json:"investigation_id,omitempty"`
	TaskID          *string `json:"task_id,omitempty"`

	// Payload is an alias for Context.DataBlob in some contexts, or raw bytes
	Payload []byte `json:"payload,omitempty"`
}

type Metadata struct {
	SenderID  string    `json:"sender_id"`
	Timestamp time.Time `json:"timestamp"`
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
	// If IntentData is present, include it in the hash for JSON-first protocol
	var payload []byte
	var err error

	if len(env.IntentData) > 0 {
		payload, err = json.Marshal(struct {
			Intent     Intent                 `json:"intent"`
			IntentData map[string]interface{} `json:"intent_data"`
		}{
			Intent:     env.Intent,
			IntentData: env.IntentData,
		})
	} else {
		payload, err = json.Marshal(struct {
			Intent  Intent  `json:"intent"`
			Context Context `json:"context"`
		}{
			Intent:  env.Intent,
			Context: env.Context,
		})
	}

	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}
