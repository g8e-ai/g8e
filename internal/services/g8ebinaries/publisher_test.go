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
	"runtime"
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

func TestCatalogAndManifestValidationRejectsUnsupportedInputs(t *testing.T) {
	assert.Len(t, Targets(), 7)
	assert.NoError(t, ValidateArtifactName(Targets()[0].Filename))
	assert.NoError(t, ValidateArtifactName(Targets()[0].Checksum))

	for _, name := range []string{"", "../g8e-linux-amd64", "/tmp/g8e-linux-amd64", "g8e\\\\linux-amd64", "unsupported"} {
		t.Run("rejects "+name, func(t *testing.T) {
			assert.ErrorIs(t, ValidateArtifactName(name), constants.ErrG8eBinaryArtifact)
		})
	}

	target, err := PlatformTarget("linux", "amd64")
	require.NoError(t, err)
	assert.Equal(t, "g8e-linux-amd64", target.Filename)
	hostTarget, err := HostTarget()
	if runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" || runtime.GOARCH == "386") {
		require.NoError(t, err)
		assert.Equal(t, runtime.GOOS, hostTarget.OS)
	} else {
		assert.ErrorIs(t, err, constants.ErrG8eBinaryArtifact)
	}
	_, err = PlatformTarget("plan9", "amd64")
	assert.ErrorIs(t, err, constants.ErrG8eBinaryArtifact)

	manifest := Manifest{
		SchemaVersion:  1,
		Version:        "2.1.12",
		BuildID:        "build-1",
		BuildTime:      time.Now().UTC().Format(time.RFC3339),
		SourceRevision: "revision",
		SourceTreeHash: strings.Repeat("a", 64),
	}
	for _, target := range Targets() {
		manifest.Targets = append(manifest.Targets, Artifact{Target: target, Size: 1, SHA256: strings.Repeat("a", 64)})
	}
	assert.NoError(t, manifest.Validate())
	manifest.BuildTime = "not-a-timestamp"
	assert.ErrorIs(t, manifest.Validate(), constants.ErrG8eBinaryManifest)
}

func TestReaderAndManifestFiles_ValidateCataloguedArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mirror")
	_, err := OpenReader(root)
	require.NoError(t, err)
	reader, err := OpenReader(root)
	require.NoError(t, err)
	assert.False(t, reader.HasManifest())
	_, _, err = reader.Artifact("unsupported")
	assert.ErrorIs(t, err, constants.ErrG8eBinaryArtifact)

	manifest, err := NewPublisher(root).Publish(bytes.NewReader(validArchive(t, "build-1", false)))
	require.NoError(t, err)
	reader, err = OpenReader(root)
	require.NoError(t, err)
	assert.True(t, reader.HasManifest())
	artifact, info, err := reader.Artifact(manifest.Targets[0].Filename)
	require.NoError(t, err)
	assert.Equal(t, manifest.Targets[0].Size, info.Size())
	assert.NotNil(t, artifact)
	closer, ok := artifact.(io.Closer)
	require.True(t, ok)
	require.NoError(t, closer.Close())
	_, _, err = reader.Artifact("g8e-binaries.json")
	assert.ErrorIs(t, err, constants.ErrG8eBinaryArtifact)

	digest, err := ManifestDigest(root)
	require.NoError(t, err)
	assert.Len(t, digest, sha256.Size*2)
	record := ExportRecord{ImageReference: "g8e:test", ImageID: "sha256:test", ManifestSHA256: digest}
	require.NoError(t, WriteExportRecord(root, record))
	data, err := os.ReadFile(filepath.Join(root, constants.G8eBinariesExportRecordFilename))
	require.NoError(t, err)
	assert.Contains(t, string(data), "g8e:test")
}

func TestPublisherPublishMatching_EnforcesImageProvenance(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mirror")
	archive := validArchive(t, "build-1", false)
	_, err := NewPublisher(root).PublishMatching(bytes.NewReader(archive), Provenance{
		Version:        "test",
		BuildID:        "build-1",
		BuildTime:      "wrong",
		SourceRevision: "test",
		SourceTreeHash: strings.Repeat("a", 64),
	})
	assert.ErrorIs(t, err, constants.ErrG8eBinaryManifest)
}

func TestLoadManifest_RejectsMissingAndMalformedFiles(t *testing.T) {
	root := t.TempDir()
	_, err := LoadManifest(root)
	assert.ErrorIs(t, err, constants.ErrG8eBinaryManifest)
	require.NoError(t, os.WriteFile(filepath.Join(root, constants.G8eBinariesManifestFilename), []byte("{"), constants.PermFilePublic))
	_, err = LoadManifest(root)
	assert.ErrorIs(t, err, constants.ErrG8eBinaryManifest)
}

func TestPublisherRejectsInvalidOutputAndMatchesProvenance(t *testing.T) {
	_, err := NewPublisher("").Publish(bytes.NewReader(nil))
	assert.ErrorIs(t, err, constants.ErrG8eBinaryExport)

	archive := validArchive(t, "build-1", false)
	_, err = NewPublisher(filepath.Join(t.TempDir(), "mirror")).PublishMatching(bytes.NewReader(archive), Provenance{
		Version:        "test",
		BuildID:        "build-1",
		BuildTime:      extractBuildTime(t, archive),
		SourceRevision: "test",
		SourceTreeHash: strings.Repeat("a", 64),
	})
	assert.NoError(t, err)

	_, err = NewPublisher(filepath.Join(t.TempDir(), "mirror")).Publish(bytes.NewReader([]byte("not a tar archive")))
	assert.ErrorIs(t, err, constants.ErrG8eBinaryArchive)
}

func extractBuildTime(t *testing.T, archive []byte) string {
	t.Helper()
	reader := tar.NewReader(bytes.NewReader(archive))
	for {
		header, err := reader.Next()
		require.NoError(t, err)
		if header.Name == constants.G8eBinariesManifestFilename {
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			var manifest Manifest
			require.NoError(t, json.Unmarshal(data, &manifest))
			return manifest.BuildTime
		}
	}
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
