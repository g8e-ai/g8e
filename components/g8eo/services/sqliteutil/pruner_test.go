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

package sqliteutil

import (
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/g8eo/testutil"
)

func TestNewPruner_DefaultsNegativeIntervalToOneHour(t *testing.T) {
	logger := testutil.NewTestLogger()
	p := NewPruner(nil, logger, -1*time.Second, func(_ *DB, _ *slog.Logger) {})
	assert.Equal(t, time.Hour, p.interval)
}

func TestNewPruner_DefaultsZeroIntervalToOneHour(t *testing.T) {
	logger := testutil.NewTestLogger()
	p := NewPruner(nil, logger, 0, func(_ *DB, _ *slog.Logger) {})
	assert.Equal(t, time.Hour, p.interval)
}

func TestNewPruner_RespectsPositiveInterval(t *testing.T) {
	logger := testutil.NewTestLogger()
	p := NewPruner(nil, logger, 5*time.Minute, func(_ *DB, _ *slog.Logger) {})
	assert.Equal(t, 5*time.Minute, p.interval)
}

func TestPruner_InvokesFnOnTick(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "pruner.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	var callCount atomic.Int64
	fn := func(_ *DB, _ *slog.Logger) {
		callCount.Add(1)
	}

	p := NewPruner(db, logger, 20*time.Millisecond, fn)
	p.Start()
	t.Cleanup(p.Stop)

	assert.Eventually(t, func() bool {
		return callCount.Load() >= 2
	}, 2*time.Second, 10*time.Millisecond)
}

func TestPruner_Stop_IsIdempotent(t *testing.T) {
	logger := testutil.NewTestLogger()
	p := NewPruner(nil, logger, time.Hour, func(_ *DB, _ *slog.Logger) {})
	p.Start()

	p.Stop()
	p.Stop()
}

func TestPruner_Stop_BeforeStart_DoesNotPanic(t *testing.T) {
	logger := testutil.NewTestLogger()
	p := NewPruner(nil, logger, time.Hour, func(_ *DB, _ *slog.Logger) {})

	assert.NotPanics(t, func() { p.Stop() })
}

func TestPruner_Stop_HaltsInvocations(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "halt.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	var callCount atomic.Int64
	fn := func(_ *DB, _ *slog.Logger) {
		callCount.Add(1)
	}

	p := NewPruner(db, logger, 20*time.Millisecond, fn)
	p.Start()

	assert.Eventually(t, func() bool {
		return callCount.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	p.Stop()
	countAfterStop := callCount.Load()

	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, countAfterStop, callCount.Load(), "no more invocations after Stop")
}

func TestPruner_FnReceivesCorrectLogger(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "logarg.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	var receivedLogger *slog.Logger
	done := make(chan struct{})
	fn := func(_ *DB, l *slog.Logger) {
		receivedLogger = l
		select {
		case <-done:
		default:
			close(done)
		}
	}

	p := NewPruner(db, logger, 20*time.Millisecond, fn)
	p.Start()
	t.Cleanup(p.Stop)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pruner fn was never called")
	}

	assert.Same(t, logger, receivedLogger)
}

func TestPruner_StartAfterStop_DoesNotInvokeFn(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "postStop.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	var callCount atomic.Int64
	fn := func(_ *DB, _ *slog.Logger) {
		callCount.Add(1)
	}

	p := NewPruner(db, logger, 20*time.Millisecond, fn)
	p.Start()

	assert.Eventually(t, func() bool {
		return callCount.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	p.Stop()

	p.Start()
	t.Cleanup(p.Stop)

	countAfterRestart := callCount.Load()
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, countAfterRestart, callCount.Load(), "fn must not be invoked after Start on a stopped pruner")
}

func TestPruner_FnReceivesCorrectDB(t *testing.T) {
	dir := t.TempDir()
	logger := testutil.NewTestLogger()
	cfg := DefaultDBConfig(filepath.Join(dir, "dbarg.db"))

	db, err := OpenDB(cfg, logger)
	require.NoError(t, err)
	defer db.Close()

	var receivedDB *DB
	done := make(chan struct{})
	fn := func(d *DB, _ *slog.Logger) {
		receivedDB = d
		select {
		case <-done:
		default:
			close(done)
		}
	}

	p := NewPruner(db, logger, 20*time.Millisecond, fn)
	p.Start()
	t.Cleanup(p.Stop)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pruner fn was never called")
	}

	assert.Same(t, db, receivedDB)
}
