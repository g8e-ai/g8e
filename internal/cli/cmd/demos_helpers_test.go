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
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/tui"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestToDockerPath_NonWindowsReturnsPathUnchanged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}
	assert.Equal(t, "/foo/bar/baz", toDockerPath("/foo/bar/baz"))
}

func TestToDockerPath_WindowsConvertsBackslashes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows platform")
	}
	assert.Equal(t, "C:/foo/bar", toDockerPath("C:\\foo\\bar"))
}

func TestTitleCase_CapitalizesEachWord(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "Hello World"},
		{"single", "Single"},
		{"already capitalized", "Already Capitalized"},
		{"MIXED case WORDS", "Mixed Case Words"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, titleCase(tt.input))
		})
	}
}

func TestTitleCase_EmptyStringReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", titleCase(""))
}

func TestDemoHTTPPort_AllKnownOrgs(t *testing.T) {
	tests := []struct {
		org      string
		expected string
	}{
		{constants.DemosOrgHealthcare, "8081"},
		{constants.DemosOrgGov, "8080"},
		{constants.DemosOrgFinance, "8082"},
		{constants.DemosOrgDHS, "8087"},
		{constants.DemosOrgFedRAMP, "8088"},
		{constants.DemosOrgFrontend, "8083"},
	}
	for _, tt := range tests {
		t.Run(tt.org, func(t *testing.T) {
			assert.Equal(t, tt.expected, demoHTTPPort(tt.org))
		})
	}
}

func TestDemoHTTPPort_UnknownOrgReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", demoHTTPPort("unknown-org"))
}

func TestDemoHTTPSPort_AllKnownOrgs(t *testing.T) {
	tests := []struct {
		org      string
		expected string
	}{
		{constants.DemosOrgHealthcare, "8444"},
		{constants.DemosOrgGov, "8443"},
		{constants.DemosOrgFinance, "8445"},
		{constants.DemosOrgDHS, "8450"},
		{constants.DemosOrgFedRAMP, "8451"},
		{constants.DemosOrgFrontend, "8446"},
	}
	for _, tt := range tests {
		t.Run(tt.org, func(t *testing.T) {
			assert.Equal(t, tt.expected, demoHTTPSPort(tt.org))
		})
	}
}

func TestDemoHTTPSPort_UnknownOrgReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", demoHTTPSPort("unknown-org"))
}

func TestBuildScpArgs_AllFlagsEnabled(t *testing.T) {
	args := buildScpArgs(2222, "/home/user/.ssh/id_rsa", true, true, true, true, "/src/file", "user@host:/dst")
	assert.Contains(t, args, "-P")
	assert.Contains(t, args, "2222")
	assert.Contains(t, args, "-i")
	assert.Contains(t, args, "/home/user/.ssh/id_rsa")
	assert.Contains(t, args, "-r")
	assert.Contains(t, args, "-p")
	assert.Contains(t, args, "-v")
	assert.Contains(t, args, "-C")
	assert.Equal(t, "/src/file", args[len(args)-2])
	assert.Equal(t, "user@host:/dst", args[len(args)-1])
}

func TestBuildScpArgs_NoFlagsSet(t *testing.T) {
	args := buildScpArgs(0, "", false, false, false, false, "/src/file", "user@host:/dst")
	assert.Len(t, args, 2)
	assert.Equal(t, "/src/file", args[0])
	assert.Equal(t, "user@host:/dst", args[1])
}

func TestBuildScpArgs_PartialFlags(t *testing.T) {
	args := buildScpArgs(2222, "", false, false, true, false, "/src/file", "user@host:/dst")
	assert.Contains(t, args, "-P")
	assert.Contains(t, args, "2222")
	assert.Contains(t, args, "-v")
	assert.NotContains(t, args, "-i")
	assert.NotContains(t, args, "-r")
	assert.NotContains(t, args, "-p")
	assert.NotContains(t, args, "-C")
}

