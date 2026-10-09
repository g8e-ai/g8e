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
	"runtime"
	"syscall"
	"time"

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
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	absPath := fs.Resolve(relPath)
	dir := filepath.Dir(absPath)

	if err := os.MkdirAll(dir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	tmpFile, err := os.CreateTemp(dir, ".g8e-tmp-*")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("%w: %w (%s)", constants.ErrFileWriteFailed, err, runtimeWriteFailureHint(dir))
		}
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

	// Windows prevents replacement of a file with its read-only attribute set.
	// Clear that attribute on the old file; the replacement keeps the requested
	// mode. Restore the old mode if replacement fails.
	var previousMode os.FileMode
	if runtime.GOOS == "windows" {
		info, err := os.Stat(absPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: stat destination: %w", constants.ErrFileRenameFailed, err)
		}
		if err == nil && info.Mode().Perm()&0200 == 0 {
			previousMode = info.Mode().Perm()
			if err := os.Chmod(absPath, previousMode|0200); err != nil {
				return fmt.Errorf("%w: clear read-only destination: %w", constants.ErrFileRenameFailed, err)
			}
		}
	}
	if err := renameReplace(tmpPath, absPath); err != nil {
		if previousMode != 0 {
			if restoreErr := os.Chmod(absPath, previousMode); restoreErr != nil {
				return fmt.Errorf("%w: %w; restore destination mode: %w", constants.ErrFileRenameFailed, err, restoreErr)
			}
		}
		return fmt.Errorf("%w: %w", constants.ErrFileRenameFailed, err)
	}

	return nil
}

const (
	windowsRenameRetryAttempts = 20
	windowsRenameRetryDelay    = 10 * time.Millisecond
)

// renameReplace moves tmpPath over absPath. On Windows, MoveFileEx fails with
// access denied while another handle holds the destination open, and Go's
// os.Open does not request FILE_SHARE_DELETE, so concurrent readers (for
// example a watcher polling the same record) make the replacement fail
// transiently. Retry only that case; the destination is either replaced or
// still holds the previous complete contents.
func renameReplace(tmpPath, absPath string) error {
	err := os.Rename(tmpPath, absPath)
	if runtime.GOOS != "windows" {
		return err
	}
	for attempt := 1; err != nil && errors.Is(err, os.ErrPermission) && attempt < windowsRenameRetryAttempts; attempt++ {
		time.Sleep(windowsRenameRetryDelay)
		err = os.Rename(tmpPath, absPath)
	}
	return err
}

// windowsErrorSharingViolation is ERROR_SHARING_VIOLATION: the file is open
// without a share mode that permits this access.
const windowsErrorSharingViolation syscall.Errno = 32

// openRetryable reports whether an open failure is the transient Windows
// sharing violation raised while WriteFile is replacing the destination. It
// is never true on other platforms, where errno 32 means something else.
func openRetryable(err error) bool {
	return runtime.GOOS == "windows" && errors.Is(err, windowsErrorSharingViolation)
}

// openRetrying opens absPath for reading, retrying only the transient Windows
// sharing violation from a concurrent atomic replacement. The file is either
// the previous or the new complete contents once the open succeeds.
func openRetrying(absPath string) (*os.File, error) {
	f, err := os.Open(absPath)
	for attempt := 1; err != nil && openRetryable(err) && attempt < windowsRenameRetryAttempts; attempt++ {
		time.Sleep(windowsRenameRetryDelay)
		f, err = os.Open(absPath)
	}
	return f, err
}
