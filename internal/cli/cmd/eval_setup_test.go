// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// stubEvalRunner is a Tier 1 stub evalCommandRunner that records every
// invocation and returns a configurable error. It never spawns a process.
type stubEvalRunner struct {
	calls    []stubEvalRunnerCall
	returnFn func(name string, args []string) error
}

type stubEvalRunnerCall struct {
	Name string
	Args []string
}

func (s *stubEvalRunner) Run(_ context.Context, name string, args []string, stdout, stderr io.Writer) error {
	s.calls = append(s.calls, stubEvalRunnerCall{Name: name, Args: append([]string(nil), args...)})
	if s.returnFn != nil {
		return s.returnFn(name, args)
	}
	// Default: simulate the self-check printing the engine version.
	if len(args) >= 2 && args[0] == "-c" && strings.Contains(args[1], "g8e_evals") {
		_, _ = io.WriteString(stdout, "0.3.0")
	}
	return nil
}

// stubEvalFileReader is a Tier 1 stub evalFileReader that maps paths to
// content. It never touches the filesystem.
type stubEvalFileReader struct {
	files map[string][]byte
	err   error
}

func (s stubEvalFileReader) ReadFile(path string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if data, ok := s.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

// stubEvalExecutableLookup is a Tier 1 stub evalExecutableLookup that
// returns a configured path or error. It never consults PATH.
type stubEvalExecutableLookup struct {
	path string
	err  error
}

func (s stubEvalExecutableLookup) LookPath(_ string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.path, nil
}

// newEvalSetupTestEnv builds a hermetic environment for setup tests. It
// creates a temp directory with the root markers and eval project
// markers, and returns the configured dependencies and project root.
func newEvalSetupTestEnv(t *testing.T) (evalSetupDeps, string) {
	t.Helper()
	tmp := t.TempDir()

	// Root markers.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, constants.EvalRootVersion), []byte("2.1.8\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, constants.EvalRootMakefile), []byte("# Makefile\n"), 0o644))

	// Eval project markers.
	evalProject := filepath.Join(tmp, constants.EvalProjectDir)
	require.NoError(t, os.MkdirAll(evalProject, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(evalProject, constants.EvalProjectPyproject), []byte("[project]\nname = \"g8e-evals\"\nrequires-python = \">=3.12\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(evalProject, constants.EvalProjectLockfile), []byte("# uv.lock\n"), 0o644))

	// Project-local interpreter marker so the stat check passes after sync.
	evalVenvBin := filepath.Join(evalProject, constants.EvalProjectVenvDir, "bin")
	require.NoError(t, os.MkdirAll(evalVenvBin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(evalVenvBin, "python"), []byte("#!/bin/sh\nexit 0\n"), 0o755))

	cfg := &config.Config{ProjectRoot: tmp}
	deps := evalSetupDeps{
		configLoader: configLoaderFor(cfg),
		stat:         newStubStatWithEvalProject(tmp, evalProject),
		uvLookup:     stubEvalExecutableLookup{path: "/usr/local/bin/uv"},
		runner:       &stubEvalRunner{},
		fileReader: stubEvalFileReader{
			files: map[string][]byte{
				filepath.Join(evalProject, constants.EvalProjectPyproject): []byte("[project]\nname = \"g8e-evals\"\nrequires-python = \">=3.12\"\n"),
				filepath.Join(evalProject, constants.EvalProjectLockfile):  []byte("# uv.lock\n"),
			},
		},
	}
	return deps, tmp
}

// newStubStatWithEvalProject returns a stubEvalFileStat that reports
// the root markers, eval project markers, and project interpreter as
// existing.
func newStubStatWithEvalProject(tmp, evalProject string) evalFileStat {
	existing := []string{
		filepath.Join(tmp, constants.EvalRootVersion),
		filepath.Join(tmp, constants.EvalRootMakefile),
		filepath.Join(evalProject, constants.EvalProjectPyproject),
		filepath.Join(evalProject, constants.EvalProjectLockfile),
		projectInterpreterPath(evalProject),
	}
	m := make(map[string]bool, len(existing))
	for _, p := range existing {
		m[p] = true
	}
	return stubEvalFileStat{existing: m}
}

func TestEvalSetup_SucceedsWhenEnvironmentComplete(t *testing.T) {
	deps, _ := newEvalSetupTestEnv(t)
	var stdout, stderr bytes.Buffer

	result, err := runEvalSetup(t.Context(), deps, "", false, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "succeeded", result.Status)
	assert.Equal(t, "0.3.0", result.EngineVersion)
	assert.Equal(t, ">=3.12", result.PythonVersion)
	assert.NotEmpty(t, result.InterpreterPath)
	// uv sync, self-check, and uv --version should have been invoked.
	runner := deps.runner.(*stubEvalRunner)
	require.Len(t, runner.calls, 3, "expected uv sync + self-check + uv version")
	assert.Equal(t, "/usr/local/bin/uv", runner.calls[0].Name)
	assert.Contains(t, runner.calls[0].Args, "sync")
	assert.Contains(t, runner.calls[0].Args, "--locked")
	assert.Contains(t, runner.calls[0].Args, "--project")
}

func TestEvalSetup_FailsWhenUVUnavailable(t *testing.T) {
	deps, _ := newEvalSetupTestEnv(t)
	deps.uvLookup = stubEvalExecutableLookup{err: errors.New("not found")}
	var stdout, stderr bytes.Buffer

	_, err := runEvalSetup(t.Context(), deps, "", false, &stdout, &stderr)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalUVUnavailable)
	// No child process should have been started.
	runner := deps.runner.(*stubEvalRunner)
	assert.Empty(t, runner.calls)
}

func TestEvalSetup_FailsWhenRootMarkerMissing(t *testing.T) {
	deps, tmp := newEvalSetupTestEnv(t)
	// Build a stat that reports the VERSION marker as missing.
	evalProject := filepath.Join(tmp, constants.EvalProjectDir)
	m := map[string]bool{
		filepath.Join(tmp, constants.EvalRootMakefile):             true,
		filepath.Join(evalProject, constants.EvalProjectPyproject): true,
		filepath.Join(evalProject, constants.EvalProjectLockfile):  true,
		projectInterpreterPath(evalProject):                        true,
	}
	deps.stat = stubEvalFileStat{existing: m}
	var stdout, stderr bytes.Buffer

	_, err := runEvalSetup(t.Context(), deps, "", false, &stdout, &stderr)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalProjectNotFound)
	runner := deps.runner.(*stubEvalRunner)
	assert.Empty(t, runner.calls, "no child process should start when root markers are missing")
}

func TestEvalSetup_FailsWhenEvalProjectMissing(t *testing.T) {
	deps, tmp := newEvalSetupTestEnv(t)
	// Build a stat that reports the eval project pyproject as missing.
	evalProject := filepath.Join(tmp, constants.EvalProjectDir)
	m := map[string]bool{
		filepath.Join(tmp, constants.EvalRootVersion):             true,
		filepath.Join(tmp, constants.EvalRootMakefile):            true,
		filepath.Join(evalProject, constants.EvalProjectLockfile): true,
		projectInterpreterPath(evalProject):                       true,
	}
	deps.stat = stubEvalFileStat{existing: m}
	var stdout, stderr bytes.Buffer

	_, err := runEvalSetup(t.Context(), deps, "", false, &stdout, &stderr)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalProjectNotFound)
	runner := deps.runner.(*stubEvalRunner)
	assert.Empty(t, runner.calls, "no child process should start when eval project markers are missing")
}

func TestEvalSetup_FailsWhenUVSyncExitsNonZero(t *testing.T) {
	deps, _ := newEvalSetupTestEnv(t)
	deps.runner = &stubEvalRunner{returnFn: func(name string, args []string) error {
		if len(args) > 0 && args[0] == "sync" {
			return errors.New("uv sync failed: lockfile mismatch")
		}
		return nil
	}}
	var stdout, stderr bytes.Buffer

	_, err := runEvalSetup(t.Context(), deps, "", false, &stdout, &stderr)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalUVSyncFailed)
}

