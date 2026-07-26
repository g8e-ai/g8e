// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package governance

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/timesvc"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// GovernanceEnvelope is an alias for the canonical GovernanceEnvelope proto message.
// This preserves JSON compatibility for inbound requests while enforcing
// a single schema for both directions.
type GovernanceEnvelope = commonv1.GovernanceEnvelope

// GenerateMessageID creates a deterministic hash of the critical envelope fields.
// Canonicalization rules (from docs/architecture/governance.md):
// - Field names in proto definition order
// - Strings as UTF-8
// - Numbers as decimal integers
// - Absent optional fields omitted
// - Nested messages recursed
// - Bytes as base64
// - Result hashed with SHA-256
func GenerateMessageID(env *GovernanceEnvelope) (string, error) {
	if env == nil {
		return "", constants.ErrTxInvalidEnvelope
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
		canonical.WriteString(timesvc.FormatTimestamp(expiresAt))
		canonical.WriteByte('|')
	}

	// 7. intent_data (struct) - canonicalized recursively
	if env.IntentData != nil {
		intentStr, err := canonicalizeStruct(env.IntentData)
		if err != nil {
			return "", fmt.Errorf("envelope: canonicalize intent_data: %w", err)
		}
		canonical.WriteString(intentStr)
		canonical.WriteByte('|')
	}

	// 8. requestor_user_id (string) - the human user who authorized the action
	if env.RequestorUserId != "" {
		canonical.WriteString(env.RequestorUserId)
		canonical.WriteByte('|')
	}

	// 9. acting_app_id (string) - the app/tool acting on behalf of the user
	if env.ActingAppId != "" {
		canonical.WriteString(env.ActingAppId)
		canonical.WriteByte('|')
	}

	// NOTE: L3 proof is intentionally NOT included in the transaction hash.
	// The protocol ordering is L1 → L2 → L3 → L4: L2 (machine consensus) signs
	// the transaction hash before L3 (human notary) is asked. Including L3 in
	// the hash would create a circular dependency — L2 couldn't sign until the
	// human had already acted, violating the invariant that the human is never
	// bothered until all machine-checkable layers pass.
	// Tamper-evidence for L3 is provided by verifyL3Posture, which checks the
	// proof against envelope.TransactionHash at verification time.

	canonicalStr := canonical.String()
	hash := sha256.Sum256([]byte(canonicalStr))
	return hex.EncodeToString(hash[:]), nil
}

// canonicalizeStruct recursively converts a structpb.Struct to a deterministic
// string representation. Keys are sorted alphabetically. Values are serialized
// based on type. Returns an error if any value is of an unsupported type.
func canonicalizeStruct(s *structpb.Struct) (string, error) {
	if s == nil || len(s.Fields) == 0 {
		return "", nil
	}

	keys := make([]string, 0, len(s.Fields))
	for k := range s.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var canonical strings.Builder
	for i, k := range keys {
		canonical.WriteString(k)
		canonical.WriteByte('=')
		valStr, err := canonicalizeValue(s.Fields[k])
		if err != nil {
			return "", fmt.Errorf("envelope: canonicalize key %q: %w", k, err)
		}
		canonical.WriteString(valStr)
		if i < len(keys)-1 {
			canonical.WriteByte(',')
		}
	}
	return canonical.String(), nil
}

// canonicalizeValue converts a structpb.Value to its canonical string representation.
// Returns an error for types outside the explicit switch cases to ensure
// cross-language hash parity — unknown types would produce different output
// in Go vs Python.
func canonicalizeValue(v *structpb.Value) (string, error) {
	if v == nil || v.Kind == nil {
		return "", nil
	}
	switch kind := v.Kind.(type) {
	case *structpb.Value_StringValue:
		return kind.StringValue, nil
	case *structpb.Value_NumberValue:
		return fmt.Sprintf("%f", kind.NumberValue), nil
	case *structpb.Value_BoolValue:
		return fmt.Sprintf("%t", kind.BoolValue), nil
	case *structpb.Value_StructValue:
		return canonicalizeStruct(kind.StructValue)
	case *structpb.Value_ListValue:
		values := kind.ListValue.GetValues()
		parts := make([]string, 0, len(values))
		for _, item := range values {
			part, err := canonicalizeValue(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, part)
		}
		return "[" + strings.Join(parts, ",") + "]", nil
	case *structpb.Value_NullValue:
		return "", nil
	default:
		return "", fmt.Errorf("envelope: canonicalize: %w: type %T", constants.ErrTxCanonicalizeFailed, v.Kind)
	}
}
