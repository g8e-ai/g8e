// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	g8ebinaries "github.com/g8e-ai/g8e/v2/internal/services/g8ebinaries"
)

func TestDockerCommandSubcommands(t *testing.T) {
	t.Run("docker command has expected subcommands", func(t *testing.T) {
		cmd := dockerCmd()
		require.NotNil(t, cmd)
		assert.Equal(t, "docker", cmd.Use)

		expectedSubcommands := []string{
			"init",
			"start",
			"stop",
			"status",
			"build",
			"clean",
			"reset",
			"rebuild",
			"logs",
			"binaries",
		}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.Truef(t, found, "docker command should have %s subcommand", subcmd)
		}
	})
}

func TestDockerBinariesCommandHasExportSubcommand(t *testing.T) {
	cmd := dockerCmd()
	binaries, _, err := cmd.Find([]string{"binaries"})
	require.NoError(t, err)
	require.Equal(t, "binaries", binaries.Name())

	export, _, err := binaries.Find([]string{"export"})
	require.NoError(t, err)
	assert.Equal(t, "export", export.Name())
}

func TestDockerCommand_RegisteredOnRoot(t *testing.T) {
	root := NewRootCmd("dev", serve.VersionInfo{})
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "docker" {
			found = true
			break
		}
	}
	assert.True(t, found, "root command should register the docker subcommand")
}

// isNotFoundErr reports whether err wraps constants.ErrNotFound.
func isNotFoundErr(err error) bool {
	return errors.Is(err, constants.ErrNotFound)
}

// writeRootCompose creates a temp cwd containing a root docker-compose.yml so
// commands that check for the compose file pass the existence guard.
func writeRootCompose(t *testing.T) string {
	t.Helper()
	tmpDir := chdirTemp(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, constants.DockerComposeFile),
		[]byte("version: '3'\n"), 0o644))
	return tmpDir
}

func TestDockerStart_MissingComposeFileReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := dockerStartCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.True(t, isNotFoundErr(err), "expected ErrNotFound when compose file is missing")
}

func TestDockerStop_MissingComposeFileReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := dockerStopCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.True(t, isNotFoundErr(err))
}

func TestDockerStatus_MissingComposeFileReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := dockerStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.True(t, isNotFoundErr(err))
}

func TestDockerBuild_MissingComposeFileReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := dockerBuildCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.True(t, isNotFoundErr(err))
}

func TestDockerClean_MissingComposeFileReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := dockerCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.True(t, isNotFoundErr(err))
}

func TestDockerReset_MissingComposeFileReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := dockerResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.True(t, isNotFoundErr(err))
}

func TestDockerRebuild_MissingComposeFileReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := dockerRebuildCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.True(t, isNotFoundErr(err))
}

func TestDockerLogs_MissingComposeFileReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := dockerLogsCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.True(t, isNotFoundErr(err))
}

