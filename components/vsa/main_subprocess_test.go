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

package main

// Subprocess-based tests for CLI entry-point functions that call os.Exit.
//
// Each test follows the pattern used in services/pubsub/pubsub_commands_coverage_test.go:
//   - The parent process spawns a subprocess via exec.Command(os.Args[0], "-test.run=TestXxx")
//     with a sentinel env var set.
//   - The subprocess detects the env var, runs the target function, and the expected
//     os.Exit call terminates the subprocess.
//   - The parent asserts on the subprocess exit code.
//
// This exercises the actual function bodies (handleRekeyVault, handleVerifyVault,
// handleResetVault, handleVaultCommand, runListenMode, runOpenClawMode) so that
// coverage tooling registers them as entered, while keeping the parent test process safe.

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	vaultpkg "github.com/g8e-ai/g8e/components/vsa/services/vault"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// handleRekeyVault — missing old key → ExitConfigError
// ---------------------------------------------------------------------------

func TestHandleRekeyVault_MissingOldKey_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_REKEY_NO_OLD_KEY") == "1" {
		logger := testutil.NewTestLogger()
		dir := t.TempDir()
		v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dir, Logger: logger})
		if err != nil {
			os.Exit(constants.ExitConfigError)
		}
		defer v.Close()
		handleRekeyVault(v, "", "new-key", logger)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestHandleRekeyVault_MissingOldKey_Subprocess")
	cmd.Env = append(os.Environ(), "G8E_TEST_REKEY_NO_OLD_KEY=1")
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "subprocess must exit with a non-zero code, got: %v", err)
	assert.Equal(t, constants.ExitConfigError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// handleRekeyVault — vault not initialized → ExitConfigError
// ---------------------------------------------------------------------------

func TestHandleRekeyVault_VaultNotInitialized_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_REKEY_NOT_INIT") == "1" {
		logger := testutil.NewTestLogger()
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dir, Logger: logger})
		if err != nil {
			os.Exit(constants.ExitConfigError)
		}
		defer v.Close()
		handleRekeyVault(v, "old-key", "new-key", logger)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestHandleRekeyVault_VaultNotInitialized_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_REKEY_NOT_INIT=1",
		"G8E_TEST_TMPDIR="+dir,
	)
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "subprocess must exit with non-zero code, got: %v", err)
	assert.Equal(t, constants.ExitConfigError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// handleRekeyVault — vault initialized, successful rekey → ExitSuccess
// ---------------------------------------------------------------------------

func TestHandleRekeyVault_Success_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_REKEY_SUCCESS") == "1" {
		logger := testutil.NewTestLogger()
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dir, Logger: logger})
		if err != nil {
			os.Exit(constants.ExitConfigError)
		}
		defer v.Close()
		handleRekeyVault(v, "old-key", "new-key", logger)
		return
	}

	dir := t.TempDir()
	header, _, err := vaultpkg.NewVaultHeader("old-key")
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))

	cmd := exec.Command(os.Args[0], "-test.run=TestHandleRekeyVault_Success_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_REKEY_SUCCESS=1",
		"G8E_TEST_TMPDIR="+dir,
	)
	err = cmd.Run()
	assert.NoError(t, err, "successful rekey must exit with ExitSuccess (0)")
}

// ---------------------------------------------------------------------------
// handleVerifyVault — vault not initialized → ExitSuccess (treated as ok)
// ---------------------------------------------------------------------------

func TestHandleVerifyVault_NotInitialized_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_VERIFY_NOT_INIT") == "1" {
		logger := testutil.NewTestLogger()
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dir, Logger: logger})
		if err != nil {
			os.Exit(constants.ExitConfigError)
		}
		defer v.Close()
		handleVerifyVault(v, "any-key", logger)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestHandleVerifyVault_NotInitialized_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_VERIFY_NOT_INIT=1",
		"G8E_TEST_TMPDIR="+dir,
	)
	err := cmd.Run()
	assert.NoError(t, err, "verify on non-initialized vault must exit 0 (ExitSuccess)")
}

// ---------------------------------------------------------------------------
// handleVerifyVault — vault initialized, correct key → ExitSuccess
// ---------------------------------------------------------------------------

