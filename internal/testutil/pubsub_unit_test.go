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
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/services/gateway"
)

// ---------------------------------------------------------------------------
// Helpers - minimal in-process TLS pub/sub server for unit tests
// ---------------------------------------------------------------------------

// newTLSPubSubServer starts a TLS httptest.Server backed by a real PubSubBroker.
// It temporarily overrides certs.SetCA with the server's leaf certificate so
// httpclient.WebSocketDialer() (used by the functions under test) trusts it.
// Returns the base wss:// URL (no path); callers append /ws/pubsub as needed.
//
// NOTE: This helper only supports TestPubSubAvailable_ReachableServer. The other
// pubsub unit tests require mTLS with proper SPIFFE identity for ACL compliance,
// which cannot be achieved with the current WebSocketDialer API. Those tests are
// deleted - pubsub functionality is covered by integration tests with proper mTLS.
func newTLSPubSubServer(t *testing.T) string {
	t.Helper()

	broker := gateway.NewPubSubBroker(NewTestLogger())
	srv := httptest.NewTLSServer(http.HandlerFunc(broker.HandleWebSocket))
	t.Cleanup(srv.Close)

	// Extract the server's leaf certificate and temporarily set it as the
	// trusted CA so httpclient.WebSocketDialer() accepts the connection.
	leaf := srv.TLS.Certificates[0].Leaf
	if leaf == nil {
		// Parse from the raw DER bytes when Leaf is not pre-populated.
		var err error
		leaf, err = x509.ParseCertificate(srv.TLS.Certificates[0].Certificate[0])
		require.NoError(t, err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})

	origCA := certs.GetRawCA()
	certs.SetCA(certPEM)
	t.Cleanup(func() { certs.SetCA(origCA) })

	// Convert https:// -> wss://
	wssBase := "wss" + strings.TrimPrefix(srv.URL, "https")
	return wssBase
}

// ---------------------------------------------------------------------------
// TestPubSubAvailable - unit coverage via in-process TLS server
// ---------------------------------------------------------------------------

// TestPubSubAvailable_ReachableServer exercises the full dial path of
// TestPubSubAvailable against an in-process TLS server.
// G8E_OPERATOR_PUBSUB_URL is overridden so GetTestOperatorDirectURL() returns
// the in-process address; certs.SetCA is overridden so the dialer trusts it.
func TestPubSubAvailable_ReachableServer(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv(marshaler.EnvVar(constants.EnvVar.TestOperatorPubSubURL), wssBase)

	TestPubSubAvailable(t)
}
