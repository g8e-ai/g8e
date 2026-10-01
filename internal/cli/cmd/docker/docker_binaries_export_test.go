// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	g8ebinaries "github.com/g8e-ai/g8e/v2/internal/services/g8ebinaries"
)

var (
	errRunnerInspect = errors.New("inspect exploded")
	errRunnerCreate  = errors.New("create exploded")
	errRunnerCopy    = errors.New("copy exploded")
	errRunnerRemove  = errors.New("remove exploded")
)

const (
	testExportBuildID   = "build-export-1"
	testExportVersion   = "v9.9.9"
	testExportRevision  = "rev-abc123"
	testExportBuildTime = "2026-09-23T00:00:00Z"
)

var testExportTreeHash = strings.Repeat("a", 64)

// exportArchive builds a complete, valid g8e-binary mirror archive for the
// full target matrix, laid out the way `docker cp <dir>/. -` emits it.
func exportArchive(t *testing.T, buildID string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	write := func(name string, data []byte, mode uint32) {
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Mode: int64(mode), Size: int64(len(data))}))
		_, err := writer.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: int64(constants.PermDirPrivate)}))
	artifacts := make([]g8ebinaries.Artifact, 0, len(g8ebinaries.Targets()))
	for _, target := range g8ebinaries.Targets() {
		data := []byte("binary-" + target.Filename)
		digest := sha256.Sum256(data)
		sum := hex.EncodeToString(digest[:])
		artifacts = append(artifacts, g8ebinaries.Artifact{Target: target, Size: int64(len(data)), SHA256: sum})
		write(target.Filename, data, constants.PermFileExecutable)
		write(target.Checksum, []byte(sum+"  "+target.Filename+"\n"), constants.PermFilePublic)
	}
	manifest, err := json.Marshal(g8ebinaries.Manifest{
		SchemaVersion: 1, Version: testExportVersion, BuildID: buildID, BuildTime: testExportBuildTime,
		SourceRevision: testExportRevision, SourceTreeHash: testExportTreeHash, Targets: artifacts,
	})
	require.NoError(t, err)
	write(constants.G8eBinariesManifestFilename, manifest, constants.PermFilePublic)
	require.NoError(t, writer.Close())
	return archive.Bytes()
}

func provenanceLabels(buildID string) map[string]string {
	return map[string]string{
		constants.G8eImageVersionLabel:   testExportVersion,
		constants.G8eBuildIDLabel:        buildID,
		constants.G8eBuildTimeLabel:      testExportBuildTime,
		constants.G8eSourceRevisionLabel: testExportRevision,
		constants.G8eSourceTreeHashLabel: testExportTreeHash,
	}
}

// scriptedBinaryRunner is a dockerBinaryRunner whose every step is scripted.
type scriptedBinaryRunner struct {
	image      dockerImage
	archive    []byte
	inspectErr error
	createErr  error
	copyErr    error
	removeErr  error

	created  []string
	copied   []string
	removed  []string
	copyPath string
}

func (r *scriptedBinaryRunner) InspectImage(context.Context, string) (dockerImage, error) {
	return r.image, r.inspectErr
}

func (r *scriptedBinaryRunner) CreateContainer(_ context.Context, image string) (string, error) {
	if r.createErr != nil {
		return "", r.createErr
	}
	r.created = append(r.created, image)
	return "cont-9", nil
}

func (r *scriptedBinaryRunner) CopyContainerPath(_ context.Context, container, path string, destination io.Writer) error {
	r.copied = append(r.copied, container)
	r.copyPath = path
	if r.copyErr != nil {
		return r.copyErr
	}
	_, err := destination.Write(r.archive)
	return err
}

func (r *scriptedBinaryRunner) RemoveContainer(_ context.Context, container string) error {
	r.removed = append(r.removed, container)
	return r.removeErr
}

func newExportRunner(t *testing.T, buildID string) *scriptedBinaryRunner {
	t.Helper()
	return &scriptedBinaryRunner{
		image:   dockerImage{ID: "sha256:img-export", Config: dockerImageConfig{Labels: provenanceLabels(testExportBuildID)}},
		archive: exportArchive(t, buildID),
	}
}

