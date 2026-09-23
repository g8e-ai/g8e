//go:build integration

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package g8ebinaries

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestPublisherPublish_ValidatesAndReplacesCompleteMirror(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "mirror")
	require.NoError(t, os.MkdirAll(old, constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(filepath.Join(old, "old.txt"), []byte("old"), constants.PermFilePublic))

	archive := validArchive(t, "build-1", false)
	manifest, err := NewPublisher(old).Publish(bytes.NewReader(archive))

	require.NoError(t, err)
	assert.Equal(t, "build-1", manifest.BuildID)
	assert.NoFileExists(t, filepath.Join(old, "old.txt"))
	assert.FileExists(t, filepath.Join(old, constants.G8eBinariesManifestFilename))
	reader, err := OpenReader(old)
	require.NoError(t, err)
	assert.True(t, reader.HasManifest())
}

func TestPublisherPublish_AcceptsDockerCopyRootDirectory(t *testing.T) {
	output := filepath.Join(t.TempDir(), "mirror")

	manifest, err := NewPublisher(output).Publish(bytes.NewReader(validArchive(t, "build-1", true)))

	require.NoError(t, err)
	assert.Equal(t, "build-1", manifest.BuildID)
	assert.FileExists(t, filepath.Join(output, constants.G8eBinariesManifestFilename))
}

func TestPublisherPublish_RejectsUnsafeArchiveWithoutReplacingMirror(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "mirror")
	require.NoError(t, os.MkdirAll(output, constants.PermDirPrivate))
	marker := filepath.Join(output, "marker")
	require.NoError(t, os.WriteFile(marker, []byte("current"), constants.PermFilePublic))

	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "../escape", Mode: int64(constants.PermFilePublic), Size: 1}))
	_, err := writer.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	_, err = NewPublisher(output).Publish(&archive)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrG8eBinaryArchive)
	data, readErr := os.ReadFile(marker)
	require.NoError(t, readErr)
	assert.Equal(t, "current", string(data))
}

func TestPublisherPublish_RejectsDuplicateArchiveEntries(t *testing.T) {
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	for range 2 {
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: constants.G8eBinariesManifestFilename, Mode: int64(constants.PermFilePublic), Size: 2}))
		_, err := writer.Write([]byte("{}"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	_, err := NewPublisher(filepath.Join(t.TempDir(), "mirror")).Publish(&archive)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrG8eBinaryArchive)
}

func validArchive(t *testing.T, buildID string, includeRoot bool) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	if includeRoot {
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: int64(constants.PermDirPrivate)}))
	}
	artifacts := make([]Artifact, 0, len(Targets()))
	for _, target := range Targets() {
		data := []byte("binary-" + target.Filename)
		digest := sha256.Sum256(data)
		hexDigest := hex.EncodeToString(digest[:])
		artifacts = append(artifacts, Artifact{Target: target, Size: int64(len(data)), SHA256: hexDigest})
		writeTarFile(t, writer, target.Filename, data, constants.PermFileExecutable)
		writeTarFile(t, writer, target.Checksum, []byte(hexDigest+"  "+target.Filename+"\n"), constants.PermFilePublic)
	}
	manifest := Manifest{SchemaVersion: 1, Version: "test", BuildID: buildID, BuildTime: time.Now().UTC().Format(time.RFC3339), SourceRevision: "test", SourceTreeHash: strings.Repeat("a", 64), Targets: artifacts}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	writeTarFile(t, writer, constants.G8eBinariesManifestFilename, data, constants.PermFilePublic)
	require.NoError(t, writer.Close())
	return archive.Bytes()
}

func writeTarFile(t *testing.T, writer *tar.Writer, name string, data []byte, mode uint32) {
	t.Helper()
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Mode: int64(mode), Size: int64(len(data))}))
	_, err := io.Copy(writer, bytes.NewReader(data))
	require.NoError(t, err)
}
