// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package httpclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertBaseTransportTimeouts(t *testing.T, transport *http.Transport) {
	t.Helper()
	assert.Equal(t, DefaultTLSTimeout, transport.TLSHandshakeTimeout)
	assert.Equal(t, DefaultIdleConnTimeout, transport.IdleConnTimeout)
	assert.Equal(t, 10, transport.MaxIdleConns)
	assert.Equal(t, 5, transport.MaxIdleConnsPerHost)
	assert.NotNil(t, transport.DialContext)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, 30*time.Second, DefaultTimeout)
	assert.Equal(t, 10*time.Second, DefaultDialTimeout)
	assert.Equal(t, 10*time.Second, DefaultTLSTimeout)
	assert.Equal(t, 90*time.Second, DefaultIdleConnTimeout)
}

func TestNewWithTLS(t *testing.T) {
	customTLS := &tls.Config{
		MinVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{
			{},
		},
	}

	client := NewWithTLS(customTLS)
	require.NotNil(t, client)

	assert.Equal(t, DefaultTimeout, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, customTLS, transport.TLSClientConfig)
	assertBaseTransportTimeouts(t, transport)
}

func TestWebSocketDialerWithTLS(t *testing.T) {
	customTLS := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	dialer := WebSocketDialerWithTLS(customTLS)
	require.NotNil(t, dialer)

	assert.Equal(t, customTLS, dialer.TLSClientConfig)
	assert.Equal(t, DefaultTLSTimeout, dialer.HandshakeTimeout)
}

// listenIPv4Only starts an httptest server bound to 127.0.0.1 only (no
// IPv6 listener on [::1]). This simulates the IDE port-forward that only
// listens on IPv4 — the failure mode the IPv4 dialer exists to fix.
func listenIPv4Only(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &httptest.Server{Listener: ln, Config: &http.Server{Handler: handler}}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// listenIPv6Only starts an httptest server bound to [::1] only. Used to
// prove the IPv4 dialer cannot reach an IPv6-only listener even when the
// hostname resolves to ::1.
func listenIPv6Only(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		// IPv6 loopback not available in this environment — skip, not fail.
		t.Skipf("ipv6 loopback unavailable: %v", err)
	}
	srv := &httptest.Server{Listener: ln, Config: &http.Server{Handler: handler}}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// TestIPv4DialContext_ResolvesLocalhostToIPv4Only is the root-cause
// confirmation test for the Windows localhost→::1 dial failure. It dials
// `localhost` via IPv4DialContext against an IPv4-only listener and asserts
// success, then dials `localhost` against an IPv6-only listener and asserts
// failure (IPv6 is excluded entirely, not merely deprioritized).
func TestIPv4DialContext_ResolvesLocalhostToIPv4Only(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ipv4Srv := listenIPv4Only(t, ok)
	// Dial localhost via the IPv4 dialer against the IPv4-only listener.
	// The dialer must resolve localhost to 127.0.0.1 and connect via tcp4.
	_, ipv4Port, err := net.SplitHostPort(ipv4Srv.Listener.Addr().String())
	require.NoError(t, err)
	conn, err := IPv4DialContext(context.Background(), "tcp", net.JoinHostPort("localhost", ipv4Port))
	require.NoError(t, err)
	require.NotNil(t, conn)
	conn.Close() //nolint:errcheck

	// Dial localhost via the IPv4 dialer against an IPv6-only listener.
	// The dialer must NOT reach [::1] — it resolves localhost via ip4 and
	// dials tcp4, so an IPv6-only listener is unreachable.
	ipv6Srv := listenIPv6Only(t, ok)
	_, ipv6Port, err := net.SplitHostPort(ipv6Srv.Listener.Addr().String())
	require.NoError(t, err)
	conn, err = IPv4DialContext(context.Background(), "tcp", net.JoinHostPort("localhost", ipv6Port))
	require.Error(t, err, "IPv4 dialer must not reach an IPv6-only listener")
	if conn != nil {
		conn.Close() //nolint:errcheck
	}
}

// TestIPv4DialContext_RejectsLiteralIPv6Address proves a literal IPv6
// address (e.g. "[::1]") cannot be dialed through the IPv4 dialer — the
// ip4 lookup fails and returns an error, never an IPv6 connection.
func TestIPv4DialContext_RejectsLiteralIPv6Address(t *testing.T) {
	_, err := IPv4DialContext(context.Background(), "tcp", "[::1]:8443")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ipv4 dial: resolve")
}

// TestIPv4DialContext_SplitHostPortError confirms a malformed address
// surfaces the split error rather than panicking.
func TestIPv4DialContext_SplitHostPortError(t *testing.T) {
	_, err := IPv4DialContext(context.Background(), "tcp", "no-port-here")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ipv4 dial: split host/port")
}

// TestNewIPv4Transport_DialsIPv4Only verifies the transport's DialContext
// is the IPv4 dialer and that an HTTP client built on it reaches an
// IPv4-only listener via localhost.
func TestNewIPv4Transport_DialsIPv4Only(t *testing.T) {
	srv := listenIPv4Only(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	}))
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	transport := NewIPv4Transport(nil)
	require.NotNil(t, transport.DialContext)
	assert.Equal(t, DefaultTLSTimeout, transport.TLSHandshakeTimeout)
	assert.Equal(t, DefaultIdleConnTimeout, transport.IdleConnTimeout)

	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%s/", port))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestNewIPv4Transport_RejectsIPv6OnlyHost confirms an HTTP client on the
// IPv4 transport cannot reach an IPv6-only listener via localhost.
func TestNewIPv4Transport_RejectsIPv6OnlyHost(t *testing.T) {
	srv := listenIPv6Only(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	client := &http.Client{Transport: NewIPv4Transport(nil), Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%s/", port))
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err, "IPv4 transport must not reach an IPv6-only listener")
}

// TestNewIPv4Transport_LiteralIPv4DialsDirectly confirms a literal IPv4
// address dials directly (no DNS lookup needed) through the IPv4 transport.
func TestNewIPv4Transport_LiteralIPv4DialsDirectly(t *testing.T) {
	srv := listenIPv4Only(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	client := &http.Client{Transport: NewIPv4Transport(nil), Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/", port))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
