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
