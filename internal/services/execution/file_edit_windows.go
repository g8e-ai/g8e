// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows
// +build windows

package execution

import (
	"os"

	"github.com/g8e-ai/g8e/internal/models"
)

// collectFileOwnership is a no-op on Windows (ownership handled differently)
func (fes *FileEditService) collectFileOwnership(fileInfo os.FileInfo, stats *models.FileStats) error {
	// Windows file ownership is handled via ACLs, not UID/GID
	// For now, leave ownership fields nil
	return nil
}
