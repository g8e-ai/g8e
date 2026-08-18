// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	t.Run("narrow store getters return non-nil", func(t *testing.T) {
		assert.NotNil(t, ls.GetDocStore())
		assert.NotNil(t, ls.GetConsensusStore())
		assert.NotNil(t, ls.GetSignerStore())
		assert.NotNil(t, ls.GetAuditStore())
		assert.NotNil(t, ls.GetStateRootSvc())
		assert.NotNil(t, ls.GetKVStore())
		assert.NotNil(t, ls.GetReplayStore())
	})

	t.Run("GetHTTPPort returns 0 when not started", func(t *testing.T) {
		assert.Equal(t, 0, ls.GetHTTPPort())
	})

	t.Run("GetHTTPSPort returns 0 when not started", func(t *testing.T) {
		assert.Equal(t, 0, ls.GetHTTPSPort())
	})
}
