// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux && (amd64 || arm64 || ppc64 || ppc64le || mips64 || mips64le || s390x || loong64)

package mcp

import "syscall"

func setStatfsBsize(stat *syscall.Statfs_t, bsize uint32) {
	stat.Bsize = int64(bsize)
}
