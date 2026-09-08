// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package jsonschema

import (
	"fmt"
	"strconv"
	"strings"
)

// escapeJSONPointerToken escapes a token per RFC 6901: ~ becomes ~0, / becomes ~1.
func escapeJSONPointerToken(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return token
}

// unescapeJSONPointerToken unescapes a token per RFC 6901: ~1 becomes /, ~0 becomes ~.
func unescapeJSONPointerToken(token string) string {
	token = strings.ReplaceAll(token, "~1", "/")
	token = strings.ReplaceAll(token, "~0", "~")
	return token
}

// resolveJSONPointer resolves a JSON Pointer (RFC 6901) against a JSON value.
// It returns the resolved value or an error if the pointer is invalid or
// does not resolve.
func resolveJSONPointer(root any, ptr string) (any, error) {
	if ptr == "" {
		return root, nil
	}
	if !strings.HasPrefix(ptr, "/") {
		return nil, fmt.Errorf("invalid JSON Pointer: must start with / or be empty")
	}
	tokens := strings.Split(ptr[1:], "/")
	current := root
	for _, token := range tokens {
		token = unescapeJSONPointerToken(token)
		switch v := current.(type) {
		case map[string]any:
			val, ok := v[token]
			if !ok {
				return nil, fmt.Errorf("JSON Pointer token %q not found", token)
			}
			current = val
		case []any:
			idx, err := strconv.Atoi(token)
			if err != nil {
				return nil, fmt.Errorf("JSON Pointer token %q is not a valid array index", token)
			}
			if idx < 0 || idx >= len(v) {
				return nil, fmt.Errorf("JSON Pointer index %d out of bounds (len=%d)", idx, len(v))
			}
			current = v[idx]
		default:
			return nil, fmt.Errorf("JSON Pointer token %q cannot resolve into non-container value", token)
		}
	}
	return current, nil
}
