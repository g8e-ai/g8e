// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/services/g8ebinaries"
)

func main() {
	root := flag.String("root", "bin", "artifact directory")
	version := flag.String("version", "unknown", "g8e version")
	buildID := flag.String("build-id", "unknown", "build identifier")
	buildTime := flag.String("build-time", "", "RFC3339 build time")
	revision := flag.String("source-revision", "unknown", "source revision")
	treeHash := flag.String("source-tree-hash", "unknown", "source tree hash")
	flag.Parse()
	if err := generate(*root, *version, *buildID, *buildTime, *revision, *treeHash); err != nil {
		fmt.Fprintf(os.Stderr, "g8e-binaries: %v\n", err)
		os.Exit(1)
	}
}

func generate(root, version, buildID, buildTime, revision, treeHash string) error {
	if buildTime == "" {
		buildTime = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, buildTime); err != nil {
		return fmt.Errorf("invalid build time: %w", err)
	}
	if strings.TrimSpace(version) == "" || strings.TrimSpace(buildID) == "" || strings.TrimSpace(treeHash) == "" {
		return fmt.Errorf("provenance fields must not be empty")
	}
	artifacts := make([]g8ebinaries.Artifact, 0, len(g8ebinaries.Targets()))
	for _, target := range g8ebinaries.Targets() {
		path := filepath.Join(root, target.Filename)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", target.Filename, err)
		}
		digest := sha256.Sum256(data)
		checksum := filepath.Join(root, target.Checksum)
		if err := os.WriteFile(checksum, []byte(hex.EncodeToString(digest[:])+"  "+target.Filename+"\n"), 0644); err != nil {
			return fmt.Errorf("write %s: %w", target.Checksum, err)
		}
		artifacts = append(artifacts, g8ebinaries.Artifact{Target: target, Size: int64(len(data)), SHA256: hex.EncodeToString(digest[:])})
	}
	manifest := g8ebinaries.Manifest{SchemaVersion: 1, Version: version, BuildID: buildID, BuildTime: buildTime, SourceRevision: revision, SourceTreeHash: treeHash, Targets: artifacts}
	if err := manifest.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	data = append(data, '\n')
	staging := filepath.Join(root, ".g8e-binaries.json.new")
	if err := os.WriteFile(staging, data, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := os.Rename(staging, filepath.Join(root, "g8e-binaries.json")); err != nil {
		return fmt.Errorf("publish manifest: %w", err)
	}
	return nil
}