func TestDockerStart_WithComposeFileButNoDocker(t *testing.T) {
	if dockerAvailable() {
		t.Skip("test exercises the no-Docker error path")
	}
	writeRootCompose(t)

	cmd := dockerStartCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestDockerStop_WithComposeFileButNoDocker(t *testing.T) {
	if dockerAvailable() {
		t.Skip("test exercises the no-Docker error path")
	}
	writeRootCompose(t)

	cmd := dockerStopCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestDockerStatus_WithComposeFileButNoDocker(t *testing.T) {
	if dockerAvailable() {
		t.Skip("test exercises the no-Docker error path")
	}
	writeRootCompose(t)

	cmd := dockerStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestDockerBuild_WithComposeFileButNoDocker(t *testing.T) {
	if dockerAvailable() {
		t.Skip("test exercises the no-Docker error path")
	}
	writeRootCompose(t)

	cmd := dockerBuildCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestDockerRebuild_WithComposeFileButNoDocker(t *testing.T) {
	if dockerAvailable() {
		t.Skip("test exercises the no-Docker error path")
	}
	writeRootCompose(t)

	cmd := dockerRebuildCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestDockerLogs_WithComposeFileButNoDocker(t *testing.T) {
	if dockerAvailable() {
		t.Skip("test exercises the no-Docker error path")
	}
	writeRootCompose(t)

	cmd := dockerLogsCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestDockerClean_WithComposeFileButNoDockerSucceeds(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("docker not available")
	}
	writeRootCompose(t)

	cmd := dockerCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	assert.NoError(t, err, "clean should succeed when Docker is available (no-op on stopped containers)")
}

func TestDockerReset_WithComposeFileButNoDocker(t *testing.T) {
	if dockerAvailable() {
		t.Skip("test exercises the no-Docker error path")
	}
	writeRootCompose(t)

	cmd := dockerResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestResolveDockerProfile(t *testing.T) {
	assert.Equal(t, "", resolveDockerProfile(false, ""))
	assert.Equal(t, constants.DockerBootstrappedProfile, resolveDockerProfile(true, ""))
	assert.Equal(t, constants.DockerBootstrappedProfile, resolveDockerProfile(false, constants.DockerBootstrappedProfile))
	assert.Equal(t, "custom", resolveDockerProfile(true, "custom"), "explicit profile overrides --full")
}

func TestDockerTeardownProfiles(t *testing.T) {
	assert.Equal(t, []string{"custom"}, dockerTeardownProfiles("custom"))
	assert.Equal(t, []string{
		constants.DockerBootstrappedProfile,
		constants.DockerEvaluationProfile,
	}, dockerTeardownProfiles(""))
}

func TestImageProvenance_RequiresAllLabels(t *testing.T) {
	_, err := imageProvenance(dockerImage{ID: "sha256:image", Config: dockerImageConfig{Labels: map[string]string{
		constants.G8eImageVersionLabel: "2.1.12",
	}}})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrG8eBinaryExport)
}

func TestPublisherMatchProvenance_RejectsMismatchedImageLabels(t *testing.T) {
	manifest := g8ebinaries.Manifest{Version: "2.1.12", BuildID: "build", BuildTime: "2026-09-23T00:00:00Z", SourceRevision: "revision", SourceTreeHash: strings.Repeat("a", 64)}
	provenance := g8ebinaries.Provenance{Version: manifest.Version, BuildID: "different", BuildTime: manifest.BuildTime, SourceRevision: manifest.SourceRevision, SourceTreeHash: manifest.SourceTreeHash}

	err := g8ebinaries.MatchProvenance(manifest, provenance)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrG8eBinaryManifest)
}

func TestDockerBuildArgs_IncludesSourceProvenance(t *testing.T) {
	vi := serve.VersionInfo{
		Version:             "v2.1.12",
		BuildTime:           "2026-09-23T00:00:00Z",
		BuildID:             "abc123",
		SourceRevision:      "revision-123",
		SourceTreeStateHash: "a" + strings.Repeat("1", 63),
	}
	tests := []struct {
		name     string
		noCache  bool
		expected []string
	}{
		{name: "cached build", expected: []string{"build", "--build-arg", "VERSION=v2.1.12", "--build-arg", "BUILD_TIME=2026-09-23T00:00:00Z", "--build-arg", "BUILD_ID=abc123", "--build-arg", "SOURCE_REVISION=revision-123", "--build-arg", "SOURCE_TREE_HASH=" + vi.SourceTreeStateHash}},
		{name: "uncached build", noCache: true, expected: []string{"build", "--build-arg", "VERSION=v2.1.12", "--build-arg", "BUILD_TIME=2026-09-23T00:00:00Z", "--build-arg", "BUILD_ID=abc123", "--build-arg", "SOURCE_REVISION=revision-123", "--build-arg", "SOURCE_TREE_HASH=" + vi.SourceTreeStateHash, "--no-cache"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := dockerBuildArgs(vi, tt.noCache)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, args)
		})
	}
}

func TestDockerBuildArgs_UsesSourceTreeHashForUnstampedBuildID(t *testing.T) {
	hash := "a" + strings.Repeat("1", 63)
	args, err := dockerBuildArgs(serve.VersionInfo{
		BuildID:             constants.BuildMetadataUnavailable,
		SourceRevision:      "revision-123",
		SourceTreeStateHash: hash,
	}, false)

	require.NoError(t, err)
	assert.Contains(t, args, "BUILD_ID="+hash)
}

func TestDockerBuildArgs_RejectsUnstampedSourceHash(t *testing.T) {
	_, err := dockerBuildArgs(serve.VersionInfo{SourceTreeStateHash: string(constants.SystemHealthUnknown)}, false)
	assert.ErrorIs(t, err, constants.ErrSourceTreeHashInvalid)
}

func TestDockerComposePath_ResolvesFromCwd(t *testing.T) {
	tmpDir := chdirTemp(t)
	p, err := dockerComposePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, constants.DockerComposeFile), p)
}

type fakeDockerBinaryRunner struct {
	image     dockerImage
	container string
	binary    []byte
}

func (r *fakeDockerBinaryRunner) InspectImage(context.Context, string) (dockerImage, error) {
	return r.image, nil
}

func (r *fakeDockerBinaryRunner) CreateContainer(context.Context, string) (string, error) {
	return r.container, nil
}

func (r *fakeDockerBinaryRunner) CopyContainerPath(_ context.Context, container, path string, destination io.Writer) error {
	if container != r.container || path != dockerRuntimeBinaryPath {
		return fmt.Errorf("unexpected copy request: %s:%s", container, path)
	}
	_, err := destination.Write(r.binary)
	return err
}

func (r *fakeDockerBinaryRunner) RemoveContainer(context.Context, string) error { return nil }

func TestExportDockerRuntimeBinary_WritesExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	destination := filepath.Join(tmpDir, "g8e")
	runner := &fakeDockerBinaryRunner{
		image:     dockerImage{ID: "sha256:image"},
		container: "container-id",
		binary:    []byte("runtime-binary"),
	}

	err := exportDockerRuntimeBinary(t.Context(), runner, "g8e-gateway", destination)
	require.NoError(t, err)

	data, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, []byte("runtime-binary"), data)
}