func TestCopyFile_SmallFile(t *testing.T) {
	src := filepath.Join(testutil.TempDir(t), "src.txt")
	dst := filepath.Join(testutil.TempDir(t), "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("hello world"), 0o644))

	require.NoError(t, copyFile(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestCopyFile_NonExistentSource(t *testing.T) {
	dst := filepath.Join(testutil.TempDir(t), "dst.txt")
	err := copyFile("/nonexistent/source/file.txt", dst)
	assert.Error(t, err)
}

func TestCopyFile_DestinationInNonExistentDir(t *testing.T) {
	src := filepath.Join(testutil.TempDir(t), "src.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))
	dst := filepath.Join(testutil.TempDir(t), "nonexistent", "dst.txt")
	err := copyFile(src, dst)
	assert.Error(t, err)
}

func TestCopyFile_PreservesFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows - Unix file modes not supported")
	}
	src := filepath.Join(testutil.TempDir(t), "src.sh")
	dst := filepath.Join(testutil.TempDir(t), "dst.sh")
	require.NoError(t, os.WriteFile(src, []byte("#!/bin/sh\n"), 0o755))

	require.NoError(t, copyFile(src, dst))

	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode())
}

func TestCopyFile_EmptyFile(t *testing.T) {
	src := filepath.Join(testutil.TempDir(t), "empty.txt")
	dst := filepath.Join(testutil.TempDir(t), "copy.txt")
	require.NoError(t, os.WriteFile(src, []byte{}, 0o644))

	require.NoError(t, copyFile(src, dst))

	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.Size())
}

func TestHarnessRun_ExecMode(t *testing.T) {
	cfg := defaultHarnessConfig("agent-runtime")
	args := harnessRun("mcp_basic_read", cfg)

	assert.Equal(t, "docker", args[0])
	assert.Equal(t, "compose", args[1])
	assert.Equal(t, "exec", args[2])
	assert.Equal(t, "-T", args[3])
	assert.Equal(t, "agent-runtime", args[4])
	assert.Equal(t, "/g8e", args[5])
	assert.Equal(t, "demos", args[6])
	assert.Equal(t, "scenarios", args[7])
	assert.Equal(t, "run", args[8])
	assert.Contains(t, args, "--mtls-url")
	assert.Contains(t, args, "https://g8e.local:8443")
	assert.Contains(t, args, "--public-url")
	assert.Contains(t, args, "http://g8e.local:8080")
	assert.Contains(t, args, "--cert")
	assert.Contains(t, args, constants.ContainerOperatorCert)
	assert.Contains(t, args, "--key")
	assert.Contains(t, args, constants.ContainerOperatorKey)
	assert.Contains(t, args, "--ca")
	assert.Contains(t, args, constants.ContainerCABundle)
	assert.Contains(t, args, "--ensemble")
	assert.Contains(t, args, "3")
	assert.Contains(t, args, "--l3-mode")
	assert.Contains(t, args, "mock")
	assert.Equal(t, "mcp_basic_read", args[len(args)-1])
}

func TestHarnessRun_RunMode(t *testing.T) {
	cfg := defaultHarnessConfig("agent-runtime")
	cfg.UseRun = true
	args := harnessRun("a2a_discover", cfg)

	assert.Equal(t, "docker", args[0])
	assert.Equal(t, "compose", args[1])
	assert.Equal(t, "run", args[2])
	assert.Equal(t, "--rm", args[3])
	assert.Equal(t, "-T", args[4])
	assert.Equal(t, "--no-deps", args[5])
	assert.Equal(t, "agent-runtime", args[6])
	assert.Equal(t, "demos", args[7])
	assert.Equal(t, "scenarios", args[8])
	assert.Equal(t, "run", args[9])
}

func TestHarnessRun_WithConsensusSeedAndTribunalID(t *testing.T) {
	cfg := defaultHarnessConfig("operator")
	cfg.ConsensusSeed = "abc123"
	cfg.TribunalID = "trib-001"
	args := harnessRun("gov_cui_exfil", cfg)

	assert.Contains(t, args, "--consensus-seed")
	assert.Contains(t, args, "abc123")
	assert.Contains(t, args, "--tribunal-id")
	assert.Contains(t, args, "trib-001")
}

func TestHarnessRun_WithoutEnsembleOrL3Mode(t *testing.T) {
	cfg := harnessConfig{
		Container: "operator",
		MTLSURL:   "https://g8e.local:8443",
		PublicURL: "http://g8e.local:8080",
		CertPath:  "/certs/operator.pem",
		KeyPath:   "/certs/operator-key.pem",
		CAPath:    "/certs/ca-bundle.pem",
	}
	args := harnessRun("test_scenario", cfg)

	assert.NotContains(t, args, "--ensemble")
	assert.NotContains(t, args, "--l3-mode")
	assert.NotContains(t, args, "--consensus-seed")
	assert.NotContains(t, args, "--tribunal-id")
}

func TestDefaultHarnessConfig_ReturnsExpectedDefaults(t *testing.T) {
	cfg := defaultHarnessConfig("my-container")

	assert.Equal(t, "my-container", cfg.Container)
	assert.Equal(t, "https://g8e.local:8443", cfg.MTLSURL)
	assert.Equal(t, "http://g8e.local:8080", cfg.PublicURL)
	assert.Equal(t, constants.ContainerOperatorCert, cfg.CertPath)
	assert.Equal(t, constants.ContainerOperatorKey, cfg.KeyPath)
	assert.Equal(t, constants.ContainerCABundle, cfg.CAPath)
	assert.Equal(t, 3, cfg.EnsembleSize)
	assert.Equal(t, "mock", cfg.L3Mode)
	assert.False(t, cfg.UseRun)
}

func TestCheckDemoDirExists_ExistingDirReturnsNil(t *testing.T) {
	tmp := testutil.TempDir(t)
	assert.NoError(t, checkDemoDirExists(tmp, "test-org"))
}

func TestCheckDemoDirExists_NonExistentReturnsNotFound(t *testing.T) {
	tmp := testutil.TempDir(t)
	nonExistent := filepath.Join(tmp, "no-such-dir")
	err := checkDemoDirExists(nonExistent, "no-such-org")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotFound))
}