// ---------------------------------------------------------------------------
// exportDockerG8eBinaries
// ---------------------------------------------------------------------------

func TestExportDockerG8eBinaries_PublishesVerifiedMirrorAndRecordsItsProvenance(t *testing.T) {
	runner := newExportRunner(t, testExportBuildID)
	output := filepath.Join(t.TempDir(), "mirror")

	require.NoError(t, exportDockerG8eBinaries(t.Context(), runner, "g8e-gateway:release", output))

	assert.Equal(t, []string{"sha256:img-export"}, runner.created, "the immutable image ID is used, not the mutable tag")
	assert.Equal(t, constants.G8eBinariesArchiveRoot+"/.", runner.copyPath)
	assert.Equal(t, []string{"cont-9"}, runner.removed, "the export container is always removed")
	for _, target := range g8ebinaries.Targets() {
		data, err := os.ReadFile(filepath.Join(output, target.Filename))
		require.NoError(t, err, target.Filename)
		assert.Equal(t, "binary-"+target.Filename, string(data))
	}
	digest, err := g8ebinaries.ManifestDigest(output)
	require.NoError(t, err)
	recordBytes, err := os.ReadFile(filepath.Join(output, constants.G8eBinariesExportRecordFilename))
	require.NoError(t, err)
	var record g8ebinaries.ExportRecord
	require.NoError(t, json.Unmarshal(recordBytes, &record))
	assert.Equal(t, g8ebinaries.ExportRecord{
		SchemaVersion: 1, ImageReference: "g8e-gateway:release", ImageID: "sha256:img-export", ManifestSHA256: digest,
	}, record)
}

func TestExportDockerG8eBinaries_RefusesArchiveWhoseProvenanceDiffersFromTheImageLabels(t *testing.T) {
	runner := newExportRunner(t, "a-different-build")
	output := filepath.Join(t.TempDir(), "mirror")

	err := exportDockerG8eBinaries(t.Context(), runner, "img", output)

	require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
	require.ErrorIs(t, err, constants.ErrG8eBinaryManifest)
	assert.NoDirExists(t, output, "a mismatched archive must never be published")
	assert.Equal(t, []string{"cont-9"}, runner.removed, "the container is cleaned up even when publishing fails")
}

