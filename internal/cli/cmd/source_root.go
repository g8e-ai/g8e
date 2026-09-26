// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"os"
	"path/filepath"
)

// cliSourceRoot is the directory used for source-tree discovery (Swagger specs
// and demo directories). Production uses the process working directory. Tests
// point it at testutil.TempDir so they do not call os.Chdir.
var cliSourceRoot = os.Getwd

// absUnderSourceRoot resolves path against cliSourceRoot. Absolute paths are
// cleaned and returned unchanged. Relative paths match filepath.Abs when
// cliSourceRoot is os.Getwd.
func absUnderSourceRoot(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	root, err := cliSourceRoot()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(root, path))
}
