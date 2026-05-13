package uap

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	commonv1 "github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/commonv1"
)

// UAPEnvelope is an alias for the canonical UniversalEnvelope proto message.
// This preserves JSON compatibility for inbound requests while enforcing
// a single schema for both directions.
type UAPEnvelope = commonv1.UniversalEnvelope

// GenerateMessageID creates a deterministic hash of the critical envelope fields.
// Canonicalization rules (from docs/architecture/governance.md):
// - Field names in proto definition order
// - Strings as UTF-8
// - Numbers as decimal integers
// - Absent optional fields omitted
// - Nested messages recursed
// - Bytes as base64
// - Result hashed with SHA-256
func GenerateMessageID(env *UAPEnvelope) (string, error) {
	if env == nil {
		return "", fmt.Errorf("envelope is nil")
	}

	// Build canonical string representation in proto field order
	var canonical strings.Builder

	// 1. action_type (string)
	if env.ActionType != "" {
		canonical.WriteString(env.ActionType)
		canonical.WriteByte('|')
	}

	// 2. target_resource (string)
	if env.TargetResource != "" {
		canonical.WriteString(env.TargetResource)
		canonical.WriteByte('|')
	}

	// 3. payload (bytes) - base64 encoded
	if len(env.Payload) > 0 {
		canonical.WriteString(base64.StdEncoding.EncodeToString(env.Payload))
		canonical.WriteByte('|')
	}

	// 4. state_merkle_root (string)
	if env.StateMerkleRoot != "" {
		canonical.WriteString(env.StateMerkleRoot)
		canonical.WriteByte('|')
	}

	// 5. nonce (string)
	if env.Nonce != "" {
		canonical.WriteString(env.Nonce)
		canonical.WriteByte('|')
	}

	// 6. expires_at (timestamp) - UTC RFC3339 format
	if env.ExpiresAt != nil {
		expiresAt := env.ExpiresAt.AsTime()
		canonical.WriteString(expiresAt.UTC().Format(time.RFC3339Nano))
		canonical.WriteByte('|')
	}

	// 7. intent_data (struct) - canonicalized recursively
	if env.IntentData != nil {
		intentMap := env.IntentData.AsMap()
		canonical.WriteString(canonicalizeMap(intentMap))
		canonical.WriteByte('|')
	}

	canonicalStr := canonical.String()
	hash := sha256.Sum256([]byte(canonicalStr))
	return hex.EncodeToString(hash[:]), nil
}

// canonicalizeMap recursively converts a map to a deterministic string representation.
// Keys are sorted alphabetically. Values are serialized based on type.
func canonicalizeMap(m map[string]interface{}) string {
	if len(m) == 0 {
		return ""
	}

	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var canonical strings.Builder
	for i, k := range keys {
		canonical.WriteString(k)
		canonical.WriteByte('=')
		canonical.WriteString(canonicalizeValue(m[k]))
		if i < len(keys)-1 {
			canonical.WriteByte(',')
		}
	}
	return canonical.String()
}

// canonicalizeValue converts a value to its canonical string representation.
func canonicalizeValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", val)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	case float32, float64:
		return fmt.Sprintf("%f", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case []interface{}:
		// Arrays are not expected in intent_data, but handle defensively
		var parts []string
		for _, item := range val {
			parts = append(parts, canonicalizeValue(item))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case map[string]interface{}:
		return canonicalizeMap(val)
	case nil:
		return ""
	default:
		// Fallback to JSON for unknown types
		bytes, _ := json.Marshal(v)
		return string(bytes)
	}
}
