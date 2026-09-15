// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// evalCommandRunner abstracts subprocess execution so Tier 1 tests do not
// spawn processes. The real implementation uses exec.CommandContext
// directly without a shell. The facade never invokes a shell for engine
// execution.
type evalCommandRunner interface {
	Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error
}

// realEvalCommandRunner is the production evalCommandRunner backed by
// exec.CommandContext. It connects stdout and stderr directly to the
// supplied writers and respects context cancellation.
type realEvalCommandRunner struct{}

func (realEvalCommandRunner) Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// evalFileReader abstracts file reads so Tier 1 tests do not touch the
// filesystem. The real implementation calls os.ReadFile.
type evalFileReader interface {
	ReadFile(path string) ([]byte, error)
}

// realEvalFileReader is the production evalFileReader backed by os.ReadFile.
type realEvalFileReader struct{}

func (realEvalFileReader) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// realEvalExecutableLookup is the production evalExecutableLookup backed
// by exec.LookPath.
type realEvalExecutableLookup struct{}

func (realEvalExecutableLookup) LookPath(file string) (string, error) { return exec.LookPath(file) }

// evalRoots is the resolved and validated repository root and eval
// project path. Both are absolute. The eval project is validated through
// the owned markers; the repository root is validated through the root
// markers.
type evalRoots struct {
	RepositoryRoot string
	EvalProject    string
}

// resolveEvalRoots validates the repository root markers and returns the
// resolved eval project path. It fails closed with a typed error if the
// root markers are missing. The eval project markers are validated
// separately by the environment resolver.
func resolveEvalRoots(ctx context.Context, projectRoot string, stat evalFileStat) (evalRoots, error) {
	if projectRoot == "" {
		return evalRoots{}, fmt.Errorf("%w: project root is empty", constants.ErrEvalProjectNotFound)
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return evalRoots{}, fmt.Errorf("%w: resolve project root: %w", constants.ErrEvalProjectNotFound, err)
	}
	for _, marker := range []string{constants.EvalRootVersion, constants.EvalRootMakefile} {
		markerPath := filepath.Join(absRoot, marker)
		if _, err := stat.Stat(markerPath); err != nil {
			return evalRoots{}, fmt.Errorf("%w: missing root marker %s", constants.ErrEvalProjectNotFound, marker)
		}
	}
	evalProject := filepath.Join(absRoot, constants.EvalProjectDir)
	return evalRoots{RepositoryRoot: absRoot, EvalProject: evalProject}, nil
}

// evalProjectRootFromConfig resolves the project root from the config
// loader or an explicit override. The override takes precedence when
// supplied and non-empty.
func evalProjectRootFromConfig(configLoader func(string) (*config.Config, error), projectRootOverride string) (string, error) {
	cfg, err := configLoader(projectRootOverride)
	if err != nil {
		return "", fmt.Errorf("eval: load config: %w", err)
	}
	return cfg.ProjectRoot, nil
}

// evalCmd returns the parent `eval` Cobra command. It registers the
// setup and doctor subcommands implemented in U3. Later phases add
// diagnostic, campaign, controller, lease, bundle, verify, publish,
// qualification, and bench-synthetic subcommands.
func evalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Evaluation engine orchestration (setup, doctor, diagnostics, campaigns)",
		Long: `eval owns the unified evaluation operator surface.

The Go facade resolves the eval Python project, validates the locked
environment, and dispatches typed engine requests to the internal Python
engine. Operators never invoke uv, activate a virtualenv, change into
ensemble/evals, or call g8e-evals directly.

Run 'g8e eval setup' to create or synchronize the eval environment.
Run 'g8e eval doctor' for read-only environment diagnostics.`,
	}
	cmd.PersistentFlags().String("project-root", "", "Override the repository root (defaults to cwd)")
	cmd.AddCommand(
		evalSetupCmd(),
		evalDoctorCmd(),
		evalDiagnosticCmd(),
		evalCampaignCmd(),
		evalControllerCmd(),
		evalLeaseCmd(),
	)
	return cmd
}
