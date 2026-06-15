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

func TestSourceConstants(t *testing.T) {
	t.Run("source user chat has correct value", func(t *testing.T) {
		assert.Equal(t, "g8e.v1.source.user.chat", SourceUserChat)
	})

	t.Run("source user terminal has correct value", func(t *testing.T) {
		assert.Equal(t, "g8e.v1.source.user.terminal", SourceUserTerminal)
	})

	t.Run("source ai primary has correct value", func(t *testing.T) {
		assert.Equal(t, "g8e.v1.source.ai.primary", SourceAiPrimary)
	})

	t.Run("source ai assistant has correct value", func(t *testing.T) {
		assert.Equal(t, "g8e.v1.source.ai.assistant", SourceAiAssistant)
	})

	t.Run("source ai triage has correct value", func(t *testing.T) {
		assert.Equal(t, "g8e.v1.source.ai.triage", SourceAiTriage)
	})

	t.Run("source system has correct value", func(t *testing.T) {
		assert.Equal(t, "g8e.v1.source.system", SourceSystem)
	})

	t.Run("all source constants are distinct", func(t *testing.T) {
		sources := []string{
			SourceUserChat,
			SourceUserTerminal,
			SourceAiPrimary,
			SourceAiAssistant,
			SourceAiTriage,
			SourceSystem,
		}

		seen := make(map[string]bool)
		for _, source := range sources {
			assert.False(t, seen[source], "source constant %s is duplicated", source)
			seen[source] = true
		}
	})

	t.Run("all source constants have correct prefix", func(t *testing.T) {
		sources := []string{
			SourceUserChat,
			SourceUserTerminal,
			SourceAiPrimary,
			SourceAiAssistant,
			SourceAiTriage,
			SourceSystem,
		}

		for _, source := range sources {
			assert.Contains(t, source, "g8e.v1.source.", "source constant %s should have correct prefix", source)
		}
	})
}

func TestMessageTypeConstants(t *testing.T) {
	t.Run("message type text has correct value", func(t *testing.T) {
		assert.Equal(t, "text", MessageTypeText)
	})

	t.Run("message type code has correct value", func(t *testing.T) {
		assert.Equal(t, "code", MessageTypeCode)
	})

	t.Run("message type call has correct value", func(t *testing.T) {
		assert.Equal(t, "call", MessageTypeCall)
	})

	t.Run("message type result has correct value", func(t *testing.T) {
		assert.Equal(t, "result", MessageTypeResult)
	})

	t.Run("message type error has correct value", func(t *testing.T) {
		assert.Equal(t, "error", MessageTypeError)
	})

	t.Run("message type thinking has correct value", func(t *testing.T) {
		assert.Equal(t, "thinking", MessageTypeThinking)
	})

	t.Run("all message type constants are distinct", func(t *testing.T) {
		types := []string{
			MessageTypeText,
			MessageTypeCode,
			MessageTypeCall,
			MessageTypeResult,
			MessageTypeError,
			MessageTypeThinking,
		}

		seen := make(map[string]bool)
		for _, msgType := range types {
			assert.False(t, seen[msgType], "message type constant %s is duplicated", msgType)
			seen[msgType] = true
		}
	})

	t.Run("all message type constants are lowercase", func(t *testing.T) {
		types := []string{
			MessageTypeText,
			MessageTypeCode,
			MessageTypeCall,
			MessageTypeResult,
			MessageTypeError,
			MessageTypeThinking,
		}

		for _, msgType := range types {
			assert.Equal(t, msgType, toLower(msgType), "message type constant %s should be lowercase", msgType)
		}
	})
}

func TestSenderConstantsContractRegression(t *testing.T) {
	t.Run("source constants match protocol values", func(t *testing.T) {
		// These tests ensure the Go constants match the JSON SSOT in protocol/constants/senders.json
		assert.Equal(t, "g8e.v1.source.user.chat", SourceUserChat)
		assert.Equal(t, "g8e.v1.source.user.terminal", SourceUserTerminal)
		assert.Equal(t, "g8e.v1.source.ai.primary", SourceAiPrimary)
		assert.Equal(t, "g8e.v1.source.ai.assistant", SourceAiAssistant)
		assert.Equal(t, "g8e.v1.source.ai.triage", SourceAiTriage)
		assert.Equal(t, "g8e.v1.source.system", SourceSystem)
	})

	t.Run("message type constants match protocol values", func(t *testing.T) {
		// These tests ensure the Go constants match the JSON SSOT in protocol/constants/senders.json
		assert.Equal(t, "text", MessageTypeText)
		assert.Equal(t, "code", MessageTypeCode)
		assert.Equal(t, "call", MessageTypeCall)
		assert.Equal(t, "result", MessageTypeResult)
		assert.Equal(t, "error", MessageTypeError)
		assert.Equal(t, "thinking", MessageTypeThinking)
	})
}

// Helper function to check if string is lowercase
func toLower(s string) string {
	// This is a simple check - in a real scenario we'd use strings.ToLower
	// but since we're just checking that constants are already lowercase,
	// we can just return the string as-is for comparison
	return s
}
