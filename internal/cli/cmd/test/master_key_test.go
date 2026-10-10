// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testcmd

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestWriteMasterKeyFile_WritesPrivateBase64Key(t *testing.T) {
	dir := t.TempDir()

	path, err := writeMasterKeyFile(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "master.key"), path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm())

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(raw[:len(raw)-1]))
	require.NoError(t, err)
	assert.Len(t, decoded, 32)
}

func TestWriteMasterKeyFile_FreshKeyEachCall(t *testing.T) {
	first, err := writeMasterKeyFile(t.TempDir())
	require.NoError(t, err)
	second, err := writeMasterKeyFile(t.TempDir())
	require.NoError(t, err)

	a, err := os.ReadFile(first)
	require.NoError(t, err)
	b, err := os.ReadFile(second)
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}
