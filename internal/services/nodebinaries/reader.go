// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package nodebinaries

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Reader provides validated, catalogued node artifacts from an immutable root.
type Reader struct {
	root     string
	manifest *Manifest
}

// OpenReader validates an existing artifact root. A missing root represents a
// source-only development environment and is kept unavailable rather than fatal.
func OpenReader(root string) (*Reader, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return &Reader{root: root}, nil
		}
		return nil, fmt.Errorf("%w: stat root: %w", constants.ErrNodeBinaryManifest, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: root is not a directory", constants.ErrNodeBinaryManifest)
	}
	manifest, err := LoadManifest(root)
	if err != nil {
		return nil, err
	}
	for _, artifact := range manifest.Targets {
		path := filepath.Join(root, artifact.Filename)
		size, digest, err := artifactDigest(path)
		if err != nil || size != artifact.Size || digest != artifact.SHA256 {
			return nil, fmt.Errorf("%w: verify %q: size or digest mismatch", constants.ErrNodeBinaryManifest, artifact.Filename)
		}
		checksum, err := os.ReadFile(filepath.Join(root, artifact.Checksum))
		if err != nil {
			return nil, fmt.Errorf("%w: read checksum %q: %w", constants.ErrNodeBinaryManifest, artifact.Checksum, err)
		}
		fields := strings.Fields(string(checksum))
		if len(fields) < 2 || fields[0] != artifact.SHA256 || fields[1] != artifact.Filename {
			return nil, fmt.Errorf("%w: verify checksum %q", constants.ErrNodeBinaryManifest, artifact.Checksum)
		}
	}
	return &Reader{root: root, manifest: &manifest}, nil
}

// Artifact opens a manifest-listed executable or checksum sidecar.
func (r *Reader) Artifact(name string) (io.ReadSeeker, os.FileInfo, error) {
	if err := validateFilename(name); err != nil {
		return nil, nil, err
	}
	if r.manifest != nil {
		found := false
		for _, artifact := range r.manifest.Targets {
			if artifact.Filename == name || artifact.Checksum == name {
				found = true
				break
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("%w: %q", constants.ErrNotFound, name)
		}
	}
	path := filepath.Join(r.root, name)
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: open %q: %w", constants.ErrNotFound, name, err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("%w: stat %q: %w", constants.ErrNodeBinaryArtifact, name, err)
	}
	if info.IsDir() {
		file.Close()
		return nil, nil, fmt.Errorf("%w: %q is a directory", constants.ErrNodeBinaryArtifact, name)
	}
	return file, info, nil
}

// HasManifest reports whether the reader was initialized from a validated image manifest.
func (r *Reader) HasManifest() bool { return r.manifest != nil }
