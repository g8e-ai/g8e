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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapMethodToPath(t *testing.T) {
	t.Run("tools/list maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("tools/list")
		assert.NoError(t, err)
		assert.Equal(t, "/tools/list", path)
	})

	t.Run("tools/call maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("tools/call")
		assert.NoError(t, err)
		assert.Equal(t, "/tools/call", path)
	})

	t.Run("resources/list maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("resources/list")
		assert.NoError(t, err)
		assert.Equal(t, "/resources/list", path)
	})

	t.Run("resources/read maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("resources/read")
		assert.NoError(t, err)
		assert.Equal(t, "/resources/read", path)
	})

	t.Run("prompts/list maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("prompts/list")
		assert.NoError(t, err)
		assert.Equal(t, "/prompts/list", path)
	})

	t.Run("prompts/get maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("prompts/get")
		assert.NoError(t, err)
		assert.Equal(t, "/prompts/get", path)
	})

	t.Run("unsupported method returns error", func(t *testing.T) {
		path, err := mapMethodToPath("unknown/method")
		assert.Error(t, err)
		assert.Empty(t, path)
		assert.Contains(t, err.Error(), "unsupported method")
	})
}
