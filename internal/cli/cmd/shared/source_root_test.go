// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package shared

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// swapSourceRoot points CLISourceRoot at root for one test. It must not be
// used from parallel tests because CLISourceRoot is shared state.
func swapSourceRoot(t *testing.T, root func() (string, error)) {
	t.Helper()
	original := CLISourceRoot
	CLISourceRoot = root
	t.Cleanup(func() { CLISourceRoot = original })
}

func TestAbsUnderSourceRoot_ResolvesRelativePathsAgainstTheSourceRoot(t *testing.T) {
	root := testutil.TempDir(t)
	swapSourceRoot(t, func() (string, error) { return root, nil })

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain relative path", input: "docs/swagger.json", want: filepath.Join(root, "docs", "swagger.json")},
		{name: "dot-prefixed path", input: "./demos", want: filepath.Join(root, "demos")},
		{name: "redundant segments are cleaned", input: "a//b/../c", want: filepath.Join(root, "a", "c")},
		{name: "empty path is the root itself", input: "", want: root},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AbsUnderSourceRoot(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAbsUnderSourceRoot_ReturnsAbsolutePathsCleanedWithoutConsultingTheSourceRoot(t *testing.T) {
	swapSourceRoot(t, func() (string, error) {
		return "", errors.New("source root must not be consulted for absolute paths")
	})
	absolute := filepath.Join(testutil.TempDir(t), "x", "..", "y")

	got, err := AbsUnderSourceRoot(absolute)

	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(absolute), got)
}

func TestAbsUnderSourceRoot_PropagatesSourceRootFailureForRelativePaths(t *testing.T) {
	cause := errors.New("working directory deleted")
	swapSourceRoot(t, func() (string, error) { return "", cause })

	got, err := AbsUnderSourceRoot("docs")

	assert.Empty(t, got)
	assert.ErrorIs(t, err, cause)
}

func TestAbsUnderSourceRoot_MatchesFilepathAbsWithTheDefaultSourceRoot(t *testing.T) {
	want, err := filepath.Abs("demos/fedramp")
	require.NoError(t, err)

	got, err := AbsUnderSourceRoot("demos/fedramp")

	require.NoError(t, err)
	assert.Equal(t, want, got)
}
