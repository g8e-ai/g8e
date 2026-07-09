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

package pubsub

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
)

func TestCmdChannel(t *testing.T) {
	t.Parallel()
	t.Run("formats channel with operator ID and session ID", func(t *testing.T) {
		t.Parallel()
		ch := CmdChannel("op-1", "session-1")
		assert.Equal(t, "cmd:op-1:session-1", ch)
	})

	t.Run("formats with empty strings", func(t *testing.T) {
		t.Parallel()
		ch := CmdChannel("", "")
		assert.Equal(t, "cmd::", ch)
	})

	t.Run("uses correct prefix", func(t *testing.T) {
		t.Parallel()
		ch := CmdChannel("op-1", "session-1")
		assert.Contains(t, ch, constants.ChannelPrefixCmd)
	})
}

func TestResultsChannel(t *testing.T) {
	t.Parallel()
	t.Run("formats channel with operator ID and session ID", func(t *testing.T) {
		t.Parallel()
		ch := ResultsChannel("op-1", "session-1")
		assert.Equal(t, "results:op-1:session-1", ch)
	})

	t.Run("formats with empty strings", func(t *testing.T) {
		t.Parallel()
		ch := ResultsChannel("", "")
		assert.Equal(t, "results::", ch)
	})

	t.Run("uses correct prefix", func(t *testing.T) {
		t.Parallel()
		ch := ResultsChannel("op-1", "session-1")
		assert.Contains(t, ch, constants.ChannelPrefixResults)
	})
}

func TestHeartbeatChannel(t *testing.T) {
	t.Parallel()
	t.Run("formats channel with operator ID and session ID", func(t *testing.T) {
		t.Parallel()
		ch := HeartbeatChannel("op-1", "session-1")
		assert.Equal(t, "heartbeat:op-1:session-1", ch)
	})

	t.Run("formats with empty strings", func(t *testing.T) {
		t.Parallel()
		ch := HeartbeatChannel("", "")
		assert.Equal(t, "heartbeat::", ch)
	})

	t.Run("uses correct prefix", func(t *testing.T) {
		t.Parallel()
		ch := HeartbeatChannel("op-1", "session-1")
		assert.Contains(t, ch, constants.ChannelPrefixHeartbeat)
	})
}

func TestChannelsAreDistinct(t *testing.T) {
	t.Parallel()
	opID := "op-1"
	sessionID := "session-1"
	cmd := CmdChannel(opID, sessionID)
	results := ResultsChannel(opID, sessionID)
	heartbeat := HeartbeatChannel(opID, sessionID)

	assert.NotEqual(t, cmd, results)
	assert.NotEqual(t, cmd, heartbeat)
	assert.NotEqual(t, results, heartbeat)
}
