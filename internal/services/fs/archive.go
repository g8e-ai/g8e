// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package fs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ArchiveRuntime renames the .g8e/ runtime directory aside to a sibling named
// .g8e-<MMDDHHMM> instead of deleting it, and returns the archive's absolute
// path. It returns "" and no error when the runtime directory does not exist.
// If the timestamped name is already taken (two cleans in the same minute), a
// numeric suffix is appended; an existing archive is never overwritten.
func (fs *localFS) ArchiveRuntime(ctx context.Context, now time.Time) (string, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	if _, err := os.Lstat(fs.runtimeDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("%w: stat %s: %w", constants.ErrRuntimeArchiveFailed, fs.runtimeDir, err)
	}

	base := fs.runtimeDir + "-" + now.Format(constants.RuntimeArchiveTimestampLayout)
	target := base
	for n := 2; ; n++ {
		if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
			break
		} else if err != nil {
			return "", fmt.Errorf("%w: stat %s: %w", constants.ErrRuntimeArchiveFailed, target, err)
		}
		target = fmt.Sprintf("%s-%d", base, n)
	}

	if err := os.Rename(fs.runtimeDir, target); err != nil {
		return "", fmt.Errorf("%w: rename %s to %s: %w", constants.ErrRuntimeArchiveFailed, fs.runtimeDir, filepath.Base(target), err)
	}
	return target, nil
}
