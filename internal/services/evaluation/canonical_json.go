// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// MarshalCanonicalJSONObject returns deterministic JSON bytes for one decoded
// JSON value. Object keys are sorted lexicographically at every depth. Leaf
// values use encoding/json.Marshal, including HTML-safe escapes for <, >, and &.
// This is the authoritative wire form for chat-probe trace digests.
func MarshalCanonicalJSONObject(value any) ([]byte, error) {
	return marshalSortedJSON(value)
}

func marshalSortedJSON(value any) ([]byte, error) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var buf bytes.Buffer
		buf.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			keyBytes, err := json.Marshal(key)
			if err != nil {
				return nil, err
			}
			buf.Write(keyBytes)
			buf.WriteByte(':')
			valueBytes, err := marshalSortedJSON(typed[key])
			if err != nil {
				return nil, err
			}
			buf.Write(valueBytes)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil
	case []any:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, item := range typed {
			if i > 0 {
				buf.WriteByte(',')
			}
			itemBytes, err := marshalSortedJSON(item)
			if err != nil {
				return nil, err
			}
			buf.Write(itemBytes)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	default:
		return json.Marshal(typed)
	}
}

// ComputeChatProbeTraceDigest returns the SHA-256 digest for one decoded
// chat-probe trace object with trace_digest cleared before canonicalization.
func ComputeChatProbeTraceDigest(trace map[string]any) (string, error) {
	if len(trace) == 0 {
		return "", fmt.Errorf("evaluation: compute chat probe trace digest: trace is required")
	}
	payload := make(map[string]any, len(trace))
	for key, value := range trace {
		payload[key] = value
	}
	payload["trace_digest"] = ""
	canonicalRaw, err := marshalSortedJSON(payload)
	if err != nil {
		return "", fmt.Errorf("evaluation: compute chat probe trace digest: %w", err)
	}
	return models.SHA256Hex(canonicalRaw), nil
}
