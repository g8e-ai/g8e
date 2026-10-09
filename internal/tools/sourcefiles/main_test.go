// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSourcefiles_PrintsSlashSeparatedDependencies(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "ui", "src"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "ui", "tests"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ui", "src", "app.ts"), []byte("app"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ui", "tests", "app.test.ts"), []byte("test"), 0o644))
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runSourcefiles([]string{"-base", root, "-exclude", "tests", "ui"}, &stdout, &stderr)

	assert.Zero(t, code)
	assert.Equal(t, "ui/src/app.ts\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRunSourcefiles_RejectsMissingEntries(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runSourcefiles(nil, &stdout, &stderr)

	assert.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "at least one source entry")
}
