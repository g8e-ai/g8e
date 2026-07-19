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

// Package publisherstest provides test-only ResultsPublisher implementations.
// This is separate from pubsubtest to avoid an import cycle: pubsubtest is
// imported by pubsub test files, so it cannot itself import pubsub.
package publisherstest

import (
	"context"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/internal/services/pubsub"
)

// PublishedResult records a single ResultsPublisher call for test assertions.
type PublishedResult struct {
	Method      string
	Message     proto.Message
	OriginalMsg *pubsub.PubSubCommandMessage
}

// TestResultsPublisher is an in-memory recording fake implementing pubsub.ResultsPublisher.
// It records all published results by method and returns nil by default.
// Use SetError to inject failures for specific scenarios.
type TestResultsPublisher struct {
	mu         sync.Mutex
	results    []PublishedResult
	heartbeats []proto.Message
	err        error
}

// NewTestResultsPublisher creates a new TestResultsPublisher.
func NewTestResultsPublisher() *TestResultsPublisher {
	return &TestResultsPublisher{}
}

func (f *TestResultsPublisher) PublishExecutionResult(_ context.Context, result proto.Message, originalMsg *pubsub.PubSubCommandMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.results = append(f.results, PublishedResult{Method: "PublishExecutionResult", Message: result, OriginalMsg: originalMsg})
	return nil
}

func (f *TestResultsPublisher) PublishCancellationResult(_ context.Context, result proto.Message, originalMsg *pubsub.PubSubCommandMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.results = append(f.results, PublishedResult{Method: "PublishCancellationResult", Message: result, OriginalMsg: originalMsg})
	return nil
}

func (f *TestResultsPublisher) PublishFileEditResult(_ context.Context, result proto.Message, originalMsg *pubsub.PubSubCommandMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.results = append(f.results, PublishedResult{Method: "PublishFileEditResult", Message: result, OriginalMsg: originalMsg})
	return nil
}

func (f *TestResultsPublisher) PublishFsListResult(_ context.Context, result proto.Message, originalMsg *pubsub.PubSubCommandMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.results = append(f.results, PublishedResult{Method: "PublishFsListResult", Message: result, OriginalMsg: originalMsg})
	return nil
}

func (f *TestResultsPublisher) PublishFsGrepResult(_ context.Context, result proto.Message, originalMsg *pubsub.PubSubCommandMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.results = append(f.results, PublishedResult{Method: "PublishFsGrepResult", Message: result, OriginalMsg: originalMsg})
	return nil
}

func (f *TestResultsPublisher) PublishExecutionStatus(_ context.Context, status proto.Message, originalMsg *pubsub.PubSubCommandMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.results = append(f.results, PublishedResult{Method: "PublishExecutionStatus", Message: status, OriginalMsg: originalMsg})
	return nil
}

func (f *TestResultsPublisher) PublishHeartbeat(_ context.Context, heartbeat proto.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.heartbeats = append(f.heartbeats, heartbeat)
	return nil
}

// Results returns a copy of all published results (excludes heartbeats).
func (f *TestResultsPublisher) Results() []PublishedResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]PublishedResult, len(f.results))
	copy(out, f.results)
	return out
}

// Heartbeats returns a copy of all published heartbeat messages.
func (f *TestResultsPublisher) Heartbeats() []proto.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]proto.Message, len(f.heartbeats))
	copy(out, f.heartbeats)
	return out
}

// SetError configures the fake to return the given error on all subsequent calls.
// Pass nil to clear the error.
func (f *TestResultsPublisher) SetError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// Reset clears all recorded results and heartbeats.
func (f *TestResultsPublisher) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = nil
	f.heartbeats = nil
	f.err = nil
}

var _ pubsub.ResultsPublisher = (*TestResultsPublisher)(nil)
