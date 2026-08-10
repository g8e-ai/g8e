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
	if opts.posture != "" {
		cfg.Gateway.Posture = opts.posture
	}

	ls, err := NewGatewayModeService(cfg, fileSvc, logger)
	require.NoError(t, err)
	// HTTP handler is now built during NewGatewayModeService construction.
	// InitHTTPHandler is a no-op kept for backward compatibility.
	require.NoError(t, ls.InitHTTPHandler())
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
