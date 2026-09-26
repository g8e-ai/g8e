// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)

// GovernanceEnvelope is an alias for the canonical GovernanceEnvelope proto message.
// This preserves JSON compatibility for inbound requests while enforcing
// a single schema for both directions.
type GovernanceEnvelope = commonv1.GovernanceEnvelope

const (
	// GovernanceProtocolVersionV1 is retained for historical record verification only.
	GovernanceProtocolVersionV1 = "1.0"
	// GovernanceProtocolVersionV2 is required for all new ingress.
	GovernanceProtocolVersionV2 = "2"
	txHashV2Prefix              = "g8e-tx-v2|"
)

// GenerateMessageID creates a deterministic hash of the critical envelope fields.
// Canonicalization rules (from docs/architecture/governance.md):
//   - Fields appended in the documented spec order: action_type,
//     target_resource, payload, state_merkle_root, nonce, expires_at,
//     intent_data, requestor_user_id, acting_app_id, operator_id,
//     operator_session_id, case_id, investigation_id, task_id,
//     web_session_id, cli_session_id
//   - Strings as UTF-8
//   - Numbers as decimal integers
//   - Absent optional fields omitted
//   - Nested messages recursed
//   - Bytes as base64
//   - Result hashed with SHA-256
func GenerateMessageID(env *GovernanceEnvelope) (string, error) {
	if env == nil {
		return "", constants.ErrTxInvalidEnvelope
	}
	switch envelopeProtocolVersion(env) {
	case GovernanceProtocolVersionV2:
		return generateMessageIDV2(env)
	case GovernanceProtocolVersionV1, "":
		return generateMessageIDV1(env)
	default:
		return "", constants.ErrTxProtocolVersionUnsupported
	}
}

// VerifyTransactionHash recomputes the canonical transaction hash for an envelope
// and compares it to the provided hash. Dispatches on protocol_version.
func VerifyTransactionHash(env *GovernanceEnvelope, expectedHash string) error {
	if env == nil {
		return constants.ErrTxInvalidEnvelope
	}
	computed, err := GenerateMessageID(env)
	if err != nil {
		return err
	}
	if expectedHash == "" {
		return constants.ErrTxTransactionHashMissing
	}
	if expectedHash != computed {
		return constants.ErrTxTransactionHashMismatch
	}
	return nil
}

func envelopeProtocolVersion(env *GovernanceEnvelope) string {
	if env == nil {
		return ""
	}
	return env.ProtocolVersion
}

func generateMessageIDV1(env *GovernanceEnvelope) (string, error) {
	canonicalStr, err := canonicalEnvelopeFieldsV1(env)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(canonicalStr))
	return hex.EncodeToString(hash[:]), nil
}

func generateMessageIDV2(env *GovernanceEnvelope) (string, error) {
	if env.EventType == "" {
		return "", constants.ErrTxEventTypeMissing
	}
	v1Canonical, err := canonicalEnvelopeFieldsV1(env)
	if err != nil {
		return "", err
	}
	var canonical strings.Builder
	canonical.WriteString(txHashV2Prefix)
	if env.ActionType != "" {
		canonical.WriteString(env.ActionType)
	}
	canonical.WriteByte('|')
	canonical.WriteString(env.EventType)
	canonical.WriteByte('|')
	canonical.WriteString(v1Canonical)
	hash := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(hash[:]), nil
}

func canonicalEnvelopeFieldsV1(env *GovernanceEnvelope) (string, error) {
	var canonical strings.Builder

	if env.ActionType != "" {
		canonical.WriteString(env.ActionType)
		canonical.WriteByte('|')
	}
	if env.TargetResource != "" {
		canonical.WriteString(env.TargetResource)
		canonical.WriteByte('|')
	}
	if len(env.Payload) > 0 {
		canonical.WriteString(base64.StdEncoding.EncodeToString(env.Payload))
		canonical.WriteByte('|')
	}
	if env.StateMerkleRoot != "" {
		canonical.WriteString(env.StateMerkleRoot)
		canonical.WriteByte('|')
	}
	if env.Nonce != "" {
		canonical.WriteString(env.Nonce)
		canonical.WriteByte('|')
	}
	if env.ExpiresAt != nil {
		expiresAt := env.ExpiresAt.AsTime()
		canonical.WriteString(timesvc.FormatTimestamp(expiresAt))
		canonical.WriteByte('|')
	}
	if env.IntentData != nil {
		intentStr, err := canonicalizeStruct(env.IntentData)
		if err != nil {
			return "", fmt.Errorf("envelope: canonicalize intent_data: %w", err)
		}
		canonical.WriteString(intentStr)
		canonical.WriteByte('|')
	}
	if env.RequestorUserId != "" {
		canonical.WriteString(env.RequestorUserId)
		canonical.WriteByte('|')
	}
	if env.ActingAppId != "" {
		canonical.WriteString(env.ActingAppId)
		canonical.WriteByte('|')
	}
	if env.OperatorId != "" {
		canonical.WriteString(env.OperatorId)
		canonical.WriteByte('|')
	}
	if env.OperatorSessionId != "" {
		canonical.WriteString(env.OperatorSessionId)
		canonical.WriteByte('|')
	}
	if env.CaseId != "" {
		canonical.WriteString(env.CaseId)
		canonical.WriteByte('|')
	}
	if env.InvestigationId != "" {
		canonical.WriteString(env.InvestigationId)
		canonical.WriteByte('|')
	}
	if env.TaskId != "" {
		canonical.WriteString(env.TaskId)
		canonical.WriteByte('|')
	}
	if env.WebSessionId != "" {
		canonical.WriteString(env.WebSessionId)
		canonical.WriteByte('|')
	}
	if env.CliSessionId != "" {
		canonical.WriteString(env.CliSessionId)
		canonical.WriteByte('|')
	}

	return canonical.String(), nil
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
