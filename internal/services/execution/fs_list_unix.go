//go:build unix || linux || darwin

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package execution

import (
	"os/user"
	"strconv"
	"syscall"
)

// getUsername returns the username for a given UID
func getUsername(uid uint32) string {
	if u, err := user.LookupId(strconv.Itoa(int(uid))); err == nil {
		return u.Username
	}
	return ""
}

// getGroupname returns the group name for a given GID
func getGroupname(gid uint32) string {
	if g, err := user.LookupGroupId(strconv.Itoa(int(gid))); err == nil {
		return g.Name
	}
	return ""
}

// getNlink returns the Nlink field from Stat_t as uint64.
// Nlink type varies by architecture (uint16 on amd64, uint32 on arm64/386),
// but casting to uint64 works uniformly across all Unix architectures.
func getNlink(stat *syscall.Stat_t) uint64 {
	return uint64(stat.Nlink) //nolint:unconvert // necessary for cross-platform compatibility
}
