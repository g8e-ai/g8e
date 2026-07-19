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

package publisherstest

import (
	"context"
	"errors"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/pubsub"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestResultsPublisher_PublishExecutionResult(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	msg := &operatorv1.CommandResult{ExecutionId: "exec-1"}
	origMsg := &pubsub.PubSubCommandMessage{ID: "msg-1"}

	err := fake.PublishExecutionResult(ctx, msg, origMsg)
	require.NoError(t, err)

	results := fake.Results()
	require.Len(t, results, 1)
	assert.Equal(t, "PublishExecutionResult", results[0].Method)
	assert.Equal(t, msg, results[0].Message)
	assert.Equal(t, origMsg, results[0].OriginalMsg)
}

func TestTestResultsPublisher_PublishCancellationResult(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	msg := &operatorv1.CommandResult{ExecutionId: "cancel-1"}
	origMsg := &pubsub.PubSubCommandMessage{ID: "msg-2"}

	err := fake.PublishCancellationResult(ctx, msg, origMsg)
	require.NoError(t, err)

	results := fake.Results()
	require.Len(t, results, 1)
	assert.Equal(t, "PublishCancellationResult", results[0].Method)
}

func TestTestResultsPublisher_PublishFileEditResult(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	msg := &operatorv1.FileEditResult{ExecutionId: "edit-1"}
	origMsg := &pubsub.PubSubCommandMessage{ID: "msg-3"}

	err := fake.PublishFileEditResult(ctx, msg, origMsg)
	require.NoError(t, err)

	results := fake.Results()
	require.Len(t, results, 1)
	assert.Equal(t, "PublishFileEditResult", results[0].Method)
}

func TestTestResultsPublisher_PublishFsListResult(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	msg := &operatorv1.FsListResult{ExecutionId: "ls-1"}
	origMsg := &pubsub.PubSubCommandMessage{ID: "msg-4"}

	err := fake.PublishFsListResult(ctx, msg, origMsg)
	require.NoError(t, err)

	results := fake.Results()
	require.Len(t, results, 1)
	assert.Equal(t, "PublishFsListResult", results[0].Method)
}

func TestTestResultsPublisher_PublishFsGrepResult(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	msg := &operatorv1.FsGrepResult{ExecutionId: "grep-1"}
	origMsg := &pubsub.PubSubCommandMessage{ID: "msg-5"}

	err := fake.PublishFsGrepResult(ctx, msg, origMsg)
	require.NoError(t, err)

	results := fake.Results()
	require.Len(t, results, 1)
	assert.Equal(t, "PublishFsGrepResult", results[0].Method)
}

func TestTestResultsPublisher_PublishExecutionStatus(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	msg := &operatorv1.ExecutionStatusUpdate{ExecutionId: "exec-1"}
	origMsg := &pubsub.PubSubCommandMessage{ID: "msg-6"}

	err := fake.PublishExecutionStatus(ctx, msg, origMsg)
	require.NoError(t, err)

	results := fake.Results()
	require.Len(t, results, 1)
	assert.Equal(t, "PublishExecutionStatus", results[0].Method)
}

func TestTestResultsPublisher_PublishHeartbeat(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	hb := &operatorv1.HeartbeatResult{OperatorId: "op-1"}

	err := fake.PublishHeartbeat(ctx, hb)
	require.NoError(t, err)

	assert.Empty(t, fake.Results(), "heartbeat should not appear in Results()")
	heartbeats := fake.Heartbeats()
	require.Len(t, heartbeats, 1)
	assert.Equal(t, hb, heartbeats[0])
}

func TestTestResultsPublisher_NilOriginalMsg(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	msg := &operatorv1.CommandResult{ExecutionId: "exec-1"}

	err := fake.PublishExecutionResult(ctx, msg, nil)
	require.NoError(t, err)

	results := fake.Results()
	require.Len(t, results, 1)
	assert.Nil(t, results[0].OriginalMsg)
}

func TestTestResultsPublisher_SetError(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	injected := errors.New("publisher unavailable")
	fake.SetError(injected)

	err := fake.PublishExecutionResult(ctx, &operatorv1.CommandResult{}, &pubsub.PubSubCommandMessage{ID: "msg"})
	require.Error(t, err)
	assert.ErrorIs(t, err, injected)

	assert.Empty(t, fake.Results(), "failed publish should not record a result")

	fake.SetError(nil)
	err = fake.PublishExecutionResult(ctx, &operatorv1.CommandResult{}, &pubsub.PubSubCommandMessage{ID: "msg"})
	require.NoError(t, err)
	assert.Len(t, fake.Results(), 1)
}

func TestTestResultsPublisher_MultipleResults(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	require.NoError(t, fake.PublishExecutionResult(ctx, &operatorv1.CommandResult{}, &pubsub.PubSubCommandMessage{ID: "m1"}))
	require.NoError(t, fake.PublishFileEditResult(ctx, &operatorv1.FileEditResult{}, &pubsub.PubSubCommandMessage{ID: "m2"}))
	require.NoError(t, fake.PublishHeartbeat(ctx, &operatorv1.HeartbeatResult{}))
	require.NoError(t, fake.PublishCancellationResult(ctx, &operatorv1.CommandResult{}, &pubsub.PubSubCommandMessage{ID: "m3"}))

	results := fake.Results()
	assert.Len(t, results, 3)
	assert.Len(t, fake.Heartbeats(), 1)
}

func TestTestResultsPublisher_Reset(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	require.NoError(t, fake.PublishExecutionResult(ctx, &operatorv1.CommandResult{}, &pubsub.PubSubCommandMessage{ID: "m1"}))
	require.NoError(t, fake.PublishHeartbeat(ctx, &operatorv1.HeartbeatResult{}))
	fake.SetError(errors.New("err"))

	fake.Reset()

	assert.Empty(t, fake.Results())
	assert.Empty(t, fake.Heartbeats())
	assert.NoError(t, fake.PublishExecutionResult(ctx, &operatorv1.CommandResult{}, &pubsub.PubSubCommandMessage{ID: "m2"}))
}

func TestTestResultsPublisher_ResultsReturnsCopy(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	require.NoError(t, fake.PublishExecutionResult(ctx, &operatorv1.CommandResult{}, &pubsub.PubSubCommandMessage{ID: "m1"}))

	r1 := fake.Results()
	require.Len(t, r1, 1)

	require.NoError(t, fake.PublishExecutionResult(ctx, &operatorv1.CommandResult{}, &pubsub.PubSubCommandMessage{ID: "m2"}))
	r2 := fake.Results()
	assert.Len(t, r2, 2, "Results() should return a copy, not a live reference")
	assert.Len(t, r1, 1, "earlier copy should be unaffected by later calls")
}

func TestTestResultsPublisher_HeartbeatsReturnsCopy(t *testing.T) {
	t.Parallel()
	fake := NewTestResultsPublisher()
	ctx := context.Background()

	require.NoError(t, fake.PublishHeartbeat(ctx, &operatorv1.HeartbeatResult{}))

	h1 := fake.Heartbeats()
	require.Len(t, h1, 1)

	require.NoError(t, fake.PublishHeartbeat(ctx, &operatorv1.HeartbeatResult{}))
	h2 := fake.Heartbeats()
	assert.Len(t, h2, 2)
	assert.Len(t, h1, 1)
}