func TestExportDockerG8eBinaries_FailureModes(t *testing.T) {
	t.Run("blank image or output", func(t *testing.T) {
		runner := newExportRunner(t, testExportBuildID)
		require.ErrorIs(t, exportDockerG8eBinaries(t.Context(), runner, "  ", "out"), constants.ErrG8eBinaryExport)
		require.ErrorIs(t, exportDockerG8eBinaries(t.Context(), runner, "img", ""), constants.ErrG8eBinaryExport)
		assert.Empty(t, runner.created, "nothing is created for invalid arguments")
	})

	t.Run("image inspection fails", func(t *testing.T) {
		runner := newExportRunner(t, testExportBuildID)
		runner.inspectErr = errRunnerInspect

		err := exportDockerG8eBinaries(t.Context(), runner, "img", filepath.Join(t.TempDir(), "m"))

		require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
		require.ErrorIs(t, err, errRunnerInspect)
		assert.Empty(t, runner.created)
	})

	t.Run("image lacks provenance labels", func(t *testing.T) {
		runner := newExportRunner(t, testExportBuildID)
		runner.image.Config.Labels = map[string]string{constants.G8eImageVersionLabel: testExportVersion}

		err := exportDockerG8eBinaries(t.Context(), runner, "img", filepath.Join(t.TempDir(), "m"))

		require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
		assert.Contains(t, err.Error(), "missing g8e provenance labels")
		assert.Empty(t, runner.created, "an unattributable image is rejected before a container is created")
	})

	t.Run("container creation fails", func(t *testing.T) {
		runner := newExportRunner(t, testExportBuildID)
		runner.createErr = errRunnerCreate

		err := exportDockerG8eBinaries(t.Context(), runner, "img", filepath.Join(t.TempDir(), "m"))

		require.ErrorIs(t, err, errRunnerCreate)
		assert.Empty(t, runner.removed, "there is no container to remove")
	})

	t.Run("copy from the container fails", func(t *testing.T) {
		runner := newExportRunner(t, testExportBuildID)
		runner.copyErr = errRunnerCopy
		output := filepath.Join(t.TempDir(), "mirror")

		err := exportDockerG8eBinaries(t.Context(), runner, "img", output)

		require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
		require.ErrorIs(t, err, errRunnerCopy)
		assert.NoDirExists(t, output)
		assert.Equal(t, []string{"cont-9"}, runner.removed)
	})

	t.Run("archive is not a tar stream", func(t *testing.T) {
		runner := newExportRunner(t, testExportBuildID)
		runner.archive = []byte("this is not a tar archive")
		output := filepath.Join(t.TempDir(), "mirror")

		err := exportDockerG8eBinaries(t.Context(), runner, "img", output)

		require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
		assert.NoDirExists(t, output)
		assert.Equal(t, []string{"cont-9"}, runner.removed)
	})

	t.Run("a cleanup failure is surfaced even though the mirror was published", func(t *testing.T) {
		runner := newExportRunner(t, testExportBuildID)
		runner.removeErr = errRunnerRemove

		err := exportDockerG8eBinaries(t.Context(), runner, "img", filepath.Join(t.TempDir(), "mirror"))

		require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
		require.ErrorIs(t, err, errRunnerRemove)
		assert.Contains(t, err.Error(), "cleanup export container")
	})
}

// ---------------------------------------------------------------------------
// exportDockerRuntimeBinary — archive validation and cleanup
// ---------------------------------------------------------------------------

func runtimeArchive(t *testing.T, entries ...tar.Header) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	for _, header := range entries {
		h := header
		require.NoError(t, writer.WriteHeader(&h))
		if h.Typeflag == tar.TypeReg {
			_, err := writer.Write(bytes.Repeat([]byte("x"), int(h.Size)))
			require.NoError(t, err)
		}
	}
	require.NoError(t, writer.Close())
	return archive.Bytes()
}

func TestExportDockerRuntimeBinary_RejectsMalformedRuntimeArchives(t *testing.T) {
	reg := func(name string, size int64) tar.Header {
		return tar.Header{Name: name, Mode: 0o755, Size: size, Typeflag: tar.TypeReg}
	}
	tests := []struct {
		name    string
		archive []byte
	}{
		{"entry has the wrong name", runtimeArchive(t, reg("not-g8e", 3))},
		{"entry is not a regular file", runtimeArchive(t, tar.Header{Name: "g8e", Typeflag: tar.TypeDir, Mode: 0o755})},
		{"entry is empty", runtimeArchive(t, reg("g8e", 0))},
		{"archive carries extra entries", runtimeArchive(t, reg("g8e", 3), reg("extra", 1))},
		{"archive is empty", runtimeArchive(t)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &scriptedBinaryRunner{image: dockerImage{ID: "sha256:i"}, archive: tt.archive}
			destination := filepath.Join(t.TempDir(), "g8e")

			err := exportDockerRuntimeBinary(t.Context(), runner, "img", destination)

			require.ErrorIs(t, err, constants.ErrG8eBinaryArchive)
			assert.NoFileExists(t, destination, "an invalid archive must not produce a binary")
			assert.NoFileExists(t, destination+".new", "no staging file may be left behind")
			assert.Equal(t, []string{"cont-9"}, runner.removed)
		})
	}
}

