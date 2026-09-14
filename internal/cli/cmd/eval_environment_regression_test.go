// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvalEnvironmentSelection_IgnoresUnrelatedRootVenvAndStalePathExecutable
// is the U0 regression test for the unified evaluation tooling defect
// documented in
// .local.dev/docs/plans/in-progress/2026-09-14-opendevops-ai-production-online-good-enough.md.
//
// The reproduced defect: invoking `uv run --locked g8e-evals campaign check
// campaign.json` from the repository root selects the unrelated root `.venv`,
// reports that `--locked` has no effect outside a project, imports a stale
// `g8e-evals` entry point, and fails because numpy is absent.
//
// The fix is not a warning telling users to change directory. The root cause
// is that the user-facing command does not own its engine, project,
// environment, and runtime identity. The `./g8e eval` facade resolves the
// eval project interpreter directly and never consults an unrelated active
// virtualenv or a `g8e-evals` executable on `PATH`.
//
// This test constructs a contaminated repository root containing an
// unrelated `.venv` and a stale `g8e-evals` executable on a poisoned `PATH`,
// then asserts the facade's environment resolver selects only the eval
// project interpreter. It fails to compile until U2 introduces
// evalEnvironmentResolver; that failure is the implementation gate.
func TestEvalEnvironmentSelection_IgnoresUnrelatedRootVenvAndStalePathExecutable(t *testing.T) {
	tmp := t.TempDir()

	// Contaminate the repository root with an unrelated `.venv` that would
	// be selected by `uv run` project discovery. It contains a Python
	// interpreter shim and a stale `g8e-evals` entry point that imports a
	// missing module, reproducing the numpy-absent failure mode.
	rootVenv := filepath.Join(tmp, ".venv", "bin")
	require.NoError(t, os.MkdirAll(rootVenv, 0o755))

	stalePython := filepath.Join(rootVenv, "python")
	require.NoError(t, os.WriteFile(stalePython, []byte("#!/bin/sh\nexit 1\n"), 0o755))

	staleEntry := filepath.Join(rootVenv, "g8e-evals")
	require.NoError(t, os.WriteFile(staleEntry, []byte("#!/bin/sh\nexit 1\n"), 0o755))

	// Place a second stale `g8e-evals` on a poisoned PATH directory so the
	// test proves PATH lookup does not influence engine selection.
	poisonPath := filepath.Join(tmp, "poison-path")
	require.NoError(t, os.MkdirAll(poisonPath, 0o755))
	poisonEntry := filepath.Join(poisonPath, "g8e-evals")
	require.NoError(t, os.WriteFile(poisonEntry, []byte("#!/bin/sh\nexit 1\n"), 0o755))

	// Build the eval project tree with a real project-local interpreter that
	// the facade is expected to select. The interpreter is a marker shim
	// whose path the resolver must return.
	evalProject := filepath.Join(tmp, "ensemble", "evals")
	evalVenvBin := filepath.Join(evalProject, ".venv", "bin")
	require.NoError(t, os.MkdirAll(evalVenvBin, 0o755))
	projectInterpreter := filepath.Join(evalVenvBin, "python")
	require.NoError(t, os.WriteFile(projectInterpreter, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	// Mark the eval project so the resolver can validate it as the owned
	// engine project. The exact markers are centralized in U2; this test
	// only requires that the resolver accepts a project containing the
	// pyproject and lockfile sentinels.
	require.NoError(t, os.WriteFile(filepath.Join(evalProject, "pyproject.toml"), []byte("[project]\nname = \"g8e-evals\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(evalProject, "uv.lock"), []byte("# lockfile\n"), 0o644))

	t.Setenv("PATH", poisonPath+string(filepath.ListSeparator)+os.Getenv("PATH"))
	t.Setenv("VIRTUAL_ENV", rootVenv)

	resolver := newEvalEnvironmentResolver()
	result, err := resolver.Resolve(t.Context(), evalProjectRootSpec{RepositoryRoot: tmp, EvalProject: evalProject})
	require.NoError(t, err, "resolver must select the eval project interpreter without falling back to the root .venv or PATH")

	assert.Equal(t, projectInterpreter, result.InterpreterPath, "resolver must return the project-local interpreter, not the unrelated root .venv or PATH executable")
	assert.False(t, result.FellBackToPath, "resolver must never fall back to a g8e-evals executable on PATH")
	assert.False(t, result.FellBackToRootVenv, "resolver must never select the unrelated repository-root .venv")
}
