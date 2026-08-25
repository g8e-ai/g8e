// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
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

func TestReceiptsChannel(t *testing.T) {
	t.Parallel()
	t.Run("formats channel with operator ID and session ID", func(t *testing.T) {
		t.Parallel()
		ch := ReceiptsChannel("op-1", "session-1")
		assert.Equal(t, "receipts:op-1:session-1", ch)
	})

	t.Run("formats with empty strings", func(t *testing.T) {
		t.Parallel()
		ch := ReceiptsChannel("", "")
		assert.Equal(t, "receipts::", ch)
	})

	t.Run("uses correct prefix", func(t *testing.T) {
		t.Parallel()
		ch := ReceiptsChannel("op-1", "session-1")
		assert.Contains(t, ch, constants.ChannelPrefixReceipts)
	})
}

func TestChannelsAreDistinct(t *testing.T) {
	t.Parallel()
	opID := "op-1"
	sessionID := "session-1"
	cmd := CmdChannel(opID, sessionID)
	results := ResultsChannel(opID, sessionID)
	heartbeat := HeartbeatChannel(opID, sessionID)
	receipts := ReceiptsChannel(opID, sessionID)

	assert.NotEqual(t, cmd, results)
	assert.NotEqual(t, cmd, heartbeat)
	assert.NotEqual(t, cmd, receipts)
	assert.NotEqual(t, results, heartbeat)
	assert.NotEqual(t, results, receipts)
	assert.NotEqual(t, heartbeat, receipts)
}
