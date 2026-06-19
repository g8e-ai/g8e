//go:build integration

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

package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenBrowser(t *testing.T) {
	t.Run("OpenBrowser returns error for invalid URL", func(t *testing.T) {
		err := OpenBrowser("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open browser")
	})

	t.Run("OpenBrowser attempts to open valid URL", func(t *testing.T) {
		err := OpenBrowser("https://example.com")
		// This will likely fail in test environment due to no display,
		// but should not panic
		if err != nil {
			assert.Contains(t, err.Error(), "failed to open browser")
		}
	})
}
