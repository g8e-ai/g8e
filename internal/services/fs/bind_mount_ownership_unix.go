//go:build unix

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package fs

import (
	"os"
	"path/filepath"
	"syscall"
)

func alignBindMountOwnership(absPath string) error {
	dirInfo, err := os.Stat(filepath.Dir(absPath))
	if err != nil {
		return nil
	}
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return nil
	}
	dirStat, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	fileStat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if dirStat.Uid == fileStat.Uid && dirStat.Gid == fileStat.Gid {
		return nil
	}
	return os.Chown(absPath, int(dirStat.Uid), int(dirStat.Gid))
}

func repairBindMountTree(absRoot string) error {
	return filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		return alignBindMountOwnership(path)
	})
}
