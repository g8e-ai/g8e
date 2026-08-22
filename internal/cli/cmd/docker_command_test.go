// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/serve"
	"github.com/g8e-ai/g8e/internal/constants"
)

func TestDockerCommandSubcommands(t *testing.T) {
	t.Run("docker command has expected subcommands", func(t *testing.T) {
		cmd := dockerCmd()
		require.NotNil(t, cmd)
		assert.Equal(t, "docker", cmd.Use)

		expectedSubcommands := []string{
			"start",
			"stop",
			"status",
			"build",
			"clean",
			"reset",
			"rebuild",
			"logs",
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

func TestDockerComposePath_ResolvesFromCwd(t *testing.T) {
	tmpDir := chdirTemp(t)
	p, err := dockerComposePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, constants.DockerComposeFile), p)
}
