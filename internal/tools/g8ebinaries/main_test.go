// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/g8ebinaries"
)

func TestGenerate_WritesChecksumsAndValidatedManifest(t *testing.T) {
	root := t.TempDir()
	for _, target := range g8ebinaries.Targets() {
		require.NoError(t, os.WriteFile(filepath.Join(root, target.Filename), []byte("binary-"+target.Filename), constants.PermFileExecutable))
	}

	require.NoError(t, generate(root, "2.1.12", "build-1", "2026-09-23T00:00:00Z", "revision", "tree"))
	manifest, err := g8ebinaries.LoadManifest(root)
	require.NoError(t, err)
	assert.Equal(t, "2.1.12", manifest.Version)
	assert.Equal(t, "build-1", manifest.BuildID)
	for _, target := range g8ebinaries.Targets() {
		checksum, err := os.ReadFile(filepath.Join(root, target.Checksum))
		require.NoError(t, err)
		assert.Contains(t, string(checksum), target.Filename)
	}
}

func TestGenerate_RejectsInvalidProvenanceAndMissingArtifact(t *testing.T) {
	root := t.TempDir()
	err := generate(root, "", "build-1", "2026-09-23T00:00:00Z", "revision", "tree")
	assert.Error(t, err)
	err = generate(root, "2.1.12", "build-1", "invalid", "revision", "tree")
	assert.Error(t, err)
	err = generate(root, "2.1.12", "build-1", "2026-09-23T00:00:00Z", "revision", "tree")
	assert.Error(t, err)
}

func TestGenerate_DefaultsBuildTime(t *testing.T) {
	root := t.TempDir()
	for _, target := range g8ebinaries.Targets() {
		require.NoError(t, os.WriteFile(filepath.Join(root, target.Filename), []byte("binary"), constants.PermFileExecutable))
	}
	require.NoError(t, generate(root, "2.1.12", "build-1", "", "revision", "tree"))
	data, err := os.ReadFile(filepath.Join(root, constants.G8eBinariesManifestFilename))
	require.NoError(t, err)
	var manifest g8ebinaries.Manifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	assert.NotEmpty(t, manifest.BuildTime)
}
