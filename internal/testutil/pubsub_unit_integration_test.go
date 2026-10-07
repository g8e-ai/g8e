// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package testutil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/httpclient"
	"github.com/stretchr/testify/require"
)

// newTLSPubSubServer starts a TLS httptest.Server backed by a real GatewayWebSocketHandler.
// It returns the base wss:// URL (no path) and a *tls.Config that trusts the
// server's leaf certificate. Callers append /ws/pubsub as needed.
//
// NOTE: This helper only supports TestPubSubAvailable_ReachableServer. The other
// pubsub unit tests require mTLS with proper SPIFFE identity for ACL compliance,
// which cannot be achieved with the current WebSocketDialer API. Those tests are
// deleted - pubsub functionality is covered by integration tests with proper mTLS.
func newTLSPubSubServer(t *testing.T) (string, *tls.Config) {
	t.Helper()

	broker := newTestGatewayWebSocketHandler(NewTestLogger())
	srv := httptest.NewTLSServer(http.HandlerFunc(broker.HandleWebSocket))
	t.Cleanup(srv.Close)
	t.Cleanup(broker.Close)

	// Extract the server's leaf certificate and build a trust pool.
	leaf := srv.TLS.Certificates[0].Leaf
	if leaf == nil {
		var err error
		leaf, err = x509.ParseCertificate(srv.TLS.Certificates[0].Certificate[0])
		require.NoError(t, err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)
	tlsCfg := &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS13,
	}

	// Convert https:// -> wss://
	wssBase := "wss" + strings.TrimPrefix(srv.URL, "https")
	return wssBase, tlsCfg
}

// TestPubSubAvailable_ReachableServer exercises the full dial path of
// TestPubSubAvailable against an in-process TLS server using DI-based TLS.
func TestPubSubAvailable_ReachableServer(t *testing.T) {
	wssBase, tlsCfg := newTLSPubSubServer(t)
	wsURL := wssBase + "/ws/pubsub"
	dialer := httpclient.WebSocketDialerWithTLS(tlsCfg)
	ws, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	if resp != nil {
		resp.Body.Close()
	}
	ws.Close()
}
