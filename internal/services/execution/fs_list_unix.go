//go:build unix || linux || darwin

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
