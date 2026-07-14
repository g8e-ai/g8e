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

//go:build integration || e2e

package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// NewLiveOperatorHTTPClient creates an API client configured for mTLS
// against a running g8e platform using the platform's api.NewClient.
//
// This helper requires the platform to be running via `./g8e platform start`
// and authenticated via `./g8e auth login`.
//
// Parameters:
//   - t: testing.T for assertions
//   - repoRoot: path to the repository root (typically derived from test directory)
//
// Returns:
//   - *api.Client: configured API client with mTLS and CA verification
//   - *config.Config: loaded CLI configuration (for ports and paths)
func NewLiveOperatorHTTPClient(t require.TestingT, repoRoot string) (*api.Client, *config.Config) {
	// Load CLI config to get ports and paths
	cliCfg, err := config.Load(repoRoot)
	require.NoError(t, err, "failed to load CLI config")

	// Construct fileSvc for credential loading
	fileSvc, err := fs.NewRuntimeFileService(repoRoot, testutil.NewTestLogger())
	require.NoError(t, err, "failed to create file service")

	// Use platform's api.NewClient with 5-second timeout
	client, err := api.NewClient(fileSvc, cliCfg)
	require.NoError(t, err, "failed to create API client - run './g8e auth login' first")

	return client, cliCfg
}

// ResolveRepoRootFromTestDir finds the repository root using go list -m.
// This is more robust than directory navigation and works regardless of
// the current working directory.
func ResolveRepoRootFromTestDir(t require.TestingT) string {
	// Use go list -m to find the module directory
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	output, err := cmd.Output()
	require.NoError(t, err, "failed to run go list -m to find repository root")

	repoRoot := string(output)
	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))
	require.NotEmpty(t, repoRoot, "go list -m returned empty directory")

	return repoRoot
}

// RunCLICommand executes ./g8e commands with proper error handling and output capture.
// It ensures the g8e binary is built, sets the working directory to the repo root,
// and returns the combined stdout/stderr output along with any error.
//
// Parameters:
//   - t: testing.T for assertions
//   - repoRoot: path to the repository root
//   - args: command arguments (e.g., "gw", "status")
//
// Returns:
//   - string: combined stdout/stderr output
//   - error: nil if command succeeded, otherwise the execution error
func RunCLICommand(t *testing.T, repoRoot string, args ...string) (string, error) {
	t.Helper()

	g8ePath := filepath.Join(repoRoot, "g8e")
	if _, err := os.Stat(g8ePath); os.IsNotExist(err) {
		// Build the binary if it doesn't exist
		buildCmd := exec.Command("go", "build", "-o", g8ePath, "./cmd/g8e")
		buildCmd.Dir = repoRoot
		if output, err := buildCmd.CombinedOutput(); err != nil {
			require.NoError(t, err, "failed to build g8e binary: %s", string(output))
		}
	}

	cmd := exec.Command(g8ePath, args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// RunCLICommandRequire executes a CLI command and requires it to succeed.
// If the command fails, it calls t.Fatalf with the output.
//
// This is a convenience wrapper around RunCLICommand for cases where
// the command must succeed for the test to continue.
func RunCLICommandRequire(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()

	output, err := RunCLICommand(t, repoRoot, args...)
	if err != nil {
		t.Fatalf("CLI command '%v' failed: %v\nOutput: %s", args, err, output)
	}
	return output
}