func TestHandleVerifyVault_ValidKey_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_VERIFY_VALID") == "1" {
		logger := testutil.NewTestLogger()
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dir, Logger: logger})
		if err != nil {
			os.Exit(constants.ExitConfigError)
		}
		defer v.Close()
		handleVerifyVault(v, "correct-key", logger)
		return
	}

	dir := t.TempDir()
	header, _, err := vaultpkg.NewVaultHeader("correct-key")
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))

	cmd := exec.Command(os.Args[0], "-test.run=TestHandleVerifyVault_ValidKey_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_VERIFY_VALID=1",
		"G8E_TEST_TMPDIR="+dir,
	)
	err = cmd.Run()
	assert.NoError(t, err, "verify with correct key must exit 0 (ExitSuccess)")
}

// ---------------------------------------------------------------------------
// handleVerifyVault — vault initialized, wrong key → ExitGeneralError
// ---------------------------------------------------------------------------

func TestHandleVerifyVault_WrongKey_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_VERIFY_WRONG") == "1" {
		logger := testutil.NewTestLogger()
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dir, Logger: logger})
		if err != nil {
			os.Exit(constants.ExitConfigError)
		}
		defer v.Close()
		handleVerifyVault(v, "wrong-key", logger)
		return
	}

	dir := t.TempDir()
	header, _, err := vaultpkg.NewVaultHeader("correct-key")
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))

	cmd := exec.Command(os.Args[0], "-test.run=TestHandleVerifyVault_WrongKey_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_VERIFY_WRONG=1",
		"G8E_TEST_TMPDIR="+dir,
	)
	err = cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "wrong key must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitGeneralError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// handleVerifyVault — missing API key → ExitConfigError
// ---------------------------------------------------------------------------

func TestHandleVerifyVault_MissingAPIKey_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_VERIFY_NO_KEY") == "1" {
		logger := testutil.NewTestLogger()
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dir, Logger: logger})
		if err != nil {
			os.Exit(constants.ExitConfigError)
		}
		defer v.Close()
		handleVerifyVault(v, "", logger)
		return
	}

	dir := t.TempDir()
	header, _, err := vaultpkg.NewVaultHeader("some-key")
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))

	cmd := exec.Command(os.Args[0], "-test.run=TestHandleVerifyVault_MissingAPIKey_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_VERIFY_NO_KEY=1",
		"G8E_TEST_TMPDIR="+dir,
		"G8E_OPERATOR_API_KEY=",
	)
	err = cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "missing api key must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitConfigError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// handleResetVault — vault not initialized → ExitSuccess (no-op)
// ---------------------------------------------------------------------------

func TestHandleResetVault_NotInitialized_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_RESET_NOT_INIT") == "1" {
		logger := testutil.NewTestLogger()
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dir, Logger: logger})
		if err != nil {
			os.Exit(constants.ExitConfigError)
		}
		defer v.Close()
		handleResetVault(v, logger)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestHandleResetVault_NotInitialized_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_RESET_NOT_INIT=1",
		"G8E_TEST_TMPDIR="+dir,
	)
	err := cmd.Run()
	assert.NoError(t, err, "reset on uninitialized vault must exit 0 (ExitSuccess)")
}

// ---------------------------------------------------------------------------
// handleResetVault — initialized, wrong confirmation → ExitSuccess (cancelled)
// ---------------------------------------------------------------------------

func TestHandleResetVault_WrongConfirmation_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_RESET_WRONG_CONFIRM") == "1" {
		logger := testutil.NewTestLogger()
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dir, Logger: logger})
		if err != nil {
			os.Exit(constants.ExitConfigError)
		}
		defer v.Close()
		// Pipe "NOPE" as stdin so the confirmation check fails
		handleResetVault(v, logger)
		return
	}

	dir := t.TempDir()
	header, _, err := vaultpkg.NewVaultHeader("some-key")
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))

	cmd := exec.Command(os.Args[0], "-test.run=TestHandleResetVault_WrongConfirmation_Subprocess", "-test.timeout=15s")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_RESET_WRONG_CONFIRM=1",
		"G8E_TEST_TMPDIR="+dir,
	)
	// Provide wrong confirmation word via stdin so fmt.Fscan reads it and returns
	cmd.Stdin = nopCloser("NOPE\n")
	err = cmd.Run()
	assert.NoError(t, err, "cancelled reset must exit 0 (ExitSuccess)")
}

// ---------------------------------------------------------------------------
// handleVaultCommand — bad log level → ExitConfigError
// ---------------------------------------------------------------------------

