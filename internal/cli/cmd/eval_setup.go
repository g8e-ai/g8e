// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// evalSetupDeps carries the injected dependencies for the setup command.
// All fields are interfaces so Tier 1 tests can stub every side effect
// without touching the filesystem or spawning processes.
type evalSetupDeps struct {
	configLoader func(string) (*config.Config, error)
	stat         evalFileStat
	uvLookup     evalExecutableLookup
	runner       evalCommandRunner
	fileReader   evalFileReader
}

// evalSetupResult is the typed result emitted by `eval setup --json`.
type evalSetupResult struct {
	Status          string `json:"status"`
	EngineVersion   string `json:"engine_version"`
	PythonVersion   string `json:"python_version"`
	UVVersion       string `json:"uv_version"`
	LockfileDigest  string `json:"lockfile_digest"`
	InterpreterPath string `json:"interpreter_path"`
}

// evalSetupCmd returns the production `eval setup` command with real
// dependencies.
func evalSetupCmd() *cobra.Command {
	return evalSetupCmdWithDeps(evalSetupDeps{
		configLoader: config.Load,
		stat:         realEvalFileStat{},
		uvLookup:     realEvalExecutableLookup{},
		runner:       realEvalCommandRunner{},
		fileReader:   realEvalFileReader{},
	})
}

