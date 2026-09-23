// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package nodebinaries

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const maxExportArtifactSize int64 = 512 << 20

// Publisher validates a Docker-exported node-binary archive before replacing
// the complete host mirror.
type Publisher struct {
	root string
}

// NewPublisher creates a publisher for a repository artifact directory.
func NewPublisher(root string) *Publisher { return &Publisher{root: root} }

// Publish consumes a tar stream and atomically publishes the complete catalog.
// The existing mirror remains untouched when archive validation or publication
// fails.
func (p *Publisher) Publish(reader io.Reader) (Manifest, error) {
	if strings.TrimSpace(p.root) == "" {
		return Manifest{}, fmt.Errorf("%w: output root is empty", constants.ErrNodeBinaryExport)
	}
	parent := filepath.Dir(p.root)
	staging, err := os.MkdirTemp(parent, constants.NodeBinariesStagingPrefix)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: create staging directory: %w", constants.ErrNodeBinaryExport, err)
	}
	defer os.RemoveAll(staging)

	if err := extractArchive(reader, staging); err != nil {
		return Manifest{}, err
	}
	manifest, err := OpenReader(staging)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: validate exported artifacts: %w", constants.ErrNodeBinaryExport, err)
	}
	if err := publishDirectory(staging, p.root); err != nil {
		return Manifest{}, err
	}
	return *manifest.manifest, nil
}

func extractArchive(reader io.Reader, root string) error {
	tarReader := tar.NewReader(reader)
	seen := make(map[string]struct{})
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: read tar stream: %w", constants.ErrNodeBinaryArchive, err)
		}
		name, err := archiveName(header.Name)
		if err != nil {
			return err
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("%w: duplicate entry %q", constants.ErrNodeBinaryArchive, name)
		}
		seen[name] = struct{}{}
		if header.Typeflag != tar.TypeReg {
			return fmt.Errorf("%w: entry %q is not a regular file", constants.ErrNodeBinaryArchive, name)
		}
		if header.Size < 0 || header.Size > maxExportArtifactSize {
			return fmt.Errorf("%w: entry %q exceeds size limit", constants.ErrNodeBinaryArchive, name)
		}
		if _, ok := targetByFilename(name); !ok && name != constants.NodeBinariesManifestFilename {
			return fmt.Errorf("%w: unsupported entry %q", constants.ErrNodeBinaryArchive, name)
		}
		path := filepath.Join(root, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, constants.PermFilePublic)
		if err != nil {
			return fmt.Errorf("%w: create %q: %w", constants.ErrNodeBinaryArchive, name, err)
		}
		_, copyErr := io.CopyN(file, tarReader, header.Size)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("%w: extract %q: %w", constants.ErrNodeBinaryArchive, name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("%w: close %q: %w", constants.ErrNodeBinaryArchive, name, closeErr)
		}
		if isExecutableName(name) {
			if err := os.Chmod(path, constants.PermFileExecutable); err != nil {
				return fmt.Errorf("%w: chmod %q: %w", constants.ErrNodeBinaryArchive, name, err)
			}
		}
	}
	return nil
}

func archiveName(name string) (string, error) {
	name = strings.TrimPrefix(name, "./")
	if name == "" || filepath.IsAbs(name) || filepath.Base(name) != name || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("%w: unsafe archive path %q", constants.ErrNodeBinaryArchive, name)
	}
	return name, nil
}

func isExecutableName(name string) bool {
	for _, target := range targets {
		if target.Filename == name {
			return true
		}
	}
	return false
}

func publishDirectory(staging, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), constants.PermDirPrivate); err != nil {
		return fmt.Errorf("%w: create output parent: %w", constants.ErrNodeBinaryExport, err)
	}
	backup := destination + constants.NodeBinariesPreviousSuffix
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("%w: remove old backup: %w", constants.ErrNodeBinaryExport, err)
	}
	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("%w: stage previous mirror: %w", constants.ErrNodeBinaryExport, err)
		}
	}
	if err := os.Rename(staging, destination); err != nil {
		_ = os.Rename(backup, destination)
		return fmt.Errorf("%w: publish mirror: %w", constants.ErrNodeBinaryExport, err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("%w: remove previous mirror: %w", constants.ErrNodeBinaryExport, err)
	}
	return nil
}

// ExportRecord identifies the immutable image that produced a host mirror.
type ExportRecord struct {
	SchemaVersion  int    `json:"schema_version"`
	ImageReference string `json:"image_reference"`
	ImageID        string `json:"image_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

// WriteExportRecord writes the host-only export record after a successful
// publication.
func WriteExportRecord(root string, record ExportRecord) error {
	if record.SchemaVersion == 0 {
		record.SchemaVersion = 1
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: marshal export record: %w", constants.ErrNodeBinaryExport, err)
	}
	data = append(data, '\n')
	path := filepath.Join(root, constants.NodeBinariesExportRecordFilename)
	if err := os.WriteFile(path, data, constants.PermFilePublic); err != nil {
		return fmt.Errorf("%w: write export record: %w", constants.ErrNodeBinaryExport, err)
	}
	return nil
}

// ManifestDigest returns the SHA-256 digest of the manifest file in root.
func ManifestDigest(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, constants.NodeBinariesManifestFilename))
	if err != nil {
		return "", fmt.Errorf("%w: read manifest for digest: %w", constants.ErrNodeBinaryExport, err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
