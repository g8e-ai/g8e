// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// testGatewayOpts configures optional parameters for newTestGatewayService.
type testGatewayOpts struct {
	posture  config.GatewayPosture
	httpPort int
}

// newTestGatewayService creates a GatewayModeService via NewGatewayModeService
// with test-appropriate configuration. Cleanup is registered via t.Cleanup.
func newTestGatewayService(t *testing.T, opts testGatewayOpts) *GatewayModeService {
	t.Helper()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	fileSvc := newTestFileSvc(t)

	cfg.Gateway.DataDir = testutil.TempDir(t)
	cfg.Gateway.PKIDir = testutil.TempDir(t)
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.VaultDir = fileSvc.Resolve(constants.VaultDirname)
	cfg.Gateway.HTTPPort = opts.httpPort
	cfg.Gateway.Posture = opts.posture

	ls, err := NewGatewayModeService(cfg, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { ls.Stop(context.Background()) })
	return ls
}

func TestNewGatewayModeService(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	assert.NotNil(t, ls)
	assert.NotNil(t, ls.server)
	assert.NotNil(t, ls.pki)
	assert.False(t, ls.running)
}

func TestGatewayModeService_StateManagement(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	t.Run("Initial state", func(t *testing.T) {
		assert.False(t, ls.running)
		assert.False(t, ls.IsReady())
	})

	t.Run("IsReady returns false when not ready", func(t *testing.T) {
		assert.False(t, ls.IsReady())
	})
}

func TestDetectBasicNonLoopbackIPv4Addresses(t *testing.T) {
	ips := detectBasicNonLoopbackIPv4Addresses()
	assert.NotNil(t, ips)
}
