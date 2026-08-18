// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"errors"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
)

func TestLoadConfigSuccess(t *testing.T) {
	cfg, err := loadConfig("")
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.ProjectRoot)
}

func TestLoadConfigWrapsError(t *testing.T) {
	originalLoad := configLoad
	configLoad = func(string) (*config.Config, error) {
		return nil, errors.New("disk read failure")
	}
	defer func() { configLoad = originalLoad }()

	cfg, err := loadConfig("")
	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
	assert.Contains(t, err.Error(), "disk read failure")
}
