// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package auth

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestCheckOperatorRunningAtURL_ProbesTheTCPListener(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	t.Cleanup(func() { _ = listener.Close() })

	t.Run("a listening gateway is reported running", func(t *testing.T) {
		require.NoError(t, CheckOperatorRunningAtURL(fmt.Sprintf("http://127.0.0.1:%d", port)))
	})

	t.Run("localhost is resolved over IPv4", func(t *testing.T) {
		require.NoError(t, CheckOperatorRunningAtURL(fmt.Sprintf("http://localhost:%d", port)))
	})

	t.Run("nothing listening reads as service unavailable", func(t *testing.T) {
		closed, err := net.Listen("tcp4", "127.0.0.1:0")
		require.NoError(t, err)
		closedPort := closed.Addr().(*net.TCPAddr).Port
		require.NoError(t, closed.Close())

		err = CheckOperatorRunningAtURL(fmt.Sprintf("http://127.0.0.1:%d", closedPort))

		require.ErrorIs(t, err, constants.ErrServiceUnavailable)
	})

	invalid := map[string]string{
		"empty url":        "",
		"no host":          "http://",
		"unparseable url":  "http://[::1",
		"path only":        "/just/a/path",
		"scheme-less host": "gateway-without-scheme",
	}
	for name, url := range invalid {
		t.Run("rejects "+name, func(t *testing.T) {
			require.ErrorIs(t, CheckOperatorRunningAtURL(url), constants.ErrGatewayURLRequired)
		})
	}
}

func TestCheckOperatorRunning_UsesTheConfiguredDiscoveryURL(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	_, cfg := newAuthTestEnv(t)
	cfg.Paths.Host = fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)

	require.NoError(t, CheckOperatorRunning(cfg))

	require.NoError(t, listener.Close())
	require.ErrorIs(t, CheckOperatorRunning(cfg), constants.ErrServiceUnavailable)
}
