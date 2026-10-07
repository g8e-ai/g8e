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
)

func TestCheckOperatorRunning_Success(t *testing.T) {

	// Start a test server on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	url := fmt.Sprintf("http://127.0.0.1:%s", port)
	err = CheckOperatorRunningAtURL(url)
	require.NoError(t, err)
}

func TestCheckOperatorRunning_LocalhostReplacement(t *testing.T) {

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	url := fmt.Sprintf("http://localhost:%s", port)
	err = CheckOperatorRunningAtURL(url)
	require.NoError(t, err)
}
