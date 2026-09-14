// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// evalProjectRootSpec is the input to the eval environment resolver. It
// carries the resolved repository root and the eval project directory
// within it. The resolver validates the project markers and selects the
// project-local interpreter; it never consults an unrelated active
// virtualenv or a g8e-evals executable on PATH.
type evalProjectRootSpec struct {
	RepositoryRoot string
	EvalProject    string
}

// evalEnvironment is the resolved eval engine environment. The
// InterpreterPath is the only engine entry point the facade uses.
// FellBackToPath and FellBackToRootVenv are always false in the real
// resolver; they exist so the regression test can assert the negative.
type evalEnvironment struct {
	InterpreterPath    string
	FellBackToPath     bool
	FellBackToRootVenv bool
}

// evalFileStat abstracts file metadata checks so Tier 1 tests do not
// touch the filesystem. The real implementation calls os.Stat.
type evalFileStat interface {
	Stat(path string) (os.FileInfo, error)
}

// evalExecutableLookup abstracts executable resolution so Tier 1 tests
// do not consult PATH. The real implementation calls exec.LookPath.
type evalExecutableLookup interface {
	LookPath(file string) (string, error)
}

// evalEnvironmentResolver resolves the eval Python environment for the
// facade. It owns engine, project, environment, and runtime identity so
// the user-facing command does not depend on cwd, an unrelated active
// virtualenv, or a stale g8e-evals executable on PATH.
type evalEnvironmentResolver struct {
	stat evalFileStat
}

// newEvalEnvironmentResolver constructs the real resolver backed by the
// filesystem. Tests construct their own resolver with stub stat/lookup
// implementations to avoid touching the filesystem.
func newEvalEnvironmentResolver() *evalEnvironmentResolver {
	return &evalEnvironmentResolver{stat: realEvalFileStat{}}
}

// realEvalFileStat is the filesystem-backed evalFileStat implementation.
type realEvalFileStat struct{}

func (realEvalFileStat) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }

// Resolve validates the eval project markers and returns the
// project-local interpreter path. It fails closed with a typed error if
// the project markers are missing, the project is not a directory, or the
// interpreter is absent. It never reads VIRTUAL_ENV, never calls
// exec.LookPath for g8e-evals, and never falls back to a root .venv.
func (r *evalEnvironmentResolver) Resolve(ctx context.Context, spec evalProjectRootSpec) (evalEnvironment, error) {
	if spec.RepositoryRoot == "" {
		return evalEnvironment{}, fmt.Errorf("%w: repository root is empty", constants.ErrEvalProjectNotFound)
	}
	if spec.EvalProject == "" {
		return evalEnvironment{}, fmt.Errorf("%w: eval project is empty", constants.ErrEvalProjectNotFound)
	}

	pyproject := filepath.Join(spec.EvalProject, constants.EvalProjectPyproject)
	lockfile := filepath.Join(spec.EvalProject, constants.EvalProjectLockfile)
	if _, err := r.stat.Stat(pyproject); err != nil {
		return evalEnvironment{}, fmt.Errorf("%w: missing %s", constants.ErrEvalProjectNotFound, constants.EvalProjectPyproject)
	}
	if _, err := r.stat.Stat(lockfile); err != nil {
		return evalEnvironment{}, fmt.Errorf("%w: missing %s", constants.ErrEvalProjectNotFound, constants.EvalProjectLockfile)
	}

	interpreterPath := projectInterpreterPath(spec.EvalProject)
	if _, err := r.stat.Stat(interpreterPath); err != nil {
		return evalEnvironment{}, fmt.Errorf("%w: run './g8e eval setup'", constants.ErrEvalEngineNotSetUp)
	}

	return evalEnvironment{InterpreterPath: interpreterPath}, nil
}

// projectInterpreterPath returns the absolute path to the project-local
// Python interpreter for the current platform.
func projectInterpreterPath(evalProject string) string {
	bin := constants.EvalVenvPythonUnix
	if runtime.GOOS == "windows" {
		bin = constants.EvalVenvPythonWindows
	}
	return filepath.Join(evalProject, constants.EvalProjectVenvDir, bin)
}

// errEvalFileStatStub is a sentinel for stub stat implementations in
// tests. It is not used by production code.
var errEvalFileStatStub = errors.New("eval: stub stat failure")
