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

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCmdChannel(t *testing.T) {
	t.Run("formats correctly", func(t *testing.T) {
		result := CmdChannel("op-123", "sess-456")
		assert.Equal(t, "cmd:op-123:sess-456", result)
	})

	t.Run("empty operator ID", func(t *testing.T) {
		result := CmdChannel("", "sess-456")
		assert.Equal(t, "cmd::sess-456", result)
	})

	t.Run("empty session ID", func(t *testing.T) {
		result := CmdChannel("op-123", "")
		assert.Equal(t, "cmd:op-123:", result)
	})

	t.Run("both empty", func(t *testing.T) {
		result := CmdChannel("", "")
		assert.Equal(t, "cmd::", result)
	})
}

func TestResultsChannel(t *testing.T) {
	t.Run("formats correctly", func(t *testing.T) {
		result := ResultsChannel("op-123", "sess-456")
		assert.Equal(t, "results:op-123:sess-456", result)
	})

	t.Run("empty operator ID", func(t *testing.T) {
		result := ResultsChannel("", "sess-456")
		assert.Equal(t, "results::sess-456", result)
	})

	t.Run("empty session ID", func(t *testing.T) {
		result := ResultsChannel("op-123", "")
		assert.Equal(t, "results:op-123:", result)
	})
}

func TestHeartbeatChannel(t *testing.T) {
	t.Run("formats correctly", func(t *testing.T) {
		result := HeartbeatChannel("op-123", "sess-456")
		assert.Equal(t, "heartbeat:op-123:sess-456", result)
	})

	t.Run("empty operator ID", func(t *testing.T) {
		result := HeartbeatChannel("", "sess-456")
		assert.Equal(t, "heartbeat::sess-456", result)
	})

	t.Run("empty session ID", func(t *testing.T) {
		result := HeartbeatChannel("op-123", "")
		assert.Equal(t, "heartbeat:op-123:", result)
	})
}

func TestChannelPrefixes_AreDistinct(t *testing.T) {
	opID := "op-abc"
	sessID := "sess-xyz"

	cmd := CmdChannel(opID, sessID)
	results := ResultsChannel(opID, sessID)
	hb := HeartbeatChannel(opID, sessID)

	assert.NotEqual(t, cmd, results)
	assert.NotEqual(t, cmd, hb)
	assert.NotEqual(t, results, hb)
}

func TestChannelContractRegression(t *testing.T) {
	t.Run("results channel with realistic session ID", func(t *testing.T) {
		opID := "op-abc123"
		sessID := "operator_session_1764000000000_abc-123-def-456-ghi-789"
		expected := "results:op-abc123:operator_session_1764000000000_abc-123-def-456-ghi-789"
		actual := ResultsChannel(opID, sessID)
		assert.Equal(t, expected, actual)
	})

	t.Run("cmd channel with realistic session ID", func(t *testing.T) {
		opID := "op-xyz789"
		sessID := "operator_session_1764000000000_987-fed-654-321-cba"
		expected := "cmd:op-xyz789:operator_session_1764000000000_987-fed-654-321-cba"
		actual := CmdChannel(opID, sessID)
		assert.Equal(t, expected, actual)
	})

	t.Run("heartbeat channel with realistic session ID", func(t *testing.T) {
		opID := "op-def456"
		sessID := "operator_session_1764000000000_111-222-333-444-555"
		expected := "heartbeat:op-def456:operator_session_1764000000000_111-222-333-444-555"
		actual := HeartbeatChannel(opID, sessID)
		assert.Equal(t, expected, actual)
	})

	t.Run("handles colon in session ID", func(t *testing.T) {
		opID := "op-colon-test"
		sessID := "operator_session_1764000000000_with:colon:inside"
		expected := "cmd:op-colon-test:operator_session_1764000000000_with:colon:inside"
		actual := CmdChannel(opID, sessID)
		assert.Equal(t, expected, actual)
	})
}
