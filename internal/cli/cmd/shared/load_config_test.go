// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package shared

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// swapConfigLoad replaces the package-level loader for one test. It must not
// be used from parallel tests because ConfigLoad is shared state.
func swapConfigLoad(t *testing.T, loader func(string) (*config.Config, error)) {
	t.Helper()
	original := ConfigLoad
	ConfigLoad = loader
	t.Cleanup(func() { ConfigLoad = original })
}

func TestLoadConfig_ReturnsTheConfigFromTheUnderlyingLoader(t *testing.T) {
	want := &config.Config{ProjectRoot: "/project"}
	var gotRoot string
	swapConfigLoad(t, func(projectRoot string) (*config.Config, error) {
		gotRoot = projectRoot
		return want, nil
	})

	got, err := LoadConfig("/project")

	require.NoError(t, err)
	assert.Same(t, want, got)
	assert.Equal(t, "/project", gotRoot, "the project root must be forwarded unchanged")
}

func TestLoadConfig_WrapsLoaderFailureWithSentinelAndKeepsTheCause(t *testing.T) {
	cause := errors.New("config.yaml: permission denied")
	swapConfigLoad(t, func(string) (*config.Config, error) { return nil, cause })

	got, err := LoadConfig("/project")

	assert.Nil(t, got)
	require.ErrorIs(t, err, constants.ErrConfigLoadFailed)
	require.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestLoadConfig_DefaultLoaderResolvesConfigForARealProjectRoot(t *testing.T) {
	root := testutil.TempDir(t)

	cfg, err := LoadConfig(root)

	require.NoError(t, err)
	assert.Equal(t, root, cfg.ProjectRoot)
}
