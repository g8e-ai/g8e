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
// See the License for the specific language and governing permissions and
// limitations under the License.

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
