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

package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVSOMessage_BasicFields(t *testing.T) {
	payload := map[string]string{"key": "value"}
	msg, err := NewVSOMessage("id-1", constants.Event.Operator.Heartbeat, "case-1", "op-1", "sess-1", "fp-1", payload)

	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, "id-1", msg.ID)
	assert.Equal(t, constants.Event.Operator.Heartbeat, msg.EventType)
	assert.Equal(t, "case-1", msg.CaseID)
	assert.Equal(t, "op-1", msg.OperatorID)
	assert.Equal(t, "sess-1", msg.OperatorSessionID)
	assert.Equal(t, "fp-1", msg.SystemFingerprint)
	assert.Equal(t, constants.Status.ComponentName.VSA, msg.SourceComponent)
	assert.NotEmpty(t, msg.Timestamp)
}

func TestNewVSOMessage_TimestampIsUTC(t *testing.T) {
	before := time.Now().UTC()
	msg, err := NewVSOMessage("id-ts", constants.Event.Operator.Heartbeat, "c", "o", "s", "f", struct{}{})
	after := time.Now().UTC()

	require.NoError(t, err)

	parsed, err := ParseTimestamp(msg.Timestamp)
	require.NoError(t, err)
	assert.False(t, parsed.Before(before), "timestamp must not be before test start")
	assert.False(t, parsed.After(after), "timestamp must not be after test end")
}

func TestNewVSOMessage_PayloadIsMarshalledJSON(t *testing.T) {
	type inner struct {
		Command  string `json:"command"`
		ExitCode int    `json:"exit_code"`
	}
	payload := inner{Command: "ls", ExitCode: 0}

	msg, err := NewVSOMessage("id-2", constants.Event.Operator.Command.Completed, "c", "o", "s", "f", payload)
	require.NoError(t, err)

	var decoded inner
	require.NoError(t, json.Unmarshal(msg.Payload, &decoded))
	assert.Equal(t, "ls", decoded.Command)
	assert.Equal(t, 0, decoded.ExitCode)
}

func TestNewVSOMessage_NilPayload(t *testing.T) {
	msg, err := NewVSOMessage("id-nil", constants.Event.Operator.Heartbeat, "c", "o", "s", "f", nil)
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, json.RawMessage("null"), msg.Payload)
}

func TestNewVSOMessage_TaskIDInitiallyNil(t *testing.T) {
	msg, err := NewVSOMessage("id-3", constants.Event.Operator.Heartbeat, "c", "o", "s", "f", struct{}{})
	require.NoError(t, err)
	assert.Nil(t, msg.TaskID)
}

func TestVSOMessage_Marshal_RoundTrip(t *testing.T) {
	payload := map[string]int{"count": 42}
	msg, err := NewVSOMessage("id-rt", constants.Event.Operator.Command.Completed, "case-rt", "op-rt", "sess-rt", "fp-rt", payload)
	require.NoError(t, err)

	data, err := msg.Marshal()
	require.NoError(t, err)
	require.NotNil(t, data)

	var decoded VSOMessage
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, msg.ID, decoded.ID)
	assert.Equal(t, msg.EventType, decoded.EventType)
	assert.Equal(t, msg.CaseID, decoded.CaseID)
	assert.Equal(t, msg.OperatorID, decoded.OperatorID)
	assert.Equal(t, msg.OperatorSessionID, decoded.OperatorSessionID)
	assert.Equal(t, msg.SourceComponent, decoded.SourceComponent)
	assert.Equal(t, msg.SystemFingerprint, decoded.SystemFingerprint)
}

func TestVSOMessage_Marshal_ProducesValidJSON(t *testing.T) {
	msg, err := NewVSOMessage("id-json", constants.Event.Operator.Heartbeat, "c", "o", "s", "f", struct{}{})
	require.NoError(t, err)

	data, err := msg.Marshal()
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, raw, "id")
	assert.Contains(t, raw, "event_type")
	assert.Contains(t, raw, "timestamp")
	assert.Contains(t, raw, "source_component")
	assert.Contains(t, raw, "payload")
}
