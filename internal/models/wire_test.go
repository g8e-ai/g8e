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

package models

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
)

func TestFsGrepMatch(t *testing.T) {
	t.Run("creates valid grep match", func(t *testing.T) {
		match := &FsGrepMatch{
			Path:       "/tmp/file.go",
			LineNumber: 10,
			Content:    "test line",
			Before:     []string{"line 9"},
			After:      []string{"line 11"},
		}

		assert.Equal(t, "/tmp/file.go", match.Path)
		assert.Equal(t, 10, match.LineNumber)
		assert.Equal(t, "test line", match.Content)
		assert.Len(t, match.Before, 1)
		assert.Len(t, match.After, 1)
	})

	t.Run("creates match without context", func(t *testing.T) {
		match := &FsGrepMatch{
			Path:       "/tmp/file.go",
			LineNumber: 10,
			Content:    "test line",
		}

		assert.Equal(t, "/tmp/file.go", match.Path)
		assert.Nil(t, match.Before)
		assert.Nil(t, match.After)
	})
}

func TestRuntimeConfig(t *testing.T) {
	t.Run("creates valid runtime config", func(t *testing.T) {
		config := &RuntimeConfig{
			CloudMode:             true,
			CloudProvider:         "aws",
			ExecutionVaultEnabled: true,
			NoGit:                 false,
			LogLevel:              "info",
			HTTPPort:              constants.Ports.OperatorHttps,
		}

		assert.True(t, config.CloudMode)
		assert.Equal(t, "aws", config.CloudProvider)
		assert.True(t, config.ExecutionVaultEnabled)
		assert.False(t, config.NoGit)
		assert.Equal(t, "info", config.LogLevel)
		assert.Equal(t, constants.Ports.OperatorHttps, config.HTTPPort)
	})

	t.Run("creates config for local mode", func(t *testing.T) {
		config := &RuntimeConfig{
			CloudMode:             false,
			ExecutionVaultEnabled: true,
			NoGit:                 false,
			LogLevel:              "debug",
			HTTPPort:              constants.Ports.OperatorHttps,
		}

		assert.False(t, config.CloudMode)
		assert.Empty(t, config.CloudProvider)
	})
}
