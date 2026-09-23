// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type documentUpdateTestPayload struct {
	Active bool   `json:"active"`
	Name   string `json:"name"`
}

func TestDecodeDocumentUpdateFields_UsesTypedJSONObject(t *testing.T) {
	payload, err := json.Marshal(documentUpdateTestPayload{Active: false, Name: "worker-a"})
	require.NoError(t, err)

	fields, err := decodeDocumentUpdateFields(payload)
	require.NoError(t, err)
	require.Len(t, fields, 2)
	assert.Equal(t, "active", fields[0].Name)
	assert.Equal(t, json.RawMessage(`false`), fields[0].Value)
	assert.Equal(t, "name", fields[1].Name)
	assert.Equal(t, json.RawMessage(`"worker-a"`), fields[1].Value)
}

func TestDecodeDocumentUpdateFields_RejectsNonObjectAndEmptyObject(t *testing.T) {
	tests := []struct {
		name    string
		payload json.RawMessage
	}{
		{name: "array", payload: json.RawMessage(`[]`)},
		{name: "empty object", payload: json.RawMessage(`{}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeDocumentUpdateFields(tt.payload)
			require.Error(t, err)
		})
	}
}
