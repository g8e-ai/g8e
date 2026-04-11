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

// Subprocess-based tests for RunStream, which calls os.Exit internally.
//
// Pattern: parent spawns a subprocess via exec.Command(os.Args[0], "-test.run=TestXxx")
// with a sentinel env var. The subprocess detects the env var, calls RunStream with
// controlled arguments, and os.Exit terminates the subprocess. The parent asserts the
// exit code without its own process being terminated.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// RunStream — flag parse error (unknown flag) → ExitGeneralError
// ---------------------------------------------------------------------------

func TestRunStream_UnknownFlag_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_STREAM_BAD_FLAG") == "1" {
		RunStream([]string{"--unknownflagxyz"})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRunStream_UnknownFlag_Subprocess")
	cmd.Env = append(os.Environ(), "G8E_TEST_STREAM_BAD_FLAG=1")
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "unknown flag must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitGeneralError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// RunStream — no hosts specified → ExitGeneralError
// ---------------------------------------------------------------------------

func TestRunStream_NoHosts_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_STREAM_NO_HOSTS") == "1" {
		RunStream([]string{})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRunStream_NoHosts_Subprocess")
	cmd.Env = append(os.Environ(), "G8E_TEST_STREAM_NO_HOSTS=1")
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "no hosts must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitGeneralError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// RunStream — invalid arch → ExitConfigError
// ---------------------------------------------------------------------------

func TestRunStream_InvalidArch_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_STREAM_BAD_ARCH") == "1" {
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		RunStream([]string{
			"--arch", "mips",
			"--binary-dir", dir,
			"somehost",
		})
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestRunStream_InvalidArch_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_STREAM_BAD_ARCH=1",
		string(constants.EnvVar.TestTmpDir)+"="+dir,
	)
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "invalid arch must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitConfigError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// RunStream — binary not found → ExitGeneralError
// ---------------------------------------------------------------------------

func TestRunStream_BinaryMissing_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_STREAM_NO_BIN") == "1" {
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		RunStream([]string{
			"--arch", "amd64",
			"--binary-dir", dir,
			"somehost",
		})
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestRunStream_BinaryMissing_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_STREAM_NO_BIN=1",
		string(constants.EnvVar.TestTmpDir)+"="+dir,
	)
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "missing binary must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitGeneralError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// RunStream — help flag → ExitSuccess
// ---------------------------------------------------------------------------

func TestRunStream_HelpFlag_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_STREAM_HELP") == "1" {
		RunStream([]string{"--help"})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRunStream_HelpFlag_Subprocess")
	cmd.Env = append(os.Environ(), "G8E_TEST_STREAM_HELP=1")
	err := cmd.Run()
	assert.NoError(t, err, "--help must exit 0 (ExitSuccess)")
}

// ---------------------------------------------------------------------------
// RunStream — hosts file not found → ExitGeneralError
// ---------------------------------------------------------------------------

func TestRunStream_HostsFileNotFound_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_STREAM_BAD_HOSTS_FILE") == "1" {
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		RunStream([]string{
			"--arch", "amd64",
			"--binary-dir", dir,
			"--hosts", filepath.Join(dir, "nonexistent-hosts.txt"),
		})
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestRunStream_HostsFileNotFound_Subprocess")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_STREAM_BAD_HOSTS_FILE=1",
		string(constants.EnvVar.TestTmpDir)+"="+dir,
	)
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "missing hosts file must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitGeneralError, exitErr.ExitCode())
}

// ---------------------------------------------------------------------------
// RunStream — valid binary + one unreachable host → ExitGeneralError
// (exercises the full RunStream path through runConcurrentStream)
// ---------------------------------------------------------------------------

func TestRunStream_ValidBinary_UnreachableHost_Subprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_STREAM_UNREACHABLE") == "1" {
		dir := os.Getenv(string(constants.EnvVar.TestTmpDir))
		binDir := filepath.Join(dir, "linux-amd64")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			os.Exit(constants.ExitGeneralError)
		}
		fakeBin := filepath.Join(binDir, "g8e.operator")
		if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			os.Exit(constants.ExitGeneralError)
		}
		RunStream([]string{
			"--arch", "amd64",
			"--binary-dir", dir,
			"--timeout", "2",
			"127.0.0.2",
		})
		return
	}

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestRunStream_ValidBinary_UnreachableHost_Subprocess", "-test.timeout=12s")
	cmd.Env = append(os.Environ(),
		"G8E_TEST_STREAM_UNREACHABLE=1",
		string(constants.EnvVar.TestTmpDir)+"="+dir,
	)
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "unreachable host must exit non-zero, got: %v", err)
	assert.Equal(t, constants.ExitGeneralError, exitErr.ExitCode())
}
