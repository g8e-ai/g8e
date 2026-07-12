// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License").
// See LICENSE for full text.

//go:build linux

package mcp

import "syscall"

func setStatfsBsize(stat *syscall.Statfs_t, bsize uint32) {
	stat.Bsize = int64(bsize)
}
