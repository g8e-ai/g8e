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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// CreateTestChannel
// ---------------------------------------------------------------------------

func TestCreateTestChannel_ContainsPrefix(t *testing.T) {
	ch := CreateTestChannel(t, "results")
	assert.True(t, strings.HasPrefix(ch, "results:"), "channel must start with prefix, got: %s", ch)
}

func TestCreateTestChannel_ContainsTestName(t *testing.T) {
	ch := CreateTestChannel(t, "cmd")
	assert.Contains(t, ch, t.Name())
}

func TestCreateTestChannel_IsUnique(t *testing.T) {
	ch1 := CreateTestChannel(t, "test")
	time.Sleep(time.Nanosecond)
	ch2 := CreateTestChannel(t, "test")
	assert.NotEqual(t, ch1, ch2, "successive channels must be unique")
}

func TestCreateTestChannel_EmptyPrefix(t *testing.T) {
	ch := CreateTestChannel(t, "")
	assert.True(t, strings.HasPrefix(ch, ":test:"), "empty prefix still produces valid channel, got: %s", ch)
}

func TestCreateTestChannel_SpecialCharsInPrefix(t *testing.T) {
	ch := CreateTestChannel(t, "results:op1")
	assert.True(t, strings.HasPrefix(ch, "results:op1:"), "got: %s", ch)
}

// ---------------------------------------------------------------------------
// WaitForMessage
// ---------------------------------------------------------------------------

func TestWaitForMessage_ReceivesMessage(t *testing.T) {
	ch := make(chan []byte, 1)
	payload := []byte(`{"key":"value"}`)
	ch <- payload

	got := WaitForMessage(t, ch, time.Second)
	require.NotNil(t, got)
	assert.Equal(t, payload, got)
}

func TestWaitForMessage_ClosedChannel_ReturnsNil(t *testing.T) {
	ch := make(chan []byte)
	close(ch)

	// Closed channel returns nil immediately — WaitForMessage should not fatal
	// but will return nil. We capture the nil return without asserting t.Fatal.
	got := WaitForMessage(t, ch, time.Second)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// AssertMessageReceived
// ---------------------------------------------------------------------------

func TestAssertMessageReceived_MatchingPattern(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte(`{"status":"completed","host":"web-01"}`)

	payload := AssertMessageReceived(t, ch, time.Second, "completed")
	require.NotNil(t, payload)
	assert.Contains(t, string(payload), "completed")
}

func TestAssertMessageReceived_EmptyPattern_AcceptsAnything(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte(`{"any":"payload"}`)

	payload := AssertMessageReceived(t, ch, time.Second, "")
	require.NotNil(t, payload)
}

func TestAssertMessageReceived_MultiplePayloads_ReturnsFirst(t *testing.T) {
	ch := make(chan []byte, 3)
	ch <- []byte(`"first"`)
	ch <- []byte(`"second"`)
	ch <- []byte(`"third"`)

	payload := AssertMessageReceived(t, ch, time.Second, "first")
	assert.Contains(t, string(payload), "first")
}
