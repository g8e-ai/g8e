// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package buildinfo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ProvenanceManifestEntries lists repository paths hashed for build provenance.
// Keep in sync with PROVENANCE_SOURCE_PATHS in the root Makefile.
var ProvenanceManifestEntries = []string{
	"cmd",
	"internal",
	"protocol",
	"ensemble",
	"scripts",
	"test",
	"vendor",
	"Makefile",
	"VERSION",
	"go.mod",
	"go.sum",
	"buf.gen.yaml",
	"Dockerfile",
	"docker-compose.yml",
}

// ProvenanceManifestExcludes lists path-component patterns skipped while hashing
// the provenance manifest. Keep in sync with PROVENANCE_EXCLUDES in the Makefile.
var ProvenanceManifestExcludes = []string{
	".git",
	".env",
	".g8e*",
	".local.dev",
	".venv",
	"node_modules",
	"__pycache__",
	"*.egg-info",
	".pytest_cache",
	".ruff_cache",
	".mypy_cache",
	"bin",
	"build",
	"dist",
	"site",
	"coverage",
	"reports",
	"test-results",
	"auditor-out",
	"*.out",
	"*.test",
}

// RepositoryRoot walks upward from start until it finds a go.mod file.
func RepositoryRoot(start string) (string, error) {
	if start == "" {
		start = "."
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("buildinfo: repository root: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", constants.ErrPathNotFound
		}
		dir = parent
	}
}

// ComputeProvenanceManifestHash returns the canonical source-tree digest for
// build provenance using the same manifest and excludes as `make build`.
func ComputeProvenanceManifestHash(root string) (string, error) {
	repoRoot, err := RepositoryRoot(root)
	if err != nil {
		return "", err
	}
	entries := existingProvenanceEntries(repoRoot)
	if len(entries) == 0 {
		return "", fmt.Errorf("%w: no provenance manifest entries under %s", constants.ErrSourceTreeEntryNotFound, repoRoot)
	}
	return ComputeSourceManifestHash(repoRoot, entries, ProvenanceManifestExcludes)
}

func existingProvenanceEntries(root string) []string {
	out := make([]string, 0, len(ProvenanceManifestEntries))
	for _, entry := range ProvenanceManifestEntries {
		if _, err := os.Stat(filepath.Join(root, entry)); err == nil {
			out = append(out, entry)
		}
	}
	return out
}
