// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// DemoDirectoryProvenanceSource reads the immutable source files covered by a demo manifest.
type DemoDirectoryProvenanceSource struct {
	projectRoot string
}

// NewDemoDirectoryProvenanceSource creates a provenance source rooted at the repository project directory.
func NewDemoDirectoryProvenanceSource(projectRoot string) *DemoDirectoryProvenanceSource {
	return &DemoDirectoryProvenanceSource{projectRoot: projectRoot}
}

// Artifacts returns the compose file and every regular file in the canonical provenance subdirectories.
func (s *DemoDirectoryProvenanceSource) Artifacts(ctx context.Context, demoID string) ([]ProvenanceArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.projectRoot == "" || !validPathElement(demoID) || demoScope(demoID) == "" {
		return nil, fmt.Errorf("%w: invalid demo provenance root or demo ID", constants.ErrInvalidEvidenceGraph)
	}
	demoDir := filepath.Join(s.projectRoot, constants.DemosDirname, demoID)
	artifacts := make([]ProvenanceArtifact, 0)
	composeBody, err := os.ReadFile(filepath.Join(demoDir, constants.DemosComposeFile))
	if err != nil {
		return nil, fmt.Errorf("read demo compose provenance: %w", err)
	}
	artifacts = append(artifacts, ProvenanceArtifact{Name: constants.DemosComposeFile, Body: composeBody})
	for _, directory := range []string{constants.DemosDoctrineDir, constants.DemosTargetDataDir, constants.DemoConfigDirname} {
		entries, err := os.ReadDir(filepath.Join(demoDir, directory))
		if err != nil {
			return nil, fmt.Errorf("read demo provenance directory %s: %w", directory, err)
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := filepath.Join(directory, entry.Name())
			body, err := os.ReadFile(filepath.Join(demoDir, name))
			if err != nil {
				return nil, fmt.Errorf("read demo provenance artifact %s: %w", name, err)
			}
			artifacts = append(artifacts, ProvenanceArtifact{Name: name, Body: body})
		}
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Name < artifacts[j].Name })
	return artifacts, nil
}
