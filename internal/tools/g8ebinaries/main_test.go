// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// writeArtifacts creates one non-empty file per catalogued target under root.
func writeArtifacts(t *testing.T, root string) {
	t.Helper()
	for _, target := range g8ebinaries.Targets() {
		require.NoError(t, os.WriteFile(filepath.Join(root, target.Filename), []byte("binary-"+target.Filename), constants.PermFileExecutable))
	}
}

func TestFormatTargets_ListsEveryCataloguedPlatformAsSpaceTerminatedTokens(t *testing.T) {
	got := formatTargets()

	fields := strings.Fields(got)
	targets := g8ebinaries.Targets()
	require.Len(t, fields, len(targets))
	for i, target := range targets {
		assert.Equal(t, target.OS+"/"+target.Arch, fields[i])
	}
	assert.True(t, strings.HasSuffix(got, " "), "each token is followed by a space")
}

func TestGenerate_ChecksumFilesUseSha256SumFormatOfArtifactBytes(t *testing.T) {
	root := t.TempDir()
	writeArtifacts(t, root)

	require.NoError(t, generate(root, "2.1.12", "build-1", "2026-09-23T00:00:00Z", "revision", "tree"))

	for _, target := range g8ebinaries.Targets() {
		digest := sha256.Sum256([]byte("binary-" + target.Filename))
		want := hex.EncodeToString(digest[:]) + "  " + target.Filename + "\n"
		got, err := os.ReadFile(filepath.Join(root, target.Checksum))
		require.NoError(t, err)
		assert.Equal(t, want, string(got), target.Checksum)
	}
}

func TestGenerate_ManifestRecordsProvenanceSizesAndDigests(t *testing.T) {
	root := t.TempDir()
	writeArtifacts(t, root)

	require.NoError(t, generate(root, "2.1.12", "build-1", "2026-09-23T00:00:00Z", "abc123", "treehash"))

	manifest, err := g8ebinaries.LoadManifest(root)
	require.NoError(t, err)
	assert.Equal(t, 1, manifest.SchemaVersion)
	assert.Equal(t, "2026-09-23T00:00:00Z", manifest.BuildTime)
	assert.Equal(t, "abc123", manifest.SourceRevision)
	assert.Equal(t, "treehash", manifest.SourceTreeHash)
	require.Len(t, manifest.Targets, len(g8ebinaries.Targets()))
	for _, artifact := range manifest.Targets {
		content := []byte("binary-" + artifact.Filename)
		digest := sha256.Sum256(content)
		assert.Equal(t, int64(len(content)), artifact.Size, artifact.Filename)
		assert.Equal(t, hex.EncodeToString(digest[:]), artifact.SHA256, artifact.Filename)
	}
}

func TestGenerate_PublishesManifestAtomicallyWithoutLeavingStagingFile(t *testing.T) {
	root := t.TempDir()
	writeArtifacts(t, root)

	require.NoError(t, generate(root, "2.1.12", "build-1", "2026-09-23T00:00:00Z", "revision", "tree"))

	_, err := os.Stat(filepath.Join(root, constants.G8eBinariesManifestFilename))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(root, ".g8e-binaries.json.new"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestGenerate_AllowsEmptySourceRevision(t *testing.T) {
	root := t.TempDir()
	writeArtifacts(t, root)

	require.NoError(t, generate(root, "2.1.12", "build-1", "2026-09-23T00:00:00Z", "", "tree"))
}

func TestGenerate_RejectsBlankProvenanceFields(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		buildID  string
		treeHash string
	}{
		{name: "empty version", version: "", buildID: "build-1", treeHash: "tree"},
		{name: "whitespace version", version: "   ", buildID: "build-1", treeHash: "tree"},
		{name: "empty build id", version: "2.1.12", buildID: "", treeHash: "tree"},
		{name: "whitespace build id", version: "2.1.12", buildID: "\t", treeHash: "tree"},
		{name: "empty source tree hash", version: "2.1.12", buildID: "build-1", treeHash: ""},
		{name: "whitespace source tree hash", version: "2.1.12", buildID: "build-1", treeHash: " \n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeArtifacts(t, root)

			err := generate(root, tt.version, tt.buildID, "2026-09-23T00:00:00Z", "revision", tt.treeHash)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "provenance fields must not be empty")
			_, statErr := os.Stat(filepath.Join(root, constants.G8eBinariesManifestFilename))
			assert.ErrorIs(t, statErr, os.ErrNotExist, "no manifest may be published for invalid provenance")
		})
	}
}

func TestGenerate_RejectsNonRFC3339BuildTime(t *testing.T) {
	root := t.TempDir()
	writeArtifacts(t, root)

	err := generate(root, "2.1.12", "build-1", "2026-09-23 00:00:00", "revision", "tree")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid build time")
}

func TestGenerate_ReportsFirstMissingArtifactByFilename(t *testing.T) {
	root := t.TempDir()
	targets := g8ebinaries.Targets()
	missing := targets[len(targets)-1]
	for _, target := range targets[:len(targets)-1] {
		require.NoError(t, os.WriteFile(filepath.Join(root, target.Filename), []byte("binary"), constants.PermFileExecutable))
	}

	err := generate(root, "2.1.12", "build-1", "2026-09-23T00:00:00Z", "revision", "tree")

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), missing.Filename)
	_, statErr := os.Stat(filepath.Join(root, constants.G8eBinariesManifestFilename))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestGenerate_RejectsEmptyArtifactBecauseManifestRequiresPositiveSize(t *testing.T) {
	root := t.TempDir()
	writeArtifacts(t, root)
	empty := g8ebinaries.Targets()[0]
	require.NoError(t, os.WriteFile(filepath.Join(root, empty.Filename), nil, constants.PermFileExecutable))

	err := generate(root, "2.1.12", "build-1", "2026-09-23T00:00:00Z", "revision", "tree")

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrG8eBinaryManifest)
	assert.Contains(t, err.Error(), empty.Filename)
	_, statErr := os.Stat(filepath.Join(root, constants.G8eBinariesManifestFilename))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestGenerate_FailsWhenArtifactDirectoryDoesNotExist(t *testing.T) {
	err := generate(filepath.Join(t.TempDir(), "absent"), "2.1.12", "build-1", "2026-09-23T00:00:00Z", "revision", "tree")

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
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
