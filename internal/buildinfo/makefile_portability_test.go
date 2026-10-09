// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package buildinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakefile_FrontendSourceDiscoveryDoesNotDependOnPlatformFind(t *testing.T) {
	root, err := RepositoryRoot(".")
	require.NoError(t, err)
	body, err := os.ReadFile(filepath.Join(root, "Makefile"))
	require.NoError(t, err)

	makefile := string(body)
	assert.Contains(t, makefile, "SOURCE_FILES := go run ./internal/tools/sourcefiles -base .")
	for _, line := range strings.Split(makefile, "\n") {
		if strings.HasPrefix(line, "EXPLORER_SOURCES :=") || strings.HasPrefix(line, "CONSOLE_SOURCES :=") {
			assert.Contains(t, line, "$(SOURCE_FILES)")
			assert.NotContains(t, line, "$(shell find ")
			assert.NotContains(t, line, "/usr/bin/find")
		}
	}
}
