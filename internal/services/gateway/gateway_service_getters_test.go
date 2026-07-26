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

//go:build integration

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayModeService_Getters(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	t.Run("db is non-nil", func(t *testing.T) {
		assert.NotNil(t, ls.db)
	})

	t.Run("GetSecretManager returns non-nil", func(t *testing.T) {
		sm, err := ls.GetSecretManager()
		require.NoError(t, err)
		assert.NotNil(t, sm)
	})

	t.Run("pki is non-nil", func(t *testing.T) {
		assert.NotNil(t, ls.pki)
	})

	t.Run("GetHTTPHandler returns non-nil", func(t *testing.T) {
		assert.NotNil(t, ls.GetHTTPHandler())
	})

	t.Run("GetStores returns non-nil", func(t *testing.T) {
		assert.NotNil(t, ls.GetStores())
	})

	t.Run("GetHTTPPort returns 0 when not started", func(t *testing.T) {
		assert.Equal(t, 0, ls.GetHTTPPort())
	})

	t.Run("GetHTTPSPort returns 0 when not started", func(t *testing.T) {
		assert.Equal(t, 0, ls.GetHTTPSPort())
	})
}