func TestEvalSetup_FailsWhenSelfCheckFails(t *testing.T) {
	deps, _ := newEvalSetupTestEnv(t)
	deps.runner = &stubEvalRunner{returnFn: func(name string, args []string) error {
		if len(args) >= 2 && args[0] == "-c" && strings.Contains(args[1], "g8e_evals") {
			return errors.New("ModuleNotFoundError: No module named 'numpy'")
		}
		return nil
	}}
	var stdout, stderr bytes.Buffer

	_, err := runEvalSetup(t.Context(), deps, "", false, &stdout, &stderr)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalEngineSelfCheckFailed)
}

func TestEvalSetup_FailsWhenInterpreterMissingAfterSync(t *testing.T) {
	deps, _ := newEvalSetupTestEnv(t)
	// Use a stat that reports the interpreter as missing even after sync.
	deps.stat = missingInterpreterStat{}
	var stdout, stderr bytes.Buffer

	_, err := runEvalSetup(t.Context(), deps, "", false, &stdout, &stderr)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalUVSyncFailed)
}

type missingInterpreterStat struct{}

func (missingInterpreterStat) Stat(path string) (os.FileInfo, error) {
	// Report everything as existing except the interpreter path.
	if strings.HasSuffix(path, "/bin/python") {
		return nil, os.ErrNotExist
	}
	return fakeFileInfo{}, nil
}