// evalSetupCmdWithDeps returns the `eval setup` command wired with the
// supplied dependencies for testability.
func evalSetupCmdWithDeps(deps evalSetupDeps) *cobra.Command {
	var withTestTools bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Create or synchronize the eval Python environment (local-mutating)",
		Long: `setup is the only normal command that creates or synchronizes the eval
Python environment. It runs uv sync with the exact lockfile, validates the
project-local interpreter, and invokes an internal engine self-check that
imports every required runtime module.

This command is local-mutating. It does not require --yes because it is the
explicit environment provisioning action. It never weakens lock
enforcement, edits pyproject.toml, edits uv.lock, installs into the root
.venv, or falls back to an unlocked install.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := commandContext(cmd)
			projectRootOverride, _ := cmd.Flags().GetString("project-root")
			result, err := runEvalSetup(ctx, deps, projectRootOverride, withTestTools, cmd.OutOrStdout(), cmd.OutOrStderr())
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalSetupHuman(cmd.OutOrStdout(), result)
			return nil
		},
	}

	cmd.Flags().BoolVar(&withTestTools, "with-test-tools", false, "Add the eval test extra (pytest, ruff, pyright)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

// runEvalSetup performs the ordered setup checks and mutations:
//  1. Resolve and validate the repository and eval project roots.
//  2. Locate the approved uv executable.
//  3. Read the required Python version from pyproject.toml.
//  4. Run uv sync --project <eval-project> --locked [--extra test].
//  5. Validate the resulting project-local interpreter path.
//  6. Invoke the internal engine self-check.
//  7. Record non-secret setup metadata.
func runEvalSetup(ctx context.Context, deps evalSetupDeps, projectRootOverride string, withTestTools bool, stdout, stderr io.Writer) (evalSetupResult, error) {
	projectRoot, err := evalProjectRootFromConfig(deps.configLoader, projectRootOverride)
	if err != nil {
		return evalSetupResult{}, err
	}
	roots, err := resolveEvalRoots(ctx, projectRoot, deps.stat)
	if err != nil {
		return evalSetupResult{}, err
	}
	resolver := &evalEnvironmentResolver{stat: deps.stat}
	_, resolveErr := resolver.Resolve(ctx, evalProjectRootSpec(roots))
	if resolveErr != nil && !errors.Is(resolveErr, constants.ErrEvalEngineNotSetUp) {
		return evalSetupResult{}, resolveErr
	}

	uvPath, err := deps.uvLookup.LookPath(constants.UVExecutable)
	if err != nil {
		return evalSetupResult{}, fmt.Errorf("%w: %w", constants.ErrEvalUVUnavailable, err)
	}

	pythonVersion, err := readEvalPythonRequirement(roots.EvalProject, deps.fileReader)
	if err != nil {
		return evalSetupResult{}, err
	}

	syncArgs := []string{"sync", "--project", roots.EvalProject, "--locked"}
	if withTestTools {
		syncArgs = append(syncArgs, "--extra", constants.EvalTestExtra)
	}
	fmt.Fprintf(stdout, "Running %s %s\n", constants.UVExecutable, strings.Join(syncArgs, " "))
	if err := deps.runner.Run(ctx, uvPath, syncArgs, stdout, stderr); err != nil {
		return evalSetupResult{}, fmt.Errorf("%w: %w", constants.ErrEvalUVSyncFailed, err)
	}

	interpreterPath := projectInterpreterPath(roots.EvalProject)
	if _, err := deps.stat.Stat(interpreterPath); err != nil {
		return evalSetupResult{}, fmt.Errorf("%w: interpreter missing after sync: %s", constants.ErrEvalUVSyncFailed, interpreterPath)
	}

	engineVersion, err := runEvalSelfCheck(ctx, deps.runner, interpreterPath, stdout, stderr)
	if err != nil {
		return evalSetupResult{}, err
	}

	uvVersion, _ := getEvalUVVersion(ctx, deps.runner, uvPath)

	lockfileDigest, _ := hashEvalLockfile(roots.EvalProject, deps.fileReader)

	return evalSetupResult{
		Status:          "succeeded",
		EngineVersion:   engineVersion,
		PythonVersion:   pythonVersion,
		UVVersion:       uvVersion,
		LockfileDigest:  lockfileDigest,
		InterpreterPath: interpreterPath,
	}, nil
}

// runEvalSelfCheck invokes the project interpreter with the self-check
// import expression. It returns the engine version printed by the
// self-check. A failed import produces a typed self-check error, never a
// raw Python traceback surfaced to the operator.
func runEvalSelfCheck(ctx context.Context, runner evalCommandRunner, interpreterPath string, stdout, stderr io.Writer) (string, error) {
	var checkOut strings.Builder
	checkErr := runner.Run(ctx, interpreterPath, []string{"-c", constants.EvalSelfCheckModules}, &checkOut, stderr)
	if checkErr != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrEvalEngineSelfCheckFailed, checkErr)
	}
	return strings.TrimSpace(checkOut.String()), nil
}

// getEvalUVVersion runs `uv --version` and returns the trimmed output.
// A failure is non-fatal; the version field is left empty.
func getEvalUVVersion(ctx context.Context, runner evalCommandRunner, uvPath string) (string, error) {
	var out strings.Builder
	if err := runner.Run(ctx, uvPath, []string{"--version"}, &out, io.Discard); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// hashEvalLockfile returns the SHA-256 hex digest of the eval lockfile.
// A failure is non-fatal; the digest field is left empty.
func hashEvalLockfile(evalProject string, reader evalFileReader) (string, error) {
	lockPath := filepath.Join(evalProject, constants.EvalProjectLockfile)
	data, err := reader.ReadFile(lockPath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// readEvalPythonRequirement reads the requires-python field from the
// eval project's pyproject.toml. It validates that the field exists and
// requires Python 3.12 or later. The parsing is line-based because the
// Go module does not depend on a TOML parser; only the requires-python
// key is needed.
func readEvalPythonRequirement(evalProject string, reader evalFileReader) (string, error) {
	pyprojectPath := filepath.Join(evalProject, constants.EvalProjectPyproject)
	data, err := reader.ReadFile(pyprojectPath)
	if err != nil {
		return "", fmt.Errorf("%w: read pyproject.toml: %w", constants.ErrEvalConfigInvalid, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "requires-python") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		if !strings.Contains(value, "3.12") {
			return "", fmt.Errorf("%w: requires-python must demand 3.12 or later, got %q", constants.ErrEvalConfigInvalid, value)
		}
		return value, nil
	}
	return "", fmt.Errorf("%w: requires-python not found in pyproject.toml", constants.ErrEvalConfigInvalid)
}

// printEvalSetupHuman prints a concise human-readable setup summary.
func printEvalSetupHuman(w io.Writer, result evalSetupResult) {
	fmt.Fprintf(w, "Eval environment ready\n")
	fmt.Fprintf(w, "  Engine:   %s\n", result.EngineVersion)
	fmt.Fprintf(w, "  Python:   %s\n", result.PythonVersion)
	if result.UVVersion != "" {
		fmt.Fprintf(w, "  uv:       %s\n", result.UVVersion)
	}
	if result.LockfileDigest != "" {
		fmt.Fprintf(w, "  Lockfile: %s\n", result.LockfileDigest)
	}
	fmt.Fprintf(w, "  Interpreter: %s\n", result.InterpreterPath)
}

// emitEvalJSON writes the typed result as a single canonical JSON object
// on stdout.
func emitEvalJSON(cmd *cobra.Command, result any) error {
	out, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("eval: marshal json: %w", err)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(out))
	return err
}
