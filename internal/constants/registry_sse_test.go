// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/protocol"
)

func TestValidateSSEPush_EnsembleChatEvent(t *testing.T) {
	validation, err := ValidateSSEPush(EventAiLLMChatIterationStarted, "ensemble")
	require.NoError(t, err)
	assert.True(t, validation.Persist)
}

func TestValidateSSEPush_EphemeralStreamEvent(t *testing.T) {
	validation, err := ValidateSSEPush(EventAiLLMChatIterationTextChunkReceived, "ensemble")
	require.NoError(t, err)
	assert.False(t, validation.Persist)
}

func TestValidateSSEPush_UnknownEvent(t *testing.T) {
	_, err := ValidateSSEPush(EventType("g8e.v1.not.registered"), "ensemble")
	assert.ErrorIs(t, err, ErrSSEEventNotRegistered)
}

func TestValidateSSEPush_NotOnSSETransport(t *testing.T) {
	_, err := ValidateSSEPush(EventOperatorCommandRequested, "ensemble")
	assert.ErrorIs(t, err, ErrSSEEventNotOnTransport)
}

func TestValidateSSEPush_ProducerNotAllowed(t *testing.T) {
	_, err := ValidateSSEPush(EventAiAgentConflictDetected, "dashboard")
	assert.ErrorIs(t, err, ErrSSEProducerNotAllowed)
}

func TestSSEProducerFromAppID_Ensemble(t *testing.T) {
	producer, ok := SSEProducerFromAppID(protocol.EnsembleAppID)
	assert.True(t, ok)
	assert.Equal(t, "ensemble", producer)
}