func TestCheckComposeFileExists_ExistingFileReturnsNil(t *testing.T) {
	tmp := testutil.TempDir(t)
	composePath := filepath.Join(tmp, constants.DemosComposeFile)
	require.NoError(t, os.WriteFile(composePath, []byte("version: '3'"), 0o644))
	assert.NoError(t, checkComposeFileExists(composePath, "test-org"))
}

func TestCheckComposeFileExists_NonExistentReturnsNotFound(t *testing.T) {
	err := checkComposeFileExists("/nonexistent/compose.yml", "test-org")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotFound))
}

func TestConfirmAction_YesResponse(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("y\n"))
	assert.True(t, confirmAction(cmd, "Proceed?"))
}

func TestConfirmAction_NoResponse(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("n\n"))
	assert.False(t, confirmAction(cmd, "Proceed?"))
}

func TestConfirmAction_EmptyResponse(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("\n"))
	assert.False(t, confirmAction(cmd, "Proceed?"))
}

func TestConfirmAction_YesFullWord(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("yes\n"))
	assert.True(t, confirmAction(cmd, "Proceed?"))
}

func TestNewDemoEmitter_NilProgramIsNoOp(t *testing.T) {
	e := NewDemoEmitter(nil)
	assert.NotNil(t, e)
	e.Pipeline(tui.StageL1, tui.StatusActive, "tx-1", "detail")
	e.Ledger(tui.LevelInfo, "message")
	e.Consensus(constants.TribunalMemberAxiom, true, true, 2, 3, tui.ConsensusReached, "hash-1")
}

func TestDemoPrintln_VerboseMode(t *testing.T) {
	original := demoVerbose
	demoVerbose = true
	t.Cleanup(func() { demoVerbose = original })

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
		r.Close()
	})

	demoPrintln("test output")
	w.Close()
	buf.ReadFrom(r)
	assert.Contains(t, buf.String(), "test output")
}

