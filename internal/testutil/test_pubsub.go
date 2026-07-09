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
	"fmt"
	"strings"
	"testing"
	"time"
)

// WaitForMessage waits for a message on a channel with timeout
func WaitForMessage(t *testing.T, msgChan <-chan []byte, timeout time.Duration) []byte {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case msg := <-msgChan:
		return msg
	case <-timer.C:
		t.Fatalf("testutil: timeout waiting for pub/sub message")
		return nil
	}
}

// AssertMessageReceived asserts that a message is received on a channel within timeout
func AssertMessageReceived(t *testing.T, msgChan <-chan []byte, timeout time.Duration, expectedPattern string) []byte {
	t.Helper()

	payload := WaitForMessage(t, msgChan, timeout)
	if payload == nil {
		t.Fatalf("testutil: expected message but got nil")
	}

	if expectedPattern != "" && !strings.Contains(string(payload), expectedPattern) {
		t.Fatalf("testutil: expected message to contain '%s' but got: %s", expectedPattern, string(payload))
	}

	return payload
}

// CreateTestChannel returns a unique channel name for testing
func CreateTestChannel(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s:test:%s:%d", prefix, t.Name(), time.Now().UnixNano())
}

