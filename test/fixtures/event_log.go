// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration || e2e

package fixtures

import (
	"context"
	"sync"
)

// eventLog is an append-only record that waiters block on until an entry they
// want arrives. Every append wakes the waiters, so a wait never polls.
type eventLog[T any] struct {
	mu      sync.Mutex
	entries []T
	changed chan struct{}
}

func newEventLog[T any]() *eventLog[T] {
	return &eventLog[T]{changed: make(chan struct{})}
}

// append records entry and wakes every waiter.
func (l *eventLog[T]) append(entry T) {
	l.mu.Lock()
	l.entries = append(l.entries, entry)
	close(l.changed)
	l.changed = make(chan struct{})
	l.mu.Unlock()
}

// snapshot returns a copy of the entries recorded so far.
func (l *eventLog[T]) snapshot() []T {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]T(nil), l.entries...)
}

// awaitAll blocks until done reports true for the recorded entries, and
// returns them. It returns ctx.Err() if ctx ends first.
func (l *eventLog[T]) awaitAll(ctx context.Context, done func([]T) bool) ([]T, error) {
	for {
		l.mu.Lock()
		entries := append([]T(nil), l.entries...)
		changed := l.changed
		l.mu.Unlock()
		if done(entries) {
			return entries, nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return entries, ctx.Err()
		}
	}
}

// await blocks until an entry at or after index from satisfies match, and
// returns it with its index.
func (l *eventLog[T]) await(ctx context.Context, from int, match func(T) bool) (T, int, error) {
	var found T
	index := -1
	_, err := l.awaitAll(ctx, func(entries []T) bool {
		for i := from; i < len(entries); i++ {
			if match(entries[i]) {
				found, index = entries[i], i
				return true
			}
		}
		return false
	})
	return found, index, err
}

// len returns the number of entries recorded so far.
func (l *eventLog[T]) len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
