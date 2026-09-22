// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
)

type TargetState struct {
	Present    bool
	Content    []byte
	ObservedAt time.Time
}

type observerCommandResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int
	err      error
}

type observerCommandRunner func(ctx context.Context, dir, name string, args ...string) observerCommandResult

type composeTargetObserver struct {
	projectDir string
	run        observerCommandRunner
	now        func() time.Time
}

func NewComposeTargetObserver(projectDir string) TargetObserver {
	return newComposeTargetObserver(projectDir, runObserverCommand, time.Now)
}

func newComposeTargetObserver(projectDir string, run observerCommandRunner, now func() time.Time) *composeTargetObserver {
	return &composeTargetObserver{projectDir: projectDir, run: run, now: now}
}

func (o *composeTargetObserver) Observe(ctx context.Context, targetResource string) (*TargetState, error) {
	cleanTarget := filepath.Clean(targetResource)
	filename := filepath.Base(cleanTarget)
	if o == nil || o.projectDir == "" || o.run == nil || o.now == nil || !filepath.IsAbs(cleanTarget) || filepath.Dir(cleanTarget) != constants.EvaluationTargetContainerDir || !complianceevidence.ValidPathElement(filename) {
		return nil, fmt.Errorf("%w: controlled target path and observer dependencies are required", constants.ErrEvaluationObservationUnavailable)
	}
	result := o.run(ctx, o.projectDir, constants.DockerExecutable,
		"compose", "--file", constants.DockerComposeFile, "--file", constants.DockerNativeEvaluationComposeFile,
		"run", "--rm", "--no-deps", "-e", constants.EvaluationObserverTargetEnv+"="+filename, constants.DockerNativeEvaluationTargetReader,
	)
	observedAt := o.now().UTC()
	if result.exitCode == constants.EvaluationObserverAbsentExitCode {
		return &TargetState{ObservedAt: observedAt}, nil
	}
	if result.err != nil || result.exitCode != 0 {
		return nil, fmt.Errorf("%w: compose observer exited with code %d: %s", constants.ErrEvaluationObservationUnavailable, result.exitCode, bytes.TrimSpace(result.stderr))
	}
	return &TargetState{Present: true, Content: append([]byte(nil), result.stdout...), ObservedAt: observedAt}, nil
}

func runObserverCommand(ctx context.Context, dir, name string, args ...string) observerCommandResult {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := observerCommandResult{stdout: stdout.Bytes(), stderr: stderr.Bytes(), err: err}
	if err == nil {
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.exitCode = exitErr.ExitCode()
		return result
	}
	result.exitCode = -1
	return result
}
