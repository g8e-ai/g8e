// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package shared

import (
	"os"
	"path/filepath"
)

// CLISourceRoot is the directory used for source-tree discovery (Swagger specs
// and demo directories). Production uses the process working directory. Tests
// point it at testutil.TempDir so they do not call os.Chdir.
var CLISourceRoot = os.Getwd

// AbsUnderSourceRoot resolves path against CLISourceRoot. Absolute paths are
// cleaned and returned unchanged. Relative paths match filepath.Abs when
// CLISourceRoot is os.Getwd.
func AbsUnderSourceRoot(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	root, err := CLISourceRoot()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(root, path))
}
