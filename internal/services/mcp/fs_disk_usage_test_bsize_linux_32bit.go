// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License").
// See LICENSE for full text.

//go:build linux && !(amd64 || arm64 || ppc64 || ppc64le || mips64 || mips64le || s390x || loong64)

package mcp

import "syscall"

func setStatfsBsize(stat *syscall.Statfs_t, bsize uint32) {
	stat.Bsize = int32(bsize)
}
