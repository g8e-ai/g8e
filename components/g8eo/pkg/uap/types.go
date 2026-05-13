package uap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
)

// UAPEnvelope is an alias for the canonical UniversalEnvelope proto message.
// This preserves JSON compatibility for inbound requests while enforcing
// a single schema for both directions.
type UAPEnvelope = commonv1.UniversalEnvelope

// GenerateMessageID creates a deterministic hash of the critical payload.
func GenerateMessageID(env *UAPEnvelope) (string, error) {
	intentData := map[string]interface{}{}
	if env.IntentData != nil {
		intentData = env.IntentData.AsMap()
	}

	expiresAt := time.Time{}
	if env.ExpiresAt != nil {
		expiresAt = env.ExpiresAt.AsTime()
	}

	// Include all critical fields that bind the intent to the state and time
	payload, err := json.Marshal(struct {
		ActionType      string                 `json:"action_type"`
		TargetResource  string                 `json:"target_resource"`
		IntentData      map[string]interface{} `json:"intent_data"`
		Payload         []byte                 `json:"payload"`
		StateMerkleRoot string                 `json:"state_merkle_root"`
		ExpiresAt       time.Time              `json:"expires_at"`
		Nonce           string                 `json:"nonce"`
	}{
		ActionType:      env.ActionType,
		TargetResource:  env.TargetResource,
		IntentData:      intentData,
		Payload:         env.Payload,
		StateMerkleRoot: env.StateMerkleRoot,
		ExpiresAt:       expiresAt,
		Nonce:           env.Nonce,
	})

	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}
