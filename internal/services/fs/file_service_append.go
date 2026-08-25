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

// OpenForAppend opens a file for appending (O_CREATE|O_WRONLY|O_APPEND).
// Creates parent directories if needed. Returns the raw *os.File — the
// caller must Close it. Used for log files that require streaming append
// (WriteFile's atomic tmp+rename is wrong for logs).
func (fs *localFS) OpenForAppend(ctx context.Context, relPath string, mode os.FileMode) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absPath := fs.Resolve(relPath)
	parent := filepath.Dir(absPath)
	if err := os.MkdirAll(parent, constants.PermDirPrivate); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	f, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, mode)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFileOpenFailed, err)
	}
	return f, nil
}

// OpenForRead opens an existing file for streaming read. Returns
// constants.ErrNotFound if the file does not exist. The caller must Close
// the returned handle. Used for tail/follow operations on log files.
func (fs *localFS) OpenForRead(ctx context.Context, relPath string) (*os.File, error) {
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
	return f, nil
}