func TestHandleVaultCommand_BadLogLevel_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_VAULTCMD_BAD_LOG") == "1" {
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		handleVaultCommand(true, false, false, "new-key", "old-key", "notavalidlevel", dir)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestHandleVaultCommand_BadLogLevel_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_VAULTCMD_BAD_LOG=1",
		"G8E_TEST_TMPDIR="+dir,
	)
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "bad log level must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitConfigError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// handleVaultCommand — verify-vault path, vault not initialized → ExitSuccess
// ---------------------------------------------------------------------------

func TestHandleVaultCommand_VerifyVault_NotInitialized_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_VAULTCMD_VERIFY") == "1" {
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		handleVaultCommand(false, true, false, "some-key", "", "info", dir)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestHandleVaultCommand_VerifyVault_NotInitialized_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_VAULTCMD_VERIFY=1",
		"G8E_TEST_TMPDIR="+dir,
		"G8E_OPERATOR_API_KEY=",
	)
	err := cmd.Run()
	assert.NoError(t, err, "verify on uninitialized vault must exit 0")
}

// ---------------------------------------------------------------------------
// handleVaultCommand — reset-vault path, vault not initialized → ExitSuccess
// ---------------------------------------------------------------------------

func TestHandleVaultCommand_ResetVault_NotInitialized_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_VAULTCMD_RESET") == "1" {
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		handleVaultCommand(false, false, true, "", "", "info", dir)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestHandleVaultCommand_ResetVault_NotInitialized_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_VAULTCMD_RESET=1",
		"G8E_TEST_TMPDIR="+dir,
	)
	err := cmd.Run()
	assert.NoError(t, err, "reset on uninitialized vault must exit 0")
}

// ---------------------------------------------------------------------------
// runListenMode — invalid log level → ExitConfigError
// ---------------------------------------------------------------------------

func TestRunListenMode_BadLogLevel_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_LISTEN_BAD_LOG") == "1" {
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		runListenMode(0, 0, dir, "", dir, "", "", "notavalidlevel")
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestRunListenMode_BadLogLevel_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_LISTEN_BAD_LOG=1",
		"G8E_TEST_TMPDIR="+dir,
	)
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "bad log level must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitConfigError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// runOpenClawMode — invalid log level → ExitConfigError
// ---------------------------------------------------------------------------

func TestRunOpenClawMode_BadLogLevel_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_OPENCLAW_BAD_LOG") == "1" {
		runOpenClawMode("ws://localhost:18789", "", "", "", "", "notavalidlevel")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRunOpenClawMode_BadLogLevel_Subprocess")
	cmd.Env = append(os.Environ(), "G8E_TEST_OPENCLAW_BAD_LOG=1")
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "bad log level must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitConfigError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// runOpenClawMode — empty gateway URL → ExitConfigError
// ---------------------------------------------------------------------------

func TestRunOpenClawMode_EmptyURL_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_OPENCLAW_NO_URL") == "1" {
		runOpenClawMode("", "token", "node", "display", "", "info")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRunOpenClawMode_EmptyURL_Subprocess")
	cmd.Env = append(os.Environ(), "G8E_TEST_OPENCLAW_NO_URL=1")
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "empty URL must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitConfigError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// nopCloser adapts a string for use as cmd.Stdin
// ---------------------------------------------------------------------------

type stringReader struct{ s string }

func (r *stringReader) Read(p []byte) (int, error) {
	if len(r.s) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}

func nopCloser(s string) *stringReader { return &stringReader{s: s} }

// ---------------------------------------------------------------------------
// Vault header save helper — uses a known relative path so the subprocess
// can find it via G8E_TEST_TMPDIR.
// ---------------------------------------------------------------------------

func initVaultInDir(t *testing.T, dir, apiKey string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0700))
	header, _, err := vaultpkg.NewVaultHeader(apiKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))
}

func TestHandleRekeyVault_Success_VaultDataVerified(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	header, _, err := vaultpkg.NewVaultHeader("old-key")
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))

	v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	require.True(t, v.IsInitialized())
	require.NoError(t, v.Rekey("old-key", "new-key"))
	require.NoError(t, v.VerifyIntegrity("new-key"))
	require.Error(t, v.VerifyIntegrity("old-key"))
}

func TestHandleVaultCommand_DataDirResolution(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, ".g8e", "data")
	require.NoError(t, os.MkdirAll(dataDir, 0700))

	logger, err := configureLogger("info")
	require.NoError(t, err)
	require.NotNil(t, logger)

	v, err := vaultpkg.NewVault(&vaultpkg.VaultConfig{DataDir: dataDir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	assert.False(t, v.IsInitialized())
}
