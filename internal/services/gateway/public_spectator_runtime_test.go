// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestPublicSpectatorRuntime_StartsMirrorListeners(t *testing.T) {
	privatePort := mustFreePort(t)
	publicPort := mustFreePort(t)
	fileSvc := newProducerFileSvc(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime, err := NewPublicSpectatorRuntime(PublicSpectatorConfig{
		Enabled:               true,
		PrivateListenAddress:  "127.0.0.1:" + privatePort,
		PublicListenAddress:   "127.0.0.1:" + publicPort,
		ExplorerListenAddress: "",
	}, fileSvc, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx))

	waitForBootstrap(t, "http://127.0.0.1:"+publicPort+"/bootstrap")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, runtime.Stop(stopCtx))
}

func TestValidatePublicMirrorListenAddresses_RejectsDuplicate(t *testing.T) {
	err := ValidatePublicMirrorListenAddresses("127.0.0.1:8081", "127.0.0.1:8081", false)
	assert.ErrorIs(t, err, constants.ErrPublicFeedListenAddress)
}

func TestValidatePublicMirrorListenAddresses_AllowsContainerBind(t *testing.T) {
	err := ValidatePublicMirrorListenAddresses("0.0.0.0:8081", "0.0.0.0:8082", true)
	assert.NoError(t, err)
	err = ValidatePublicMirrorListenAddresses("0.0.0.0:8081", "0.0.0.0:8082", false)
	assert.ErrorIs(t, err, constants.ErrPublicFeedListenAddress)
}

func mustFreePort(t *testing.T) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	return port
}

func waitForBootstrap(t *testing.T, url string) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("bootstrap never became healthy at %s", url)
}
