// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package nodebinaries

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Manifest is the image and host mirror contract for node binaries.
type Manifest struct {
	SchemaVersion  int        `json:"schema_version"`
	Version        string     `json:"version"`
	BuildID        string     `json:"build_id"`
	BuildTime      string     `json:"build_time"`
	SourceRevision string     `json:"source_revision"`
	SourceTreeHash string     `json:"source_tree_hash"`
	Targets        []Artifact `json:"targets"`
}

// Artifact contains integrity metadata for one executable.
type Artifact struct {
	Target
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != 1 || strings.TrimSpace(m.Version) == "" || strings.TrimSpace(m.BuildID) == "" || strings.TrimSpace(m.BuildTime) == "" || strings.TrimSpace(m.SourceTreeHash) == "" {
		return fmt.Errorf("%w: malformed provenance", constants.ErrNodeBinaryManifest)
	}
	if _, err := time.Parse(time.RFC3339, m.BuildTime); err != nil {
		return fmt.Errorf("%w: build time: %w", constants.ErrNodeBinaryManifest, err)
	}
	expected := sortedTargets(targets)
	actual := sortedArtifacts(m.Targets)
	if len(actual) != len(expected) {
		return fmt.Errorf("%w: expected %d targets, got %d", constants.ErrNodeBinaryManifest, len(expected), len(actual))
	}
	for i, target := range expected {
		artifact := actual[i]
		if artifact.Filename != target.Filename || artifact.Checksum != target.Checksum || artifact.OS != target.OS || artifact.Arch != target.Arch || artifact.Mode != target.Mode || artifact.Size <= 0 || !isSHA256(artifact.SHA256) {
			return fmt.Errorf("%w: target %q does not match catalog", constants.ErrNodeBinaryManifest, artifact.Filename)
		}
	}
	return nil
}

func sortedArtifacts(entries []Artifact) []Artifact {
	result := append([]Artifact(nil), entries...)
	for i := range result {
		for j := i + 1; j < len(result); j++ {
			if result[j].Filename < result[i].Filename {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// LoadManifest reads and validates a manifest from root.
func LoadManifest(root string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, constants.NodeBinariesManifestFilename))
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: read manifest: %w", constants.ErrNodeBinaryManifest, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: decode manifest: %w", constants.ErrNodeBinaryManifest, err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func artifactDigest(path string) (int64, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}
