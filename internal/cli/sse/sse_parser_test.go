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

package sse

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSSEStream_SingleLineData(t *testing.T) {
	t.Parallel()

	input := "event: test\ndata: hello\n\n"
	var gotType, gotData string

	err := parseSSEStream(context.Background(), strings.NewReader(input), func(eventType, data string) {
		gotType = eventType
		gotData = data
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, "test", gotType)
	assert.Equal(t, "hello", gotData)
}

func TestParseSSEStream_MultiLineData(t *testing.T) {
	t.Parallel()

	input := "event: test\ndata: line1\ndata: line2\ndata: line3\n\n"
	var gotType, gotData string

	err := parseSSEStream(context.Background(), strings.NewReader(input), func(eventType, data string) {
		gotType = eventType
		gotData = data
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, "test", gotType)
	assert.Equal(t, "line1\nline2\nline3", gotData,
		"multiple data: lines must be concatenated with newlines per SSE spec")
}

func TestParseSSEStream_MultipleEvents(t *testing.T) {
	t.Parallel()

	input := "event: first\ndata: a\n\nevent: second\ndata: b\n\n"
	var events []struct{ Type, Data string }

	err := parseSSEStream(context.Background(), strings.NewReader(input), func(eventType, data string) {
		events = append(events, struct{ Type, Data string }{eventType, data})
	}, nil)
	assert.NoError(t, err)
	assert.Len(t, events, 2)
	assert.Equal(t, "first", events[0].Type)
	assert.Equal(t, "a", events[0].Data)
	assert.Equal(t, "second", events[1].Type)
	assert.Equal(t, "b", events[1].Data)
}

func TestParseSSEStream_EmptyData(t *testing.T) {
	t.Parallel()

	input := "event: test\n\n"
	called := false

	err := parseSSEStream(context.Background(), strings.NewReader(input), func(eventType, data string) {
		called = true
	}, nil)
	assert.NoError(t, err)
	assert.False(t, called, "handler should not be called when data is empty")
}

func TestParseSSEStream_DataOnlyNoEvent(t *testing.T) {
	t.Parallel()

	input := "data: payload\n\n"
	var gotType, gotData string

	err := parseSSEStream(context.Background(), strings.NewReader(input), func(eventType, data string) {
		gotType = eventType
		gotData = data
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, "", gotType, "event type should be empty when not set")
	assert.Equal(t, "payload", gotData)
}
