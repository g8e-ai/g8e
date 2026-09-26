// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import "testing"

// useCLISourceRoot points source-tree discovery at dir for the rest of t.
// Callers pass testutil.TempDir and do not change the process working directory.
func useCLISourceRoot(t *testing.T, dir string) {
	t.Helper()
	previous := cliSourceRoot
	cliSourceRoot = func() (string, error) { return dir, nil }
	t.Cleanup(func() { cliSourceRoot = previous })
}
