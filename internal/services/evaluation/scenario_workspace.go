// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// scenarioWorkspaceDigestHexLength is how many hex characters of the
// run/attempt digest name one workspace directory.
const scenarioWorkspaceDigestHexLength = 16

// ScenarioWorkspace is the attempt-scoped fixture workspace on the bound Data
// Operator. Paths are POSIX paths on the remote operator host. The root sits
// under the operator's reported working directory, so the production `./`
// instruction and the prompt's "Working Directory" line resolve to its parent,
// and it is unique per run and attempt, so concurrent or repeated attempts
// never see each other's fixtures.
type ScenarioWorkspace struct {
	Root                     string
	OperatorWorkingDirectory string
}

// NewScenarioWorkspace derives the attempt-scoped workspace for one attempt.
func NewScenarioWorkspace(operatorWorkingDirectory, runID, attemptID string) (ScenarioWorkspace, error) {
	if !path.IsAbs(operatorWorkingDirectory) || strings.Contains(operatorWorkingDirectory, "\x00") {
		return ScenarioWorkspace{}, fmt.Errorf("%w: operator working directory %q is not an absolute path", constants.ErrEvaluationWorkspaceUnavailable, operatorWorkingDirectory)
	}
	if runID == "" || attemptID == "" {
		return ScenarioWorkspace{}, fmt.Errorf("evaluation: scenario workspace: %w", constants.ErrMissingRequiredField)
	}
	sum := sha256.Sum256([]byte(runID + "\x00" + attemptID))
	workingDirectory := path.Clean(operatorWorkingDirectory)
	return ScenarioWorkspace{
		Root:                     path.Join(workingDirectory, constants.EvaluationWorkspaceDirname, constants.EvaluationWorkspacePrefix+hex.EncodeToString(sum[:])[:scenarioWorkspaceDigestHexLength]),
		OperatorWorkingDirectory: workingDirectory,
	}, nil
}

// Render replaces every ScenarioWorkspaceToken in text with the workspace root.
func (w ScenarioWorkspace) Render(text string) string {
	return strings.ReplaceAll(text, ScenarioWorkspaceToken, w.Root)
}

// FilePath returns the absolute path of one workspace-relative fixture path.
func (w ScenarioWorkspace) FilePath(relPath string) (string, error) {
	if err := validateWorkspaceRelPath(relPath); err != nil {
		return "", err
	}
	return path.Join(w.Root, relPath), nil
}

// Resolve turns a path argument a model supplied into a cleaned absolute
// path: absolute paths are cleaned, relative ones resolve against the
// operator working directory the way the operator itself resolves them.
func (w ScenarioWorkspace) Resolve(value string) string {
	if path.IsAbs(value) {
		return path.Clean(value)
	}
	return path.Clean(path.Join(w.OperatorWorkingDirectory, value))
}

// Contains reports whether value resolves to the root or a path under it.
func (w ScenarioWorkspace) Contains(value string) bool {
	return pathWithin(w.Resolve(value), w.Root)
}

// Validate checks that the workspace is the one this run and attempt derive.
func (w ScenarioWorkspace) Validate(runID, attemptID string) error {
	expected, err := NewScenarioWorkspace(w.OperatorWorkingDirectory, runID, attemptID)
	if err != nil {
		return err
	}
	if expected.Root != w.Root {
		return fmt.Errorf("%w: workspace root %q is not the attempt-scoped root %q", constants.ErrEvaluationWorkspaceUnavailable, w.Root, expected.Root)
	}
	return nil
}

// materializeScenarioWorkspace writes every fixture file, decoys included, at
// its path under the attempt-scoped workspace on the bound Data Operator and
// returns on the first failure, so a scored request is never sent against an
// empty or partial workspace. A scenario with files and no writer fails closed
// with ErrEvaluationWorkspaceUnavailable.
func materializeScenarioWorkspace(ctx context.Context, writer WorkspaceFileWriter, target Target, runID, scenarioID, attemptID string, ws ScenarioWorkspace, files []ScenarioWorkspaceFile) error {
	if len(files) == 0 {
		return nil
	}
	if writer == nil {
		return fmt.Errorf("%w: scenario %s requires a workspace file writer", constants.ErrEvaluationWorkspaceUnavailable, scenarioID)
	}
	for _, file := range files {
		absPath, err := ws.FilePath(file.RelPath)
		if err != nil {
			return fmt.Errorf("%w: scenario %s: %v", constants.ErrEvaluationWorkspaceUnavailable, scenarioID, err)
		}
		if err := writer.WriteWorkspaceFile(ctx, target, runID, scenarioID, attemptID, absPath, ws.Render(file.Content)); err != nil {
			return fmt.Errorf("write workspace file %s: %w", absPath, err)
		}
	}
	return nil
}

func pathWithin(candidate, root string) bool {
	return candidate == root || strings.HasPrefix(candidate, strings.TrimSuffix(root, "/")+"/")
}

func validateWorkspaceRelPath(relPath string) error {
	if relPath == "" || path.IsAbs(relPath) || path.Clean(relPath) != relPath || relPath == "." || strings.HasPrefix(relPath, "../") || strings.Contains(relPath, "\x00") {
		return fmt.Errorf("evaluation: workspace file path %q must be a clean relative path inside the workspace", relPath)
	}
	return nil
}
