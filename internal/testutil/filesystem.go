// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testutil

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
)

// FileMode is the permission mode os.Stat reports for a requested mode.
// Windows exposes only the read-only attribute through Go's mode bits; these
// assertions do not test Windows ACLs or prove owner-only access.
func FileMode(mode os.FileMode, directory bool) os.FileMode {
	if runtime.GOOS != "windows" {
		return mode
	}
	result := os.FileMode(0444)
	if mode&0200 != 0 {
		result |= 0222
	}
	if directory {
		result |= 0111
	}
	return result
}

// Symlink skips only when Windows denies the fixture's symlink privilege.
func Symlink(t *testing.T, oldname, newname string) {
	t.Helper()
	err := os.Symlink(oldname, newname)
	if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
		t.Skip("Windows symlink fixtures require Developer Mode or symlink privilege")
	}
	if err != nil {
		t.Fatalf("create symlink fixture: %v", err)
	}
}
