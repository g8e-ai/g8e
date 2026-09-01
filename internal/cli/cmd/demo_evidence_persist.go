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
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// persistDemoScenarioResult writes a typed DemoScenarioResult as canonical
// protojson to the per-run demo evidence tree under the runtime compliance
// directory. The path is data/compliance/demo-evidence/<run-id>/scenario-results.jsonl.
// Each result is appended as a single JSON line so multiple scenario results
// from the same run accumulate in one file without overwriting prior runs.
//
// Only evidence-grade results (those carrying a RunId and ScenarioRef) are
// persisted. Minimal display-only results from scenarios without typed
// evidence fields are skipped silently — they remain in the terminal summary
// table but do not produce canonical evidence until their full typed slice
// is implemented.
func persistDemoScenarioResult(ctx context.Context, fileSvc fs.RuntimeFileService, result *compliancev1.DemoScenarioResult) error {
	if result == nil || result.RunId == "" || result.ScenarioRef == nil {
		return nil
	}

	runDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, result.RunId)
	if err := fileSvc.MkdirAll(ctx, runDir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create demo evidence run dir: %w", constants.ErrDemoEvidencePersistFailed, err)
	}

	encoded, err := compliancev1.MarshalCanonical(result)
	if err != nil {
		return fmt.Errorf("%w: marshal demo scenario result: %w", constants.ErrDemoEvidencePersistFailed, err)
	}

	relPath := filepath.Join(runDir, constants.DemoRunResultsFilename)
	existing, readErr := fileSvc.ReadFile(ctx, relPath)
	if readErr != nil && !errors.Is(readErr, constants.ErrNotFound) {
		return fmt.Errorf("%w: read existing demo results: %w", constants.ErrDemoEvidencePersistFailed, readErr)
	}

	var data []byte
	if len(existing) > 0 {
		data = append(existing, '\n')
	}
	data = append(data, encoded...)

	if err := fileSvc.WriteFile(ctx, relPath, data, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write demo scenario result: %w", constants.ErrDemoEvidencePersistFailed, err)
	}

	return nil
}
