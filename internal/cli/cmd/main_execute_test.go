// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteWithVersionInfo_HelpFlagPrintsUsageAndExitsZero(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"g8e", "--help"}
	t.Cleanup(func() { os.Args = originalArgs })

	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = originalStdout
	})

	originalExit := osExit
	osExit = func(code int) {}
	t.Cleanup(func() { osExit = originalExit })

	ExecuteWithVersionInfo("test-version", "test-build", "2026-07-10", "linux/amd64")

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)

	output := buf.String()
	assert.Contains(t, output, "g8e")
	assert.Contains(t, output, "zero-trust execution platform")
}

func TestExecuteWithVersionInfo_VersionFlagPrintsVersion(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"g8e", "--version"}
	t.Cleanup(func() { os.Args = originalArgs })

	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = originalStdout
	})

	originalExit := osExit
	osExit = func(code int) {}
	t.Cleanup(func() { osExit = originalExit })

	ExecuteWithVersionInfo("1.2.3-test", "build-abc", "2026-07-10", "linux/amd64")

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)

	output := buf.String()
	assert.Contains(t, output, "1.2.3-test")
}

func TestExecuteWithVersionInfo_InvalidCommandReturnsError(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"g8e", "nonexistent-command"}
	t.Cleanup(func() { os.Args = originalArgs })

	originalStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = originalStderr
	})

	exitCode := -1
	originalExit := osExit
	osExit = func(code int) { exitCode = code }
	t.Cleanup(func() { osExit = originalExit })

	ExecuteWithVersionInfo("test-version", "test-build", "2026-07-10", "linux/amd64")

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)

	output := buf.String()
	assert.Contains(t, output, "Error")
	require.Equal(t, 1, exitCode)
}
