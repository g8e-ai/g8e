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
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
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

// Definitions returns the canonical DemoScenarioDefinition bodies for the
// given demo ID, loaded from the canonical catalog and marshaled as canonical
// compact JSON. The definitions are sorted by scenario ID for deterministic
// output. An empty result is returned when the demo ID has no matching
// definitions in the canonical catalog.
func (s *DemoDirectoryProvenanceSource) Definitions(ctx context.Context, demoID string) ([]DemoDefinitionArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.projectRoot == "" || !validPathElement(demoID) || demoScope(demoID) == "" {
		return nil, fmt.Errorf("%w: invalid demo provenance root or demo ID", constants.ErrInvalidEvidenceGraph)
	}
	assertions, frameworks, _, err := catalog.LoadCanonicalCatalogs()
	if err != nil {
		return nil, fmt.Errorf("load canonical catalogs for definitions: %w", err)
	}
	scenarioCatalog, err := catalog.LoadDemoScenarioCatalog(assertions, frameworks)
	if err != nil {
		return nil, fmt.Errorf("load demo scenario catalog: %w", err)
	}
	prefix := demoID + "-"
	matching := make([]*compliancev1.DemoScenarioDefinition, 0)
	for _, definition := range scenarioCatalog.GetDefinitions() {
		if strings.HasPrefix(definition.GetScenarioId(), prefix) {
			matching = append(matching, definition)
		}
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].GetScenarioId() < matching[j].GetScenarioId() })
	definitions := make([]DemoDefinitionArtifact, 0, len(matching))
	for _, definition := range matching {
		body, err := compliancev1.MarshalCanonical(definition)
		if err != nil {
			return nil, fmt.Errorf("marshal demo scenario definition %s: %w", definition.GetScenarioId(), err)
		}
		definitions = append(definitions, DemoDefinitionArtifact{Body: body})
	}
	return definitions, nil
}
