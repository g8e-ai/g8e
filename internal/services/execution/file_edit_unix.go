//go:build unix || linux || darwin

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package execution

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// collectFileOwnership collects file ownership information (Unix-specific)
func (fes *FileEditService) collectFileOwnership(fileInfo os.FileInfo, stats *models.FileStats) error {
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("file_edit: collect ownership: %w", constants.ErrFileOwnershipExtract)
	}

	uidStr := strconv.Itoa(int(stat.Uid))
	gidStr := strconv.Itoa(int(stat.Gid))

	if u, err := user.LookupId(uidStr); err != nil {
		fes.logger.Warn("Failed to lookup user by UID",
			"uid", uidStr,
			"error", err)
	} else {
		stats.Owner = u.Username
	}

	if g, err := user.LookupGroupId(gidStr); err != nil {
		fes.logger.Warn("Failed to lookup group by GID",
			"gid", gidStr,
			"error", err)
	} else {
		stats.Group = g.Name
	}

	return nil
}
