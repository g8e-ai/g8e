// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testutil

import (
	"crypto/tls"
	"testing"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/httpclient"
)

// TestPubSubAvailable checks if the client pub/sub gateway is reachable.
// Fatally fails the test when client is unavailable - all callers are integration
// tests that require a live stack.
// baseURL is optional; if empty, uses GetTestOperatorDirectURL().
func TestPubSubAvailable(t *testing.T, baseURL string) {
	t.Helper()
	if baseURL == "" {
		baseURL = GetTestOperatorDirectURL()
	}
	wsURL := baseURL + "/ws/pubsub"
	trustStore := GetTestTrustStore()
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})
	tlsConfig := certs.NewTLSConfig(trustStore, clientIdentity)
	stdTLS, err := tlsConfig.GetTLSConfig()
	if err != nil {
		t.Fatalf("testutil: TLS config build failed: %v", err)
	}
	dialer := httpclient.WebSocketDialerWithTLS(stdTLS)
	ws, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		t.Logf("testutil: pub/sub not available at %s: %v", baseURL, err)
		t.Skip("skipping integration test; pub/sub stack not available")
	}
	ws.Close()
}
