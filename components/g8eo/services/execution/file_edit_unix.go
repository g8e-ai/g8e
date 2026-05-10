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
	"os"
	"os/user"
	"strconv"
	"syscall"

	"github.com/g8e-ai/g8e/components/g8eo/models"
)

// collectFileOwnership collects file ownership information (Unix-specific)
func (fes *FileEditService) collectFileOwnership(fileInfo os.FileInfo, stats *models.FileStats) {
	if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
		if u, err := user.LookupId(strconv.Itoa(int(stat.Uid))); err == nil {
			stats.Owner = &u.Username
		}
		if g, err := user.LookupGroupId(strconv.Itoa(int(stat.Gid))); err == nil {
			stats.Group = &g.Name
		}
	}
}
