// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

// formationCatalog reads and writes the checked-in formation catalog overlay:
// the project's create/update/remove deltas layered on top of the in-binary
// default execution topologies.
type formationCatalog struct {
	projectRoot string
}

func newFormationCatalog(projectRoot string) formationCatalog {
	return formationCatalog{projectRoot: projectRoot}
}

func resolveProjectRoot(cmd *cobra.Command) (string, error) {
	projectRoot, err := cmd.Flags().GetString("project-root")
	if err != nil {
		return "", fmt.Errorf("evaluation: read project root: %w", err)
	}
	if projectRoot != "" {
		return projectRoot, nil
	}
	projectRoot, err = os.Getwd()
	if err != nil {
		return "", fmt.Errorf("evaluation: resolve project root: %w", err)
	}
	return projectRoot, nil
}

func (f formationCatalog) overlayPath() string {
	return filepath.Join(f.projectRoot, evaluation.DefaultFormationCatalogOverlayRelPath)
}

// overlay reads the checked-in overlay file, returning an empty overlay when
// the project has not customized the catalog yet.
func (f formationCatalog) overlay() (*evaluation.FormationCatalogOverlay, error) {
	overlay, err := evaluation.LoadFormationCatalogOverlayFile(f.overlayPath())
	if err == nil {
		return overlay, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return &evaluation.FormationCatalogOverlay{}, nil
	}
	return nil, err
}

// topologies returns the checked-in default formations merged with the
// project's overlay.
func (f formationCatalog) topologies() (*evaluation.ExecutionTopologies, error) {
	overlay, err := f.overlay()
	if err != nil {
		return nil, err
	}
	return evaluation.ApplyFormationCatalogOverlay(overlay)
}

// save atomically replaces the checked-in overlay file.
func (f formationCatalog) save(overlay *evaluation.FormationCatalogOverlay) error {
	payload, err := evaluation.MarshalFormationCatalogOverlay(*overlay)
	if err != nil {
		return err
	}
	return writeCatalogInventory(f.overlayPath(), payload)
}
