//go:build unix

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

package fs

import (
	"os"
	"syscall"
)

func runtimeDirRepairHint(runtimeDir string) string {
	info, err := os.Stat(runtimeDir)
	if err != nil {
		return ""
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return ""
	}
	return "Docker created .g8e as root; run: sudo chown -R $(id -u):$(id -g) " + runtimeDir
}
