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

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// createFakeAgentBinary creates a fake executable named agentName in a temp dir
// and prepends that dir to PATH so exec.LookPath finds it.
func createFakeAgentBinary(t *testing.T, agentName string) string {
	t.Helper()
	binDir := testutil.TempDir(t)
	binPath := filepath.Join(binDir, agentName)
	require.NoError(t, os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0755))

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)
	return binDir
}

// ─── prepareAgentLaunch: verify=true for each supported agent ────────────────

func TestPrepareAgentLaunch_Claude_VerifyTrue(t *testing.T) {
	tmpHome := testutil.TempDir(t)
	t.Setenv("HOME", tmpHome)
	createFakeAgentBinary(t, "claude")

	configPath, cleanup, launchArgs, err := prepareAgentLaunch("claude", true)
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	assert.NotEmpty(t, configPath)
	assert.FileExists(t, configPath)
	assert.Contains(t, launchArgs, "--strict-mcp-config")
	assert.Contains(t, launchArgs, "--disallowed-tools")
	assert.True(t, strings.Contains(strings.Join(launchArgs, " "), "--mcp-config"))
}

func TestPrepareAgentLaunch_Codex_VerifyTrue(t *testing.T) {
	tmpHome := testutil.TempDir(t)
	t.Setenv("HOME", tmpHome)
	createFakeAgentBinary(t, "codex")

	configPath, cleanup, launchArgs, err := prepareAgentLaunch("codex", true)
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	assert.NotEmpty(t, configPath)
	assert.FileExists(t, configPath)
	assert.Contains(t, launchArgs, "--strict-mcp-config")
	assert.Contains(t, launchArgs, "--disallowed-tools")
}

func TestPrepareAgentLaunch_Goose_VerifyTrue(t *testing.T) {
	tmpHome := testutil.TempDir(t)
	t.Setenv("HOME", tmpHome)
	createFakeAgentBinary(t, "goose")

	configPath, cleanup, launchArgs, err := prepareAgentLaunch("goose", true)
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	assert.NotEmpty(t, configPath)
	assert.FileExists(t, configPath)
	assert.Contains(t, launchArgs, "--no-profile")
	assert.Contains(t, launchArgs, "session")
	assert.Contains(t, launchArgs, "--with-extension")

	// Verify goose config.yaml was written to the fake HOME
	gooseConfig := filepath.Join(tmpHome, ".config", "goose", "config.yaml")
	assert.FileExists(t, gooseConfig)
}

func TestPrepareAgentLaunch_Gemini_VerifyTrue(t *testing.T) {
	tmpHome := testutil.TempDir(t)
	t.Setenv("HOME", tmpHome)
	createFakeAgentBinary(t, "gemini")

	configPath, cleanup, launchArgs, err := prepareAgentLaunch("gemini", true)
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	assert.NotEmpty(t, configPath)
	assert.FileExists(t, configPath)
	assert.Empty(t, launchArgs)

	// Verify gemini settings.json was written to the fake HOME
	geminiSettings := filepath.Join(tmpHome, ".gemini", "settings.json")
	assert.FileExists(t, geminiSettings)

	// Verify tools.core is set to empty array
	data, err := os.ReadFile(geminiSettings)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"core"`)
}

// ─── prepareAgentLaunch: verify=false skips verification ─────────────────────

func TestPrepareAgentLaunch_Devin_VerifyTrue(t *testing.T) {
	tmpHome := testutil.TempDir(t)
	t.Setenv("HOME", tmpHome)
	createFakeAgentBinary(t, "devin")

	configPath, cleanup, launchArgs, err := prepareAgentLaunch("devin", true)
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	assert.NotEmpty(t, configPath)
	assert.FileExists(t, configPath)
	assert.Empty(t, launchArgs)

	// Verify devin config.json was written to the fake HOME
	devinConfig := filepath.Join(tmpHome, ".config", "devin", "config.json")
	assert.FileExists(t, devinConfig)

	// Verify g8e MCP server is in the config
	data, err := os.ReadFile(devinConfig)
	require.NoError(t, err)
	assert.Contains(t, string(data), "g8e")
}

// ─── prepareAgentLaunch: verify=false skips verification ─────────────────────

func TestPrepareAgentLaunch_VerifyFalse_SkipsVerification(t *testing.T) {
	tmpHome := testutil.TempDir(t)
	t.Setenv("HOME", tmpHome)
	createFakeAgentBinary(t, "claude")

	configPath, cleanup, launchArgs, err := prepareAgentLaunch("claude", false)
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	assert.NotEmpty(t, configPath)
	assert.FileExists(t, configPath)
	assert.Contains(t, launchArgs, "--strict-mcp-config")
}

// ─── prepareAgentLaunch: unsupported agent returns error ─────────────────────

func TestPrepareAgentLaunch_UnsupportedAgent(t *testing.T) {
	tmpHome := testutil.TempDir(t)
	t.Setenv("HOME", tmpHome)
	createFakeAgentBinary(t, "cursor")

	_, _, _, err := prepareAgentLaunch("cursor", true)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
}

// ─── prepareAgentLaunch: agent binary not in PATH ────────────────────────────

func TestPrepareAgentLaunch_AgentNotInPath(t *testing.T) {
	tmpHome := testutil.TempDir(t)
	t.Setenv("HOME", tmpHome)

	// Ensure "nonexistent-agent-xyz" is not in PATH
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", "")

	_, _, _, err := prepareAgentLaunch("nonexistent-agent-xyz", true)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAgentNotInPath)

	_ = originalPath // silence linter
}