func TestExportDockerRuntimeBinary_FailureModesAndCleanup(t *testing.T) {
	good := runtimeArchive(t, tar.Header{Name: "g8e", Mode: 0o755, Size: 4, Typeflag: tar.TypeReg})

	t.Run("blank image or destination", func(t *testing.T) {
		runner := &scriptedBinaryRunner{image: dockerImage{ID: "sha256:i"}, archive: good}
		require.ErrorIs(t, exportDockerRuntimeBinary(t.Context(), runner, "", "dest"), constants.ErrG8eBinaryExport)
		require.ErrorIs(t, exportDockerRuntimeBinary(t.Context(), runner, "img", " "), constants.ErrG8eBinaryExport)
		assert.Empty(t, runner.created)
	})

	t.Run("inspection failure", func(t *testing.T) {
		runner := &scriptedBinaryRunner{inspectErr: errRunnerInspect}
		err := exportDockerRuntimeBinary(t.Context(), runner, "img", filepath.Join(t.TempDir(), "g8e"))
		require.ErrorIs(t, err, errRunnerInspect)
		require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
	})

	t.Run("container creation failure", func(t *testing.T) {
		runner := &scriptedBinaryRunner{image: dockerImage{ID: "sha256:i"}, createErr: errRunnerCreate}
		err := exportDockerRuntimeBinary(t.Context(), runner, "img", filepath.Join(t.TempDir(), "g8e"))
		require.ErrorIs(t, err, errRunnerCreate)
	})

	t.Run("copy failure still removes the container", func(t *testing.T) {
		runner := &scriptedBinaryRunner{image: dockerImage{ID: "sha256:i"}, copyErr: errRunnerCopy}
		err := exportDockerRuntimeBinary(t.Context(), runner, "img", filepath.Join(t.TempDir(), "g8e"))
		require.ErrorIs(t, err, errRunnerCopy)
		assert.Equal(t, []string{"cont-9"}, runner.removed)
	})

	t.Run("cleanup failure after a successful export is reported", func(t *testing.T) {
		runner := &scriptedBinaryRunner{image: dockerImage{ID: "sha256:i"}, archive: good, removeErr: errRunnerRemove}
		destination := filepath.Join(t.TempDir(), "g8e")

		err := exportDockerRuntimeBinary(t.Context(), runner, "img", destination)

		require.ErrorIs(t, err, errRunnerRemove)
		assert.Contains(t, err.Error(), "cleanup export container")
		assert.FileExists(t, destination, "the binary was exported even though cleanup failed")
	})

	t.Run("destination directory does not exist", func(t *testing.T) {
		runner := &scriptedBinaryRunner{image: dockerImage{ID: "sha256:i"}, archive: good}
		destination := filepath.Join(t.TempDir(), "missing-dir", "g8e")

		err := exportDockerRuntimeBinary(t.Context(), runner, "img", destination)

		require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
		assert.Contains(t, err.Error(), "write runtime binary")
	})

	t.Run("an existing binary is replaced atomically", func(t *testing.T) {
		runner := &scriptedBinaryRunner{image: dockerImage{ID: "sha256:i"}, archive: good}
		destination := filepath.Join(t.TempDir(), "g8e")
		require.NoError(t, os.WriteFile(destination, []byte("old"), 0o755))

		require.NoError(t, exportDockerRuntimeBinary(t.Context(), runner, "img", destination))

		data, err := os.ReadFile(destination)
		require.NoError(t, err)
		assert.Equal(t, "xxxx", string(data))
		assert.NoFileExists(t, destination+".new")
	})
}

// ---------------------------------------------------------------------------
// The real exec runner against a fake docker executable
// ---------------------------------------------------------------------------

