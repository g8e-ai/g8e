// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tests

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/test/fixtures"
)

func TestGateway_HTTPSNegotiatesHTTP2(t *testing.T) {
	f := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName: "gateway-http2", Posture: config.PostureDoctrine, AllowTestPortZero: true,
	})
	identity := fixtures.EnrollClientIdentity(t, f, "http2-user", "http2-org", "http2-fingerprint", "http2-host")
	client := fixtures.CreateMTLSClient(t, f, identity)
	client.Transport.(*http.Transport).ForceAttemptHTTP2 = true
	t.Cleanup(client.CloseIdleConnections)
	url := fmt.Sprintf("https://%s:%d%s", constants.LocalhostIP, f.Service.GetHTTPSPort(), constants.APIPaths.Health)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, resp.ProtoMajor, "concurrent dispatch should multiplex over HTTP/2")

	// Check the advertised capacity on the real listener. Negotiation alone
	// still causes a burst of new connections when the default stream limit
	// is reached by a whole-fleet dispatch.
	tlsConfig := client.Transport.(*http.Transport).TLSClientConfig.Clone()
	tlsConfig.NextProtos = []string{http2.NextProtoTLS}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp4",
		fmt.Sprintf("%s:%d", constants.LocalhostIP, f.Service.GetHTTPSPort()), tlsConfig)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = io.WriteString(conn, http2.ClientPreface)
	require.NoError(t, err)
	framer := http2.NewFramer(conn, conn)
	require.NoError(t, framer.WriteSettings())
	frame, err := framer.ReadFrame()
	require.NoError(t, err)
	settings, ok := frame.(*http2.SettingsFrame)
	require.True(t, ok, "the server must send its HTTP/2 settings first")
	streams, ok := settings.Value(http2.SettingMaxConcurrentStreams)
	require.True(t, ok)
	require.Equal(t, uint32(constants.GatewayHTTP2MaxConcurrentStreams), streams)
}
