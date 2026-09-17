// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package fs

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// RepairSharedRuntimeOwnership aligns bind-mounted public feed and mirror files
// with their parent directory owners so host CLI processes can share the same
// tree with containerized gateway publishers.
func RepairSharedRuntimeOwnership(ctx context.Context, fileSvc RuntimeFileService) error {
	if svc, ok := fileSvc.(*localFS); ok {
		return svc.RepairBindMountOwnership(ctx)
	}
	return nil
}

func isBindMountedRuntimePath(relPath string) bool {
	clean := filepath.ToSlash(filepath.Clean(relPath))
	switch {
	case clean == constants.PublicFeedDirname || strings.HasPrefix(clean, constants.PublicFeedDirname+"/"):
		return true
	case clean == constants.PublicMirrorDirname || strings.HasPrefix(clean, constants.PublicMirrorDirname+"/"):
		return true
	default:
		return false
	}
}

// RepairBindMountOwnership aligns bind-mounted public feed and mirror files with
// their parent directory owners so host CLI processes can share the same tree
// with containerized gateway publishers.
func (fs *localFS) RepairBindMountOwnership(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, relPath := range []string{constants.PublicFeedDirname, constants.PublicMirrorDirname} {
		exists, err := fs.FileExists(ctx, relPath)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := repairBindMountTree(fs.Resolve(relPath)); err != nil {
			return err
		}
	}
	return nil
}
