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

package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/constants"
)

// EnforceDirPermissions recursively enforces directory permissions on
// the given path and all contents.
func (fs *localFS) EnforceDirPermissions(ctx context.Context, relPath string, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	absPath := fs.Resolve(relPath)

	return filepath.WalkDir(absPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("fs: enforce dir permissions %s: %w", path, err)
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
