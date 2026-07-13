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

// WriteFile atomically writes data to a file within the runtime directory.
// Uses os.CreateTemp to generate a unique temp file in the target directory,
// then renames it into place. This avoids collisions under concurrent writes
// to the same target path (replacing the fixed .tmp suffix pattern previously
// used in keystore.go and secret_manager.go).
//
// Parent directories are created with PermDirStandard (0755) as a safety net.
// EnsureRuntimeTree should be called at startup to create the full directory
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
