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
	"testing"

	"log/slog"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
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
			wantErr:    constants.ErrFieldPathEmptyCollection,
		},
		{
			name:       "empty field path",
			collection: "investigations",
			fieldPath:  "",
			wantErr:    constants.ErrFieldPathEmpty,
		},
		{
			name:       "invalid collection",
			collection: "unknown_collection",
			fieldPath:  "status",
			wantErr:    constants.ErrFieldPathInvalidCollection,
		},
		{
			name:       "forbidden field - credentials",
			collection: "investigations",
			fieldPath:  "credentials",
			wantErr:    constants.ErrFieldPathForbidden,
		},
		{
			name:       "forbidden field - api_keys",
			collection: "memories",
			fieldPath:  "api_keys",
			wantErr:    constants.ErrFieldPathForbidden,
		},
		{
			name:       "forbidden nested field",
			collection: "investigations",
			fieldPath:  "metadata.credentials.password",
			wantErr:    constants.ErrFieldPathForbidden,
		},
		{
			name:       "field not in allowlist",
			collection: "investigations",
			fieldPath:  "unknown_field",
			wantErr:    constants.ErrFieldPathInvalid,
		},
		{
			name:       "invalid path syntax - empty component",
			collection: "investigations",
			fieldPath:  "suspect_ip_addresses.",
			wantErr:    constants.ErrFieldPathSyntax,
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