func TestDemoPrintln_NonVerboseMode(t *testing.T) {
	original := demoVerbose
	demoVerbose = false
	t.Cleanup(func() { demoVerbose = original })

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
		r.Close()
	})

	demoPrintln("should not appear")
	w.Close()
	buf.ReadFrom(r)
	assert.Empty(t, buf.String())
}

func TestDemoPrintf_VerboseMode(t *testing.T) {
	original := demoVerbose
	demoVerbose = true
	t.Cleanup(func() { demoVerbose = original })

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
		r.Close()
	})

	demoPrintf("formatted %s", "output")
	w.Close()
	buf.ReadFrom(r)
	assert.Contains(t, buf.String(), "formatted output")
}

func TestDemoStep_CommandSucceeds(t *testing.T) {
	original := demoVerbose
	demoVerbose = false
	t.Cleanup(func() { demoVerbose = original })

	tmp := testutil.TempDir(t)
	err := demoStep(tmp, "true command", false, "true")
	assert.NoError(t, err)
}

func TestDemoStep_CommandFails(t *testing.T) {
	original := demoVerbose
	demoVerbose = false
	t.Cleanup(func() { demoVerbose = original })

	tmp := testutil.TempDir(t)
	err := demoStep(tmp, "false command", false, "false")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "false command")
}

func TestDemoStep_FatalError(t *testing.T) {
	original := demoVerbose
	demoVerbose = false
	t.Cleanup(func() { demoVerbose = original })

	tmp := testutil.TempDir(t)
	err := demoStep(tmp, "critical step", true, "false")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "critical step")
	assert.NotContains(t, err.Error(), "non-fatal")
}

func TestDemoScenarioStep_Success(t *testing.T) {
	original := demoVerbose
	demoVerbose = false
	t.Cleanup(func() { demoVerbose = original })

	tmp := testutil.TempDir(t)
	assert.True(t, demoScenarioStep(tmp, "step that succeeds", []string{"true"}))
}

func TestDemoScenarioStep_Failure(t *testing.T) {
	original := demoVerbose
	demoVerbose = false
	t.Cleanup(func() { demoVerbose = original })

	tmp := testutil.TempDir(t)
	assert.False(t, demoScenarioStep(tmp, "step that fails", []string{"false"}))
}

func TestDemoStepWarn_FailurePrintsWarning(t *testing.T) {
	original := demoVerbose
	demoVerbose = false
	t.Cleanup(func() { demoVerbose = original })

	tmp := testutil.TempDir(t)
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
		r.Close()
	})

	demoStepWarn(tmp, "warning step", "false")
	w.Close()
	buf.ReadFrom(r)
	assert.Contains(t, buf.String(), "warning")
}

func TestPrintResultsTable_OutputContainsAllRows(t *testing.T) {
	results := []scenarioResult{
		{number: "1", name: "First Scenario", status: "PASS", metrics: "100% good"},
		{number: "2", name: "Second Scenario", status: "FAIL", metrics: "timeout"},
	}

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	printResultsTable(cmd, "healthcare", results)
	output := buf.String()
	assert.Contains(t, output, "Healthcare")
	assert.Contains(t, output, "First Scenario")
	assert.Contains(t, output, "PASS")
	assert.Contains(t, output, "Second Scenario")
	assert.Contains(t, output, "FAIL")
}

func TestRunScenarioWithResult_UnknownOrgReturnsNotFound(t *testing.T) {
	_, err := runScenarioWithResult("unknown-org", "/tmp", "1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotFound))
}

func TestRunAllScenarios_UnknownOrgReturnsNotFound(t *testing.T) {
	cmd := &cobra.Command{}
	err := runAllScenarios(cmd, "unknown-org", "/tmp")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotFound))
}

func TestRunDemosRun_NoArgsReturnsMissingRequiredField(t *testing.T) {
	cmd := &cobra.Command{}
	err := runDemosRun(cmd, []string{}, false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMissingRequiredField))
}

func TestRunDemosRun_TooManyArgsReturnsValidationFailed(t *testing.T) {
	cmd := &cobra.Command{}
	err := runDemosRun(cmd, []string{"org", "1", "extra"}, false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrValidationFailed))
}
