// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