func TestExecDockerBinaryRunner_InspectImage(t *testing.T) {
	t.Run("decodes the inspected image and requests the JSON template", func(t *testing.T) {
		callLog := installFakeDocker(t, `case "$1 $2" in "image inspect") printf '{"Id":"sha256:abc","Os":"linux","Architecture":"arm64","Config":{"Labels":{"k":"v"}}}\n';; esac; exit 0`)

		image, err := execDockerBinaryRunner{}.InspectImage(t.Context(), "g8e-gateway")

		require.NoError(t, err)
		assert.Equal(t, dockerImage{ID: "sha256:abc", Os: "linux", Architecture: "arm64", Config: dockerImageConfig{Labels: map[string]string{"k": "v"}}}, image)
		assert.Equal(t, []string{"image inspect --format={{json .}} g8e-gateway"}, fakeDockerCalls(t, callLog))
	})

	failures := map[string]string{
		"docker fails":   `exit 1`,
		"malformed JSON": `echo '{nope'`,
	}
	for name, body := range failures {
		t.Run(name, func(t *testing.T) {
			installFakeDocker(t, body)

			image, err := execDockerBinaryRunner{}.InspectImage(t.Context(), "img")

			require.Error(t, err)
			assert.Equal(t, dockerImage{}, image)
		})
	}

	t.Run("an image without an immutable ID is an export error", func(t *testing.T) {
		installFakeDocker(t, `echo '{"Id":""}'`)

		_, err := execDockerBinaryRunner{}.InspectImage(t.Context(), "img")

		require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
	})
}

func TestExecDockerBinaryRunner_CreateCopyAndRemoveContainer(t *testing.T) {
	t.Run("CreateContainer returns the trimmed container id", func(t *testing.T) {
		callLog := installFakeDocker(t, `[ "$1" = create ] && echo "  cont-42  "; exit 0`)

		id, err := execDockerBinaryRunner{}.CreateContainer(t.Context(), "sha256:abc")

		require.NoError(t, err)
		assert.Equal(t, "cont-42", id)
		assert.Equal(t, []string{"create sha256:abc"}, fakeDockerCalls(t, callLog))
	})

	t.Run("CreateContainer rejects an empty container id", func(t *testing.T) {
		installFakeDocker(t, "exit 0")

		id, err := execDockerBinaryRunner{}.CreateContainer(t.Context(), "img")

		require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
		assert.Empty(t, id)
	})

	t.Run("CreateContainer surfaces docker failure", func(t *testing.T) {
		installFakeDocker(t, "exit 1")

		_, err := execDockerBinaryRunner{}.CreateContainer(t.Context(), "img")

		require.Error(t, err)
		assert.Contains(t, err.Error(), `create export container from "img"`)
	})

	t.Run("CopyContainerPath streams the tar to the destination", func(t *testing.T) {
		callLog := installFakeDocker(t, `[ "$1" = cp ] && printf 'TAR-BYTES'; exit 0`)
		var out bytes.Buffer

		require.NoError(t, execDockerBinaryRunner{}.CopyContainerPath(t.Context(), "cont-1", "/g8e", &out))

		assert.Equal(t, "TAR-BYTES", out.String())
		assert.Equal(t, []string{"cp cont-1:/g8e -"}, fakeDockerCalls(t, callLog))
	})

	t.Run("CopyContainerPath surfaces docker failure", func(t *testing.T) {
		installFakeDocker(t, `[ "$1" = cp ] && exit 1; exit 0`)

		err := execDockerBinaryRunner{}.CopyContainerPath(t.Context(), "cont-1", "/g8e", io.Discard)

		require.Error(t, err)
		assert.Contains(t, err.Error(), `copy g8e binaries from container "cont-1"`)
	})

	t.Run("RemoveContainer succeeds and fails as docker does", func(t *testing.T) {
		callLog := installFakeDocker(t, "exit 0")
		require.NoError(t, execDockerBinaryRunner{}.RemoveContainer(t.Context(), "cont-1"))
		assert.Equal(t, []string{"rm cont-1"}, fakeDockerCalls(t, callLog))

		installFakeDocker(t, "exit 1")
		err := execDockerBinaryRunner{}.RemoveContainer(t.Context(), "cont-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `remove export container "cont-1"`)
	})
}

// ---------------------------------------------------------------------------
// `g8e docker binaries export`
// ---------------------------------------------------------------------------

func fakeDockerForMirrorExport(t *testing.T, image dockerImage, archive []byte) string {
	t.Helper()
	dir := t.TempDir()
	inspect := filepath.Join(dir, "inspect.json")
	tarPath := filepath.Join(dir, "mirror.tar")
	data, err := json.Marshal(image)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(inspect, data, 0o644))
	require.NoError(t, os.WriteFile(tarPath, archive, 0o644))
	t.Setenv("FAKE_DOCKER_INSPECT", inspect)
	t.Setenv("FAKE_DOCKER_ARCHIVE", tarPath)
	return installFakeDocker(t, `
case "$1 $2" in
  "image inspect") cat "$FAKE_DOCKER_INSPECT"; exit 0;;
esac
case "$1" in
  create) echo cont-1; exit 0;;
  cp) cat "$FAKE_DOCKER_ARCHIVE"; exit 0;;
esac
exit 0`)
}

