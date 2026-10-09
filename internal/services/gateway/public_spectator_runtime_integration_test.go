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
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestPublicSpectatorRuntime_StartsMirrorListeners(t *testing.T) {
	privatePort := mustFreePort(t)
	publicPort := mustFreePort(t)
	fileSvc := newProducerFileSvc(t)
	logger := testutil.NewTestLogger()
	ks := newTestKeystore(t, fileSvc, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime, err := NewPublicSpectatorRuntime(PublicSpectatorConfig{
		Enabled:               true,
		PrivateListenAddress:  "127.0.0.1:" + privatePort,
		PublicListenAddress:   "127.0.0.1:" + publicPort,
		ExplorerListenAddress: "",
		TrustedProxyCIDRs:     []string{"172.28.0.1/32"},
	}, fileSvc, ks, logger)
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx))

	requireBootstrapHealthy(t, "http://127.0.0.1:"+publicPort+"/bootstrap")
	require.Len(t, runtime.mirror.trustedProxyNetworks, 1)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, runtime.Stop(stopCtx))
}

func TestPublicSpectatorRuntime_StartsDedicatedExplorerListener(t *testing.T) {
	privatePort := mustFreePort(t)
	publicPort := mustFreePort(t)
	explorerPort := mustFreePort(t)
	fileSvc := newProducerFileSvc(t)
	logger := testutil.NewTestLogger()
	ks := newTestKeystore(t, fileSvc, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime, err := NewPublicSpectatorRuntime(PublicSpectatorConfig{
		Enabled:               true,
		PrivateListenAddress:  "127.0.0.1:" + privatePort,
		PublicListenAddress:   "127.0.0.1:" + publicPort,
		ExplorerListenAddress: "127.0.0.1:" + explorerPort,
	}, fileSvc, ks, logger)
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx))

	requireBootstrapHealthy(t, "http://127.0.0.1:"+publicPort+"/bootstrap")
	requireExplorerRuntime(t, "http://127.0.0.1:"+explorerPort+"/runtime.json", "http://127.0.0.1:"+publicPort)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, runtime.Stop(stopCtx))
}

func TestNewPublicSpectatorRuntime_ContainerBindRequiresExplicitConfig(t *testing.T) {
	// The retired env switch must not relax the network-exposure check.
	t.Setenv("G8E_DOCKER_COMPOSE", "1")

	cfg := PublicSpectatorConfig{
		Enabled:               true,
		PrivateListenAddress:  "0.0.0.0:" + mustFreePort(t),
		PublicListenAddress:   "0.0.0.0:" + mustFreePort(t),
		ExplorerListenAddress: "",
	}

	fileSvc := newProducerFileSvc(t)
	logger := testutil.NewTestLogger()
	ks := newTestKeystore(t, fileSvc, logger)

	_, err := NewPublicSpectatorRuntime(cfg, fileSvc, ks, logger)
	require.ErrorIs(t, err, constants.ErrPublicFeedListenAddress)

	cfg.AllowContainerBind = true
	_, err = NewPublicSpectatorRuntime(cfg, fileSvc, ks, logger)
	require.NoError(t, err)
}

func mustFreePort(t *testing.T) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	return port
}
