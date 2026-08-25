// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// EnforceDirPermissions recursively enforces directory permissions on
// the given path and all contents.
func (fs *localFS) EnforceDirPermissions(ctx context.Context, relPath string, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	absPath := fs.Resolve(relPath)

	root, err := os.OpenRoot(absPath)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", constants.ErrEnforcePermissions, absPath, err)
	}
	defer root.Close()

	return filepath.WalkDir(absPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(absPath, path)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", constants.ErrEnforcePermissions, path, err)
		}
		if err := root.Chmod(rel, mode); err != nil {
			return fmt.Errorf("%w: %s: %w", constants.ErrEnforcePermissions, path, err)
		}
		return nil
	})
}

// EnforceFilePermissions enforces file permissions on a single file.
func (fs *localFS) EnforceFilePermissions(ctx context.Context, relPath string, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	absPath := fs.Resolve(relPath)
	if err := os.Chmod(absPath, mode); err != nil {
		return fmt.Errorf("%w: %s: %w", constants.ErrFileWriteFailed, absPath, err)
	}
	return nil
}