func TestDockerBinariesExportCmd_ExportsTheMirrorFromTheNamedImage(t *testing.T) {
	writeRootCompose(t)
	image := dockerImage{ID: "sha256:img-cmd", Config: dockerImageConfig{Labels: provenanceLabels(testExportBuildID)}}
	callLog := fakeDockerForMirrorExport(t, image, exportArchive(t, testExportBuildID))
	output := filepath.Join(t.TempDir(), "mirror")
	cmd := dockerBinariesExportCmd()
	require.NoError(t, cmd.Flags().Set("image", "registry.example/g8e-gateway:1.2.3"))
	require.NoError(t, cmd.Flags().Set("output", output))

	out, err := runDockerCommand(t, cmd)

	require.NoError(t, err)
	assert.Contains(t, out, "Exported g8e binaries from registry.example/g8e-gateway:1.2.3 to "+output+".")
	assert.FileExists(t, filepath.Join(output, constants.G8eBinariesManifestFilename))
	assert.FileExists(t, filepath.Join(output, constants.G8eBinariesExportRecordFilename))
	calls := fakeDockerCalls(t, callLog)
	assert.Contains(t, calls, "image inspect --format={{json .}} registry.example/g8e-gateway:1.2.3")
	assert.Contains(t, calls, "create sha256:img-cmd")
	assert.Contains(t, calls, "rm cont-1")
}

func TestDockerBinariesExportCmd_DefaultsToTheGatewayImageAndTheRepoBinDirectory(t *testing.T) {
	tmp := writeRootCompose(t)
	image := dockerImage{ID: "sha256:img-default", Config: dockerImageConfig{Labels: provenanceLabels(testExportBuildID)}}
	callLog := fakeDockerForMirrorExport(t, image, exportArchive(t, testExportBuildID))

	_, err := runDockerCommand(t, dockerBinariesExportCmd())

	require.NoError(t, err)
	assert.Contains(t, fakeDockerCalls(t, callLog), "image inspect --format={{json .}} "+defaultDockerGatewayImage)
	assert.FileExists(t, filepath.Join(tmp, constants.BinDirname, constants.G8eBinariesManifestFilename))
}

func TestDockerBinariesExportCmd_FailsClosed(t *testing.T) {
	t.Run("docker unavailable", func(t *testing.T) {
		writeRootCompose(t)
		withoutDocker(t)

		_, err := runDockerCommand(t, dockerBinariesExportCmd())

		require.ErrorIs(t, err, constants.ErrServiceUnavailable)
	})

	t.Run("image with mismatched provenance publishes nothing", func(t *testing.T) {
		writeRootCompose(t)
		image := dockerImage{ID: "sha256:img", Config: dockerImageConfig{Labels: provenanceLabels(testExportBuildID)}}
		fakeDockerForMirrorExport(t, image, exportArchive(t, "tampered-build"))
		output := filepath.Join(t.TempDir(), "mirror")
		cmd := dockerBinariesExportCmd()
		require.NoError(t, cmd.Flags().Set("output", output))

		out, err := runDockerCommand(t, cmd)

		require.ErrorIs(t, err, constants.ErrG8eBinaryManifest)
		assert.NoDirExists(t, output)
		assert.NotContains(t, out, "Exported g8e binaries")
	})
}
