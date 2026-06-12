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

package mcp

import (
	"encoding/json"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFieldPathRegistry(t *testing.T) {
	t.Parallel()

	logger := slog.Default()
	registry, err := NewFieldPathRegistry(logger)
	require.NoError(t, err)
	require.NotNil(t, registry)
}

func TestValidateFieldPath_AllowedPaths(t *testing.T) {
	t.Parallel()

	logger := slog.Default()
	registry, err := NewFieldPathRegistry(logger)
	require.NoError(t, err)

	tests := []struct {
		name       string
		collection string
		fieldPath  string
		wantErr    error
	}{
		{
			name:       "valid investigation field",
			collection: "investigations",
			fieldPath:  "suspect_ip_addresses",
			wantErr:    nil,
		},
		{
			name:       "valid nested field",
			collection: "investigations",
			fieldPath:  "metadata.tags.priority",
			wantErr:    nil,
		},
		{
			name:       "valid memory field",
			collection: "memories",
			fieldPath:  "content",
			wantErr:    nil,
		},
		{
			name:       "valid case field",
			collection: "cases",
			fieldPath:  "status",
			wantErr:    nil,
		},
		{
			name:       "empty collection",
			collection: "",
			fieldPath:  "status",
			wantErr:    ErrEmptyCollection,
		},
		{
			name:       "empty field path",
			collection: "investigations",
			fieldPath:  "",
			wantErr:    ErrEmptyFieldPath,
		},
		{
			name:       "invalid collection",
			collection: "unknown_collection",
			fieldPath:  "status",
			wantErr:    ErrInvalidCollection,
		},
		{
			name:       "forbidden field - credentials",
			collection: "investigations",
			fieldPath:  "credentials",
			wantErr:    ErrForbiddenFieldPath,
		},
		{
			name:       "forbidden field - api_keys",
			collection: "memories",
			fieldPath:  "api_keys",
			wantErr:    ErrForbiddenFieldPath,
		},
		{
			name:       "forbidden nested field",
			collection: "investigations",
			fieldPath:  "metadata.credentials.password",
			wantErr:    ErrForbiddenFieldPath,
		},
		{
			name:       "field not in allowlist",
			collection: "investigations",
			fieldPath:  "unknown_field",
			wantErr:    ErrInvalidFieldPath,
		},
		{
			name:       "invalid path syntax - empty component",
			collection: "investigations",
			fieldPath:  "suspect_ip_addresses.",
			wantErr:    ErrInvalidPathSyntax,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := registry.ValidateFieldPath(tt.collection, tt.fieldPath)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParseFieldPath(t *testing.T) {
	t.Parallel()

	doc := json.RawMessage(`{
		"suspect_ip_addresses": ["192.168.1.42", "10.0.0.5"],
		"status": "open",
		"priority": 1,
		"metadata": {
			"tags": {
				"priority": "high"
			}
		}
	}`)

	tests := []struct {
		name      string
		document  json.RawMessage
		fieldPath string
		want      interface{}
		wantErr   bool
	}{
		{
			name:      "simple field",
			document:  doc,
			fieldPath: "status",
			want:      "open",
			wantErr:   false,
		},
		{
			name:      "array field",
			document:  doc,
			fieldPath: "suspect_ip_addresses",
			want:      []interface{}{"192.168.1.42", "10.0.0.5"},
			wantErr:   false,
		},
		{
			name:      "nested field",
			document:  doc,
			fieldPath: "metadata.tags.priority",
			want:      "high",
			wantErr:   false,
		},
		{
			name:      "field not found",
			document:  doc,
			fieldPath: "unknown_field",
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "nested field not found",
			document:  doc,
			fieldPath: "metadata.unknown",
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "empty field path",
			document:  doc,
			fieldPath: "",
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "nil document",
			document:  nil,
			fieldPath: "status",
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "invalid JSON",
			document:  json.RawMessage(`invalid json`),
			fieldPath: "status",
			want:      nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseFieldPath(tt.document, tt.fieldPath)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
