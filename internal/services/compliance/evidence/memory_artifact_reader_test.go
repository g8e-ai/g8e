// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// memoryArtifactReader is an in-memory ArtifactReader for importer tests.
type memoryArtifactReader struct {
	files map[string][]byte
}

func (r *memoryArtifactReader) ReadFile(_ context.Context, path string) ([]byte, error) {
	body, ok := r.files[path]
	if !ok {
		return nil, constants.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func (r *memoryArtifactReader) ReadDir(_ context.Context, path string) ([]os.DirEntry, error) {
	prefix := path + string(os.PathSeparator)
	names := make(map[string]bool)
	for candidate := range r.files {
		if !strings.HasPrefix(candidate, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(candidate, prefix)
		parts := strings.SplitN(remainder, string(os.PathSeparator), 2)
		names[parts[0]] = len(parts) == 2
	}
	if len(names) == 0 {
		return nil, constants.ErrNotFound
	}
	entries := make([]os.DirEntry, 0, len(names))
	for name, directory := range names {
		entries = append(entries, memoryDirEntry{name: name, directory: directory})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

type memoryDirEntry struct {
	name      string
	directory bool
}

func (e memoryDirEntry) Name() string               { return e.name }
func (e memoryDirEntry) IsDir() bool                { return e.directory }
func (e memoryDirEntry) Type() os.FileMode          { return 0 }
func (e memoryDirEntry) Info() (os.FileInfo, error) { return nil, nil }