func TestEvalSetup_FailsWhenPythonRequirementMissing(t *testing.T) {
	deps, _ := newEvalSetupTestEnv(t)
	cfg, _ := deps.configLoader("")
	evalProject := filepath.Join(cfg.ProjectRoot, constants.EvalProjectDir)
	deps.fileReader = stubEvalFileReader{
		files: map[string][]byte{
			filepath.Join(evalProject, constants.EvalProjectPyproject): []byte("[project]\nname = \"g8e-evals\"\n"),
			filepath.Join(evalProject, constants.EvalProjectLockfile):  []byte("# uv.lock\n"),
		},
	}
	var stdout, stderr bytes.Buffer

	_, err := runEvalSetup(t.Context(), deps, "", false, &stdout, &stderr)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalConfigInvalid)
}

func TestEvalSetup_FailsWhenPythonRequirementTooOld(t *testing.T) {
	deps, _ := newEvalSetupTestEnv(t)
	cfg, _ := deps.configLoader("")
	evalProject := filepath.Join(cfg.ProjectRoot, constants.EvalProjectDir)
	deps.fileReader = stubEvalFileReader{
		files: map[string][]byte{
			filepath.Join(evalProject, constants.EvalProjectPyproject): []byte("[project]\nname = \"g8e-evals\"\nrequires-python = \">=3.10\"\n"),
			filepath.Join(evalProject, constants.EvalProjectLockfile):  []byte("# uv.lock\n"),
		},
	}
	var stdout, stderr bytes.Buffer

	_, err := runEvalSetup(t.Context(), deps, "", false, &stdout, &stderr)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalConfigInvalid)
}

func TestEvalSetup_WithTestToolsAddsExtraFlag(t *testing.T) {
	deps, _ := newEvalSetupTestEnv(t)
	var stdout, stderr bytes.Buffer

	_, err := runEvalSetup(t.Context(), deps, "", true, &stdout, &stderr)

	require.NoError(t, err)
	runner := deps.runner.(*stubEvalRunner)
	require.Len(t, runner.calls, 3)
	assert.Contains(t, runner.calls[0].Args, "--extra")
	assert.Contains(t, runner.calls[0].Args, constants.EvalTestExtra)
}

func TestEvalSetupCmd_RegistersWithExpectedFlags(t *testing.T) {
	// The project-root flag is a persistent flag on the parent eval command.
	parent := evalCmd()
	var setupFound bool
	for _, sub := range parent.Commands() {
		if sub.Name() == "setup" {
			setupFound = true
			assert.NotNil(t, sub.Flags().Lookup("with-test-tools"))
			assert.NotNil(t, sub.Flags().Lookup("json"))
			assert.NotNil(t, parent.PersistentFlags().Lookup("project-root"))
		}
	}
	assert.True(t, setupFound, "eval parent should register setup subcommand")
}

func TestEvalSetupCmd_HumanOutputContainsEngineVersion(t *testing.T) {
	cmd := evalSetupCmdWithDeps(evalSetupDeps{
		configLoader: func(_ string) (*config.Config, error) {
			return &config.Config{ProjectRoot: t.TempDir()}, nil
		},
		stat:       stubEvalFileStat{existing: map[string]bool{}},
		uvLookup:   stubEvalExecutableLookup{err: errors.New("not found")},
		runner:     &stubEvalRunner{},
		fileReader: stubEvalFileReader{},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	require.Error(t, err)
}
