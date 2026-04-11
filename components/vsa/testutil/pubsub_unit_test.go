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
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/vsa/certs"
	"github.com/g8e-ai/g8e/components/vsa/services/listen"
)

// ---------------------------------------------------------------------------
// Helpers — minimal in-process TLS pub/sub server for unit tests
// ---------------------------------------------------------------------------

// newTLSPubSubServer starts a TLS httptest.Server backed by a real PubSubBroker.
// It temporarily overrides certs.SetCA with the server's leaf certificate so
// httpclient.WebSocketDialer() (used by the functions under test) trusts it.
// Returns the base wss:// URL (no path); callers append /ws/pubsub as needed.
func newTLSPubSubServer(t *testing.T) string {
	t.Helper()

	broker := listen.NewPubSubBroker(NewTestLogger())
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
// TestPubSubAvailable — unit coverage via in-process TLS server
// ---------------------------------------------------------------------------

// TestPubSubAvailable_ReachableServer exercises the full dial path of
// TestPubSubAvailable against an in-process TLS server.
// G8E_OPERATOR_PUBSUB_URL is overridden so GetTestVSODBDirectURL() returns
// the in-process address; certs.SetCA is overridden so the dialer trusts it.
func TestPubSubAvailable_ReachableServer(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", wssBase)

	TestPubSubAvailable(t)
}

// ---------------------------------------------------------------------------
// SubscribeToChannel — unit coverage via in-process TLS server
// ---------------------------------------------------------------------------

func TestSubscribeToChannel_ReturnsChannel(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", wssBase)

	ch := SubscribeToChannel(t, wssBase, "results:op1:sess1")
	require.NotNil(t, ch)
}

func TestSubscribeToChannel_ReceivesPublishedPayload(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", wssBase)

	channel := CreateTestChannel(t, "results")
	ch := SubscribeToChannel(t, wssBase, channel)

	time.Sleep(50 * time.Millisecond)

	PublishTestMessage(t, wssBase, channel, `{"event_type":"command.completed","operator_id":"op-42"}`)

	msg := WaitForMessage(t, ch, 5*time.Second)
	require.NotNil(t, msg)

	var got map[string]string
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "command.completed", got["event_type"])
	assert.Equal(t, "op-42", got["operator_id"])
}

func TestSubscribeToChannel_IgnoresNonMessageFrames(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", wssBase)

	channel := CreateTestChannel(t, "results")
	ch := SubscribeToChannel(t, wssBase, channel)

	time.Sleep(50 * time.Millisecond)

	select {
	case unexpected := <-ch:
		t.Fatalf("ACK frame must not reach caller, got: %s", unexpected)
	case <-time.After(100 * time.Millisecond):
		// correct: channel is empty
	}
}

func TestSubscribeToChannel_MultipleMessages(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", wssBase)

	channel := CreateTestChannel(t, "results")
	ch := SubscribeToChannel(t, wssBase, channel)

	time.Sleep(50 * time.Millisecond)

	PublishTestMessage(t, wssBase, channel, `{"seq":0}`)
	PublishTestMessage(t, wssBase, channel, `{"seq":1}`)
	PublishTestMessage(t, wssBase, channel, `{"seq":2}`)

	for i := 0; i < 3; i++ {
		msg := WaitForMessage(t, ch, 5*time.Second)
		require.NotNil(t, msg, "expected message %d", i)
	}
}

func TestSubscribeToChannel_ChannelIsolation(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", wssBase)

	ch1 := CreateTestChannel(t, "results")
	ch2 := CreateTestChannel(t, "results")

	sub1 := SubscribeToChannel(t, wssBase, ch1)
	sub2 := SubscribeToChannel(t, wssBase, ch2)

	time.Sleep(50 * time.Millisecond)

	PublishTestMessage(t, wssBase, ch1, `{"target":"ch1-only"}`)

	msg := WaitForMessage(t, sub1, 5*time.Second)
	require.NotNil(t, msg)
	assert.Contains(t, string(msg), "ch1-only")

	select {
	case unexpected := <-sub2:
		t.Fatalf("ch2 must not receive ch1 message, got: %s", unexpected)
	case <-time.After(150 * time.Millisecond):
		// correct
	}
}

// ---------------------------------------------------------------------------
// PublishTestMessage — unit coverage via in-process TLS server
// ---------------------------------------------------------------------------

func TestPublishTestMessage_JSONPayload(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", wssBase)

	channel := CreateTestChannel(t, "cmd")
	ch := SubscribeToChannel(t, wssBase, channel)
	time.Sleep(50 * time.Millisecond)

	PublishTestMessage(t, wssBase, channel, `{"event_type":"command.requested","hostname":"web-01"}`)

	msg := WaitForMessage(t, ch, 5*time.Second)
	require.NotNil(t, msg)

	var got map[string]string
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "command.requested", got["event_type"])
	assert.Equal(t, "web-01", got["hostname"])
}

func TestPublishTestMessage_NonJSONPayload_WrappedAsQuotedString(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", wssBase)

	channel := CreateTestChannel(t, "cmd")
	ch := SubscribeToChannel(t, wssBase, channel)
	time.Sleep(50 * time.Millisecond)

	PublishTestMessage(t, wssBase, channel, "plain text payload")

	msg := WaitForMessage(t, ch, 5*time.Second)
	require.NotNil(t, msg)
	assert.NotEmpty(t, msg)
}

func TestPublishTestMessage_DeliveredToSubscriber(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", wssBase)

	channel := CreateTestChannel(t, "heartbeat")
	ch := SubscribeToChannel(t, wssBase, channel)
	time.Sleep(50 * time.Millisecond)

	PublishTestMessage(t, wssBase, channel, `{"status":"online","operator_id":"op-99"}`)

	payload := AssertMessageReceived(t, ch, 5*time.Second, "op-99")
	require.NotNil(t, payload)
	assert.Contains(t, string(payload), "online")
}

func TestPublishTestMessage_MultipleSequential(t *testing.T) {
	wssBase := newTLSPubSubServer(t)
	t.Setenv("G8E_OPERATOR_PUBSUB_URL", wssBase)

	channel := CreateTestChannel(t, "results")
	ch := SubscribeToChannel(t, wssBase, channel)
	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 3; i++ {
		PublishTestMessage(t, wssBase, channel, `{"seq":`+string(rune('0'+i))+`}`)
	}

	for i := 0; i < 3; i++ {
		msg := WaitForMessage(t, ch, 5*time.Second)
		require.NotNil(t, msg, "expected message %d", i)
	}
}
