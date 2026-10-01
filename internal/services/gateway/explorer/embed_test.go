// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package explorer

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assetReference matches the root-relative asset URLs index.html loads.
var assetReference = regexp.MustCompile(`(?:src|href)="/(assets/[^"]+)"`)

func TestStaticFS_RootsTheFilesystemAtTheStaticDirectory(t *testing.T) {
	static, err := StaticFS()
	require.NoError(t, err)

	for _, name := range []string{"index.html", "runtime.json"} {
		info, err := fs.Stat(static, name)
		require.NoError(t, err, name)
		assert.False(t, info.IsDir(), name)
		assert.NotZero(t, info.Size(), name)
	}
	_, err = fs.Stat(static, "static/index.html")
	assert.ErrorIs(t, err, fs.ErrNotExist, "the static/ prefix must be stripped from served paths")
}

func TestStaticFS_ServesAnHTMLDocumentAsTheEntryPoint(t *testing.T) {
	static, err := StaticFS()
	require.NoError(t, err)

	index, err := fs.ReadFile(static, "index.html")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(strings.ToLower(string(index)), "<!doctype html>"))
	assert.Contains(t, string(index), "</html>")
}

func TestStaticFS_EveryAssetReferencedByIndexIsEmbedded(t *testing.T) {
	static, err := StaticFS()
	require.NoError(t, err)
	index, err := fs.ReadFile(static, "index.html")
	require.NoError(t, err)

	references := assetReference.FindAllStringSubmatch(string(index), -1)

	require.NotEmpty(t, references, "index.html should load at least its script and stylesheet")
	for _, match := range references {
		info, err := fs.Stat(static, match[1])
		require.NoError(t, err, "index.html references %s which is not embedded; rebuild the explorer bundle", match[1])
		assert.NotZero(t, info.Size(), match[1])
	}
}

func TestStaticFS_RuntimeConfigIsAJSONObjectWithSchemaVersion(t *testing.T) {
	static, err := StaticFS()
	require.NoError(t, err)

	raw, err := fs.ReadFile(static, "runtime.json")
	require.NoError(t, err)

	var runtime map[string]any
	require.NoError(t, json.Unmarshal(raw, &runtime))
	assert.NotEmpty(t, runtime["schema_version"])
}

func TestStaticFS_MissingPathReportsNotExist(t *testing.T) {
	static, err := StaticFS()
	require.NoError(t, err)

	_, err = fs.ReadFile(static, "no-such-file.txt")

	assert.ErrorIs(t, err, fs.ErrNotExist)
}
