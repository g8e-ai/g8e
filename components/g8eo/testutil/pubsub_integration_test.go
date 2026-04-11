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

//go:build integration

package testutil

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestPubSubAvailable
// ---------------------------------------------------------------------------

func TestPubSubAvailable_ConnectsToLiveG8es(t *testing.T) {
	TestPubSubAvailable(t)
}

// ---------------------------------------------------------------------------
// SubscribeToChannel + PublishTestMessage — round-trip
// ---------------------------------------------------------------------------

func TestSubscribeToChannel_ReceivesPublishedMessage(t *testing.T) {
	TestPubSubAvailable(t)

	channel := CreateTestChannel(t, "integration")
	baseURL := GetTestG8esDirectURL()

	ch := SubscribeToChannel(t, baseURL, channel)

	time.Sleep(200 * time.Millisecond)

	payload := `{"event":"test","value":42}`
	PublishTestMessage(t, baseURL, channel, payload)

	msg := WaitForMessage(t, ch, 10*time.Second)
	require.NotNil(t, msg)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "test", got["event"])
	assert.Equal(t, float64(42), got["value"])
}

func TestSubscribeToChannel_MultipleMessages(t *testing.T) {
	TestPubSubAvailable(t)

	channel := CreateTestChannel(t, "integration")
	baseURL := GetTestG8esDirectURL()

	ch := SubscribeToChannel(t, baseURL, channel)

	time.Sleep(200 * time.Millisecond)

	for i := 0; i < 3; i++ {
		PublishTestMessage(t, baseURL, channel, `{"seq":`+string(rune('0'+i))+`}`)
	}

	received := 0
	for received < 3 {
		msg := WaitForMessage(t, ch, 10*time.Second)
		require.NotNil(t, msg, "expected message %d", received+1)
		received++
	}
	assert.Equal(t, 3, received)
}

func TestPublishTestMessage_NonJSONPayload_WrappedAsString(t *testing.T) {
	TestPubSubAvailable(t)

	channel := CreateTestChannel(t, "integration")
	baseURL := GetTestG8esDirectURL()

	ch := SubscribeToChannel(t, baseURL, channel)

	time.Sleep(200 * time.Millisecond)

	PublishTestMessage(t, baseURL, channel, "plain text payload")

	msg := WaitForMessage(t, ch, 10*time.Second)
	require.NotNil(t, msg)
	assert.NotEmpty(t, msg)
}

func TestAssertMessageReceived_Integration(t *testing.T) {
	TestPubSubAvailable(t)

	channel := CreateTestChannel(t, "integration")
	baseURL := GetTestG8esDirectURL()

	ch := SubscribeToChannel(t, baseURL, channel)

	time.Sleep(200 * time.Millisecond)

	PublishTestMessage(t, baseURL, channel, `{"status":"completed","operator_id":"op-123"}`)

	payload := AssertMessageReceived(t, ch, 10*time.Second, "completed")
	require.NotNil(t, payload)
	assert.Contains(t, string(payload), "op-123")
}

func TestSubscribeToChannel_IsolatedChannels_NoBleed(t *testing.T) {
	TestPubSubAvailable(t)

	baseURL := GetTestG8esDirectURL()
	ch1 := CreateTestChannel(t, "ch1")
	ch2 := CreateTestChannel(t, "ch2")

	sub1 := SubscribeToChannel(t, baseURL, ch1)
	sub2 := SubscribeToChannel(t, baseURL, ch2)

	time.Sleep(200 * time.Millisecond)

	PublishTestMessage(t, baseURL, ch1, `{"target":"ch1"}`)

	msg1 := WaitForMessage(t, sub1, 10*time.Second)
	require.NotNil(t, msg1)
	assert.Contains(t, string(msg1), "ch1")

	// ch2 must not receive the ch1 message
	select {
	case unexpected := <-sub2:
		t.Fatalf("ch2 received unexpected message: %s", unexpected)
	case <-time.After(300 * time.Millisecond):
		// correct — no bleed
	}
}
