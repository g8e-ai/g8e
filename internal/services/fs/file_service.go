// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package fs provides a focused file service for the .g8e/ runtime directory.
// It wraps os.* calls with opinionated defaults for atomic writes, permissions,
// and error wrapping. This is not a virtual filesystem abstraction — it is a
// thin service layer that enforces consistent file I/O patterns across the
// codebase.
package fs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pathutil"
	"github.com/g8e-ai/g8e/internal/paths"
)

// RuntimeFileService provides safe file operations within the .g8e runtime directory.
// All paths are resolved relative to the runtime directory root.
type RuntimeFileService interface {
	// MkdirAll creates a directory and all parents with the given mode.
	// Path must be within the runtime directory.
	MkdirAll(ctx context.Context, relPath string, mode os.FileMode) error

	// CreateRuntimeTree creates the full .g8e/ directory tree with correct
	// permissions. Called once at startup. Idempotent.
	CreateRuntimeTree(ctx context.Context) error

	// ReadFile reads a file within the runtime directory.
	// Returns wrapped constants.ErrNotFound if file does not exist.
	ReadFile(ctx context.Context, relPath string) ([]byte, error)

	// FileExists checks if a file exists. Returns false, nil for non-existent.
	FileExists(ctx context.Context, relPath string) (bool, error)

	// Stat returns FileInfo for a path within the runtime directory.
	Stat(ctx context.Context, relPath string) (os.FileInfo, error)

	// WriteFile atomically writes data to a file within the runtime directory.
	// Uses tmp+rename pattern with a unique temp file per call. Creates parent
	// directories if needed.
	WriteFile(ctx context.Context, relPath string, data []byte, mode os.FileMode) error

	// Remove deletes a file. No-op if file doesn't exist.
	Remove(ctx context.Context, relPath string) error

	// RemoveAll deletes a directory tree. No-op if path doesn't exist.
	RemoveAll(ctx context.Context, relPath string) error

	// ReadDir lists directory entries.
	ReadDir(ctx context.Context, relPath string) ([]os.DirEntry, error)

	// Rename atomically renames a file or directory.
	Rename(ctx context.Context, oldPath, newPath string) error

	// EnforceDirPermissions recursively enforces directory permissions.
	EnforceDirPermissions(ctx context.Context, relPath string, mode os.FileMode) error

	// EnforceFilePermissions enforces file permissions on a single file.
	EnforceFilePermissions(ctx context.Context, relPath string, mode os.FileMode) error

	// Resolve converts a relative path within .g8e/ to an absolute path.
	Resolve(relPath string) string

	// Rel converts an absolute path within .g8e/ to a path relative to the runtime dir.
	// Returns an error if the path is outside the runtime directory.
	Rel(absPath string) (string, error)
}

// localFS is the default RuntimeFileService implementation, operating
// directly on the local filesystem within the .g8e/ runtime directory.
type localFS struct {
	baseDir    string
	runtimeDir string
	logger     *slog.Logger
}

// NewRuntimeFileService creates a RuntimeFileService scoped to the .g8e/
// directory under baseDir. Callers must call paths.InitWithBase before
// constructing the service.
func NewRuntimeFileService(baseDir string, logger *slog.Logger) (RuntimeFileService, error) {
	if baseDir == "" {
		var err error
		baseDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("fs: get working directory: %w", err)
		}
	}

	if paths.Infra.RuntimeDir == "" {
		return nil, fmt.Errorf("fs: paths.Infra not initialized — call paths.InitWithBase first")
	}

	return &localFS{
		baseDir:    baseDir,
		runtimeDir: pathutil.SafeJoin(baseDir, constants.RuntimeDirname),
		logger:     logger,
	}, nil
}

// Resolve converts a relative path within .g8e/ to an absolute path.
// It uses pathutil.SafeJoin and verifies the resolved path is within
// the runtime directory to prevent path traversal.
func (fs *localFS) Resolve(relPath string) string {
	if relPath == "" {
		return fs.runtimeDir
	}
	absPath := pathutil.SafeJoin(fs.runtimeDir, relPath)
	return absPath
}

// Rel converts an absolute path within .g8e/ to a path relative to the runtime dir.
func (fs *localFS) Rel(absPath string) (string, error) {
	rel, err := filepath.Rel(fs.runtimeDir, absPath)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", constants.ErrPathValidation, absPath, err)
	}
	if !fs.isWithinRuntimeDir(absPath) {
		return "", fmt.Errorf("%w: %s is outside runtime dir %s", constants.ErrPathValidation, absPath, fs.runtimeDir)
	}
	return rel, nil
}

// isWithinRuntimeDir verifies that absPath is within the runtime directory.
func (fs *localFS) isWithinRuntimeDir(absPath string) bool {
	rel, err := filepath.Rel(fs.runtimeDir, absPath)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..") && rel != ".."
}

// MkdirAll creates a directory and all parents with the given mode.
func (fs *localFS) MkdirAll(ctx context.Context, relPath string, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	absPath := fs.Resolve(relPath)
	if err := os.MkdirAll(absPath, mode); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	return nil
}

// ReadFile reads a file within the runtime directory.
// Returns wrapped constants.ErrNotFound if file does not exist.
func (fs *localFS) ReadFile(ctx context.Context, relPath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absPath := fs.Resolve(relPath)
	f, err := os.Open(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", constants.ErrNotFound, absPath)
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrFileOpenFailed, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFileReadFailed, err)
	}
	return data, nil
}

// FileExists checks if a file exists. Returns false, nil for non-existent.
func (fs *localFS) FileExists(ctx context.Context, relPath string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	absPath := fs.Resolve(relPath)
	_, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
	}
	return true, nil
}

// Stat returns FileInfo for a path within the runtime directory.
func (fs *localFS) Stat(ctx context.Context, relPath string) (os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absPath := fs.Resolve(relPath)
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", constants.ErrNotFound, absPath)
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
	}
	return info, nil
}

// Remove deletes a file. No-op if file doesn't exist.
func (fs *localFS) Remove(ctx context.Context, relPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	absPath := fs.Resolve(relPath)
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %s: %w", constants.ErrFileRemoveFailed, relPath, err)
	}
	return nil
}

// RemoveAll deletes a directory tree. No-op if path doesn't exist.
func (fs *localFS) RemoveAll(ctx context.Context, relPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	absPath := fs.Resolve(relPath)
	if err := os.RemoveAll(absPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %s: %w", constants.ErrDirRemoveFailed, relPath, err)
	}
	return nil
}

// ReadDir lists directory entries.
func (fs *localFS) ReadDir(ctx context.Context, relPath string) ([]os.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absPath := fs.Resolve(relPath)
	entries, err := os.ReadDir(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", constants.ErrNotFound, absPath)
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrDirectoryRead, err)
	}
	return entries, nil
}

// Rename atomically renames a file or directory.
func (fs *localFS) Rename(ctx context.Context, oldPath, newPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	oldAbs := fs.Resolve(oldPath)
	newAbs := fs.Resolve(newPath)
	if err := os.Rename(oldAbs, newAbs); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRenameFailed, err)
	}
	return nil
}
