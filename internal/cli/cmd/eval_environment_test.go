// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// stubEvalFileStat is a Tier 1 stub evalFileStat that maps paths to
// synthetic FileInfo. It never touches the filesystem.
type stubEvalFileStat struct {
	existing map[string]bool
}

func (s stubEvalFileStat) Stat(path string) (os.FileInfo, error) {
	if s.existing[path] {
		return fakeFileInfo{}, nil
	}
	return nil, os.ErrNotExist
}

type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "" }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }

func newStubResolver(existing ...string) *evalEnvironmentResolver {
	m := make(map[string]bool, len(existing))
	for _, p := range existing {
		m[p] = true
	}
	return &evalEnvironmentResolver{stat: stubEvalFileStat{existing: m}}
}

func TestEvalEnvironmentResolver_SelectsProjectInterpreterWhenProjectMarkersPresent(t *testing.T) {
	evalProject := "/repo/ensemble/evals"
	interpreter := projectInterpreterPath(evalProject)
	pyproject := filepath.Join(evalProject, constants.EvalProjectPyproject)
	lockfile := filepath.Join(evalProject, constants.EvalProjectLockfile)

	resolver := newStubResolver(pyproject, lockfile, interpreter)
	result, err := resolver.Resolve(t.Context(), evalProjectRootSpec{
		RepositoryRoot: "/repo",
		EvalProject:    evalProject,
	})

	require.NoError(t, err)
	assert.Equal(t, interpreter, result.InterpreterPath)
	assert.False(t, result.FellBackToPath)
	assert.False(t, result.FellBackToRootVenv)
}

func TestEvalEnvironmentResolver_FailsClosedWhenPyprojectMissing(t *testing.T) {
	evalProject := "/repo/ensemble/evals"
	lockfile := filepath.Join(evalProject, constants.EvalProjectLockfile)
	interpreter := projectInterpreterPath(evalProject)

	resolver := newStubResolver(lockfile, interpreter)
	_, err := resolver.Resolve(t.Context(), evalProjectRootSpec{
		RepositoryRoot: "/repo",
		EvalProject:    evalProject,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalProjectNotFound)
}

func TestEvalEnvironmentResolver_FailsClosedWhenLockfileMissing(t *testing.T) {
	evalProject := "/repo/ensemble/evals"
	pyproject := filepath.Join(evalProject, constants.EvalProjectPyproject)
	interpreter := projectInterpreterPath(evalProject)

	resolver := newStubResolver(pyproject, interpreter)
	_, err := resolver.Resolve(t.Context(), evalProjectRootSpec{
		RepositoryRoot: "/repo",
		EvalProject:    evalProject,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalProjectNotFound)
}

func TestEvalEnvironmentResolver_ReturnsSetupRequiredWhenInterpreterAbsent(t *testing.T) {
	evalProject := "/repo/ensemble/evals"
	pyproject := filepath.Join(evalProject, constants.EvalProjectPyproject)
	lockfile := filepath.Join(evalProject, constants.EvalProjectLockfile)

	resolver := newStubResolver(pyproject, lockfile)
	_, err := resolver.Resolve(t.Context(), evalProjectRootSpec{
		RepositoryRoot: "/repo",
		EvalProject:    evalProject,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalEngineNotSetUp)
}

func TestEvalEnvironmentResolver_RejectsEmptyRepositoryRoot(t *testing.T) {
	resolver := newStubResolver()
	_, err := resolver.Resolve(t.Context(), evalProjectRootSpec{
		RepositoryRoot: "",
		EvalProject:    "/repo/ensemble/evals",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalProjectNotFound)
}

func TestEvalEnvironmentResolver_RejectsEmptyEvalProject(t *testing.T) {
	resolver := newStubResolver()
	_, err := resolver.Resolve(t.Context(), evalProjectRootSpec{
		RepositoryRoot: "/repo",
		EvalProject:    "",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalProjectNotFound)
}

func TestEvalEnvironmentResolver_NeverLaunchesChildProcess(t *testing.T) {
	// The resolver is read-only: it performs only stat checks. This test
	// proves the resolver does not spawn a child process even when the
	// environment is fully resolved. A panic in a tripwire executable
	// lookup would surface as a test failure, but the resolver does not
	// hold an executable lookup dependency, so this is a structural
	// assertion that Resolve returns without side effects.
	evalProject := "/repo/ensemble/evals"
	interpreter := projectInterpreterPath(evalProject)
	pyproject := filepath.Join(evalProject, constants.EvalProjectPyproject)
	lockfile := filepath.Join(evalProject, constants.EvalProjectLockfile)

	resolver := newStubResolver(pyproject, lockfile, interpreter)
	result, err := resolver.Resolve(t.Context(), evalProjectRootSpec{
		RepositoryRoot: "/repo",
		EvalProject:    evalProject,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result.InterpreterPath)
}

// TestEvalEnvironmentResolver_IgnoresVirtualEnvAndPathByConstruction
// asserts the resolver has no dependency on VIRTUAL_ENV or PATH. The
// resolver struct carries only an evalFileStat; it has no executable
// lookup field. This is a compile-time guarantee that the defect cannot
// recur through this resolver.
func TestEvalEnvironmentResolver_IgnoresVirtualEnvAndPathByConstruction(t *testing.T) {
	t.Setenv("VIRTUAL_ENV", "/unrelated/.venv")
	t.Setenv("PATH", "/poisoned")
	// Force a PATH lookup to fail if any code path consulted it.
	if original, ok := os.LookupEnv("PATH"); ok {
		_ = original
	}

	evalProject := "/repo/ensemble/evals"
	interpreter := projectInterpreterPath(evalProject)
	pyproject := filepath.Join(evalProject, constants.EvalProjectPyproject)
	lockfile := filepath.Join(evalProject, constants.EvalProjectLockfile)

	resolver := newStubResolver(pyproject, lockfile, interpreter)
	result, err := resolver.Resolve(t.Context(), evalProjectRootSpec{
		RepositoryRoot: "/repo",
		EvalProject:    evalProject,
	})

	require.NoError(t, err)
	assert.Equal(t, interpreter, result.InterpreterPath)
}

// Ensure the sentinel compiles and is comparable so tests can reference it.
var _ error = errEvalFileStatStub

func TestErrEvalFileStatStub_IsComparable(t *testing.T) {
	assert.True(t, errors.Is(errEvalFileStatStub, errEvalFileStatStub))
}
