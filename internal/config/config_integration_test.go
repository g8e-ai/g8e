// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package config

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestResolveGatewayPorts(t *testing.T) {
	// Try to bind to a port to make it unavailable
	ln, err := net.Listen("tcp", net.JoinHostPort(constants.LocalhostIP, "0"))
	require.NoError(t, err)
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	takenPort, _ := strconv.Atoi(portStr)

	t.Run("resolves when port is taken", func(t *testing.T) {
		h, s := ResolveGatewayPorts(takenPort, takenPort+1)
		assert.NotEqual(t, takenPort, h)
		assert.Greater(t, h, takenPort)
		assert.Equal(t, s, h+1)
	})

	t.Run("resolves when all are available", func(t *testing.T) {
		// Use very high ports that are likely free
		h, s := ResolveGatewayPorts(55000, 55001)
		// Verify ports are sequential and >= requested values
		assert.GreaterOrEqual(t, h, 55000)
		assert.GreaterOrEqual(t, s, 55001)
		assert.Equal(t, s, h+1)
	})
}

func TestLoadGateway_IncrementalPorts(t *testing.T) {
	// Try to find a port to block
	basePort := 56000
	var ln net.Listener
	var err error
	for i := 0; i < 10; i++ {
		ln, err = net.Listen("tcp", net.JoinHostPort(constants.LocalhostIP, strconv.Itoa(basePort+i)))
		if err == nil {
			basePort = basePort + i
			break
		}
	}
	if ln == nil {
		t.Skip("Could not find a port to block for test")
		return
	}
	defer ln.Close()

	// Now try to load gateway with that base port
	cfg, err := LoadGateway(GatewayOptions{
		HTTPPort:          basePort,
		HTTPSPort:         basePort + 10,
		AllowTestPortZero: false,
	})
	require.NoError(t, err)
	assert.NotEqual(t, basePort, cfg.Gateway.HTTPPort)
	assert.Greater(t, cfg.Gateway.HTTPPort, basePort)
}
