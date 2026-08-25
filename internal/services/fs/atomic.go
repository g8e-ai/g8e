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

// WriteFile atomically writes data to a file within the runtime directory.
// Uses os.CreateTemp to generate a unique temp file in the target directory,
// then renames it into place. This avoids collisions under concurrent writes
// to the same target path (replacing the fixed .tmp suffix pattern previously
// used in keystore.go and secret_manager.go).
//
// Parent directories are created with PermDirStandard (0755) as a safety net.
// CreateRuntimeTree should be called at startup to create the full directory
// tree with correct permissions.
func (fs *localFS) WriteFile(ctx context.Context, relPath string, data []byte, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	absPath := fs.Resolve(relPath)
	dir := filepath.Dir(absPath)

	if err := os.MkdirAll(dir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	tmpFile, err := os.CreateTemp(dir, ".g8e-tmp-*")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if _, err := os.Stat(tmpPath); err == nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}

	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}

	if err := os.Rename(tmpPath, absPath); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRenameFailed, err)
	}

	return nil
}
