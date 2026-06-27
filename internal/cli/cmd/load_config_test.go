// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
